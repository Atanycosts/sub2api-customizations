package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
	"github.com/tidwall/gjson"
)

// timingSSEObserver retains at most 64 KiB for classification and never persists
// payloads. Large single-line JSON is classified in place; oversized multiline
// frames remain unclassified rather than retaining unbounded payloads.
type timingSSEObserver struct {
	ctx           context.Context
	event         string
	data          strings.Builder
	oversized     bool
	content       bool
	largeSemantic bool
	largeText     bool
}

func (o *timingSSEObserver) Line(line string) {
	if line == "" {
		if !o.oversized {
			payload := o.data.String()
			event := o.event
			if event == "" {
				event = gjson.Get(payload, "type").String()
			}
			if event == "error" || event == "response.failed" || gjson.Get(payload, "error").Exists() {
				requesttiming.Outcome(o.ctx, "upstream_error")
			} else if event == "response.completed" || event == "response.done" {
				requesttiming.Outcome(o.ctx, "")
			}
			semantic, text := classifyOpenAITimingPayload(payload, event)
			semantic = semantic || o.largeSemantic
			text = text || o.largeText
			requesttiming.Observe(o.ctx, semantic, text)
			o.content = o.content || semantic
		}
		o.data.Reset()
		o.event = ""
		o.oversized = false
		o.largeSemantic = false
		o.largeText = false
		return
	}
	if event, ok := extractOpenAISSEEventLine(line); ok {
		o.event = event
	}
	if data, ok := extractOpenAISSEDataLine(line); ok && !o.oversized {
		if len(data) > 64*1024 && o.data.Len() == 0 {
			o.largeSemantic, o.largeText = classifyOpenAITimingPayload(data, o.event)
			return
		}
		if o.data.Len()+len(data) > 64*1024 {
			o.data.Reset()
			o.oversized = true
			return
		}
		if o.data.Len() > 0 {
			_ = o.data.WriteByte('\n')
		}
		_, _ = o.data.WriteString(data)
	}
}
func (o *timingSSEObserver) Flushed() {
	if o.content {
		requesttiming.ContentFlushed(o.ctx)
		o.content = false
	}
}
func classifyOpenAITimingPayload(payload, event string) (semantic, text bool) {
	p := gjson.Parse(payload)
	if !p.IsObject() {
		return false, false
	}
	if event == "" {
		event = p.Get("type").String()
	}
	switch event {
	case "response.output_text.delta", "response.refusal.delta":
		text = p.Get("delta").String() != ""
		return text, text
	case "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.function_call_arguments.delta":
		return p.Get("delta").String() != "", false
	case "response.output_item.added":
		return p.Get("item.type").String() == "function_call", false
	case "error", "response.failed", "response.completed", "response.done", "response.created", "response.in_progress":
		return false, false
	}
	for _, choice := range p.Get("choices").Array() {
		d := choice.Get("delta")
		if !d.Exists() {
			d = choice.Get("message")
		}
		hasText := d.Get("content").String() != "" || d.Get("refusal").String() != ""
		text = text || hasText
		semantic = semantic || hasText || d.Get("reasoning_content").String() != "" || d.Get("reasoning").String() != ""
		for _, tool := range d.Get("tool_calls").Array() {
			semantic = semantic || tool.Get("function.arguments").String() != "" || tool.Get("function.name").String() != ""
		}
		semantic = semantic || d.Get("function_call.arguments").String() != ""
	}
	for _, item := range p.Get("output").Array() {
		if item.Get("type").String() == "function_call" {
			semantic = true
		}
		for _, content := range item.Get("content").Array() {
			if content.Get("text").String() != "" || content.Get("refusal").String() != "" {
				text = true
				semantic = true
			}
		}
	}
	return semantic, text
}
func finishOpenAITiming(ctx context.Context, err error) {
	if err != nil {
		requesttiming.Outcome(ctx, "upstream_error")
		requesttiming.LastAttemptOutcome(ctx, "upstream_error")
	}
}
