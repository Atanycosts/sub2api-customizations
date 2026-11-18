package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
)

type RequestTimingFilter struct {
	RequestID string
	Outcome   string
	AccountID int64
	Limit     int
	Before    time.Time
}
type RequestTimingLink struct {
	TraceID   string
	UsageID   int64
	RequestID string
	APIKeyID  int64
}
type RequestTimingRepository interface {
	Save(context.Context, requesttiming.Trace) error
	LinkUsage(context.Context, RequestTimingLink) error
	List(context.Context, RequestTimingFilter) ([]requesttiming.Trace, error)
	Get(context.Context, string) (*requesttiming.Trace, error)
	Prune(context.Context, time.Time) error
}
type timingWrite struct {
	trace *requesttiming.Trace
	link  *RequestTimingLink
}
type RequestTimingStats struct {
	Queued      int    `json:"queued"`
	Dropped     uint64 `json:"dropped"`
	WriteFailed uint64 `json:"write_failed"`
}

// RequestTimingService has one bounded queue and one writer. Database latency
// never enters the response path. Retention is seven days, pruned hourly.
type RequestTimingService struct {
	repo         RequestTimingRepository
	queue        chan timingWrite
	mu           sync.RWMutex
	closed       bool
	done         chan struct{}
	dropped      atomic.Uint64
	failed       atomic.Uint64
	workerCtx    context.Context
	stop         context.CancelFunc
	drainTimeout time.Duration
}

func NewRequestTimingService(repo RequestTimingRepository) *RequestTimingService {
	return newRequestTimingService(repo, 512)
}
func newRequestTimingService(repo RequestTimingRepository, capacity int) *RequestTimingService {
	ctx, cancel := context.WithCancel(context.Background())
	s := &RequestTimingService{repo: repo, queue: make(chan timingWrite, capacity), done: make(chan struct{}), workerCtx: ctx, stop: cancel, drainTimeout: 3 * time.Second}
	go s.run()
	return s
}
func (s *RequestTimingService) enqueue(item timingWrite) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		s.dropped.Add(1)
		return
	}
	select {
	case s.queue <- item:
	default:
		s.dropped.Add(1)
	}
}
func (s *RequestTimingService) Submit(trace requesttiming.Trace) {
	s.enqueue(timingWrite{trace: &trace})
}
func (s *RequestTimingService) AttachUsage(id string, usageID int64, requestID string, apiKeyID int64) {
	s.enqueue(timingWrite{link: &RequestTimingLink{id, usageID, requestID, apiKeyID}})
}
func (s *RequestTimingService) Stats() RequestTimingStats {
	return RequestTimingStats{len(s.queue), s.dropped.Load(), s.failed.Load()}
}
func (s *RequestTimingService) List(ctx context.Context, f RequestTimingFilter) ([]requesttiming.Trace, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	return s.repo.List(ctx, f)
}
func (s *RequestTimingService) Get(ctx context.Context, id string) (*requesttiming.Trace, error) {
	return s.repo.Get(ctx, id)
}
func (s *RequestTimingService) Close() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.queue)
	}
	s.mu.Unlock()
	timer := time.NewTimer(s.drainTimeout)
	defer timer.Stop()
	select {
	case <-s.done:
	case <-timer.C:
		s.stop()
		<-s.done
	}
}
func (s *RequestTimingService) run() {
	defer close(s.done)
	defer s.stop()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-s.workerCtx.Done():
			s.dropped.Add(uint64(len(s.queue)))
			return
		case item, ok := <-s.queue:
			if !ok {
				return
			}
			if s.workerCtx.Err() != nil {
				s.dropped.Add(uint64(len(s.queue) + 1))
				return
			}
			ctx, cancel := context.WithTimeout(s.workerCtx, 2*time.Second)
			var err error
			if item.trace != nil {
				err = s.repo.Save(ctx, *item.trace)
			} else {
				err = s.repo.LinkUsage(ctx, *item.link)
			}
			cancel()
			if err != nil {
				s.failed.Add(1)
			}
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(s.workerCtx, 2*time.Second)
			if err := s.repo.Prune(ctx, time.Now().Add(-7*24*time.Hour)); err != nil {
				s.failed.Add(1)
			}
			cancel()
		}
	}
}
