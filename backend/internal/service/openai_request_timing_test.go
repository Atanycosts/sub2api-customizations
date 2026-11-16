package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
)

func TestTimingSSEClassifiesOnlyCompleteContent(t *testing.T) {
	cases := []struct {
		name, payload  string
		semantic, text bool
	}{
		{"role", `{"choices":[{"delta":{"role":"assistant"}}]}`, false, false},
		{"usage", `{"choices":[],"usage":{"completion_tokens":2}}`, false, false},
		{"created", `{"type":"response.created","response":{"id":"r"}}`, false, false},
		{"heartbeat", `{"type":"ping"}`, false, false},
		{"chat text", `{"choices":[{"delta":{"content":"hi"}}]}`, true, true},
		{"reasoning", `{"choices":[{"delta":{"reasoning_content":"plan"}}]}`, true, false},
		{"tool", `{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"{}"}}]}}]}`, true, false},
		{"responses tool", `{"type":"response.function_call_arguments.delta","delta":"{}"}`, true, false},
		{"responses text", `{"type":"response.output_text.delta","delta":"hi"}`, true, true},
		{"terminal", `{"type":"response.completed","response":{"output":[]}}`, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			progress := 0
			ctx, c := requesttiming.New(context.Background(), "r", "/v1/responses")
			ctx = requesttiming.WithProgress(ctx, func() { progress++ })
			o := timingSSEObserver{ctx: ctx}
			o.Line("data: " + tc.payload)
			// No event is observed until its separator, even if the JSON line is complete.
			if progress != 0 {
				t.Fatal("incomplete SSE event progressed")
			}
			o.Line("")
			o.Flushed()
			trace := c.Finish(200, false)
			if (trace.FirstSemanticMS != nil) != tc.semantic || (trace.FirstTextMS != nil) != tc.text || (trace.FirstContentFlushMS != nil) != tc.semantic {
				t.Fatalf("classification: %#v", trace)
			}
			if (progress > 0) != tc.semantic {
				t.Fatalf("progress=%d", progress)
			}
		})
	}
}
func TestTimingEventNameAndMultilineData(t *testing.T) {
	ctx, c := requesttiming.New(context.Background(), "r", "/v1/responses")
	o := timingSSEObserver{ctx: ctx}
	for _, line := range strings.Split("event: response.output_text.delta\ndata: {\ndata: \"delta\":\"hello\"}\n", "\n") {
		o.Line(line)
	}
	o.Flushed()
	trace := c.Finish(200, false)
	if trace.FirstTextMS == nil || trace.FirstContentFlushMS == nil {
		t.Fatal("multiline SSE missed")
	}
}
