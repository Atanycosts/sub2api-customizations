package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

func validateGatewayExperiment(in *GatewayExperimentInput) error {
	if _, err := uuid.Parse(in.SubmitID); err != nil {
		return errors.New("valid submit_id UUID required")
	}
	if in.Kind != "pelican" && in.Kind != "experiment" {
		return errors.New("kind must be pelican or experiment")
	}
	if in.Protocol != "chat" && in.Protocol != "responses" {
		return errors.New("explicit protocol chat or responses required")
	}
	if in.AccountID <= 0 || strings.TrimSpace(in.Model) == "" || len(in.Model) > 200 {
		return errors.New("account and model required")
	}
	switch in.Effort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return errors.New("unsupported reasoning effort")
	}
	if in.Kind == "pelican" {
		in.Prompt = GatewayPelicanPrompt
		in.PromptVersion = GatewayPelicanVersion
	}
	if len(in.Prompt) == 0 || len(in.Prompt) > 64<<10 || len(in.PromptVersion) == 0 || len(in.PromptVersion) > 100 {
		return errors.New("prompt (1–65536 bytes) and version required")
	}
	if in.MaxOutputTokens < 64 || in.MaxOutputTokens > 16384 {
		return errors.New("max output tokens must be 64–16384")
	}
	if math.IsNaN(in.BudgetUSD) || math.IsInf(in.BudgetUSD, 0) || in.BudgetUSD <= 0 || in.BudgetUSD > 100 {
		return errors.New("per-submit budget must be 0–100 USD")
	}
	return nil
}

// RunExperiment reuses the existing account-test transport once. No scheduler,
// model failover, account mutation, customer usage write or billing claim runs here.
func (s *GatewayOperationsService) RunExperiment(ctx context.Context, userID int64, in GatewayExperimentInput) (*GatewayExperiment, error) {
	if err := validateGatewayExperiment(&in); err != nil {
		return nil, err
	}
	existing, err := s.repo.Experiment(ctx, in.SubmitID, userID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	account, err := s.accounts.GetByID(ctx, in.AccountID)
	if err != nil {
		return nil, err
	}
	if account.Platform != "openai" || account.Type != "apikey" || account.IsCredentialShadow() {
		return nil, errors.New("observation currently requires a direct OpenAI-compatible API-key account")
	}
	model := account.GetMappedModel(in.Model)
	estimate, err := s.billing.GetEstimatedCost(model, len([]byte(in.Prompt))*2+1024, in.MaxOutputTokens)
	if err != nil {
		return nil, fmt.Errorf("price unavailable: %w", err)
	}
	if estimate <= 0 || estimate > in.BudgetUSD {
		return nil, fmt.Errorf("estimated maximum %.6f USD exceeds reservation %.6f USD, or pricing unavailable", estimate, in.BudgetUSD)
	}
	base := account.GetOpenAIBaseURL()
	if base == "" {
		base = "https://api.openai.com"
	}
	base, err = s.tests.validateUpstreamBaseURL(base)
	if err != nil {
		return nil, err
	}
	base = strings.TrimSuffix(strings.TrimRight(base, "/"), "/v1")
	endpoint := "/v1/responses"
	payload := map[string]any{"model": model, "input": in.Prompt, "max_output_tokens": in.MaxOutputTokens, "stream": false, "store": false}
	if in.Effort != "" {
		payload["reasoning"] = map[string]string{"effort": in.Effort}
	}
	if in.Protocol == "chat" {
		endpoint = "/v1/chat/completions"
		payload = map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": in.Prompt}}, "max_completion_tokens": in.MaxOutputTokens, "stream": false}
		if in.Effort != "" {
			payload["reasoning_effort"] = in.Effort
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	run := &GatewayExperiment{ID: in.SubmitID, CreatedAt: time.Now().UTC(), RequestedBy: userID, Frozen: in, UpstreamModel: model, Endpoint: endpoint, State: "running", ReservedUSD: in.BudgetUSD}
	if err = s.repo.CreateExperiment(ctx, run); err != nil {
		if errors.Is(err, ErrGatewayExperimentExists) {
			return s.repo.Experiment(ctx, in.SubmitID, userID)
		}
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		run.State = "failed"
		run.Error = "submission canceled before upstream send"
		now := time.Now().UTC()
		run.FinishedAt = &now
		persistCtx, persistCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer persistCancel()
		if persistErr := s.repo.FinishExperiment(persistCtx, run); persistErr != nil {
			return nil, persistErr
		}
		return run, nil
	}
	// Persist before sending. A process crash leaves an unknown result and retains
	// its reservation. It is never resumed by a worker.
	executionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
	defer cancel()
	executionCtx = WithHTTPUpstreamRedirectsDisabled(executionCtx)
	started := time.Now()
	req, err := http.NewRequestWithContext(executionCtx, http.MethodPost, base+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+account.GetOpenAIProtocolAPIKey())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", run.ID)
	account.ApplyHeaderOverrides(req.Header)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, sendErr := s.tests.doOpenAIAccountTestUpstream(req, proxyURL, account, false)
	run.State = "unknown"
	if sendErr != nil {
		run.Error = "upstream execution outcome unknown; no automatic retry"
	} else {
		run.HTTPStatus = resp.StatusCode
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20+1))
		_ = resp.Body.Close()
		if readErr != nil || len(data) > 2<<20 {
			run.Error = "upstream result incomplete or exceeded 2 MiB; execution outcome unknown"
		} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			run.State = "failed"
			run.Error = fmt.Sprintf("upstream HTTP %d", resp.StatusCode)
		} else if !gjson.ValidBytes(data) {
			run.Error = "invalid upstream JSON; execution outcome unknown"
		} else {
			run.RawResponse = json.RawMessage(data)
			run.Output = gatewayExperimentOutput(in.Protocol, data)
			if in.Protocol == "chat" {
				run.Terminal = gjson.GetBytes(data, "choices.0.finish_reason").String()
			} else {
				run.Terminal = gjson.GetBytes(data, "status").String()
			}
			usage := gjson.GetBytes(data, "usage")
			if usage.Exists() {
				run.Usage = json.RawMessage(usage.Raw)
				if tokens, known := gatewayExperimentTokens(in.Protocol, usage); known {
					cost, costErr := s.billing.CalculateCost(model, tokens, 1)
					if costErr == nil {
						run.CostUSD = &cost.TotalCost
					}
				}
			}
			if in.Protocol == "responses" && gjson.GetBytes(data, "status").String() != "completed" {
				run.Error = "Responses terminal status was not completed"
			} else if in.Protocol == "chat" && gjson.GetBytes(data, "choices.0.finish_reason").String() != "stop" {
				run.Error = "Chat terminal finish_reason was not stop"
			} else {
				run.State = "completed"
			}
		}
	}
	run.DurationMS = time.Since(started).Milliseconds()
	now := time.Now().UTC()
	run.FinishedAt = &now
	persistCtx, persistCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer persistCancel()
	if err = s.repo.FinishExperiment(persistCtx, run); err != nil {
		return nil, fmt.Errorf("execution finished but result persistence failed; do not resubmit: %w", err)
	}
	return run, nil
}
func gatewayExperimentOutput(protocol string, b []byte) string {
	if protocol == "chat" {
		return gjson.GetBytes(b, "choices.0.message.content").String()
	}
	var out strings.Builder
	gjson.GetBytes(b, "output").ForEach(func(_, item gjson.Result) bool {
		item.Get("content").ForEach(func(_, content gjson.Result) bool {
			if content.Get("type").String() == "output_text" {
				_, _ = out.WriteString(content.Get("text").String())
			}
			return true
		})
		return true
	})
	return out.String()
}
func gatewayExperimentTokens(protocol string, u gjson.Result) (UsageTokens, bool) {
	input, output, cached := u.Get("input_tokens"), u.Get("output_tokens"), u.Get("input_tokens_details.cached_tokens")
	if protocol == "chat" {
		input, output, cached = u.Get("prompt_tokens"), u.Get("completion_tokens"), u.Get("prompt_tokens_details.cached_tokens")
	}
	for _, value := range []gjson.Result{input, output} {
		if value.Type != gjson.Number || value.Float() < 0 || value.Float() != float64(value.Int()) {
			return UsageTokens{}, false
		}
	}
	if cached.Exists() && (cached.Type != gjson.Number || cached.Float() < 0 || cached.Float() != float64(cached.Int()) || cached.Int() > input.Int()) {
		return UsageTokens{}, false
	}
	return UsageTokens{InputTokens: int(input.Int() - cached.Int()), OutputTokens: int(output.Int()), CacheReadTokens: int(cached.Int())}, true
}
