package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPelicanPublicProjectionWhitelistsSensitiveFields(t *testing.T) {
	run := PelicanRun{PelicanPublicRun: PelicanPublicRun{ID: "public", Output: "<html></html>", Terminal: "https://private.example/sk-secret", ErrorKind: "private-provider-error"}, AccountID: 999, Error: "credential secret", Endpoint: "private-url", ReservedUSD: 12, RawResponse: json.RawMessage(`{"secret":"private"}`), RequestedBy: 4}
	data, err := json.Marshal(pelicanPublic(run))
	require.NoError(t, err)
	for _, secret := range []string{"account_id", "reserved_usd", "raw_response", "requested_by", "private", "credential", "endpoint"} {
		require.NotContains(t, string(data), secret)
	}
	require.Equal(t, "other", gjson.GetBytes(data, "terminal").String())
	require.Equal(t, "upstream_error", gjson.GetBytes(data, "error_kind").String())
}
func TestPelicanBodiesFreezePromptVersionAndOutputCap(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		p := PelicanPlanInput{Model: "alias", Effort: "high", Protocol: protocol, MaxOutputTokens: 4096}
		b := pelicanBody(p)
		require.Equal(t, "alias", gjson.GetBytes(b, "model").String())
		require.True(t, gjson.GetBytes(b, "stream").Bool())
		if protocol == "chat" {
			require.Equal(t, int64(4096), gjson.GetBytes(b, "max_completion_tokens").Int())
			require.Equal(t, GatewayPelicanPrompt, gjson.GetBytes(b, "messages.0.content").String())
			require.Equal(t, "high", gjson.GetBytes(b, "reasoning_effort").String())
		} else {
			require.Equal(t, int64(4096), gjson.GetBytes(b, "max_output_tokens").Int())
			require.Equal(t, GatewayPelicanPrompt, gjson.GetBytes(b, "input").String())
		}
	}
}
func TestPelicanWriterCancelsAtOutputBound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &pelicanWriter{header: make(http.Header), cancel: cancel}
	n, err := w.Write(bytes.Repeat([]byte("x"), gatewayExperimentResponseLimit+1))
	require.ErrorIs(t, err, io.ErrShortWrite)
	require.Equal(t, gatewayExperimentResponseLimit, n)
	require.Len(t, w.body.Bytes(), gatewayExperimentResponseLimit)
	require.True(t, w.truncated)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}
func TestPelicanVisualOutputRequiresDocument(t *testing.T) {
	for _, output := range []string{"OK", "I cannot comply", "<html>partial", "just pelican text"} {
		require.False(t, pelicanVisualOutput(output))
	}
	require.True(t, pelicanVisualOutput("<!DOCTYPE html><HTML><body>pelican</body></HTML>"))
	require.True(t, pelicanVisualOutput("<svg><path /></svg>"))
}
