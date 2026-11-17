package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type gatewayOpsStreamingTransport struct {
	HTTPUpstream
	calls       int
	request     *http.Request
	requestBody []byte
	status      int
	header      http.Header
	response    string
	body        io.ReadCloser
	err         error
}

func (t *gatewayOpsStreamingTransport) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	t.calls++
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	t.request = req
	t.requestBody = body
	if t.err != nil {
		return nil, t.err
	}
	status := t.status
	if status == 0 {
		status = http.StatusOK
	}
	header := t.header.Clone()
	if header == nil {
		header = http.Header{}
	}
	responseBody := t.body
	if responseBody == nil {
		responseBody = io.NopCloser(strings.NewReader(t.response))
	}
	return &http.Response{StatusCode: status, Body: responseBody, Header: header}, nil
}

type gatewayOpsErrorAfterDataReader struct {
	data []byte
	read bool
}

func (r *gatewayOpsErrorAfterDataReader) Read(p []byte) (int, error) {
	if !r.read {
		r.read = true
		return copy(p, r.data), nil
	}
	return 0, errors.New("private upstream read detail")
}

func (r *gatewayOpsErrorAfterDataReader) Close() error { return nil }

func newGatewayOpsStreamingService(t *testing.T, transport HTTPUpstream) (*GatewayOperationsService, *gatewayOpsRepoStub) {
	t.Helper()
	repo := &gatewayOpsRepoStub{}
	accounts := gatewayOpsAccountStub{account: &Account{
		ID:          7,
		Platform:    "openai",
		Type:        "apikey",
		Credentials: map[string]any{"api_key": "test-only", "base_url": "https://upstream.example"},
	}}
	billing := NewBillingService(&config.Config{}, &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gpt-4o": {
			InputCostPerToken:  0.000001,
			OutputCostPerToken: 0.000002,
			Mode:               "chat",
			LiteLLMProvider:    "openai",
		},
	}})
	service := NewGatewayOperationsService(
		repo,
		gatewayOpsCipher{},
		accounts,
		&AccountTestService{httpUpstream: transport, cfg: &config.Config{}},
		billing,
		nil,
	)
	return service, repo
}

func runGatewayOpsStreamingExperiment(t *testing.T, service *GatewayOperationsService, protocol string) *GatewayExperiment {
	t.Helper()
	run, err := service.RunExperiment(context.Background(), 8, GatewayExperimentInput{
		SubmitID:        uuid.NewString(),
		Kind:            "pelican",
		AccountID:       7,
		Model:           "gpt-4o",
		Protocol:        protocol,
		MaxOutputTokens: 64,
		BudgetUSD:       10,
	})
	require.NoError(t, err)
	return run
}

func TestGatewayOperationsExperimentRequestUsesSSE(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			transport := &gatewayOpsStreamingTransport{err: errors.New("upstream unavailable")}
			service, repo := newGatewayOpsStreamingService(t, transport)
			run := runGatewayOpsStreamingExperiment(t, service, protocol)

			require.Equal(t, "unknown", run.State)
			require.Equal(t, "sse", run.Transport)
			require.Equal(t, "sse", repo.createdTransport)
			require.Equal(t, 1, transport.calls)
			require.Equal(t, "text/event-stream", transport.request.Header.Get("Accept"))
			require.True(t, gjson.ValidBytes(transport.requestBody))
			require.True(t, gjson.GetBytes(transport.requestBody, "stream").Bool())
			if protocol == "chat" {
				require.True(t, gjson.GetBytes(transport.requestBody, "stream_options.include_usage").Bool())
			} else {
				require.False(t, gjson.GetBytes(transport.requestBody, "stream_options").Exists())
			}
		})
	}
}

func TestGatewayOperationsChatSSEAggregatesContentAndUsage(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","metadata":{"text":"not output"},"choices":[{"index":0,"delta":{"role":"assistant","content":"Hel"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","choices":[{"index":1,"delta":{"content":"wrong-index"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"lo"},"finish_reason":"stop"}]}`,
		"",
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2,"prompt_tokens_details":{"cached_tokens":3}}}`,
		"",
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","choices":[],"usage":null}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	result := gatewayExperimentParseSSE("chat", []byte(body))
	require.Equal(t, "completed", result.State)
	require.Equal(t, "Hello", result.Output)
	require.Equal(t, "stop", result.Terminal)
	require.Equal(t, int64(10), gjson.GetBytes(result.Usage, "prompt_tokens").Int())
}

func TestGatewayOperationsChatSSEErrorIsNeverCompleted(t *testing.T) {
	for _, body := range []string{
		strings.Join([]string{
			`data: {"choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":"stop"}]}`,
			"",
			`data: {"error":{"message":"private upstream detail"}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n"),
		strings.Join([]string{
			`data: {"choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":"stop"}]}`,
			"",
			"event: error",
			`data: {"message":"private upstream detail"}`,
			"",
			"data: [DONE]",
			"",
		}, "\n"),
	} {
		result := gatewayExperimentParseSSE("chat", []byte(body))
		require.Equal(t, "failed", result.State)
		require.Equal(t, "Chat stream reported an error", result.Error)
		require.NotContains(t, result.Error, "private upstream detail")
	}
}

func TestGatewayOperationsChatSSERequiresStopAndDone(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		terminal string
	}{
		{
			name: "done without stop",
			body: strings.Join([]string{
				`data: {"choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}`,
				"",
				"data: [DONE]",
				"",
			}, "\n"),
		},
		{
			name: "stop without done",
			body: strings.Join([]string{
				`data: {"choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":"stop"}]}`,
				"",
			}, "\n"),
			terminal: "stop",
		},
		{
			name: "length",
			body: strings.Join([]string{
				`data: {"choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":"length"}]}`,
				"",
				"data: [DONE]",
				"",
			}, "\n"),
			terminal: "length",
		},
		{
			name: "done alone",
			body: "data: [DONE]\n\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := gatewayExperimentParseSSE("chat", []byte(test.body))
			require.Equal(t, "unknown", result.State)
			require.Equal(t, test.terminal, result.Terminal)
			require.NotEmpty(t, result.Error)
		})
	}
}

func TestGatewayOperationsResponsesSSEusesCompletedResponse(t *testing.T) {
	body := strings.Join([]string{
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","delta":"duplicate"}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"final"}]}],"usage":{"input_tokens":10,"output_tokens":2}}}`,
		"",
	}, "\n")

	result := gatewayExperimentParseSSE("responses", []byte(body))
	require.Equal(t, "completed", result.State)
	require.Equal(t, "final", result.Output)
	require.Equal(t, "completed", result.Terminal)
	require.Equal(t, int64(10), gjson.GetBytes(result.Usage, "input_tokens").Int())
}

func TestGatewayOperationsResponsesSSEConflictingTerminalsAreUnknown(t *testing.T) {
	body := strings.Join([]string{
		"event: response.completed",
		`data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"final"}]}],"usage":{"input_tokens":10,"output_tokens":2}}}`,
		"",
		"event: response.failed",
		`data: {"type":"response.failed","response":{"status":"failed","output":[],"usage":{"input_tokens":10,"output_tokens":0}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	result := gatewayExperimentParseSSE("responses", []byte(body))
	require.Equal(t, "unknown", result.State)
	require.Equal(t, "conflicting", result.Terminal)
	require.Contains(t, result.Error, "conflicting terminal events")
}

func TestGatewayOperationsResponsesCompletedRequiresExplicitStatus(t *testing.T) {
	body := "event: response.completed\n" +
		`data: {"type":"response.completed","response":{"output":[],"usage":{"input_tokens":10,"output_tokens":2}}}` + "\n\n"
	result := gatewayExperimentParseSSE("responses", []byte(body))
	require.Equal(t, "unknown", result.State)
	require.NotEqual(t, "completed", result.State)
}

func TestGatewayOperationsResponsesSSEFailureTerminals(t *testing.T) {
	for _, status := range []string{"failed", "incomplete"} {
		t.Run(status, func(t *testing.T) {
			body := "event: response." + status + "\n" +
				`data: {"type":"response.` + status + `","response":{"status":"` + status + `","output":[],"usage":{"input_tokens":4,"output_tokens":0}}}` + "\n\n"
			result := gatewayExperimentParseSSE("responses", []byte(body))
			require.Equal(t, "failed", result.State)
			require.Equal(t, status, result.Terminal)
			require.Contains(t, result.Error, status)
		})
	}
}

func TestGatewayOperationsStreamMalformedOrMissingTerminalIsUnknown(t *testing.T) {
	for _, test := range []struct {
		protocol string
		body     string
	}{
		{"chat", `data: {"metadata":"not a completion"}` + "\n\n"},
		{"responses", `data: {"type":"response.output_text.delta","delta":"partial"}` + "\n\n"},
	} {
		t.Run(test.protocol, func(t *testing.T) {
			result := gatewayExperimentParseSSE(test.protocol, []byte(test.body))
			require.Equal(t, "unknown", result.State)
			require.NotEmpty(t, result.Error)
		})
	}
}

func TestGatewayOperationsSSEResultFreezesRawEnvelopeAndCost(t *testing.T) {
	tests := []struct {
		protocol string
		body     string
		output   string
		terminal string
	}{
		{
			protocol: "chat",
			body: strings.Join([]string{
				`data: {"choices":[{"index":0,"delta":{"content":"OK"},"finish_reason":"stop"}]}`,
				"",
				`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2}}`,
				"",
				"data: [DONE]",
				"",
			}, "\n"),
			output:   "OK",
			terminal: "stop",
		},
		{
			protocol: "responses",
			body: "event: response.completed\n" +
				`data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":10,"output_tokens":2}}}` + "\n\n",
			output:   "OK",
			terminal: "completed",
		},
	}
	for _, test := range tests {
		t.Run(test.protocol, func(t *testing.T) {
			transport := &gatewayOpsStreamingTransport{
				header:   http.Header{"Content-Type": []string{"text/event-stream"}},
				response: test.body,
			}
			service, _ := newGatewayOpsStreamingService(t, transport)
			run := runGatewayOpsStreamingExperiment(t, service, test.protocol)

			require.Equal(t, "completed", run.State)
			require.Equal(t, "sse", run.Transport)
			require.Equal(t, test.output, run.Output)
			require.Equal(t, test.terminal, run.Terminal)
			require.NotNil(t, run.CostUSD)
			var envelope gatewayExperimentRawEnvelope
			require.NoError(t, json.Unmarshal(run.RawResponse, &envelope))
			require.Equal(t, "sse", envelope.Transport)
			require.True(t, envelope.Complete)
			require.False(t, envelope.Truncated)
			require.False(t, envelope.ReadError)
			require.Equal(t, test.body, envelope.Body)
		})
	}
}

func TestGatewayOperationsJSONFallbackIsMarkedJSON(t *testing.T) {
	transport := &gatewayOpsStreamingTransport{
		header:   http.Header{"Content-Type": []string{"application/json"}},
		response: `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`,
	}
	service, _ := newGatewayOpsStreamingService(t, transport)
	run := runGatewayOpsStreamingExperiment(t, service, "chat")

	require.Equal(t, "completed", run.State)
	require.Equal(t, "json", run.Transport)
	require.Equal(t, "OK", run.Output)
	require.NotNil(t, run.CostUSD)
	require.JSONEq(t, transport.response, string(run.RawResponse))
}

func TestGatewayOperationsWrongJSONShapeIsUnknown(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			transport := &gatewayOpsStreamingTransport{
				header:   http.Header{"Content-Type": []string{"application/json"}},
				response: `{"metadata":"not a terminal result"}`,
			}
			service, _ := newGatewayOpsStreamingService(t, transport)
			run := runGatewayOpsStreamingExperiment(t, service, protocol)

			require.Equal(t, "unknown", run.State)
			require.Equal(t, "json", run.Transport)
			require.NotEmpty(t, run.Error)
		})
	}
}

func TestGatewayOperationsIncompleteBodiesKeepSafeEnvelope(t *testing.T) {
	limitBody := strings.Repeat("x", gatewayExperimentResponseLimit+1)
	tests := []struct {
		name      string
		body      io.ReadCloser
		truncated bool
		readError bool
	}{
		{
			name:      "limit",
			body:      io.NopCloser(strings.NewReader(limitBody)),
			truncated: true,
		},
		{
			name:      "read error",
			body:      &gatewayOpsErrorAfterDataReader{data: []byte("data: partial")},
			readError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &gatewayOpsStreamingTransport{
				header: http.Header{"Content-Type": []string{"text/event-stream"}},
				body:   test.body,
			}
			service, _ := newGatewayOpsStreamingService(t, transport)
			run := runGatewayOpsStreamingExperiment(t, service, "chat")

			require.Equal(t, "unknown", run.State)
			require.Equal(t, "sse", run.Transport)
			require.NotContains(t, run.Error, "private upstream read detail")
			var envelope gatewayExperimentRawEnvelope
			require.NoError(t, json.Unmarshal(run.RawResponse, &envelope))
			require.False(t, envelope.Complete)
			require.Equal(t, test.truncated, envelope.Truncated)
			require.Equal(t, test.readError, envelope.ReadError)
			require.NotEmpty(t, envelope.Body)
			if test.truncated {
				require.Len(t, envelope.Body, gatewayExperimentResponseLimit)
			}
		})
	}
}

func TestGatewayOperationsHTTPOutcomeClassification(t *testing.T) {
	tests := []struct {
		status       int
		wantState    string
		wantUnknown  bool
		responseBody string
	}{
		{status: http.StatusBadRequest, wantState: "failed", responseBody: `{"error":"invalid request"}`},
		{status: http.StatusRequestTimeout, wantState: "unknown", wantUnknown: true, responseBody: `{"error":"gateway timeout"}`},
		{status: http.StatusInternalServerError, wantState: "unknown", wantUnknown: true, responseBody: `{"error":"upstream failed"}`},
		{status: 524, wantState: "unknown", wantUnknown: true, responseBody: "<html>gateway timeout</html>"},
	}
	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			transport := &gatewayOpsStreamingTransport{
				status:   test.status,
				header:   http.Header{"Content-Type": []string{"application/json"}},
				response: test.responseBody,
			}
			service, _ := newGatewayOpsStreamingService(t, transport)
			input := GatewayExperimentInput{
				SubmitID:        uuid.NewString(),
				Kind:            "pelican",
				AccountID:       7,
				Model:           "gpt-4o",
				Protocol:        "responses",
				MaxOutputTokens: 64,
				BudgetUSD:       10,
			}
			run, err := service.RunExperiment(context.Background(), 8, input)
			require.NoError(t, err)

			require.Equal(t, test.wantState, run.State)
			require.Equal(t, test.status, run.HTTPStatus)
			require.Equal(t, "sse", run.Transport)
			require.Empty(t, run.RawResponse)
			if test.wantUnknown {
				require.Contains(t, run.Error, "remote execution and billing outcome unknown")
				require.Contains(t, run.Error, "no automatic retry")
			}
			existing, err := service.RunExperiment(context.Background(), 8, input)
			require.NoError(t, err)
			require.Equal(t, run.ID, existing.ID)
			require.Equal(t, 1, transport.calls)
		})
	}
}
