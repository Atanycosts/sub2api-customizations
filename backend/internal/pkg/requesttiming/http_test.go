package requesttiming

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPTraceLoopbackReuseAndMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "private response secret")
	}))
	defer server.Close()
	client := InstrumentClient(server.Client(), 19)
	ctx, c := New(context.Background(), "inbound-id", "/v1/chat/completions")
	for range 2 {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v1/chat/completions?api_key=secret", strings.NewReader("private prompt"))
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	trace := c.Finish(200, false)
	if len(trace.Attempts) != 2 {
		t.Fatalf("attempts: %#v", trace.Attempts)
	}
	for _, a := range trace.Attempts {
		if a.AccountID != 19 || a.EndMS == nil || a.Outcome != "body_complete" {
			t.Fatalf("attempt: %#v", a)
		}
		for _, name := range []string{"get_connection", "got_connection", "upload_complete", "response_headers", "first_response_byte"} {
			if _, ok := a.Events[name]; !ok {
				t.Errorf("missing %s: %#v", name, a)
			}
		}
		if _, ok := a.Events["tls_start"]; ok {
			t.Fatal("HTTP invented TLS time")
		}
	}
	if trace.Attempts[1].Reused == nil || !*trace.Attempts[1].Reused {
		t.Fatal("connection reuse not observed")
	}
	if trace.FirstTextMS != nil || trace.FirstSemanticMS != nil || trace.FirstContentFlushMS != nil {
		t.Fatal("transport bytes became content")
	}
	encoded, _ := json.Marshal(trace)
	for _, secret := range []string{"secret", "private prompt", "private response", "api_key=secret"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("persisted %q", secret)
		}
	}
}
func TestAttemptFailureRemainsPartialAndUsageLinksAfterFinish(t *testing.T) {
	ctx, c := New(context.Background(), "request", "/v1/responses")
	i := c.BeginAttempt(13, "responses")
	c.EndAttempt(i, "transport_error")
	called := false
	c.SetUsageSink(func(traceID string, id int64, request string, key int64) {
		called = traceID != "" && id == 7 && request == "upstream-id" && key == 8
	})
	trace := c.Finish(502, false)
	c.LinkUsage(7, "upstream-id", 8)
	if !called {
		t.Fatal("late usage association lost")
	}
	if trace.Completeness != "partial" {
		t.Fatal("missing clocks became complete")
	}
	Observe(ctx, true, true)
	if trace.FirstTextMS != nil {
		t.Fatal("finished trace mutated")
	}
}
func TestBoundedAttempts(t *testing.T) {
	_, c := New(context.Background(), "r", "/v1/responses")
	for range 40 {
		c.BeginAttempt(1, "responses")
	}
	trace := c.Finish(502, false)
	if len(trace.Attempts) != 32 || trace.Completeness != "partial" {
		t.Fatalf("unbounded trace: %d %s", len(trace.Attempts), trace.Completeness)
	}
}
