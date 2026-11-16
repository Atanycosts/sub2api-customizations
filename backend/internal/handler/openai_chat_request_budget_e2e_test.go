//go:build unit

package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type chatRequestBudgetE2EUpstream struct {
	service.HTTPUpstream
	mu         sync.Mutex
	sawBudget  bool
	sawTiming  bool
	responseID string
}

func (u *chatRequestBudgetE2EUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.respond(req)
}

func (u *chatRequestBudgetE2EUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.respond(req)
}

func (u *chatRequestBudgetE2EUpstream) respond(req *http.Request) (*http.Response, error) {
	guarded, guard := service.GuardOpenAIRequestBudget(req)
	if guard != nil {
		guard.Close()
		_ = guarded.Body.Close()
	}
	u.mu.Lock()
	u.sawBudget = guard != nil
	u.sawTiming = requesttiming.FromContext(req.Context()) != nil
	u.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{u.responseID},
		},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"chatcmpl-budget-e2e",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-4o-mini",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}
		}`)),
	}, nil
}

type chatRequestBudgetUsageRepo struct {
	service.UsageLogRepository
	created     chan *service.UsageLog
	createCalls atomic.Int32
}

func (r *chatRequestBudgetUsageRepo) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	log.ID = 4242
	r.createCalls.Add(1)
	r.created <- log
	return true, nil
}

func TestOpenAIChatCompletionsBudgetTimingTerminalAndUsageE2E(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.OpenAIRequestBudget.UploadTimeoutSeconds = 30
	cfg.Gateway.OpenAIRequestBudget.PreOutputTimeoutSeconds = 30

	account := &service.Account{
		ID:          9917,
		Name:        "chat-budget-e2e",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    1,
		Credentials: map[string]any{
			"api_key":  "sk-chat-budget",
			"base_url": "https://api.example.test",
		},
		Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: false},
	}
	snapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: []*service.Account{account}}, nil, nil, nil, cfg)
	usageRepo := &chatRequestBudgetUsageRepo{created: make(chan *service.UsageLog, 1)}
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	upstream := &chatRequestBudgetE2EUpstream{responseID: "chat-budget-upstream"}
	gateway := service.NewOpenAIGatewayService(
		nil,
		usageRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		snapshot,
		nil,
		service.NewBillingService(cfg, nil),
		nil,
		billingCache,
		upstream,
		&service.DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	concurrencyCache := &concurrencyCacheMock{
		acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) {
			return true, nil
		},
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) {
			return true, nil
		},
	}
	h := NewOpenAIGatewayHandler(
		gateway,
		service.NewConcurrencyService(concurrencyCache),
		billingCache,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil,
		nil,
		nil,
		nil,
		cfg,
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(
		`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}],"stream":false}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")
	traceCtx, collector := requesttiming.New(c.Request.Context(), "chat-budget-inbound", "/v1/chat/completions")
	c.Request = c.Request.WithContext(traceCtx)
	usageLink := make(chan string, 1)
	collector.SetUsageSink(func(traceID string, usageID int64, requestID string, apiKeyID int64) {
		if usageID == 4242 && requestID == "chat-budget-upstream" && apiKeyID == 1817 {
			usageLink <- traceID
		}
	})
	apiKey := &service.APIKey{
		ID:     1817,
		UserID: 1717,
		User:   &service.User{ID: 1717, Status: service.StatusActive, Balance: 100},
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 1})

	h.ChatCompletions(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"id":"chatcmpl-budget-e2e"`)
	upstream.mu.Lock()
	require.True(t, upstream.sawBudget, "the real Chat handler must install the shared request budget")
	require.True(t, upstream.sawTiming, "the transport must retain the handler's request timing collector")
	upstream.mu.Unlock()

	var usage *service.UsageLog
	select {
	case usage = <-usageRepo.created:
	case <-time.After(2 * time.Second):
		t.Fatal("real Chat handler did not submit usage")
	}
	require.Equal(t, int64(1717), usage.UserID)
	require.Equal(t, int64(1817), usage.APIKeyID)
	require.Equal(t, 3, usage.InputTokens)
	require.Equal(t, 2, usage.OutputTokens)
	select {
	case traceID := <-usageLink:
		require.Equal(t, requesttiming.TraceID(traceCtx), traceID)
	case <-time.After(2 * time.Second):
		t.Fatal("usage was not linked after the usage row was created")
	}
	require.Eventually(t, func() bool {
		return usageRepo.createCalls.Load() == 1
	}, 100*time.Millisecond, 10*time.Millisecond)

	trace := collector.Finish(http.StatusOK, false)
	require.True(t, trace.AdmissionComplete)
	require.Equal(t, "chat-budget-inbound", trace.RequestID)
}
