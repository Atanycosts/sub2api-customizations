package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Archive states describe queue durability, not the truth of the archived text.
type MemoryArchiveState string

const (
	MemoryArchivePending             MemoryArchiveState = "pending"
	MemoryArchiveProcessing          MemoryArchiveState = "processing"
	MemoryArchiveSubmitted           MemoryArchiveState = "submitted"
	MemoryArchiveRetryWait           MemoryArchiveState = "retry_wait"
	MemoryArchiveNeedsReconciliation MemoryArchiveState = "needs_reconciliation"
	MemoryArchiveCompleted           MemoryArchiveState = "completed"
	MemoryArchiveFailed              MemoryArchiveState = "failed"
	MemoryArchiveCancelled           MemoryArchiveState = "cancelled"
)

var (
	ErrMemoryArchiveEventConflict = errors.New("memory archive event ID has different content")
	ErrMemoryArchiveClaimLost     = errors.New("memory archive lease or generation is no longer owned")
	ErrMemoryArchiveUserBlocked   = errors.New("memory archive user is blocked")
	ErrMemoryArchiveQueueFull     = errors.New("memory archive queue is full for this user")
)

type MemoryArchiveReceipt struct {
	ID            int64              `json:"id"`
	EventID       string             `json:"event_id"`
	State         MemoryArchiveState `json:"state"`
	Duplicate     bool               `json:"duplicate"`
	Attempts      int                `json:"attempts"`
	LastErrorCode string             `json:"last_error_code,omitempty"`
}

// LeaseToken is unique to one claim, including when a worker reclaims its own
// expired event. Generation fences user-wide deletion/block operations.
type MemoryArchiveEvent struct {
	ID                   int64
	Turn                 MemoryTurn
	Generation           int64
	State                MemoryArchiveState
	Attempts             int
	ReconciliationChecks int
	LeaseToken           string
}

type MemoryArchiveRepository interface {
	Enqueue(context.Context, MemoryTurn) (MemoryArchiveReceipt, error)
	Get(context.Context, int64, string) (MemoryArchiveReceipt, error)
	// Claim may only admit users explicitly routed to this worker's provider.
	// An empty scope claims nothing, never every tenant in a shared database.
	Claim(context.Context, string, int, time.Duration, int, []int64) ([]MemoryArchiveEvent, error)
	MarkSubmitted(context.Context, MemoryArchiveEvent) error
	Finish(context.Context, MemoryArchiveEvent, MemoryArchiveState, time.Time, string) error
	ActiveGeneration(context.Context, int64) (int64, error)
	// BlockUser permanently fences reads and future writes in this deployment.
	// It is not evidence that an in-flight engine write or a synced copy was
	// physically erased. There is deliberately no automatic unblock operation.
	BlockUser(context.Context, int64) (int64, error)
}

type MemoryArchiveReconciliation string

const (
	MemoryArchiveReconciliationPending   MemoryArchiveReconciliation = "pending"
	MemoryArchiveReconciliationCompleted MemoryArchiveReconciliation = "completed"
	MemoryArchiveReconciliationUnknown   MemoryArchiveReconciliation = "unknown"
	// Rejected is permitted only with positive evidence that the original
	// operation did not commit. An expired result cache is Unknown, not Rejected.
	MemoryArchiveReconciliationRejected MemoryArchiveReconciliation = "rejected"
)

type MemoryArchiveReconciler interface {
	ReconcileArchive(context.Context, MemoryTurn) (MemoryArchiveReconciliation, error)
}

// Provider errors can implement these interfaces. Only a positively known
// pre-submit transient error is retryable. Unclassified errors are uncertain.
type MemoryArchiveOutcomeUnknown interface{ OutcomeUnknown() bool }
type MemoryArchiveRetryable interface{ Retryable() bool }

type MemoryArchiveWorkerOptions struct {
	UserIDs                 []int64
	PollInterval            time.Duration
	Lease                   time.Duration
	OperationTimeout        time.Duration
	PersistenceTimeout      time.Duration
	RetryBase               time.Duration
	RetryMax                time.Duration
	BatchSize               int
	MaxAttempts             int
	MaxReconciliationChecks int
}

func (o MemoryArchiveWorkerOptions) normalize() MemoryArchiveWorkerOptions {
	// Own the admission scope so callers cannot change a running worker's
	// routing by mutating the configuration slice. Reject the entire scope on
	// an invalid ID rather than silently turning a misconfiguration into work.
	ids := make([]int64, 0, len(o.UserIDs))
	seen := make(map[int64]bool, len(o.UserIDs))
	for _, id := range o.UserIDs {
		if id <= 0 {
			ids = nil
			break
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	o.UserIDs = ids
	if o.PollInterval <= 0 {
		o.PollInterval = time.Second
	}
	if o.OperationTimeout <= 0 {
		o.OperationTimeout = 15 * time.Second
	}
	if o.PersistenceTimeout <= 0 {
		o.PersistenceTimeout = 3 * time.Second
	}
	// A batch is processed serially. Claim exactly one at a time so other
	// events' leases do not expire while the first provider call runs.
	if o.BatchSize <= 0 || o.BatchSize > 32 {
		o.BatchSize = 8
	}
	minimumLease := o.OperationTimeout + 2*o.PersistenceTimeout + 5*time.Second
	if o.Lease < minimumLease {
		o.Lease = minimumLease
	}
	if o.RetryBase <= 0 {
		o.RetryBase = 2 * time.Second
	}
	if o.RetryMax < o.RetryBase {
		o.RetryMax = 5 * time.Minute
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 5
	}
	if o.MaxReconciliationChecks <= 0 {
		o.MaxReconciliationChecks = 5
	}
	return o
}

type MemoryArchiveWorker struct {
	repo     MemoryArchiveRepository
	provider MemoryProvider
	options  MemoryArchiveWorkerOptions
	workerID string
	ctx      context.Context
	cancel   context.CancelFunc
	start    sync.Once
	stop     sync.Once
	wg       sync.WaitGroup
	runMu    sync.Mutex
}

func NewMemoryArchiveWorker(repo MemoryArchiveRepository, provider MemoryProvider, options MemoryArchiveWorkerOptions) *MemoryArchiveWorker {
	ctx, cancel := context.WithCancel(context.Background())
	return &MemoryArchiveWorker{repo: repo, provider: provider, options: options.normalize(), workerID: uuid.NewString(), ctx: ctx, cancel: cancel}
}

func (w *MemoryArchiveWorker) Start() {
	if !w.configured() {
		return
	}
	w.start.Do(func() {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			ticker := time.NewTicker(w.options.PollInterval)
			defer ticker.Stop()
			for {
				if w.ctx.Err() != nil {
					return
				}
				if err := w.RunOnce(w.ctx); err != nil && w.ctx.Err() == nil {
					// Do not log provider errors, payloads, or request text.
					slog.Warn("memory archive worker persistence operation failed")
				}
				select {
				case <-w.ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	})
}

func (w *MemoryArchiveWorker) Stop() {
	if w == nil {
		return
	}
	w.stop.Do(func() { w.cancel() })
	w.wg.Wait()
}

// RunOnce processes at most BatchSize events and never starts a background
// goroutine. A durable submitted transition precedes every provider write.
func (w *MemoryArchiveWorker) RunOnce(ctx context.Context) error {
	if !w.configured() {
		return errors.New("memory archive worker is not configured")
	}
	w.runMu.Lock()
	defer w.runMu.Unlock()
	for range w.options.BatchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		claimCtx, cancel := context.WithTimeout(ctx, w.options.PersistenceTimeout)
		events, err := w.repo.Claim(claimCtx, w.workerID, 1, w.options.Lease, w.options.MaxReconciliationChecks, w.options.UserIDs)
		cancel()
		if err != nil {
			return fmt.Errorf("claim memory archive: %w", err)
		}
		if len(events) == 0 {
			return nil
		}
		if err := w.process(ctx, events[0]); err != nil {
			return err
		}
	}
	return nil
}

func (w *MemoryArchiveWorker) configured() bool {
	if w == nil || w.repo == nil || w.provider == nil || len(w.options.UserIDs) == 0 {
		return false
	}
	switch w.provider.(type) {
	case NoopMemoryProvider, *NoopMemoryProvider:
		return false
	default:
		return true
	}
}

func (w *MemoryArchiveWorker) process(ctx context.Context, event MemoryArchiveEvent) error {
	if event.State == MemoryArchiveNeedsReconciliation {
		return w.reconcile(ctx, event)
	}
	if event.Attempts >= w.options.MaxAttempts {
		return w.finish(event, MemoryArchiveFailed, time.Now(), "attempt_limit")
	}
	markCtx, cancel := context.WithTimeout(ctx, w.options.PersistenceTimeout)
	err := w.repo.MarkSubmitted(markCtx, event)
	cancel()
	if err != nil {
		return fmt.Errorf("mark memory archive submitted: %w", err)
	}
	event.State = MemoryArchiveSubmitted
	event.Attempts++
	operationCtx, cancel := context.WithTimeout(ctx, w.options.OperationTimeout)
	err = invokeMemoryArchive(operationCtx, w.provider, event.Turn)
	cancel()
	if err == nil {
		return w.finish(event, MemoryArchiveCompleted, time.Now(), "")
	}
	var unknown MemoryArchiveOutcomeUnknown
	var retryable MemoryArchiveRetryable
	if errors.As(err, &unknown) && !unknown.OutcomeUnknown() {
		if errors.As(err, &retryable) && retryable.Retryable() && event.Attempts < w.options.MaxAttempts {
			return w.finish(event, MemoryArchiveRetryWait, w.nextAttempt(event.Attempts), "provider_pre_submit_retryable")
		}
		return w.finish(event, MemoryArchiveFailed, time.Now(), "provider_rejected")
	}
	return w.finish(event, MemoryArchiveNeedsReconciliation, w.nextAttempt(event.Attempts), "provider_outcome_unknown")
}

func (w *MemoryArchiveWorker) reconcile(ctx context.Context, event MemoryArchiveEvent) error {
	reconciler, ok := w.provider.(MemoryArchiveReconciler)
	if !ok {
		// No status API is not proof of non-commit. Preserve the event for
		// operator reconciliation, without making further automatic RPC calls.
		return w.finish(event, MemoryArchiveNeedsReconciliation, time.Now().Add(100*365*24*time.Hour), "reconciliation_unavailable")
	}
	operationCtx, cancel := context.WithTimeout(ctx, w.options.OperationTimeout)
	state, err := invokeMemoryArchiveReconciliation(operationCtx, reconciler, event.Turn)
	cancel()
	if err == nil && state == MemoryArchiveReconciliationCompleted {
		return w.finish(event, MemoryArchiveCompleted, time.Now(), "")
	}
	if err == nil && state == MemoryArchiveReconciliationRejected {
		if event.Attempts < w.options.MaxAttempts {
			return w.finish(event, MemoryArchiveRetryWait, w.nextAttempt(event.Attempts), "provider_confirmed_not_committed")
		}
		return w.finish(event, MemoryArchiveFailed, time.Now(), "attempt_limit")
	}
	code := "provider_outcome_unknown"
	if err == nil && state == MemoryArchiveReconciliationPending {
		code = "provider_pending"
	}
	return w.finish(event, MemoryArchiveNeedsReconciliation, w.nextAttempt(event.ReconciliationChecks), code)
}

func invokeMemoryArchive(ctx context.Context, provider MemoryProvider, turn MemoryTurn) (err error) {
	defer func() {
		if recover() != nil {
			// A provider may panic after committing. An unclassified error is
			// deliberately uncertain and must never cause a blind write replay.
			err = errors.New("memory archive provider panicked")
		}
	}()
	return provider.Archive(ctx, turn)
}

func invokeMemoryArchiveReconciliation(ctx context.Context, provider MemoryArchiveReconciler, turn MemoryTurn) (state MemoryArchiveReconciliation, err error) {
	defer func() {
		if recover() != nil {
			state = MemoryArchiveReconciliationUnknown
			err = errors.New("memory archive reconciliation panicked")
		}
	}()
	return provider.ReconcileArchive(ctx, turn)
}

func (w *MemoryArchiveWorker) nextAttempt(attempt int) time.Time {
	delay := w.options.RetryBase
	for i := 1; i < attempt && delay < w.options.RetryMax; i++ {
		if delay > w.options.RetryMax/2 {
			delay = w.options.RetryMax
			break
		}
		delay *= 2
	}
	return time.Now().Add(delay)
}

func (w *MemoryArchiveWorker) finish(event MemoryArchiveEvent, state MemoryArchiveState, next time.Time, code string) error {
	// Parent cancellation must not prevent us from recording a known reply.
	// On persistence failure the submitted row remains uncertain and recovery
	// performs read-only reconciliation after lease expiry.
	ctx, cancel := context.WithTimeout(context.Background(), w.options.PersistenceTimeout)
	defer cancel()
	if err := w.repo.Finish(ctx, event, state, next, code); err != nil {
		return fmt.Errorf("settle memory archive: %w", err)
	}
	return nil
}
