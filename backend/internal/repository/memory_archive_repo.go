package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const maxPendingMemoryEventsPerUser = 1000

type memoryArchiveRepository struct{ db *sql.DB }

func NewMemoryArchiveRepository(db *sql.DB) service.MemoryArchiveRepository {
	return &memoryArchiveRepository{db: db}
}

func (r *memoryArchiveRepository) Enqueue(ctx context.Context, turn service.MemoryTurn) (service.MemoryArchiveReceipt, error) {
	if err := turn.Validate(); err != nil {
		return service.MemoryArchiveReceipt{}, err
	}
	if turn.EventID == "" || len(turn.EventID) > 128 || !utf8.ValidString(turn.EventID) || strings.TrimSpace(turn.EventID) != turn.EventID || strings.ContainsAny(turn.EventID, "\x00\r\n\t") {
		return service.MemoryArchiveReceipt{}, errors.New("memory archive requires an event ID of 1 to 128 bytes")
	}
	payload, err := json.Marshal(turn)
	if err != nil {
		return service.MemoryArchiveReceipt{}, err
	}
	if len(payload) > 4*1024*1024 {
		return service.MemoryArchiveReceipt{}, errors.New("memory archive payload exceeds limit")
	}
	digest := sha256.Sum256(payload)
	digestText := hex.EncodeToString(digest[:])
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return service.MemoryArchiveReceipt{}, err
	}
	defer func() { _ = tx.Rollback() }()
	// A tenant row serializes enqueue, user blocking and capacity checks.
	if _, err := tx.ExecContext(ctx, `INSERT INTO memory_archive_tenants (user_id)
		SELECT id FROM users WHERE id = $1 AND deleted_at IS NULL AND status = 'active'
		ON CONFLICT (user_id) DO NOTHING`, turn.Scope.UserID); err != nil {
		return service.MemoryArchiveReceipt{}, err
	}
	var generation int64
	var blocked bool
	if err := tx.QueryRowContext(ctx, `SELECT t.generation, t.blocked FROM memory_archive_tenants t
		JOIN users u ON u.id = t.user_id AND u.deleted_at IS NULL AND u.status = 'active'
		WHERE t.user_id = $1 FOR UPDATE OF t`, turn.Scope.UserID).Scan(&generation, &blocked); err != nil {
		return service.MemoryArchiveReceipt{}, err
	}
	if blocked {
		return service.MemoryArchiveReceipt{}, service.ErrMemoryArchiveUserBlocked
	}
	var receipt service.MemoryArchiveReceipt
	var existingDigest string
	err = tx.QueryRowContext(ctx, `SELECT id, event_id, state, attempts, COALESCE(last_error_code, ''), payload_sha256
		FROM memory_archive_queue WHERE user_id = $1 AND event_id = $2`, turn.Scope.UserID, turn.EventID).
		Scan(&receipt.ID, &receipt.EventID, &receipt.State, &receipt.Attempts, &receipt.LastErrorCode, &existingDigest)
	if err == nil {
		if existingDigest != digestText {
			return service.MemoryArchiveReceipt{}, service.ErrMemoryArchiveEventConflict
		}
		receipt.Duplicate = true
		return receipt, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return service.MemoryArchiveReceipt{}, err
	}
	var pending int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_archive_queue WHERE user_id = $1
		AND state NOT IN ('completed', 'failed', 'cancelled')`, turn.Scope.UserID).Scan(&pending); err != nil {
		return service.MemoryArchiveReceipt{}, err
	}
	if pending >= maxPendingMemoryEventsPerUser {
		return service.MemoryArchiveReceipt{}, service.ErrMemoryArchiveQueueFull
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO memory_archive_queue (user_id, event_id, generation, payload, payload_sha256)
		VALUES ($1, $2, $3, $4::jsonb, $5) RETURNING id, event_id, state`, turn.Scope.UserID, turn.EventID, generation, string(payload), digestText).
		Scan(&receipt.ID, &receipt.EventID, &receipt.State)
	if err != nil {
		return service.MemoryArchiveReceipt{}, err
	}
	return receipt, tx.Commit()
}

func (r *memoryArchiveRepository) Get(ctx context.Context, userID int64, eventID string) (service.MemoryArchiveReceipt, error) {
	if userID <= 0 || eventID == "" {
		return service.MemoryArchiveReceipt{}, service.ErrMemoryUserRequired
	}
	var receipt service.MemoryArchiveReceipt
	err := r.db.QueryRowContext(ctx, `SELECT id, event_id, state, attempts, COALESCE(last_error_code, '')
		FROM memory_archive_queue WHERE user_id = $1 AND event_id = $2`, userID, eventID).
		Scan(&receipt.ID, &receipt.EventID, &receipt.State, &receipt.Attempts, &receipt.LastErrorCode)
	return receipt, err
}

// Overview returns receipt metadata only. Source text and Runtime configuration
// never leave the archive repository through the account UI.
func (r *memoryArchiveRepository) Overview(ctx context.Context, userID int64) (service.MemoryArchiveOverview, error) {
	if userID <= 0 {
		return service.MemoryArchiveOverview{}, service.ErrMemoryUserRequired
	}
	result := service.MemoryArchiveOverview{Recent: make([]service.MemoryArchiveActivity, 0)}
	err := r.db.QueryRowContext(ctx, `SELECT
		COUNT(*) FILTER (WHERE state = 'completed'),
		COUNT(*) FILTER (WHERE state IN ('pending', 'processing', 'submitted', 'retry_wait')),
		COUNT(*) FILTER (WHERE state IN ('failed', 'needs_reconciliation'))
		FROM memory_archive_queue WHERE user_id = $1`, userID).
		Scan(&result.Archived, &result.Pending, &result.Attention)
	if err != nil {
		return service.MemoryArchiveOverview{}, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT event_id, state, created_at, updated_at
		FROM memory_archive_queue WHERE user_id = $1 ORDER BY id DESC LIMIT 20`, userID)
	if err != nil {
		return service.MemoryArchiveOverview{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var event service.MemoryArchiveActivity
		if err := rows.Scan(&event.EventID, &event.State, &event.CreatedAt, &event.UpdatedAt); err != nil {
			return service.MemoryArchiveOverview{}, err
		}
		result.Recent = append(result.Recent, event)
	}
	return result, rows.Err()
}

func (r *memoryArchiveRepository) Claim(ctx context.Context, workerID string, limit int, lease time.Duration, maxReconciliationChecks int, userIDs []int64) ([]service.MemoryArchiveEvent, error) {
	if strings.TrimSpace(workerID) == "" || len(workerID) > 128 {
		return nil, errors.New("memory archive worker ID is required")
	}
	if len(userIDs) == 0 {
		return nil, nil
	}
	for _, id := range userIDs {
		if id <= 0 {
			return nil, errors.New("memory archive claim requires positive user IDs")
		}
	}
	if limit <= 0 || limit > 32 {
		limit = 1
	}
	if lease < time.Second {
		lease = time.Second
	}
	if maxReconciliationChecks <= 0 {
		maxReconciliationChecks = 5
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// Once submitted, an expired lease can ONLY enter reconciliation. A worker
	// dying before MarkSubmitted never invoked the provider, so processing is
	// safe to reclaim. Conditions are evaluated under row locks in PostgreSQL.
	if _, err := tx.ExecContext(ctx, `UPDATE memory_archive_queue
		SET state = CASE WHEN state = 'submitted' THEN 'needs_reconciliation' ELSE 'pending' END,
			lease_token = NULL, lease_until = NULL, worker_id = NULL,
			last_error_code = CASE WHEN state = 'submitted' THEN 'submitted_lease_expired' ELSE last_error_code END,
			available_at = NOW(), updated_at = NOW()
		WHERE state IN ('processing', 'submitted') AND lease_until <= NOW()
		  AND user_id = ANY($1::bigint[])`, pq.Array(userIDs)); err != nil {
		return nil, err
	}
	// Recover abandoned leases before enforcing retention. Otherwise an old
	// processing row could become pending and be delivered in this same claim
	// despite its raw text already having exceeded the retention limit.
	// Uncertain outcomes remain uncertain after text expires; never replay them.
	if _, err := tx.ExecContext(ctx, `UPDATE memory_archive_queue SET payload = NULL,
		state = CASE WHEN state IN ('pending', 'retry_wait') THEN 'failed' ELSE state END,
		last_error_code = 'payload_retention_expired', updated_at = NOW()
		WHERE payload IS NOT NULL AND created_at < NOW() - INTERVAL '7 days'
		  AND (lease_until IS NULL OR lease_until <= NOW())
		  AND state IN ('pending', 'retry_wait', 'needs_reconciliation')
		  AND user_id = ANY($1::bigint[])`, pq.Array(userIDs)); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT q.id, q.payload, q.generation, q.state, q.attempts, q.reconciliation_checks
		FROM memory_archive_queue q
		JOIN memory_archive_tenants t ON t.user_id = q.user_id
		JOIN users u ON u.id = q.user_id AND u.deleted_at IS NULL AND u.status = 'active'
		WHERE q.state IN ('pending', 'retry_wait', 'needs_reconciliation')
		  AND q.user_id = ANY($3::bigint[])
		  AND q.available_at <= NOW() AND (q.lease_until IS NULL OR q.lease_until <= NOW())
		  AND q.payload IS NOT NULL
		  AND NOT t.blocked AND t.generation = q.generation
		  AND (q.state <> 'needs_reconciliation' OR q.reconciliation_checks < $2)
		ORDER BY q.available_at, q.id LIMIT $1 FOR UPDATE OF q SKIP LOCKED`, limit, maxReconciliationChecks, pq.Array(userIDs))
	if err != nil {
		return nil, err
	}
	var events []service.MemoryArchiveEvent
	for rows.Next() {
		var event service.MemoryArchiveEvent
		var payload []byte
		if err := rows.Scan(&event.ID, &payload, &event.Generation, &event.State, &event.Attempts, &event.ReconciliationChecks); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := json.Unmarshal(payload, &event.Turn); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("memory archive payload %d is invalid", event.ID)
		}
		events = append(events, event)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range events {
		event := &events[i]
		event.LeaseToken = uuid.NewString()
		if event.State == service.MemoryArchiveNeedsReconciliation {
			event.ReconciliationChecks++
		} else {
			event.State = service.MemoryArchiveProcessing
		}
		if _, err := tx.ExecContext(ctx, `UPDATE memory_archive_queue SET state = $2,
			lease_token = $3, lease_until = NOW() + ($4 * INTERVAL '1 millisecond'),
			worker_id = $5, reconciliation_checks = $6, updated_at = NOW() WHERE id = $1`,
			event.ID, event.State, event.LeaseToken, lease.Milliseconds(), workerID, event.ReconciliationChecks); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return events, nil
}

func (r *memoryArchiveRepository) MarkSubmitted(ctx context.Context, event service.MemoryArchiveEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockMemoryArchiveTenant(ctx, tx, event); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE memory_archive_queue SET state = 'submitted', attempts = attempts + 1, updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND generation = $3 AND lease_token = $4
		  AND state = 'processing' AND lease_until > clock_timestamp()`, event.ID, event.Turn.Scope.UserID, event.Generation, event.LeaseToken)
	if err != nil {
		return err
	}
	if err := requireMemoryArchiveClaim(result); err != nil {
		return err
	}
	return tx.Commit()
}

func lockMemoryArchiveTenant(ctx context.Context, tx *sql.Tx, event service.MemoryArchiveEvent) error {
	var generation int64
	var blocked bool
	if err := tx.QueryRowContext(ctx, `SELECT t.generation, t.blocked FROM memory_archive_tenants t
		JOIN users u ON u.id = t.user_id AND u.deleted_at IS NULL AND u.status = 'active'
		WHERE t.user_id = $1 FOR UPDATE OF t`, event.Turn.Scope.UserID).Scan(&generation, &blocked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrMemoryArchiveClaimLost
		}
		return err
	}
	if blocked || generation != event.Generation {
		return service.ErrMemoryArchiveClaimLost
	}
	return nil
}

func requireMemoryArchiveClaim(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return service.ErrMemoryArchiveClaimLost
	}
	return nil
}

func (r *memoryArchiveRepository) Finish(ctx context.Context, event service.MemoryArchiveEvent, state service.MemoryArchiveState, availableAt time.Time, code string) error {
	switch state {
	case service.MemoryArchiveCompleted, service.MemoryArchiveFailed, service.MemoryArchiveRetryWait, service.MemoryArchiveNeedsReconciliation:
	default:
		return errors.New("invalid memory archive final state")
	}
	if event.State != service.MemoryArchiveSubmitted && event.State != service.MemoryArchiveNeedsReconciliation &&
		!(event.State == service.MemoryArchiveProcessing && state == service.MemoryArchiveFailed) {
		return errors.New("memory archive was not submitted or awaiting reconciliation")
	}
	// Only fixed status codes cross this boundary; raw provider errors can
	// contain request text, host paths and credentials.
	if len(code) > 64 || strings.ContainsAny(code, " \n\r\t") {
		return errors.New("invalid memory archive error code")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockMemoryArchiveTenant(ctx, tx, event); err != nil {
		return err
	}
	// Completed/failed events retain only a digest and status, never raw text.
	result, err := tx.ExecContext(ctx, `UPDATE memory_archive_queue SET state = $5,
		payload = CASE WHEN $5 IN ('completed', 'failed') THEN NULL ELSE payload END,
		available_at = $6, last_error_code = NULLIF($7, ''), lease_token = NULL, lease_until = NULL,
		worker_id = NULL, updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND generation = $3 AND lease_token = $4
		  AND state = $8 AND lease_until > clock_timestamp()`,
		event.ID, event.Turn.Scope.UserID, event.Generation, event.LeaseToken, string(state), availableAt, code, string(event.State))
	if err != nil {
		return err
	}
	if err := requireMemoryArchiveClaim(result); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *memoryArchiveRepository) ActiveGeneration(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, service.ErrMemoryUserRequired
	}
	var generation int64
	var blocked bool
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(t.generation, 1), COALESCE(t.blocked, FALSE)
		FROM users u LEFT JOIN memory_archive_tenants t ON t.user_id = u.id
		WHERE u.id = $1 AND u.deleted_at IS NULL AND u.status = 'active'`, userID).Scan(&generation, &blocked)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, service.ErrMemoryArchiveUserBlocked
	}
	if err != nil {
		return 0, err
	}
	if blocked {
		return 0, service.ErrMemoryArchiveUserBlocked
	}
	return generation, nil
}

func (r *memoryArchiveRepository) BlockUser(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, service.ErrMemoryUserRequired
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO memory_archive_tenants (user_id)
		SELECT id FROM users WHERE id = $1 AND deleted_at IS NULL ON CONFLICT (user_id) DO NOTHING`, userID); err != nil {
		return 0, err
	}
	var generation int64
	err = tx.QueryRowContext(ctx, `UPDATE memory_archive_tenants SET generation = CASE WHEN blocked THEN generation ELSE generation + 1 END,
		blocked = TRUE, updated_at = NOW() WHERE user_id = $1 RETURNING generation`, userID).Scan(&generation)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE memory_archive_queue SET
		state = CASE WHEN state IN ('completed', 'failed', 'cancelled') THEN state ELSE 'cancelled' END,
		payload = NULL, lease_token = NULL, lease_until = NULL, worker_id = NULL,
		last_error_code = 'user_blocked', updated_at = NOW() WHERE user_id = $1`, userID); err != nil {
		return 0, err
	}
	return generation, tx.Commit()
}
