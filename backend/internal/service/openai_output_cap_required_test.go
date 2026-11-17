package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIOutputCapRequiredPreservesRejectedLimitWithoutRetry(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		name := "transformed"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.5","stream":false,"max_output_tokens":128,"input":"hello"}`)
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				newOpenAIRejectedFieldTestResponse(http.StatusBadRequest, `{"error":{"code":"unknown_parameter","message":"Unknown parameter: max_output_tokens","param":"max_output_tokens"}}`),
				newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"output":[],"usage":{"input_tokens":1,"output_tokens":1}}`),
			}}
			account := newOpenAIRejectedFieldTestAccount()
			if passthrough {
				account.Extra["openai_passthrough"] = true
			}
			c := newOpenAIRejectedFieldTestContext(body)
			ctx := WithOpenAIOutputCapRequired(context.Background())
			c.Request = c.Request.WithContext(ctx)
			result, err := newOpenAIRejectedFieldTestService(upstream).Forward(ctx, c, account, body)

			require.Error(t, err)
			require.Nil(t, result)
			require.Len(t, upstream.bodies, 1, "a rejected output cap must not produce an unbounded second submission")
			require.Equal(t, int64(128), gjson.GetBytes(upstream.bodies[0], "max_output_tokens").Int())
		})
	}
}

func TestOpenAIOutputCapRequiredAllowsOtherExplicitCompatibilityRepair(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"max_output_tokens":128,"truncation":"auto","input":"hello"}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusBadRequest, `{"error":{"code":"unknown_parameter","message":"Unknown parameter: truncation","param":"truncation"}}`),
		newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"output":[],"usage":{"input_tokens":1,"output_tokens":1}}`),
	}}
	c := newOpenAIRejectedFieldTestContext(body)
	ctx := WithOpenAIOutputCapRequired(context.Background())
	c.Request = c.Request.WithContext(ctx)
	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(ctx, c, newOpenAIRejectedFieldTestAccount(), body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, int64(128), gjson.GetBytes(upstream.bodies[1], "max_output_tokens").Int())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "truncation").Exists())
}

func TestOpenAIOutputCapRequiredKeepsExplicitCapAcrossChatBridge(t *testing.T) {
	for _, required := range []bool{false, true} {
		name := "ordinary request keeps configured floor"
		if required {
			name = "internal bounded request keeps explicit cap"
		}
		t.Run(name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false,"max_completion_tokens":64}`)
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				newOpenAIRejectedFieldTestResponse(http.StatusOK, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_cap\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\ndata: [DONE]\n\n"),
			}}
			upstream.responses[0].Header.Set("Content-Type", "text/event-stream")
			account := structuredInputRetryTestAccount()
			c := newOpenAIRejectedFieldTestContext(body)
			ctx := context.Background()
			if required {
				ctx = WithOpenAIOutputCapRequired(ctx)
			}
			c.Request = c.Request.WithContext(ctx)
			svc := &OpenAIGatewayService{cfg: structuredInputRetryTestConfig(), httpUpstream: upstream}
			svc.cfg.Gateway.MinOutputTokens = 512
			result, err := svc.ForwardAsChatCompletions(ctx, c, account, body, "", "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, "/v1/responses", upstream.requests[0].URL.Path)
			want := int64(512)
			if required {
				want = 64
			}
			require.Equal(t, want, gjson.GetBytes(upstream.bodies[0], "max_output_tokens").Int())
		})
	}
}
