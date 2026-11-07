package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

var ErrMemoryNotConfigured = errors.New("memory is not configured for this user")

type MemoryGatewayOptions struct {
	UserIDs       []int64
	RecallTimeout time.Duration
}

// MemoryGateway owns request-side tenant admission and the durable worker's
// lifecycle. A missing/deleted/blocked tenant never falls back to another vault.
type MemoryGateway struct {
	service       *MemoryService
	repo          MemoryArchiveRepository
	users         map[int64]bool
	recallTimeout time.Duration
	worker        *MemoryArchiveWorker
}

func NewMemoryGateway(provider MemoryProvider, repo MemoryArchiveRepository, options MemoryGatewayOptions) *MemoryGateway {
	users := make(map[int64]bool, len(options.UserIDs))
	for _, id := range options.UserIDs {
		if id > 0 {
			users[id] = true
		}
	}
	if options.RecallTimeout <= 0 {
		options.RecallTimeout = 300 * time.Millisecond
	}
	return &MemoryGateway{service: NewMemoryService(provider), repo: repo, users: users, recallTimeout: options.RecallTimeout}
}

func ProvideMemoryGateway(cfg *config.Config, repo MemoryArchiveRepository) (*MemoryGateway, error) {
	if cfg == nil || !cfg.Memory.Enabled {
		g := NewMemoryGateway(nil, repo, MemoryGatewayOptions{})
		return g, nil
	}
	if err := cfg.Memory.Validate(); err != nil {
		return nil, err
	}
	bindings := make([]RespireMemoryBinding, 0, len(cfg.Memory.Bindings))
	ids := make([]int64, 0, len(cfg.Memory.Bindings))
	for _, b := range cfg.Memory.Bindings {
		bindings = append(bindings, RespireMemoryBinding{UserID: b.UserID, BaseURL: b.BaseURL, DataDir: b.DataDir})
		ids = append(ids, b.UserID)
	}
	provider, err := NewRespireMemoryProvider(bindings, RespireMemoryOptions{
		Timeout:          time.Duration(cfg.Memory.ArchiveTimeoutMS) * time.Millisecond,
		QueueWait:        time.Duration(min(cfg.Memory.RecallTimeoutMS, cfg.Memory.ArchiveTimeoutMS, 250)) * time.Millisecond,
		MaxResponseBytes: cfg.Memory.MaxResponseBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("configure memory: %w", err)
	}
	g := NewMemoryGateway(provider, repo, MemoryGatewayOptions{UserIDs: ids, RecallTimeout: time.Duration(cfg.Memory.RecallTimeoutMS) * time.Millisecond})
	g.worker = NewMemoryArchiveWorker(repo, provider, MemoryArchiveWorkerOptions{
		UserIDs:          ids,
		PollInterval:     time.Duration(cfg.Memory.PollIntervalMS) * time.Millisecond,
		Lease:            time.Duration(cfg.Memory.LeaseSeconds) * time.Second,
		OperationTimeout: time.Duration(cfg.Memory.ArchiveTimeoutMS) * time.Millisecond,
		MaxAttempts:      cfg.Memory.MaxAttempts,
	})
	return g, nil
}

func (g *MemoryGateway) EnabledFor(userID int64) bool {
	return g != nil && g.repo != nil && g.users[userID]
}

func (g *MemoryGateway) Recall(ctx context.Context, request MemoryRecallRequest) (MemoryRecallResult, error) {
	if !g.EnabledFor(request.Scope.UserID) {
		return MemoryRecallResult{}, ErrMemoryNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, g.recallTimeout)
	defer cancel()
	before, err := g.repo.ActiveGeneration(ctx, request.Scope.UserID)
	if err != nil {
		return MemoryRecallResult{}, err
	}
	result, err := g.service.Recall(ctx, request)
	if err != nil {
		return MemoryRecallResult{}, err
	}
	after, err := g.repo.ActiveGeneration(ctx, request.Scope.UserID)
	if err != nil {
		return MemoryRecallResult{}, err
	}
	if before != after {
		return MemoryRecallResult{}, ErrMemoryArchiveUserBlocked
	}
	return result, nil
}

func (g *MemoryGateway) Enqueue(ctx context.Context, turn MemoryTurn) (MemoryArchiveReceipt, error) {
	if !g.EnabledFor(turn.Scope.UserID) {
		return MemoryArchiveReceipt{}, ErrMemoryNotConfigured
	}
	return g.repo.Enqueue(ctx, turn)
}

func (g *MemoryGateway) ActiveGeneration(ctx context.Context, userID int64) (int64, error) {
	if !g.EnabledFor(userID) {
		return 0, ErrMemoryNotConfigured
	}
	return g.repo.ActiveGeneration(ctx, userID)
}

func (g *MemoryGateway) Suspend(ctx context.Context, userID int64) (int64, error) {
	if !g.EnabledFor(userID) {
		return 0, ErrMemoryNotConfigured
	}
	return g.repo.BlockUser(ctx, userID)
}

func (g *MemoryGateway) Event(ctx context.Context, userID int64, eventID string) (MemoryArchiveReceipt, error) {
	if !g.EnabledFor(userID) {
		return MemoryArchiveReceipt{}, ErrMemoryNotConfigured
	}
	return g.repo.Get(ctx, userID, eventID)
}

func (g *MemoryGateway) Overview(ctx context.Context, userID int64) (MemoryArchiveOverview, error) {
	if !g.EnabledFor(userID) {
		return MemoryArchiveOverview{}, ErrMemoryNotConfigured
	}
	return g.repo.Overview(ctx, userID)
}

func (g *MemoryGateway) Stop() {
	if g != nil && g.worker != nil {
		g.worker.Stop()
	}
}

// Start is called only after complete application initialization succeeds.
// Merely constructing a handler must not begin processing persistent events.
func (g *MemoryGateway) Start() {
	if g != nil && g.worker != nil {
		g.worker.Start()
	}
}

// MemoryErrorReason is a bounded diagnostic code, never a raw model, database
// or Runtime error that may contain text, connection credentials or paths.
func MemoryErrorReason(err error) string {
	if err == nil {
		return ""
	}
	var rpc *RespireMemoryError
	if errors.As(err, &rpc) {
		return rpc.Code
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, ErrMemoryNotConfigured):
		return "not_configured"
	case errors.Is(err, ErrMemoryArchiveUserBlocked):
		return "user_suspended"
	case errors.Is(err, ErrMemoryArchiveQueueFull):
		return "queue_full"
	case errors.Is(err, ErrMemoryArchiveEventConflict):
		return "event_conflict"
	default:
		return "unavailable"
	}
}

type memoryOriginalBodyKey struct{}

// WithMemoryOriginalBody preserves identity inputs before prompt injection.
// The copy is owned by the request and cannot be supplied by an HTTP header.
func WithMemoryOriginalBody(ctx context.Context, body []byte) context.Context {
	return context.WithValue(ctx, memoryOriginalBodyKey{}, append([]byte(nil), body...))
}

func memoryRequestIdentityBody(c *gin.Context, fallback []byte) []byte {
	if c != nil && c.Request != nil {
		if original, ok := c.Request.Context().Value(memoryOriginalBodyKey{}).([]byte); ok {
			return original
		}
	}
	return fallback
}
