package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"strings"
	"time"
)

func pelicanBody(p PelicanPlanInput) []byte { return pelicanBodyWithPrompt(p, GatewayPelicanPrompt) }
func pelicanBodyWithPrompt(p PelicanPlanInput, prompt string) []byte {
	payload := map[string]any{"model": p.Model, "stream": true, "store": false, "max_output_tokens": p.MaxOutputTokens, "input": prompt}
	if p.Effort != "" {
		payload["reasoning"] = map[string]string{"effort": p.Effort}
	}
	if p.Protocol == "chat" {
		payload = map[string]any{"model": p.Model, "stream": true, "max_completion_tokens": p.MaxOutputTokens, "messages": []map[string]string{{"role": "user", "content": prompt}}, "stream_options": map[string]bool{"include_usage": true}}
		if p.Effort != "" {
			payload["reasoning_effort"] = p.Effort
		}
	}
	b, _ := json.Marshal(payload)
	return b
}

type pelicanWriter struct {
	header    http.Header
	status    int
	body      bytes.Buffer
	truncated bool
	cancel    context.CancelFunc
}

func (w *pelicanWriter) Header() http.Header { return w.header }
func (w *pelicanWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *pelicanWriter) Flush() {}
func (w *pelicanWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	remaining := gatewayExperimentResponseLimit - w.body.Len()
	if len(p) > remaining {
		_, _ = w.body.Write(p[:remaining])
		w.truncated = true
		w.cancel()
		return remaining, io.ErrShortWrite
	}
	return w.body.Write(p)
}
func (s *PelicanService) execute(parent context.Context, run *PelicanRun) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()
	ctx = WithUpstreamHardDeadline(ctx)
	run.State = "failed"
	run.ErrorKind = "unavailable"
	defer func() {
		run.DurationMS = time.Since(started).Milliseconds()
		now := time.Now().UTC()
		run.FinishedAt = &now
		persist, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if err := s.repo.Finish(persist, run); err != nil {
			logger.LegacyPrintf("service.pelican", "result persistence failed run_id=%s plan_id=%d; lease will become unknown, do not replay: %v", run.ID, run.PlanID, err)
		}
	}()
	group, err := s.groups.GetByID(ctx, run.GroupID)
	if err != nil {
		run.Error = err.Error()
		return
	}
	if group.Platform != PlatformOpenAI || group.Status != StatusActive || !group.ModelAllowlist.Allows(run.Model) {
		run.Error = "group or model is no longer available"
		return
	}
	body, _, err := ApplyOpenAIReasoningEffortPolicy(pelicanBodyWithPrompt(run.Frozen, run.Prompt), group.MaxReasoningEffort, group.ReasoningEffortMappings, group.MaxReasoningEffortOverLimit)
	if err != nil {
		run.Error = err.Error()
		return
	}
	ctx = context.WithValue(ctx, ctxkey.Group, group)
	ctx = WithHTTPUpstreamRedirectsDisabled(ctx)
	ctx = WithOpenAIOutputCapRequired(ctx)
	ctx, clock := requesttiming.New(ctx, run.ID, "/internal/pelican/"+run.Protocol)
	ctx, _ = s.gateway.WithOpenAIRequestPricingContext(ctx, &group.ID)
	mapping, _ := s.gateway.ResolveChannelMappingAndRestrict(ctx, &group.ID, run.Model)
	model := run.Model
	if mapping.Mapped {
		model = mapping.MappedModel
		body = s.gateway.ReplaceModelInBody(body, model)
	}
	capability := OpenAIEndpointCapability("")
	if run.Protocol == "chat" {
		capability = OpenAIEndpointCapabilityChatCompletions
	}
	selection, _, err := s.gateway.SelectAccountWithSchedulerForCapability(ctx, &group.ID, "", "pelican:"+run.ID, model, nil, OpenAIUpstreamTransportHTTPSSE, capability, false, false, true, PlatformOpenAI)
	if err != nil {
		run.Error = err.Error()
		return
	}
	if selection == nil || selection.Account == nil {
		run.Error = "scheduler returned no account"
		return
	}
	release := selection.ReleaseFunc
	if !selection.Acquired {
		if selection.WaitPlan == nil {
			run.Error = "account concurrency unavailable"
			return
		}
		slot, e := s.concurrency.AcquireAccountSlot(ctx, selection.Account.ID, selection.WaitPlan.MaxConcurrency)
		if e != nil {
			run.Error = e.Error()
			return
		}
		if !slot.Acquired {
			run.Error = "account concurrency unavailable"
			return
		}
		release = slot.ReleaseFunc
	}
	if release != nil {
		defer release()
	}
	ctx = ContextWithSelectionProfitGate(ctx, selection)
	account, vetoed, reason := s.gateway.ProfitControlVetoLatest(ctx, selection.Account)
	if vetoed {
		run.Error = reason
		return
	}
	if account.Type != AccountTypeAPIKey {
		run.Error = "selected account cannot guarantee the explicit output token cap"
		return
	}
	if s.gateway.settingService != nil {
		if settings, e := s.gateway.settingService.GetOpenAIFastPolicySettings(ctx); e == nil && settings != nil {
			ctx = withOpenAIFastPolicyContext(ctx, settings)
		}
	}
	policyBody, e := s.gateway.applyOpenAIFastPolicyToBody(ctx, account, normalizeOpenAIModelForUpstream(account, account.GetMappedModel(model)), body)
	if e != nil {
		run.Error = e.Error()
		return
	}
	estimated, e := s.billing.CalculateCostWithServiceTier(account.GetMappedModel(model), UsageTokens{InputTokens: len([]byte(run.Prompt))*2 + 1024, OutputTokens: run.MaxOutputTokens}, 1, gjson.GetBytes(policyBody, "service_tier").String())
	if e != nil || estimated.TotalCost <= 0 || estimated.TotalCost > run.ReservedUSD {
		run.Error = "mapped model price unavailable or maximum estimate exceeds reservation"
		return
	}
	_ = s.gateway.BindStickySessionAfterProfitAdmission(ctx, &group.ID, "pelican:"+run.ID, account.ID)
	requesttiming.AdmissionComplete(ctx)
	run.AccountID = account.ID
	run.UpstreamModel = account.GetMappedModel(model)
	writer := &pelicanWriter{header: make(http.Header), cancel: cancel}
	c, _ := gin.CreateTestContext(writer)
	path := "/v1/responses"
	if run.Protocol == "chat" {
		path = "/v1/chat/completions"
	}
	c.Request, _ = http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Accept", "text/event-stream")
	c.Request.Header.Set("Session-Id", "pelican:"+run.ID)
	c.Request.Header.Set("X-Request-ID", run.ID)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	var result *OpenAIForwardResult
	run.State = "unknown"
	run.ErrorKind = "incomplete"
	if run.Protocol == "chat" {
		result, err = s.gateway.ForwardAsChatCompletions(ctx, c, account, body, "pelican:"+run.ID, "")
	} else {
		result, err = s.gateway.Forward(ctx, c, account, body)
	}
	run.HTTPStatus = writer.status
	raw := writer.body.Bytes()
	run.Transport = gatewayExperimentRawTransport(writer.header.Get("Content-Type"), raw)
	var aggregate gatewayExperimentAggregate
	if gjson.ValidBytes(raw) {
		run.Transport = "json"
		run.RawResponse = append(json.RawMessage(nil), raw...)
		aggregate = gatewayExperimentParseJSON(run.Protocol, raw)
	} else {
		run.RawResponse = gatewayExperimentRawBody(run.Transport, raw, err == nil && !writer.truncated, writer.truncated, err != nil)
		aggregate = gatewayExperimentParseSSE(run.Protocol, raw)
	}
	run.Output = aggregate.Output
	run.Terminal = aggregate.Terminal
	run.Usage = aggregate.Usage
	run.State = aggregate.State
	run.Error = aggregate.Error
	if run.State == "completed" && !pelicanVisualOutput(run.Output) {
		run.State = "failed"
		run.Error = "completed response did not contain an HTML or SVG document"
		run.ErrorKind = "invalid_output"
	} else if run.State == "completed" {
		run.ErrorKind = ""
	} else if run.State == "failed" {
		run.ErrorKind = "upstream_error"
	}
	if writer.truncated {
		run.State = "unknown"
		run.ErrorKind = "output_limit"
		run.Error = "output exceeded 2 MiB; no replay"
	} else if err != nil {
		run.Error = err.Error()
		if run.State != "failed" {
			run.State = "unknown"
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			run.ErrorKind = "timeout"
		} else if parent.Err() != nil {
			run.ErrorKind = "interrupted"
		}
	}
	if writer.status >= 400 {
		run.State = "unknown"
		run.ErrorKind = "upstream_error"
		if writer.status < 500 && writer.status != 408 {
			run.State = "failed"
		}
	}
	if result != nil {
		run.UpstreamModel = result.UpstreamModel
		if run.UpstreamModel == "" {
			run.UpstreamModel = account.GetMappedModel(model)
		}
		run.Endpoint = result.UpstreamEndpoint
		if result.ReasoningEffort != nil {
			run.Effort = *result.ReasoningEffort
		}
		if err == nil && run.State == "completed" {
			s.gateway.ReportOpenAIAccountScheduleResult(account, model, true, result.FirstTokenMs)
		}
		tokens := UsageTokens{InputTokens: max(0, result.Usage.InputTokens-result.Usage.CacheReadInputTokens-result.Usage.CacheCreationInputTokens), OutputTokens: result.Usage.OutputTokens, CacheReadTokens: result.Usage.CacheReadInputTokens, CacheCreationTokens: result.Usage.CacheCreationInputTokens}
		if result.Usage.InputTokens+result.Usage.OutputTokens > 0 {
			tier := ""
			if result.ServiceTier != nil {
				tier = *result.ServiceTier
			}
			cost, costErr := s.billing.CalculateCostWithServiceTier(run.UpstreamModel, tokens, 1, tier)
			if costErr == nil {
				run.CostUSD = &cost.TotalCost
			}
		}
	}
	if err != nil {
		if run.Output != "" {
			s.gateway.ObserveOpenAIAccountHealthFailure(ctx, account, err)
		} else {
			var failover *UpstreamFailoverError
			if errors.As(err, &failover) && failover.ShouldReportAccountScheduleFailure() {
				s.gateway.ReportOpenAIAccountScheduleResult(account, model, false, nil, err)
			}
		}
	}
	trace := clock.Finish(writer.status, ctx.Err() != nil)
	run.Timing = &trace
}

func pelicanVisualOutput(output string) bool {
	normalized := strings.ToLower(strings.TrimSpace(output))
	return (strings.Contains(normalized, "<html") && strings.Contains(normalized, "</html>")) || (strings.Contains(normalized, "<svg") && strings.Contains(normalized, "</svg>"))
}
