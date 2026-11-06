//go:build memorycomponent && !integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// An independently deployed worker can only write the users whose Runtime
// bindings it owns. Calling a foreign provider is an observable hard failure,
// rather than a fake provider silently accepting every tenant.
type memoryScopedComponentProvider struct {
	mu      sync.Mutex
	allowed map[int64]bool
	writes  []int64
}

func (p *memoryScopedComponentProvider) Archive(_ context.Context, turn service.MemoryTurn) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes = append(p.writes, turn.Scope.UserID)
	if !p.allowed[turn.Scope.UserID] {
		return fmt.Errorf("synthetic provider has no binding for user %d", turn.Scope.UserID)
	}
	return nil
}

func (*memoryScopedComponentProvider) Recall(context.Context, service.MemoryRecallRequest) (service.MemoryRecallResult, error) {
	return service.MemoryRecallResult{}, nil
}

func (*memoryScopedComponentProvider) Delete(context.Context, service.MemoryScope, string) error {
	return nil
}

func (p *memoryScopedComponentProvider) snapshot() []int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]int64(nil), p.writes...)
}

func TestMemoryArchivePostgresWorkersOnlyClaimTheirBoundUsers(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	// The foreign user's older event must not be consumed by the first worker.
	foreign, err := repo.Enqueue(ctx, memoryTestTurn(2, "u2-first-on-shared-database"))
	require.NoError(t, err)
	owned, err := repo.Enqueue(ctx, memoryTestTurn(1, "u1-second-on-shared-database"))
	require.NoError(t, err)
	p1 := &memoryScopedComponentProvider{allowed: map[int64]bool{1: true}}
	p2 := &memoryScopedComponentProvider{allowed: map[int64]bool{2: true}}
	w1 := service.NewMemoryArchiveWorker(repo, p1, service.MemoryArchiveWorkerOptions{BatchSize: 8, UserIDs: []int64{1}})
	w2 := service.NewMemoryArchiveWorker(repo, p2, service.MemoryArchiveWorkerOptions{BatchSize: 8, UserIDs: []int64{2}})
	require.NoError(t, w1.RunOnce(ctx))
	require.Equal(t, []int64{1}, p1.snapshot())
	u1, err := repo.Get(ctx, 1, owned.EventID)
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchiveCompleted, u1.State)
	u2, err := repo.Get(ctx, 2, foreign.EventID)
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchivePending, u2.State)
	require.Zero(t, u2.Attempts)
	var foreignPayload sql.NullString
	var foreignLease sql.NullString
	require.NoError(t, db.QueryRow(`SELECT payload::text, lease_token::text FROM memory_archive_queue WHERE id=$1`, foreign.ID).Scan(&foreignPayload, &foreignLease))
	require.True(t, foreignPayload.Valid, "an unbound worker must preserve the other user's source text")
	require.Contains(t, foreignPayload.String, "synthetic preference: blue")
	require.False(t, foreignLease.Valid, "an unbound worker must never claim the foreign event")
	require.NoError(t, w2.RunOnce(ctx))
	require.Equal(t, []int64{2}, p2.snapshot())
	u2, err = repo.Get(ctx, 2, foreign.EventID)
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchiveCompleted, u2.State)
}

func TestMemoryArchivePostgresClaimRequiresExplicitValidUserScope(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	for _, user := range []int64{1, 2} {
		_, err := repo.Enqueue(ctx, memoryTestTurn(user, fmt.Sprintf("scope-validation-%d", user)))
		require.NoError(t, err)
	}
	for _, ids := range [][]int64{nil, {}} {
		events, err := repo.Claim(ctx, "unscoped-worker", 2, time.Minute, 5, ids)
		require.NoError(t, err)
		require.Empty(t, events)
	}
	for _, ids := range [][]int64{{0}, {-1}, {1, 0}, {2, -1}} {
		events, err := repo.Claim(ctx, "invalid-scope-worker", 2, time.Minute, 5, ids)
		require.Error(t, err)
		require.Empty(t, events)
	}
	var pending, leased int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FILTER (WHERE state='pending' AND payload IS NOT NULL), COUNT(*) FILTER (WHERE lease_token IS NOT NULL) FROM memory_archive_queue`).Scan(&pending, &leased))
	require.Equal(t, 2, pending)
	require.Zero(t, leased, "empty or invalid admission must not change a queue claim")
	events, err := repo.Claim(ctx, "multi-user-worker", 2, time.Minute, 5, []int64{1, 2, 1})
	require.NoError(t, err)
	require.Len(t, events, 2)
	users := []int64{events[0].Turn.Scope.UserID, events[1].Turn.Scope.UserID}
	require.ElementsMatch(t, []int64{1, 2}, users, "a valid multi-user scope must include each owned user once")
}

func TestMemoryArchivePostgresWorkerRejectsMissingScopeAndOwnsItsScopeCopy(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	for _, user := range []int64{2, 1} {
		_, err := repo.Enqueue(ctx, memoryTestTurn(user, fmt.Sprintf("worker-scope-%d", user)))
		require.NoError(t, err)
	}
	provider := &memoryScopedComponentProvider{allowed: map[int64]bool{1: true}}
	for _, ids := range [][]int64{nil, {}, {0}, {1, -1}} {
		worker := service.NewMemoryArchiveWorker(repo, provider, service.MemoryArchiveWorkerOptions{BatchSize: 8, UserIDs: ids})
		require.Error(t, worker.RunOnce(ctx))
	}
	require.Empty(t, provider.snapshot())
	ids := []int64{1, 1}
	worker := service.NewMemoryArchiveWorker(repo, provider, service.MemoryArchiveWorkerOptions{BatchSize: 8, UserIDs: ids})
	ids[0], ids[1] = 2, 2
	require.NoError(t, worker.RunOnce(ctx))
	require.Equal(t, []int64{1}, provider.snapshot(), "caller mutations must not change admitted users")
	foreign, err := repo.Get(ctx, 2, "worker-scope-2")
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchivePending, foreign.State)
	require.Zero(t, foreign.Attempts)
}
