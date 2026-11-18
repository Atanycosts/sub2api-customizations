package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
)

type blockingTimingRepo struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	traces  []requesttiming.Trace
	links   []RequestTimingLink
}

func (r *blockingTimingRepo) Save(ctx context.Context, trace requesttiming.Trace) error {
	r.once.Do(func() { close(r.entered) })
	select {
	case <-r.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.traces = append(r.traces, trace)
	return nil
}
func (r *blockingTimingRepo) LinkUsage(_ context.Context, l RequestTimingLink) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.links = append(r.links, l)
	return nil
}
func (r *blockingTimingRepo) List(context.Context, RequestTimingFilter) ([]requesttiming.Trace, error) {
	return nil, nil
}
func (r *blockingTimingRepo) Get(context.Context, string) (*requesttiming.Trace, error) {
	return nil, nil
}
func (r *blockingTimingRepo) Prune(context.Context, time.Time) error { return nil }
func TestTimingWriterSaturationNeverWaitsForDatabase(t *testing.T) {
	r := &blockingTimingRepo{entered: make(chan struct{}), release: make(chan struct{})}
	s := newRequestTimingService(r, 1)
	s.Submit(requesttiming.Trace{ID: "first"})
	<-r.entered
	done := make(chan struct{})
	go func() {
		s.Submit(requesttiming.Trace{ID: "queued"})
		s.Submit(requesttiming.Trace{ID: "dropped"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("diagnostic submission blocked")
	}
	if s.Stats().Dropped != 1 {
		t.Fatalf("stats=%#v", s.Stats())
	}
	close(r.release)
	s.Close()
	if len(r.traces) != 2 {
		t.Fatalf("drain saved %d", len(r.traces))
	}
	s.Submit(requesttiming.Trace{})
	if s.Stats().Dropped != 2 {
		t.Fatal("closed writer not counted")
	}
}
func TestTimingUsageBeforeAndAfterTraceAreQueued(t *testing.T) {
	r := &blockingTimingRepo{entered: make(chan struct{}), release: make(chan struct{})}
	close(r.release)
	s := newRequestTimingService(r, 8)
	s.AttachUsage("before", 0, "request-before", 19)
	s.Submit(requesttiming.Trace{ID: "before"})
	s.Submit(requesttiming.Trace{ID: "after"})
	s.AttachUsage("after", 5, "request-after", 19)
	s.Close()
	if len(r.links) != 2 || len(r.traces) != 2 {
		t.Fatalf("links=%d traces=%d", len(r.links), len(r.traces))
	}
}

func TestTimingWriterShutdownHasOneDrainDeadline(t *testing.T) {
	r := &blockingTimingRepo{entered: make(chan struct{}), release: make(chan struct{})}
	s := newRequestTimingService(r, 32)
	s.drainTimeout = 30 * time.Millisecond
	s.Submit(requesttiming.Trace{})
	<-r.entered
	for range 20 {
		s.Submit(requesttiming.Trace{})
	}
	start := time.Now()
	s.Close()
	if time.Since(start) > time.Second {
		t.Fatal("shutdown waited per queued item")
	}
	if s.Stats().Dropped != 20 || s.Stats().WriteFailed != 1 {
		t.Fatalf("shutdown stats: %#v", s.Stats())
	}
}
