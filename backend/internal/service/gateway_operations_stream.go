package service

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

const gatewayExperimentResponseLimit = 2 << 20

type gatewayExperimentAggregate struct {
	Output   string
	Terminal string
	State    string
	Error    string
	Usage    json.RawMessage
}

type gatewayExperimentRawEnvelope struct {
	Transport string `json:"transport"`
	Body      string `json:"body"`
	Complete  bool   `json:"complete"`
	Truncated bool   `json:"truncated,omitempty"`
	ReadError bool   `json:"read_error,omitempty"`
}

func forEachGatewayExperimentSSEFrame(body string, fn func(string, string)) {
	if fn == nil || strings.TrimSpace(body) == "" {
		return
	}
	var parser openAICompatSSEFrameParser
	emit := func(frame openAICompatSSEFrame, ok bool) {
		if !ok {
			return
		}
		data := strings.TrimSpace(frame.Data)
		if data == "" {
			return
		}
		eventType := frame.EventType
		if gjson.Valid(data) {
			eventType = effectiveOpenAISSEEventType([]byte(data), eventType)
		}
		fn(eventType, data)
	}
	for _, line := range strings.Split(body, "\n") {
		emit(parser.AddLine(strings.TrimRight(line, "\r")))
	}
	emit(parser.Finish())
}

func gatewayExperimentParseSSE(protocol string, body []byte) gatewayExperimentAggregate {
	if protocol == "chat" {
		return gatewayExperimentParseChatSSE(string(body))
	}
	return gatewayExperimentParseResponsesSSE(string(body))
}

func gatewayExperimentParseChatSSE(body string) gatewayExperimentAggregate {
	result := gatewayExperimentAggregate{State: "unknown"}
	var output strings.Builder
	sawDone := false

	forEachGatewayExperimentSSEFrame(body, func(eventType, payload string) {
		if payload == "[DONE]" {
			sawDone = true
			return
		}
		if strings.TrimSpace(eventType) == "error" {
			result.State = "failed"
			result.Error = "Chat stream reported an error"
			return
		}
		if !gjson.Valid(payload) {
			if result.Error == "" {
				result.Error = "invalid upstream SSE payload; execution outcome unknown"
			}
			return
		}
		event := gjson.Parse(payload)
		if event.Get("type").String() == "error" || event.Get("error").IsObject() {
			result.State = "failed"
			result.Error = "Chat stream reported an error"
			return
		}
		choices := event.Get("choices")
		if choices.Exists() && choices.IsArray() {
			choices.ForEach(func(_, choice gjson.Result) bool {
				index := choice.Get("index")
				if index.Type != gjson.Number || index.Int() != 0 {
					return true
				}
				if content := choice.Get("delta.content"); content.Type == gjson.String {
					_, _ = output.WriteString(content.String())
				}
				if finishReason := choice.Get("finish_reason"); finishReason.Type == gjson.String && finishReason.String() != "" {
					result.Terminal = finishReason.String()
				}
				return true
			})
		}
		if usage := event.Get("usage"); usage.IsObject() && gjson.Valid(usage.Raw) {
			result.Usage = append(json.RawMessage(nil), usage.Raw...)
		}
	})
	result.Output = output.String()
	if result.Error != "" {
		return result
	}
	switch {
	case result.Terminal == "length":
		result.Error = "Chat terminal finish_reason was length; output truncated and execution outcome unknown"
	case !sawDone && result.Terminal != "":
		result.Error = "Chat stream ended before [DONE]; execution outcome unknown"
	case !sawDone:
		result.Error = "Chat stream ended without finish_reason stop and [DONE]; execution outcome unknown"
	case result.Terminal != "stop":
		if result.Terminal == "" {
			result.Error = "Chat stream ended with [DONE] without finish_reason stop; execution outcome unknown"
		} else {
			result.Error = "Chat terminal finish_reason was " + result.Terminal + "; execution outcome unknown"
		}
	default:
		result.State = "completed"
	}
	return result
}

func gatewayExperimentParseResponsesSSE(body string) gatewayExperimentAggregate {
	completed := gatewayExperimentAggregate{}
	failure := gatewayExperimentAggregate{}
	hasCompleted := false
	hasFailure := false
	invalid := false

	forEachGatewayExperimentSSEFrame(body, func(eventType, payload string) {
		if payload == "[DONE]" {
			return
		}
		if strings.TrimSpace(eventType) == "error" {
			failure = gatewayExperimentAggregate{Terminal: "error", State: "failed", Error: "Responses stream reported an error"}
			hasFailure = true
			return
		}
		if !gjson.Valid(payload) {
			invalid = true
			return
		}
		event := gjson.Parse(payload)
		switch strings.TrimSpace(eventType) {
		case "response.completed":
			response := event.Get("response")
			if !response.Exists() || !response.IsObject() {
				invalid = true
				return
			}
			status := response.Get("status").String()
			terminal := gatewayExperimentResponsesTerminal(response, status)
			switch status {
			case "completed":
				completed = terminal
				hasCompleted = true
			case "failed", "incomplete":
				failure = terminal
				hasFailure = true
			default:
				invalid = true
			}
		case "response.failed":
			response := event.Get("response")
			if response.IsObject() && response.Get("status").String() == "completed" {
				invalid = true
				return
			}
			status := response.Get("status").String()
			if status == "" {
				status = "failed"
			}
			failure = gatewayExperimentResponsesTerminal(response, status)
			failure.State = "failed"
			failure.Error = "Responses terminal status was failed"
			hasFailure = true
		case "response.incomplete":
			response := event.Get("response")
			if response.IsObject() && response.Get("status").String() == "completed" {
				invalid = true
				return
			}
			status := response.Get("status").String()
			if status == "" {
				status = "incomplete"
			}
			failure = gatewayExperimentResponsesTerminal(response, status)
			failure.State = "failed"
			failure.Error = "Responses terminal status was incomplete"
			hasFailure = true
		case "error":
			failure = gatewayExperimentAggregate{Terminal: "error", State: "failed", Error: "Responses stream reported an error"}
			hasFailure = true
		}
	})
	if invalid {
		return gatewayExperimentAggregate{State: "unknown", Error: "invalid or incomplete Responses SSE terminal; execution outcome unknown"}
	}
	if hasCompleted && hasFailure {
		return gatewayExperimentAggregate{
			Output:   completed.Output,
			Terminal: "conflicting",
			State:    "unknown",
			Error:    "Responses stream contained conflicting terminal events; execution outcome unknown",
			Usage:    completed.Usage,
		}
	}
	if hasCompleted {
		completed.State = "completed"
		return completed
	}
	if hasFailure {
		return failure
	}
	return gatewayExperimentAggregate{State: "unknown", Error: "Responses stream ended without completed, failed or incomplete terminal event; execution outcome unknown"}
}

func gatewayExperimentResponsesTerminal(response gjson.Result, fallbackStatus string) gatewayExperimentAggregate {
	result := gatewayExperimentAggregate{State: "unknown"}
	if response.Exists() && response.IsObject() {
		result.Output = gatewayExperimentOutput("responses", []byte(response.Raw))
		result.Terminal = response.Get("status").String()
		if usage := response.Get("usage"); usage.Exists() && gjson.Valid(usage.Raw) {
			result.Usage = append(json.RawMessage(nil), usage.Raw...)
		}
	}
	if result.Terminal == "" {
		result.Terminal = fallbackStatus
	}
	switch result.Terminal {
	case "completed":
		result.State = "completed"
	case "failed":
		result.State = "failed"
		result.Error = "Responses terminal status was failed"
	case "incomplete":
		result.State = "failed"
		result.Error = "Responses terminal status was incomplete"
	default:
		result.Error = "Responses terminal status was " + result.Terminal + "; execution outcome unknown"
	}
	return result
}

func gatewayExperimentParseJSON(protocol string, body []byte) gatewayExperimentAggregate {
	result := gatewayExperimentAggregate{
		Output:   gatewayExperimentOutput(protocol, body),
		Terminal: gjson.GetBytes(body, "status").String(),
		State:    "unknown",
	}
	if protocol == "chat" {
		result.Terminal = gjson.GetBytes(body, "choices.0.finish_reason").String()
		switch result.Terminal {
		case "stop":
			result.State = "completed"
		case "length":
			result.Error = "Chat terminal finish_reason was length; output truncated and execution outcome unknown"
		case "":
			result.Error = "Chat terminal finish_reason was missing; execution outcome unknown"
		default:
			result.Error = "Chat terminal finish_reason was " + result.Terminal + "; execution outcome unknown"
		}
	} else {
		switch result.Terminal {
		case "completed":
			result.State = "completed"
		case "failed":
			result.State = "failed"
			result.Error = "Responses terminal status was failed"
		case "incomplete":
			result.State = "failed"
			result.Error = "Responses terminal status was incomplete"
		case "":
			result.Error = "Responses terminal status was missing; execution outcome unknown"
		default:
			result.Error = "Responses terminal status was " + result.Terminal + "; execution outcome unknown"
		}
	}
	if usage := gjson.GetBytes(body, "usage"); usage.Exists() && gjson.Valid(usage.Raw) {
		result.Usage = append(json.RawMessage(nil), usage.Raw...)
	}
	return result
}

func gatewayExperimentRawTransport(contentType string, body []byte) string {
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		return "sse"
	}
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "data:") || strings.HasPrefix(trimmed, "event:") || strings.HasPrefix(trimmed, ":") {
		return "sse"
	}
	return "json"
}

func gatewayExperimentRawBody(transport string, body []byte, complete, truncated, readError bool) json.RawMessage {
	raw, _ := json.Marshal(gatewayExperimentRawEnvelope{
		Transport: transport,
		Body:      string(body),
		Complete:  complete,
		Truncated: truncated,
		ReadError: readError,
	})
	return raw
}
