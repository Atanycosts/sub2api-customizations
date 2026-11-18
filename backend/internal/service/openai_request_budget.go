package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
)

type openAIRequestBudgetContextKey struct{}

// OpenAIRequestBudgetPhase identifies the phase that exhausted the shared
// request budget.
type OpenAIRequestBudgetPhase string

const (
	OpenAIRequestBudgetUploadTimeout OpenAIRequestBudgetPhase = "upload_timeout"
	OpenAIRequestBudgetPreOutput     OpenAIRequestBudgetPhase = "pre_output_budget"
)

// OpenAIRequestBudgetError reports an unknown upstream execution state. Callers
// must terminate the request instead of replaying it on another account.
type OpenAIRequestBudgetError struct {
	Phase OpenAIRequestBudgetPhase
}

func (e *OpenAIRequestBudgetError) Error() string {
	if e == nil {
		return "OpenAI request budget exceeded"
	}
	switch e.Phase {
	case OpenAIRequestBudgetUploadTimeout:
		return "OpenAI upstream upload timeout; request execution state is unknown"
	case OpenAIRequestBudgetPreOutput:
		return "OpenAI pre-output budget exceeded; request execution state is unknown"
	default:
		return "OpenAI request budget exceeded; request execution state is unknown"
	}
}

func (e *OpenAIRequestBudgetError) Unwrap() error {
	return context.DeadlineExceeded
}

func (e *OpenAIRequestBudgetError) ClientMessage() string {
	if e != nil && e.Phase == OpenAIRequestBudgetUploadTimeout {
		return "Upstream request upload exceeded its deadline. Execution state is unknown; the request was not retried automatically."
	}
	return "Upstream did not produce semantic output before its deadline. Execution state is unknown; the request was not retried automatically."
}

// OpenAIRequestBudget is shared by every attempt in one inbound request. Its
// clocks start lazily at the first real upstream send, so admission and local
// queueing are not counted as upstream upload or pre-output time.
type OpenAIRequestBudget struct {
	uploadTimeout    time.Duration
	preOutputTimeout time.Duration

	mu                sync.Mutex
	started           time.Time
	uploadDeadline    time.Time
	preOutputDeadline time.Time

	semanticOnce sync.Once
	semantic     chan struct{}
}

func NewOpenAIRequestBudget(uploadTimeout, preOutputTimeout time.Duration) *OpenAIRequestBudget {
	return &OpenAIRequestBudget{
		uploadTimeout:    uploadTimeout,
		preOutputTimeout: preOutputTimeout,
		semantic:         make(chan struct{}),
	}
}

// InstallOpenAIRequestBudget associates the request-wide budget and connects
// the timing package's semantic-content hook. That hook is called only after a
// complete content event; metadata, role, usage, terminal-only events, and
// heartbeat bytes do not satisfy the pre-output budget.
func InstallOpenAIRequestBudget(ctx context.Context, budget *OpenAIRequestBudget) context.Context {
	if budget == nil || !budget.enabled() {
		return ctx
	}
	ctx = context.WithValue(ctx, openAIRequestBudgetContextKey{}, budget)
	return requesttiming.WithProgress(ctx, budget.markSemantic)
}

func (b *OpenAIRequestBudget) enabled() bool {
	return b != nil && (b.uploadTimeout > 0 || b.preOutputTimeout > 0)
}

func (b *OpenAIRequestBudget) markSemantic() {
	b.semanticOnce.Do(func() { close(b.semantic) })
}

func (b *OpenAIRequestBudget) semanticComplete() bool {
	select {
	case <-b.semantic:
		return true
	default:
		return false
	}
}

func (b *OpenAIRequestBudget) deadlines() (time.Time, time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if b.started.IsZero() {
		b.started = now
	}
	if b.uploadTimeout > 0 && b.uploadDeadline.IsZero() {
		b.uploadDeadline = b.started.Add(b.uploadTimeout)
	}
	if b.preOutputTimeout > 0 && b.preOutputDeadline.IsZero() && !b.semanticComplete() {
		b.preOutputDeadline = b.started.Add(b.preOutputTimeout)
	}
	return b.uploadDeadline, b.preOutputDeadline
}

func openAIRequestBudgetFromContext(ctx context.Context) *OpenAIRequestBudget {
	if ctx == nil {
		return nil
	}
	budget, _ := ctx.Value(openAIRequestBudgetContextKey{}).(*OpenAIRequestBudget)
	return budget
}

// OpenAIRequestBudgetGuard watches one physical upstream attempt. The same
// request budget supplies the absolute upload and pre-output deadlines to every
// guard, while each guard stops its upload timer as soon as that attempt has
// been written.
type OpenAIRequestBudgetGuard struct {
	ctx    context.Context
	cancel context.CancelCauseFunc

	uploaded   chan struct{}
	stop       chan struct{}
	done       chan struct{}
	uploadOnce sync.Once
	stopOnce   sync.Once
}

// GuardOpenAIRequestBudget installs the shared budget at the actual egress
// boundary. When both limits are disabled it returns the original request and a
// nil guard.
func GuardOpenAIRequestBudget(req *http.Request) (*http.Request, *OpenAIRequestBudgetGuard) {
	if req == nil {
		return req, nil
	}
	budget := openAIRequestBudgetFromContext(req.Context())
	if !budget.enabled() {
		return req, nil
	}

	uploadDeadline, preOutputDeadline := budget.deadlines()
	ctx, cancel := context.WithCancelCause(req.Context())
	guard := &OpenAIRequestBudgetGuard{
		ctx:      ctx,
		cancel:   cancel,
		uploaded: make(chan struct{}),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	trace := &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				guard.uploadComplete()
			}
		},
	}
	guarded := req.Clone(httptrace.WithClientTrace(ctx, trace))

	now := time.Now()
	switch {
	case !uploadDeadline.IsZero() && !now.Before(uploadDeadline):
		guard.cancel(&OpenAIRequestBudgetError{Phase: OpenAIRequestBudgetUploadTimeout})
	case !preOutputDeadline.IsZero() && !now.Before(preOutputDeadline):
		guard.cancel(&OpenAIRequestBudgetError{Phase: OpenAIRequestBudgetPreOutput})
	}
	go guard.watch(uploadDeadline, preOutputDeadline, budget.semantic)
	return guarded, guard
}

func (g *OpenAIRequestBudgetGuard) uploadComplete() {
	if g == nil {
		return
	}
	g.uploadOnce.Do(func() { close(g.uploaded) })
}

func (g *OpenAIRequestBudgetGuard) watch(uploadDeadline, preOutputDeadline time.Time, semantic <-chan struct{}) {
	defer close(g.done)

	newTimer := func(deadline time.Time) (*time.Timer, <-chan time.Time) {
		if deadline.IsZero() {
			return nil, nil
		}
		timer := time.NewTimer(time.Until(deadline))
		return timer, timer.C
	}
	uploadTimer, uploadC := newTimer(uploadDeadline)
	preOutputTimer, preOutputC := newTimer(preOutputDeadline)
	if uploadTimer != nil {
		defer uploadTimer.Stop()
	}
	if preOutputTimer != nil {
		defer preOutputTimer.Stop()
	}

	uploaded := g.uploaded
	for uploadC != nil || preOutputC != nil {
		select {
		case <-g.stop:
			return
		case <-g.ctx.Done():
			return
		case <-uploaded:
			if uploadTimer != nil {
				uploadTimer.Stop()
			}
			uploadC, uploaded = nil, nil
		case <-semantic:
			if preOutputTimer != nil {
				preOutputTimer.Stop()
			}
			preOutputC, semantic = nil, nil
		case <-uploadC:
			select {
			case <-uploaded:
				uploadC, uploaded = nil, nil
				continue
			default:
			}
			g.cancel(&OpenAIRequestBudgetError{Phase: OpenAIRequestBudgetUploadTimeout})
			return
		case <-preOutputC:
			select {
			case <-semantic:
				preOutputC, semantic = nil, nil
				continue
			default:
			}
			g.cancel(&OpenAIRequestBudgetError{Phase: OpenAIRequestBudgetPreOutput})
			return
		}
	}
}

// Finish converts budget expiry into a typed error and gives successful bodies
// ownership of the guard. Callers must not close a successful guard before the
// returned body has been consumed and closed.
func (g *OpenAIRequestBudgetGuard) Finish(resp *http.Response, err error) (*http.Response, error) {
	if g == nil {
		return resp, err
	}
	if resp != nil {
		g.uploadComplete()
	}
	if budgetErr := g.Err(); budgetErr != nil {
		g.Close()
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, budgetErr
	}
	if err != nil || resp == nil || resp.Body == nil {
		g.Close()
		return resp, err
	}
	resp.Body = &openAIRequestBudgetBody{ReadCloser: resp.Body, guard: g}
	return resp, nil
}

// Err returns the typed budget error, if this attempt exhausted a shared
// deadline. A client or parent cancellation is not represented as a budget
// failure.
func (g *OpenAIRequestBudgetGuard) Err() error {
	if g == nil {
		return nil
	}
	var budgetErr *OpenAIRequestBudgetError
	if errors.As(context.Cause(g.ctx), &budgetErr) {
		return budgetErr
	}
	return nil
}

func (g *OpenAIRequestBudgetGuard) Close() {
	if g == nil {
		return
	}
	g.stopOnce.Do(func() {
		close(g.stop)
		<-g.done
		g.cancel(context.Canceled)
	})
}

type openAIRequestBudgetBody struct {
	io.ReadCloser
	guard *OpenAIRequestBudgetGuard
}

func (b *openAIRequestBudgetBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		if budgetErr := b.guard.Err(); budgetErr != nil {
			return n, budgetErr
		}
	}
	return n, err
}

func (b *openAIRequestBudgetBody) Close() error {
	err := b.ReadCloser.Close()
	b.guard.Close()
	return err
}
