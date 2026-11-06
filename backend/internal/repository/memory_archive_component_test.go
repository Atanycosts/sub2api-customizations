//go:build memorycomponent && !integration

package repository

// These PostgreSQL component tests exercise the actual migration, transactions,
// row locks, lease fencing and worker recovery, with a fake memory provider.
// They do not establish real Runtime or LLM end-to-end success.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func memoryComponentDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("SUB2API_MEMORY_TEST_DSN")
	if dsn == "" {
		t.Fatal("SUB2API_MEMORY_TEST_DSN is required for real PostgreSQL component tests")
	}
	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })
	require.NoError(t, admin.Ping())
	schema := "memory_component_" + uuid.NewString()[:8]
	_, err = admin.Exec(`CREATE SCHEMA "` + schema + `"`)
	require.NoError(t, err)
	t.Cleanup(func() { _, err := admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`); require.NoError(t, err) })
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := sql.Open("postgres", parsed.String())
	require.NoError(t, err)
	db.SetMaxOpenConns(8)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE users (id BIGINT PRIMARY KEY, deleted_at TIMESTAMPTZ, status TEXT NOT NULL DEFAULT 'active'); INSERT INTO users (id) VALUES (1), (2)`)
	require.NoError(t, err)
	migration, err := migrations.FS.ReadFile("250_memory_archive_queue.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(migration))
	require.NoError(t, err)
	_, err = db.Exec(string(migration))
	require.NoError(t, err, "migration must be idempotent")
	return db
}

func memoryTestTurn(user int64, event string) service.MemoryTurn {
	return service.MemoryTurn{Scope: service.MemoryScope{UserID: user}, EventID: event, UserText: "synthetic preference: blue", AssistantText: "acknowledged user statement", Source: "component_test", CreatedAt: time.Date(2026, 11, 6, 1, 2, 3, 0, time.UTC)}
}
func forceMemoryLeaseExpired(t *testing.T, db *sql.DB, id int64) {
	t.Helper()
	_, err := db.Exec(`UPDATE memory_archive_queue SET lease_until = NOW() - INTERVAL '1 second' WHERE id = $1`, id)
	require.NoError(t, err)
}

func TestMemoryArchivePostgresIdempotencyIsolationAndBoundedPayload(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	turn := memoryTestTurn(1, "logical-event")
	first, err := repo.Enqueue(ctx, turn)
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchivePending, first.State)
	second, err := repo.Enqueue(ctx, turn)
	require.NoError(t, err)
	require.True(t, second.Duplicate)
	require.Equal(t, first.ID, second.ID)
	turn.UserText = "different payload"
	_, err = repo.Enqueue(ctx, turn)
	require.ErrorIs(t, err, service.ErrMemoryArchiveEventConflict)
	other, err := repo.Enqueue(ctx, memoryTestTurn(2, "logical-event"))
	require.NoError(t, err)
	require.NotEqual(t, first.ID, other.ID)
	_, err = repo.Get(ctx, 2, "unknown")
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = repo.Enqueue(ctx, memoryTestTurn(1, string(make([]byte, 129))))
	require.Error(t, err)
}

func TestMemoryArchivePostgresExpiredClaimFencedEvenSameWorker(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	_, err := repo.Enqueue(ctx, memoryTestTurn(1, "lease-event"))
	require.NoError(t, err)
	first, err := repo.Claim(ctx, "same-worker", 1, time.Second, 5, []int64{1, 2})
	require.NoError(t, err)
	require.Len(t, first, 1)
	forceMemoryLeaseExpired(t, db, first[0].ID)
	second, err := repo.Claim(ctx, "same-worker", 1, time.Minute, 5, []int64{1, 2})
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.NotEqual(t, first[0].LeaseToken, second[0].LeaseToken)
	require.ErrorIs(t, repo.MarkSubmitted(ctx, first[0]), service.ErrMemoryArchiveClaimLost)
	require.NoError(t, repo.MarkSubmitted(ctx, second[0]))
	second[0].State = service.MemoryArchiveSubmitted
	require.NoError(t, repo.Finish(ctx, second[0], service.MemoryArchiveCompleted, time.Now(), ""))
	var payload sql.NullString
	require.NoError(t, db.QueryRow(`SELECT payload::text FROM memory_archive_queue WHERE id=$1`, second[0].ID).Scan(&payload))
	require.False(t, payload.Valid, "completed delivery removes conversation text")
}

type memoryComponentProvider struct {
	archiveCalls   atomic.Int32
	archiveErr     error
	reconcile      service.MemoryArchiveReconciliation
	beforeReturn   func()
	panicAfterCall bool
}

func (p *memoryComponentProvider) Archive(context.Context, service.MemoryTurn) error {
	p.archiveCalls.Add(1)
	if p.panicAfterCall {
		panic("synthetic provider panic after possible commit")
	}
	if p.beforeReturn != nil {
		p.beforeReturn()
	}
	return p.archiveErr
}
func (*memoryComponentProvider) Recall(context.Context, service.MemoryRecallRequest) (service.MemoryRecallResult, error) {
	return service.MemoryRecallResult{}, nil
}
func (*memoryComponentProvider) Delete(context.Context, service.MemoryScope, string) error {
	return nil
}
func (p *memoryComponentProvider) ReconcileArchive(context.Context, service.MemoryTurn) (service.MemoryArchiveReconciliation, error) {
	return p.reconcile, nil
}

type componentRetryError struct{}

func (componentRetryError) Error() string        { return "known not submitted" }
func (componentRetryError) OutcomeUnknown() bool { return false }
func (componentRetryError) Retryable() bool      { return true }

func TestMemoryArchivePostgresCrashAfterSubmitOnlyReconciles(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	_, err := repo.Enqueue(ctx, memoryTestTurn(1, "crash-after-submit"))
	require.NoError(t, err)
	events, err := repo.Claim(ctx, "crashed-worker", 1, time.Second, 5, []int64{1, 2})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.NoError(t, repo.MarkSubmitted(ctx, events[0]))
	provider := &memoryComponentProvider{reconcile: service.MemoryArchiveReconciliationCompleted}
	require.NoError(t, provider.Archive(ctx, events[0].Turn))
	forceMemoryLeaseExpired(t, db, events[0].ID)
	worker := service.NewMemoryArchiveWorker(repo, provider, service.MemoryArchiveWorkerOptions{UserIDs: []int64{1}, BatchSize: 1})
	require.NoError(t, worker.RunOnce(ctx))
	require.EqualValues(t, 1, provider.archiveCalls.Load(), "recovery cannot resubmit a possibly committed operation")
	receipt, err := repo.Get(ctx, 1, "crash-after-submit")
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchiveCompleted, receipt.State)
	require.Equal(t, 1, receipt.Attempts)
}

func TestMemoryArchivePostgresUnknownOutcomesNeverAutoReplay(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	_, err := repo.Enqueue(ctx, memoryTestTurn(1, "unknown-submit"))
	require.NoError(t, err)
	provider := &memoryComponentProvider{archiveErr: errors.New("connection lost after possible commit"), reconcile: service.MemoryArchiveReconciliationUnknown}
	worker := service.NewMemoryArchiveWorker(repo, provider, service.MemoryArchiveWorkerOptions{UserIDs: []int64{1}, BatchSize: 1, MaxReconciliationChecks: 2})
	require.NoError(t, worker.RunOnce(ctx))
	for range 4 {
		_, err := db.Exec(`UPDATE memory_archive_queue SET available_at=NOW()-INTERVAL '1 second' WHERE user_id=1`)
		require.NoError(t, err)
		require.NoError(t, worker.RunOnce(ctx))
	}
	require.EqualValues(t, 1, provider.archiveCalls.Load())
	receipt, err := repo.Get(ctx, 1, "unknown-submit")
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchiveNeedsReconciliation, receipt.State)
	var checks int
	require.NoError(t, db.QueryRow(`SELECT reconciliation_checks FROM memory_archive_queue WHERE id=$1`, receipt.ID).Scan(&checks))
	require.Equal(t, 2, checks)
}

func TestMemoryArchivePostgresKnownPreSubmitRetryIsBounded(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	_, err := repo.Enqueue(ctx, memoryTestTurn(1, "known-rejection"))
	require.NoError(t, err)
	provider := &memoryComponentProvider{archiveErr: componentRetryError{}}
	worker := service.NewMemoryArchiveWorker(repo, provider, service.MemoryArchiveWorkerOptions{UserIDs: []int64{1}, BatchSize: 1, MaxAttempts: 2})
	require.NoError(t, worker.RunOnce(ctx))
	receipt, err := repo.Get(ctx, 1, "known-rejection")
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchiveRetryWait, receipt.State)
	_, err = db.Exec(`UPDATE memory_archive_queue SET available_at=NOW()-INTERVAL '1 second'`)
	require.NoError(t, err)
	require.NoError(t, worker.RunOnce(ctx))
	receipt, err = repo.Get(ctx, 1, "known-rejection")
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchiveFailed, receipt.State)
	require.Equal(t, 2, receipt.Attempts)
	require.EqualValues(t, 2, provider.archiveCalls.Load())
}

func TestMemoryArchivePostgresConcurrentClaimAndBlockFence(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	for i := range 10 {
		_, err := repo.Enqueue(ctx, memoryTestTurn(1, fmt.Sprintf("event-%d", i)))
		require.NoError(t, err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	claims := map[int64]string{}
	for i := range 4 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			events, err := repo.Claim(ctx, fmt.Sprintf("worker-%d", worker), 3, time.Minute, 5, []int64{1, 2})
			require.NoError(t, err)
			mu.Lock()
			defer mu.Unlock()
			for _, event := range events {
				_, exists := claims[event.ID]
				require.False(t, exists)
				claims[event.ID] = event.LeaseToken
			}
		}(i)
	}
	wg.Wait()
	require.Len(t, claims, 10)
	generation, err := repo.BlockUser(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, generation)
	again, err := repo.BlockUser(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, generation, again)
	_, err = repo.ActiveGeneration(ctx, 1)
	require.ErrorIs(t, err, service.ErrMemoryArchiveUserBlocked)
	_, err = repo.Enqueue(ctx, memoryTestTurn(1, "new-after-block"))
	require.ErrorIs(t, err, service.ErrMemoryArchiveUserBlocked)
	events, err := repo.Claim(ctx, "after-block", 1, time.Minute, 5, []int64{1, 2})
	require.NoError(t, err)
	require.Empty(t, events)
	var rawRows int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM memory_archive_queue WHERE payload IS NOT NULL`).Scan(&rawRows))
	require.Zero(t, rawRows)
}

func TestMemoryArchivePostgresBlockDuringRPCFencesLateCompletion(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	_, err := repo.Enqueue(ctx, memoryTestTurn(1, "late-completion"))
	require.NoError(t, err)
	provider := &memoryComponentProvider{beforeReturn: func() { _, err := repo.BlockUser(ctx, 1); require.NoError(t, err) }}
	worker := service.NewMemoryArchiveWorker(repo, provider, service.MemoryArchiveWorkerOptions{UserIDs: []int64{1}, BatchSize: 1})
	err = worker.RunOnce(ctx)
	require.ErrorIs(t, err, service.ErrMemoryArchiveClaimLost)
	receipt, err := repo.Get(ctx, 1, "late-completion")
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchiveCancelled, receipt.State)
	_, err = repo.ActiveGeneration(ctx, 1)
	require.ErrorIs(t, err, service.ErrMemoryArchiveUserBlocked)
	require.EqualValues(t, 1, provider.archiveCalls.Load(), "physical write may have happened, so block is not erasure")
}

func TestMemoryArchivePostgresUserLifecycleSuppressesNewAndPreclaimedWrites(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	_, err := repo.Enqueue(ctx, memoryTestTurn(1, "suspend-before-submit"))
	require.NoError(t, err)
	claimed, err := repo.Claim(ctx, "before-suspend", 1, time.Minute, 5, []int64{1, 2})
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	_, err = db.Exec(`UPDATE users SET status='disabled' WHERE id=1`)
	require.NoError(t, err)
	_, err = repo.ActiveGeneration(ctx, 1)
	require.ErrorIs(t, err, service.ErrMemoryArchiveUserBlocked)
	require.ErrorIs(t, repo.MarkSubmitted(ctx, claimed[0]), service.ErrMemoryArchiveClaimLost)
	_, err = repo.Enqueue(ctx, memoryTestTurn(1, "disabled-enqueue"))
	require.Error(t, err)
	forceMemoryLeaseExpired(t, db, claimed[0].ID)
	skipped, err := repo.Claim(ctx, "while-disabled", 1, time.Minute, 5, []int64{1, 2})
	require.NoError(t, err)
	require.Empty(t, skipped)
	_, err = db.Exec(`UPDATE users SET status='active', deleted_at=NOW() WHERE id=1`)
	require.NoError(t, err)
	_, err = repo.ActiveGeneration(ctx, 1)
	require.ErrorIs(t, err, service.ErrMemoryArchiveUserBlocked)
	skipped, err = repo.Claim(ctx, "while-deleted", 1, time.Minute, 5, []int64{1, 2})
	require.NoError(t, err)
	require.Empty(t, skipped)
	_, err = db.Exec(`UPDATE users SET deleted_at=NULL WHERE id=1`)
	require.NoError(t, err)
	restored, err := repo.Claim(ctx, "restored", 1, time.Minute, 5, []int64{1, 2})
	require.NoError(t, err)
	require.Len(t, restored, 1)
	require.NoError(t, repo.MarkSubmitted(ctx, restored[0]))
	_, err = db.Exec(`UPDATE users SET status='disabled' WHERE id=1`)
	require.NoError(t, err)
	forceMemoryLeaseExpired(t, db, restored[0].ID)
	skipped, err = repo.Claim(ctx, "disabled-submitted", 1, time.Minute, 5, []int64{1, 2})
	require.NoError(t, err)
	require.Empty(t, skipped)
	_, err = db.Exec(`UPDATE users SET status='active' WHERE id=1`)
	require.NoError(t, err)
	restored, err = repo.Claim(ctx, "active-after-submitted", 1, time.Minute, 5, []int64{1, 2})
	require.NoError(t, err)
	require.Len(t, restored, 1)
	require.Equal(t, service.MemoryArchiveNeedsReconciliation, restored[0].State, "reactivation cannot replay an uncertain submitted event")
}

func TestMemoryArchivePostgresRetiresOldPayloadWithoutReplayingUncertainWrite(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	_, err := repo.Enqueue(ctx, memoryTestTurn(1, "old-pending"))
	require.NoError(t, err)
	_, err = repo.Enqueue(ctx, memoryTestTurn(2, "old-unknown"))
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE memory_archive_queue SET created_at=NOW()-INTERVAL '8 days', state=CASE WHEN user_id=2 THEN 'needs_reconciliation' ELSE state END`)
	require.NoError(t, err)
	events, err := repo.Claim(ctx, "retirement", 1, time.Minute, 5, []int64{1, 2})
	require.NoError(t, err)
	require.Empty(t, events)
	pending, err := repo.Get(ctx, 1, "old-pending")
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchiveFailed, pending.State)
	uncertain, err := repo.Get(ctx, 2, "old-unknown")
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchiveNeedsReconciliation, uncertain.State)
	var raw int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM memory_archive_queue WHERE payload IS NOT NULL`).Scan(&raw))
	require.Zero(t, raw)
}

func TestMemoryArchivePostgresProviderPanicIsUncertainAndNotReplayed(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	_, err := repo.Enqueue(ctx, memoryTestTurn(1, "provider-panic"))
	require.NoError(t, err)
	provider := &memoryComponentProvider{panicAfterCall: true, reconcile: service.MemoryArchiveReconciliationUnknown}
	worker := service.NewMemoryArchiveWorker(repo, provider, service.MemoryArchiveWorkerOptions{UserIDs: []int64{1}, BatchSize: 1})
	require.NoError(t, worker.RunOnce(ctx))
	receipt, err := repo.Get(ctx, 1, "provider-panic")
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchiveNeedsReconciliation, receipt.State)
	_, err = db.Exec(`UPDATE memory_archive_queue SET available_at=NOW()-INTERVAL '1 second' WHERE id=$1`, receipt.ID)
	require.NoError(t, err)
	require.NoError(t, worker.RunOnce(ctx))
	require.EqualValues(t, 1, provider.archiveCalls.Load())
}

func TestMemoryArchivePostgresExpiredLeaseCannotBypassPayloadRetention(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	_, err := repo.Enqueue(ctx, memoryTestTurn(1, "old-processing"))
	require.NoError(t, err)
	_, err = repo.Enqueue(ctx, memoryTestTurn(2, "old-submitted"))
	require.NoError(t, err)
	events, err := repo.Claim(ctx, "old-worker", 2, time.Minute, 5, []int64{1, 2})
	require.NoError(t, err)
	require.Len(t, events, 2)
	for _, event := range events {
		if event.Turn.EventID == "old-submitted" {
			require.NoError(t, repo.MarkSubmitted(ctx, event))
		}
	}
	_, err = db.Exec(`UPDATE memory_archive_queue SET created_at=NOW()-INTERVAL '8 days', lease_until=NOW()-INTERVAL '1 second'`)
	require.NoError(t, err)
	claimed, err := repo.Claim(ctx, "recovery-worker", 2, time.Minute, 5, []int64{1, 2})
	require.NoError(t, err)
	require.Empty(t, claimed, "lease recovery must not let expired source text be delivered")
	for _, tc := range []struct {
		user  int64
		event string
		state service.MemoryArchiveState
	}{
		{1, "old-processing", service.MemoryArchiveFailed},
		{2, "old-submitted", service.MemoryArchiveNeedsReconciliation},
	} {
		receipt, err := repo.Get(ctx, tc.user, tc.event)
		require.NoError(t, err)
		require.Equal(t, tc.state, receipt.State)
		require.Equal(t, "payload_retention_expired", receipt.LastErrorCode)
	}
	var rawRows int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM memory_archive_queue WHERE payload IS NOT NULL`).Scan(&rawRows))
	require.Zero(t, rawRows)
}

func TestMemoryArchivePostgresNoopCannotAcknowledgeDurableEvent(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	_, err := repo.Enqueue(ctx, memoryTestTurn(1, "noop-must-not-complete"))
	require.NoError(t, err)
	worker := service.NewMemoryArchiveWorker(repo, service.NoopMemoryProvider{}, service.MemoryArchiveWorkerOptions{UserIDs: []int64{1}, BatchSize: 1})
	require.Error(t, worker.RunOnce(ctx))
	receipt, err := repo.Get(ctx, 1, "noop-must-not-complete")
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchivePending, receipt.State)
	require.Zero(t, receipt.Attempts)
}

func TestMemoryArchivePostgresQueueCapacityIsPerUserAndPreservesDuplicates(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	turn := memoryTestTurn(1, "first-event")
	first, err := repo.Enqueue(ctx, turn)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO memory_archive_queue (user_id,event_id,generation,payload,payload_sha256)
		SELECT 1, 'fill-' || n, 1, q.payload, q.payload_sha256 FROM memory_archive_queue q, generate_series(1,999) n WHERE q.id=$1`, first.ID)
	require.NoError(t, err)
	_, err = repo.Enqueue(ctx, memoryTestTurn(1, "over-capacity"))
	require.ErrorIs(t, err, service.ErrMemoryArchiveQueueFull)
	duplicate, err := repo.Enqueue(ctx, turn)
	require.NoError(t, err)
	require.True(t, duplicate.Duplicate, "capacity cannot hide an existing durable event receipt")
	require.Equal(t, first.ID, duplicate.ID)
	_, err = repo.Enqueue(ctx, memoryTestTurn(2, "different-user"))
	require.NoError(t, err, "one user's full queue cannot block another user's events")
}
