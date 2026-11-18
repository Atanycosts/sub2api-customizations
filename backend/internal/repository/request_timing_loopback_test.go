package repository

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func TestRequestTimingRealTransportAndStreams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, protocol := range []string{"chat", "responses", "responses passthrough"} {
		for _, truncated := range []bool{false, true} {
			name := protocol
			if truncated {
				name += " truncated"
			}
			t.Run(name, func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					w.Header().Set("Content-Type", "text/event-stream")
					metadata := `data: {"type":"response.created"}`
					content := `data: {"type":"response.output_text.delta","delta":"hello"}`
					terminal := `data: {"type":"response.completed","response":{"id":"r","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`
					if protocol == "chat" {
						metadata = `data: {"choices":[{"delta":{"role":"assistant"}}]}`
						content = `data: {"choices":[{"delta":{"content":"hello"}}]}`
						terminal = `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`
					}
					_, _ = io.WriteString(w, metadata+"\n\n: heartbeat\n\n")
					_ = http.NewResponseController(w).Flush()
					time.Sleep(15 * time.Millisecond)
					_, _ = io.WriteString(w, content+"\n\n")
					_ = http.NewResponseController(w).Flush()
					if !truncated {
						_, _ = io.WriteString(w, terminal+"\n\n")
					}
				}))
				defer srv.Close()
				cfg := &config.Config{}
				cfg.Security.URLAllowlist.AllowPrivateHosts = true
				cfg.Security.URLAllowlist.AllowInsecureHTTP = true
				upstream := NewHTTPUpstream(cfg)
				svc := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
				account := &service.Account{ID: 19, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"base_url": srv.URL, "api_key": "test"}, Extra: map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModePreserveInbound)}}
				if protocol == "responses passthrough" {
					account.Extra["openai_passthrough"] = true
				}
				path := "/v1/responses"
				body := []byte(`{"model":"gpt-5.1","stream":true,"input":"hello"}`)
				if protocol == "chat" {
					path = "/v1/chat/completions"
					body = []byte(`{"model":"gpt-5.1","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
				}
				ctx, collector := requesttiming.New(t.Context(), "inbound", path)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body))).WithContext(ctx)
				var err error
				if protocol == "chat" {
					_, err = svc.ForwardAsChatCompletions(ctx, c, account, body, "", "")
				} else {
					_, err = svc.Forward(ctx, c, account, body)
				}
				if truncated && err == nil {
					t.Fatal("truncated stream appeared successful")
				}
				if !truncated && err != nil {
					t.Fatal(err)
				}
				trace := collector.Finish(rec.Code, false)
				if trace.FirstSemanticMS == nil || trace.FirstTextMS == nil || trace.FirstContentFlushMS == nil {
					t.Fatalf("missing content boundaries: %#v", trace)
				}
				if *trace.FirstTextMS < 10 {
					t.Fatalf("metadata counted as text: %f", *trace.FirstTextMS)
				}
				if len(trace.Attempts) != 1 || trace.Attempts[0].AccountID != 19 {
					t.Fatalf("attempts: %#v", trace.Attempts)
				}
				if truncated && trace.Attempts[0].Outcome != "upstream_error" {
					t.Fatalf("attempt lost semantic error: %#v", trace.Attempts[0])
				}
				if truncated && trace.Outcome != "upstream_error" {
					t.Fatalf("truncation outcome=%s", trace.Outcome)
				}
				if !truncated && trace.Outcome != "success" {
					t.Fatalf("outcome=%s", trace.Outcome)
				}
				if !strings.Contains(rec.Body.String(), "hello") {
					t.Fatal("client content lost")
				}
			})
		}
	}
}

func TestRequestTimingBudgetNeverBecomesFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, headers := range []bool{false, true} {
		t.Run(map[bool]string{false: "waiting headers", true: "heartbeats only"}[headers], func(t *testing.T) {
			requests := 0
			release := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if headers {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, ": heartbeat\n\n")
					_ = http.NewResponseController(w).Flush()
				}
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			defer srv.Close()
			defer close(release)
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.AllowPrivateHosts = true
			cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			upstream := NewHTTPUpstream(cfg)
			svc := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
			account := &service.Account{ID: 19, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"base_url": srv.URL, "api_key": "test"}, Extra: map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModePreserveInbound)}}
			ctx, collector := requesttiming.New(t.Context(), "r", "/v1/chat/completions")
			ctx = service.InstallOpenAIRequestBudget(ctx, service.NewOpenAIRequestBudget(0, 30*time.Millisecond))
			body := []byte(`{"model":"gpt-5.1","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body))).WithContext(ctx)
			_, err := svc.ForwardAsChatCompletions(ctx, c, account, body, "", "")
			var budgetErr *service.OpenAIRequestBudgetError
			if !errors.As(err, &budgetErr) {
				t.Fatalf("budget lost: %T %v", err, err)
			}
			var failover *service.UpstreamFailoverError
			if errors.As(err, &failover) {
				t.Fatal("budget became retry")
			}
			if requests != 1 {
				t.Fatalf("submitted %d times", requests)
			}
			trace := collector.Finish(504, false)
			if trace.FirstSemanticMS != nil || trace.FirstTextMS != nil {
				t.Fatal("heartbeat released content guard")
			}
		})
	}
}
