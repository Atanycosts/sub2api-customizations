//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type pelicanAuditRepository struct {
	PelicanRepository
	finished *PelicanRun
}

func (r *pelicanAuditRepository) Finish(_ context.Context, run *PelicanRun) error {
	r.finished = run
	return nil
}

// Use an actual HTTP server after the real scheduler and Forward functions;
// this adapter changes only the destination transport, never account selection.
type pelicanAuditNetwork struct {
	HTTPUpstream
	client *http.Client
}

func (n pelicanAuditNetwork) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	return requesttiming.InstrumentClient(n.client, accountID).Do(req)
}

func TestPelicanRouteAuditRealGroupNetwork(t *testing.T) {
	for _, protocol := range []string{"responses", "chat"} {
		for _, budgetCase := range []struct {
			rate, reserved float64
			allowed        bool
			priority       bool
		}{
			{rate: 1, reserved: 1, allowed: true},
			{rate: 0.1, reserved: 0.0005, allowed: false},
			{rate: 2, reserved: 0.003, allowed: true},
			{rate: 1, reserved: 0.003, allowed: false, priority: true},
			{rate: 1, reserved: 1, allowed: true, priority: true},
		} {
			for _, busy := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/rate=%g/priority=%t/budget=%g/busy=%t", protocol, budgetCase.rate, budgetCase.priority, budgetCase.reserved, busy), func(t *testing.T) {
					var sends atomic.Int64
					var received []byte
					var receivedPath, receivedAuth string
					server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						sends.Add(1)
						received, _ = io.ReadAll(r.Body)
						receivedPath, receivedAuth = r.URL.Path, r.Header.Get("Authorization")
						w.Header().Set("Content-Type", "text/event-stream")
						if protocol == "chat" {
							event, _ := json.Marshal(map[string]any{"id": "chatcmpl_fixture", "model": "gpt-5.4", "choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": "<html><body>pelican</body></html>"}, "finish_reason": "stop"}}})
							fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":10,\"prompt_tokens_details\":{\"cached_tokens\":10}}}\n\ndata: [DONE]\n\n", event)
							return
						}
						event, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp_fixture", "status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": "<html><body>pelican</body></html>"}}}}, "usage": map[string]any{"input_tokens": 20, "output_tokens": 10, "input_tokens_details": map[string]int{"cached_tokens": 10}}}})
						fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", event)
					}))
					defer server.Close()

					group := &Group{ID: 771, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, MaxReasoningEffort: "low", ForceOpenAIFast: budgetCase.priority}
					account := *newOpenAIRejectedFieldTestAccount()
					account.ID = 772
					account.GroupIDs = []int64{group.ID}
					account.Credentials["base_url"] = server.URL
					account.Credentials["api_key"] = "sk-fixture"
					account.Credentials["model_mapping"] = map[string]any{"gpt-5.4-channel": "gpt-5.4"}
					if protocol == "chat" {
						account.Extra["openai_responses_supported"] = false
					}
					otherGroupAccount := account
					otherGroupAccount.ID = 773
					otherGroupAccount.GroupIDs = []int64{999}
					otherGroupAccount.Priority = -100
					var acquired, released []int64
					concurrency := NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{772: !busy, 773: true}, acquiredIDs: &acquired, releasedIDs: &released})
					cfg := &config.Config{}
					cfg.Default.RateMultiplier = budgetCase.rate
					gateway := &OpenAIGatewayService{
						cfg:                cfg,
						accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{otherGroupAccount, account}}},
						cache:              &schedulerTestGatewayCache{},
						concurrencyService: concurrency,
						httpUpstream:       pelicanAuditNetwork{client: server.Client()},
						channelService:     newTestChannelService(makeStandardRepo(Channel{ID: 774, Status: StatusActive, GroupIDs: []int64{group.ID}, ModelMapping: map[string]map[string]string{PlatformOpenAI: {"gpt-5.4-requested": "gpt-5.4-channel"}}}, map[int64]string{group.ID: PlatformOpenAI})),
					}
					repo := &pelicanAuditRepository{}
					billing := NewBillingService(cfg, &PricingService{pricingData: map[string]*LiteLLMModelPricing{"gpt-5.4": {InputCostPerToken: .000001, OutputCostPerToken: .000002, InputCostPerTokenPriority: .000002, OutputCostPerTokenPriority: .000004, Mode: "chat", LiteLLMProvider: "openai"}}})
					svc := NewPelicanService(repo, guardianAffinityGroupRepo{group: group}, nil, gateway, billing, concurrency)
					plan := PelicanPlanInput{GroupID: group.ID, Model: "gpt-5.4-requested", Effort: "high", Protocol: protocol, MaxOutputTokens: 256, BudgetUSD: budgetCase.reserved}
					run := &PelicanRun{PelicanPublicRun: PelicanPublicRun{ID: "11111111-1111-4111-8111-111111111111", GroupID: group.ID, Model: plan.Model, Effort: plan.Effort, Protocol: protocol, MaxOutputTokens: 256, CreatedAt: time.Now()}, Frozen: plan, Prompt: GatewayPelicanPrompt, ReservedUSD: budgetCase.reserved}
					svc.execute(context.Background(), run)

					require.Same(t, run, repo.finished)
					if busy {
						require.Zero(t, sends.Load(), "a saturated real account slot must prevent forwarding")
						require.Equal(t, "failed", run.State)
						return
					}
					if !budgetCase.allowed {
						require.Zero(t, sends.Load(), "budget must cover the actual model price and effective service tier")
						require.Equal(t, "failed", run.State)
						require.Contains(t, run.Error, "exceeds reservation")
						require.Equal(t, []int64{772}, released)
						return
					}
					require.Equal(t, int64(1), sends.Load(), run.Error)
					require.Equal(t, int64(772), run.AccountID, "scheduler must respect the selected real group")
					require.Equal(t, "Bearer sk-fixture", receivedAuth)
					require.Equal(t, "gpt-5.4", gjson.GetBytes(received, "model").String(), "channel mapping then account mapping must each apply")
					if protocol == "chat" {
						require.Equal(t, "/v1/chat/completions", receivedPath)
						require.Equal(t, "low", gjson.GetBytes(received, "reasoning_effort").String())
						require.Equal(t, int64(256), gjson.GetBytes(received, "max_completion_tokens").Int())
					} else {
						require.Equal(t, "/v1/responses", receivedPath)
						require.Equal(t, "low", gjson.GetBytes(received, "reasoning.effort").String())
						require.Equal(t, int64(256), gjson.GetBytes(received, "max_output_tokens").Int())
					}
					require.Equal(t, "completed", run.State, run.Error)
					require.Contains(t, run.Output, "<html>")
					tier := ""
					if budgetCase.priority {
						tier = "priority"
					}
					require.Equal(t, tier, gjson.GetBytes(received, "service_tier").String())
					expectedCost, costErr := billing.CalculateCostWithServiceTier("gpt-5.4", UsageTokens{InputTokens: 10, CacheReadTokens: 10, OutputTokens: 10}, 1, tier)
					require.NoError(t, costErr)
					require.NotNil(t, run.CostUSD)
					require.InDelta(t, expectedCost.TotalCost, *run.CostUSD, 1e-10, "cached input must not be charged twice in the observation estimate")
					require.Contains(t, acquired, int64(772))
					require.Equal(t, []int64{772}, released)
					require.NotNil(t, run.Timing)
					require.Len(t, run.Timing.Attempts, 1)
					require.Equal(t, int64(772), run.Timing.Attempts[0].AccountID)
				})
			}
		}
	}
}

func TestPelicanRouteAuditHardDeadlineStopsNetworkWithoutReplay(t *testing.T) {
	var sends atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sends.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()
	account := newOpenAIRejectedFieldTestAccount()
	account.Credentials["base_url"] = server.URL
	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":true,"max_output_tokens":128}`)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	ctx = WithUpstreamHardDeadline(ctx)
	c := newOpenAIRejectedFieldTestContext(body)
	c.Request = c.Request.WithContext(ctx)
	svc := newOpenAIRejectedFieldTestService(nil)
	svc.httpUpstream = pelicanAuditNetwork{client: server.Client()}
	started := time.Now()
	_, err := svc.Forward(ctx, c, account, body)
	require.Error(t, err)
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	require.Equal(t, int64(1), sends.Load())
	require.Less(t, time.Since(started), time.Second, "deadline must not start an additional upstream drain window")
}
