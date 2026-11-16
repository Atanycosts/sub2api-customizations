// Package requesttiming records observational request boundaries. Missing clocks
// remain null; these measurements never replace billing or legacy TTFT clocks.
package requesttiming

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type contextKey struct{}
type progressKey struct{}

type Span struct {
	Kind    string  `json:"kind"`
	StartMS float64 `json:"start_ms"`
	EndMS   float64 `json:"end_ms"`
}
type Attempt struct {
	Number    int                `json:"number"`
	AccountID int64              `json:"account_id"`
	Endpoint  string             `json:"endpoint"`
	StartMS   float64            `json:"start_ms"`
	EndMS     *float64           `json:"end_ms"`
	Events    map[string]float64 `json:"events_ms"`
	Reused    *bool              `json:"connection_reused"`
	Status    int                `json:"status"`
	Outcome   string             `json:"outcome"`
}
type Trace struct {
	ID                  string    `json:"trace_id"`
	RequestID           string    `json:"request_id"`
	UsageRequestID      string    `json:"usage_request_id,omitempty"`
	UsageID             *int64    `json:"usage_id"`
	UserID              int64     `json:"user_id"`
	APIKeyID            int64     `json:"api_key_id"`
	StartedAt           time.Time `json:"started_at"`
	DurationMS          *float64  `json:"duration_ms"`
	Path                string    `json:"path"`
	Status              int       `json:"status"`
	Outcome             string    `json:"outcome"`
	Completeness        string    `json:"completeness"`
	FirstSemanticMS     *float64  `json:"first_semantic_ms"`
	FirstTextMS         *float64  `json:"first_text_ms"`
	FirstContentFlushMS *float64  `json:"first_content_flush_ms"`
	AdmissionComplete   bool      `json:"admission_complete"`
	Admission           []Span    `json:"admission"`
	Attempts            []Attempt `json:"attempts"`
}
type Collector struct {
	mu        sync.Mutex
	start     time.Time
	trace     Trace
	truncated bool
	frozen    bool
	usageSink func(string, int64, string, int64)
}

func New(ctx context.Context, requestID, path string) (context.Context, *Collector) {
	now := time.Now()
	c := &Collector{start: now, trace: Trace{ID: uuid.NewString(), RequestID: requestID, Path: path, StartedAt: now, Completeness: "partial", Attempts: []Attempt{}, Admission: []Span{}}}
	return context.WithValue(ctx, contextKey{}, c), c
}
func FromContext(ctx context.Context) *Collector {
	if ctx == nil {
		return nil
	}
	c, _ := ctx.Value(contextKey{}).(*Collector)
	return c
}
func WithProgress(ctx context.Context, fn func()) context.Context {
	return context.WithValue(ctx, progressKey{}, fn)
}

// ContentProgress is emitted only for complete semantic content events, never
// metadata, usage, terminal-only events or keepalives.
func ContentProgress(ctx context.Context) {
	if fn, ok := ctx.Value(progressKey{}).(func()); ok {
		fn()
	}
}
func (c *Collector) offset() float64 { return float64(time.Since(c.start)) / float64(time.Millisecond) }
func (c *Collector) Identity(userID, keyID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.trace.UserID = userID
	c.trace.APIKeyID = keyID
}
func SetUsage(ctx context.Context, usageID int64, requestID string) {
	if c := FromContext(ctx); c != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.frozen {
			if usageID > 0 {
				c.trace.UsageID = &usageID
			}
			c.trace.UsageRequestID = requestID
		}
	}
}

// Admission records only an actual wait span; time before the first attempt is
// deliberately not labelled queue time.
func Admission(ctx context.Context, kind string, start, end time.Time) {
	if c := FromContext(ctx); c != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.frozen {
			return
		}
		if len(c.trace.Admission) >= 32 {
			c.truncated = true
			return
		}
		c.trace.Admission = append(c.trace.Admission, Span{kind, float64(start.Sub(c.start)) / float64(time.Millisecond), float64(end.Sub(c.start)) / float64(time.Millisecond)})
	}
}
func (c *Collector) BeginAttempt(accountID int64, endpoint string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.frozen {
		return -1
	}
	if len(c.trace.Attempts) >= 32 {
		c.truncated = true
		return -1
	}
	c.trace.Outcome = ""
	i := len(c.trace.Attempts)
	c.trace.Attempts = append(c.trace.Attempts, Attempt{Number: i + 1, AccountID: accountID, Endpoint: endpoint, StartMS: c.offset(), Events: map[string]float64{}, Outcome: "pending"})
	return i
}
func (c *Collector) Event(i int, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.frozen || i < 0 || i >= len(c.trace.Attempts) {
		return
	}
	a := &c.trace.Attempts[i]
	if _, ok := a.Events[name]; !ok {
		a.Events[name] = c.offset()
	}
}
func (c *Collector) Connection(i int, reused bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.frozen && i >= 0 && i < len(c.trace.Attempts) {
		c.trace.Attempts[i].Reused = &reused
	}
}
func (c *Collector) Headers(i, status int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.frozen && i >= 0 && i < len(c.trace.Attempts) {
		c.trace.Attempts[i].Status = status
		c.trace.Attempts[i].Events["response_headers"] = c.offset()
	}
}
func (c *Collector) EndAttempt(i int, outcome string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.frozen || i < 0 || i >= len(c.trace.Attempts) {
		return
	}
	a := &c.trace.Attempts[i]
	if a.EndMS == nil {
		v := c.offset()
		a.EndMS = &v
		if a.Outcome == "pending" {
			a.Outcome = outcome
		}
	}
}
func Observe(ctx context.Context, semantic, text bool) {
	if !semantic && !text {
		return
	}
	if c := FromContext(ctx); c != nil {
		c.mu.Lock()
		if !c.frozen {
			v := c.offset()
			if semantic && c.trace.FirstSemanticMS == nil {
				c.trace.FirstSemanticMS = &v
			}
			if text && c.trace.FirstTextMS == nil {
				c.trace.FirstTextMS = &v
			}
			if n := len(c.trace.Attempts); n > 0 {
				a := &c.trace.Attempts[n-1]
				if semantic {
					if _, ok := a.Events["first_semantic"]; !ok {
						a.Events["first_semantic"] = v
					}
				}
				if text {
					if _, ok := a.Events["first_text"]; !ok {
						a.Events["first_text"] = v
					}
				}
			}
		}
		c.mu.Unlock()
	}
	if semantic {
		ContentProgress(ctx)
	}
}
func ContentFlushed(ctx context.Context) {
	if c := FromContext(ctx); c != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.frozen && c.trace.FirstContentFlushMS == nil {
			v := c.offset()
			c.trace.FirstContentFlushMS = &v
			if n := len(c.trace.Attempts); n > 0 {
				c.trace.Attempts[n-1].Events["first_content_flush"] = v
			}
		}
	}
}
func Outcome(ctx context.Context, outcome string) {
	if c := FromContext(ctx); c != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.frozen {
			c.trace.Outcome = outcome
		}
	}
}
func (c *Collector) Finish(status int, canceled bool) Trace {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.frozen = true
	v := c.offset()
	c.trace.DurationMS = &v
	c.trace.Status = status
	if canceled {
		c.trace.Outcome = "client_canceled"
	}
	if c.trace.Outcome == "" {
		switch {
		case canceled:
			c.trace.Outcome = "client_canceled"
		case status >= 400:
			c.trace.Outcome = "http_error"
		default:
			c.trace.Outcome = "success"
		}
	}
	complete := len(c.trace.Attempts) > 0 && !c.truncated
	if c.trace.Outcome == "success" && c.trace.FirstSemanticMS == nil {
		complete = false
	}
	for _, a := range c.trace.Attempts {
		if a.EndMS == nil || a.Events["response_headers"] == 0 || a.Events["got_connection"] == 0 || a.Events["upload_complete"] == 0 {
			complete = false
		}
	}
	if complete {
		c.trace.Completeness = "complete"
	}
	return c.trace
}

// SetUsageSink is installed by the request middleware. It remains usable after
// Finish because usage billing intentionally runs after the HTTP request ends.
func (c *Collector) SetUsageSink(fn func(string, int64, string, int64)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.usageSink = fn
}
func (c *Collector) LinkUsage(usageID int64, requestID string, apiKeyID int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	fn := c.usageSink
	id := c.trace.ID
	c.mu.Unlock()
	if fn != nil {
		fn(id, usageID, requestID, apiKeyID)
	}
}
func TraceID(ctx context.Context) string {
	if c := FromContext(ctx); c != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.trace.ID
	}
	return ""
}

// LastAttemptOutcome adds the stream owner's semantic result to the network body
// result, including clean EOF before a protocol terminal.
func LastAttemptOutcome(ctx context.Context, outcome string) {
	if c := FromContext(ctx); c != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.frozen && len(c.trace.Attempts) > 0 {
			c.trace.Attempts[len(c.trace.Attempts)-1].Outcome = outcome
		}
	}
}

func AdmissionComplete(ctx context.Context) {
	if c := FromContext(ctx); c != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.frozen {
			c.trace.AdmissionComplete = true
		}
	}
}
