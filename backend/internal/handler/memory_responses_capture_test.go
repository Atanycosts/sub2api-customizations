//go:build unit

package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// These are unit contract tests of the downstream observer. They exercise
// real Gin writer behavior, but do not establish live model/Runtime delivery.
func TestMemoryCaptureJSONKeepsOnlyVisibleAssistantOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	w := newMemoryResponsesCapture(c.Writer, false, 8192)
	w.Header().Set("Content-Type", "application/json")
	body := `{"id":"resp_visible","status":"completed","output":[` +
		`{"type":"reasoning","summary":[{"type":"summary_text","text":"private reasoning"}]},` +
		`{"type":"message","role":"user","content":[{"type":"output_text","text":"user spoof"}]},` +
		`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"你好🧠"}]}` +
		`]}`
	// UTF-8 is deliberately split within individual code points.
	for _, b := range []byte(body) {
		n, err := w.Write([]byte{b})
		require.NoError(t, err)
		require.Equal(t, 1, n)
	}
	text, complete := w.CompletedText()
	require.True(t, complete)
	require.Equal(t, "你好🧠", text)
	require.Equal(t, body, rec.Body.String(), "observing a response must not rewrite it")
}

func TestMemoryCaptureSSEParsesSplitCRLFAndMultilineData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	w := newMemoryResponsesCapture(c.Writer, true, 8192)
	w.Header().Set("Content-Type", "text/event-stream")
	body := ": heartbeat\r\n\r\n" +
		"event: response.output_text.delta\r\n" +
		"data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"你好🧠\"}\r\n\r\n" +
		"event: response.completed\r\n" +
		"data: {\"type\":\"response.completed\",\r\n" +
		"data: \"response\":{\"id\":\"resp_split\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"你好🧠\"}]}]}}\r\n\r\n" +
		"data: [DONE]\r\n\r\n"
	for i, b := range []byte(body) {
		if i%2 == 0 {
			_, err := w.WriteString(string([]byte{b}))
			require.NoError(t, err)
		} else {
			_, err := w.Write([]byte{b})
			require.NoError(t, err)
		}
	}
	text, complete := w.CompletedText()
	require.True(t, complete)
	require.Equal(t, "你好🧠", text, "terminal output must not be appended to the deltas twice")
	require.Equal(t, body, rec.Body.String())
}

func TestMemoryCaptureSSEUsesAuthoritativeTerminalOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	w := newMemoryResponsesCapture(c.Writer, true, 8192)
	w.Header().Set("Content-Type", "text/event-stream")
	_, err := w.WriteString("data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"hidden\"}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"prefix\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"final answer\"}]}]}}\n\n")
	require.NoError(t, err)
	text, complete := w.CompletedText()
	require.True(t, complete)
	require.Equal(t, "final answer", text)
}

func TestMemoryCaptureRejectsUnconfirmedOrEmptyResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		stream bool
		status int
		body   string
	}{
		{name: "JSON incomplete", body: `{"status":"incomplete","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial"}]}]}`},
		{name: "JSON missing status", body: `{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"unconfirmed"}]}]}`},
		{name: "JSON tool only", body: `{"status":"completed","output":[{"type":"function_call","arguments":"tool data"}]}`},
		{name: "JSON tool with visible text", body: `{"status":"completed","output":[{"type":"function_call","arguments":"tool data"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"tool pending"}]}]}`},
		{name: "JSON malformed", body: `{"status":"completed","output":`},
		{name: "JSON duplicate status is ambiguous", body: `{"status":"completed","status":"incomplete","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial"}]}]}`},
		{name: "JSON duplicate content is ambiguous", body: `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"first"}],"content":[{"type":"output_text","text":"different actual response"}]}]}`},
		{name: "JSON HTTP error", status: 502, body: `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"spoofed success"}]}]}`},
		{name: "SSE incomplete", stream: true, body: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\ndata: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\"}}\n\n"},
		{name: "SSE no terminal", stream: true, body: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\ndata: [DONE]\n\n"},
		{name: "SSE duplicate terminal status is ambiguous", stream: true, body: "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"status\":\"incomplete\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"partial\"}]}]}}\n\n"},
		{name: "SSE contradictory terminal", stream: true, body: "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"incomplete\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"partial\"}]}]}}\n\n"},
		{name: "SSE failed", stream: true, body: "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\"}}\n\n"},
		{name: "SSE completed then canceled", stream: true, body: "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ambiguous final result\"}]}]}}\n\ndata: {\"type\":\"response.canceled\",\"response\":{\"status\":\"canceled\"}}\n\n"},
		{name: "SSE tool only", stream: true, body: "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"function_call\",\"arguments\":\"tool\"}]}}\n\n"},
		{name: "SSE tool delta before text terminal", stream: true, body: "data: {\"type\":\"response.function_call_arguments.delta\",\"delta\":\"tool\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"tool pending\"}]}]}}\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			w := newMemoryResponsesCapture(c.Writer, tc.stream, 8192)
			if tc.stream {
				w.Header().Set("Content-Type", "text/event-stream")
			} else {
				w.Header().Set("Content-Type", "application/json")
			}
			if tc.status != 0 {
				w.WriteHeader(tc.status)
			}
			_, err := w.WriteString(tc.body)
			require.NoError(t, err)
			_, complete := w.CompletedText()
			require.False(t, complete, "unconfirmed or non-text response must not enter normal archival")
			require.Equal(t, tc.body, rec.Body.String())
		})
	}
}

func TestMemoryCapturePreservesWriterStateAndFlush(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	inner := c.Writer
	w := newMemoryResponsesCapture(inner, true, 4096)
	w.Header().Set("X-Capture-Contract", "preserved")
	require.Equal(t, inner.Size(), w.Size())
	require.False(t, w.Written())
	w.WriteHeader(http.StatusAccepted)
	require.Equal(t, http.StatusAccepted, w.Status())
	require.False(t, w.Written(), "WriteHeader alone must preserve Gin's deferred headers")
	w.Flush()
	require.True(t, rec.Flushed)
	require.True(t, w.Written())
	require.Zero(t, w.Size())
	_, err := w.WriteString(": ping\n\n")
	require.NoError(t, err)
	require.Equal(t, inner.Size(), w.Size())
	require.Equal(t, len(": ping\n\n"), w.Size())
	require.Equal(t, "preserved", rec.Header().Get("X-Capture-Contract"))
}

func TestMemoryCaptureOverBudgetKeepsResponseFlowingWithoutArchival(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		w := newMemoryResponsesCapture(c.Writer, stream, 512)
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		body := `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + strings.Repeat("x", 4096) + `"}]}]}`
		if stream {
			body = "data: {\"type\":\"response.completed\",\"response\":" + body + "}\n\n"
		}
		_, err := w.WriteString(body)
		require.NoError(t, err)
		_, complete := w.CompletedText()
		require.False(t, complete)
		require.Equal(t, body, rec.Body.String(), "memory limits must not truncate the client's response")
	}
}

func TestMemoryCaptureHeartbeatVolumeDoesNotExhaustRetainedBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	w := newMemoryResponsesCapture(c.Writer, true, 512)
	for range 1000 {
		_, err := w.WriteString(": still waiting\n\n")
		require.NoError(t, err)
	}
	_, err := w.WriteString("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"finished\"}]}]}}\n\n")
	require.NoError(t, err)
	text, complete := w.CompletedText()
	require.True(t, complete)
	require.Equal(t, "finished", text)
	require.Greater(t, rec.Body.Len(), 512)
}

type memoryCaptureWriteErrorRecorder struct {
	*httptest.ResponseRecorder
	err        error
	shortWrite bool
}

func (w *memoryCaptureWriteErrorRecorder) Write(p []byte) (int, error) {
	n, _ := w.ResponseRecorder.Write(p)
	if w.shortWrite && n > 0 {
		n--
	}
	return n, w.err
}

func TestMemoryCaptureRejectsShortWritesWithoutTransportError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := &memoryCaptureWriteErrorRecorder{ResponseRecorder: httptest.NewRecorder(), shortWrite: true}
	c, _ := gin.CreateTestContext(rec)
	w := newMemoryResponsesCapture(c.Writer, false, 4096)
	_, err := w.WriteString(`{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"visible"}]}]}`)
	require.NoError(t, err)
	_, complete := w.CompletedText()
	require.False(t, complete)
}

func (w *memoryCaptureWriteErrorRecorder) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func TestMemoryCaptureRejectsTransportErrorEvenWhenAllBytesAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stringWrite := range []bool{false, true} {
		errTransport := errors.New("client transport failed")
		rec := &memoryCaptureWriteErrorRecorder{ResponseRecorder: httptest.NewRecorder(), err: errTransport}
		c, _ := gin.CreateTestContext(rec)
		w := newMemoryResponsesCapture(c.Writer, false, 4096)
		w.Header().Set("Content-Type", "application/json")
		body := `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"visible"}]}]}`
		var err error
		if stringWrite {
			_, err = w.WriteString(body)
		} else {
			_, err = w.Write([]byte(body))
		}
		require.ErrorIs(t, err, errTransport)
		_, complete := w.CompletedText()
		require.False(t, complete)
	}
}
