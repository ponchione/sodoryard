package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ponchione/sodoryard/internal/chain"
)

func renderEventLines(events []chain.Event, limit int) []string {
	if len(events) == 0 {
		return []string{"No events recorded."}
	}
	start := 0
	if limit > 0 && len(events) > limit {
		start = len(events) - limit
	}
	lines := make([]string, 0, len(events)-start)
	for _, event := range events[start:] {
		line := formatEventLine(event)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return []string{"No displayable events."}
	}
	return lines
}

func formatEventLine(event chain.Event) string {
	summary := formatEventSummary(event)
	if summary == "" {
		if event.EventType == chain.EventStepOutput {
			return ""
		}
		summary = strings.TrimSpace(event.EventData)
	}
	if summary == "" {
		return ""
	}
	return fmt.Sprintf("%d  %s  %-18s %s", event.ID, event.CreatedAt.Format("15:04:05"), event.EventType, summary)
}

func formatEventSummary(event chain.Event) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil {
		return ""
	}
	plain := func(key string) string {
		value, ok := payload[key]
		if !ok {
			return ""
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			return ""
		}
		return fmt.Sprintf("%s=%s", key, text)
	}
	quoted := func(key string) string {
		value, ok := payload[key]
		if !ok {
			return ""
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			return ""
		}
		return fmt.Sprintf(`%s=%q`, key, text)
	}
	join := func(parts ...string) string {
		filtered := make([]string, 0, len(parts))
		for _, part := range parts {
			if strings.TrimSpace(part) != "" {
				filtered = append(filtered, part)
			}
		}
		return strings.Join(filtered, " ")
	}

	switch event.EventType {
	case chain.EventChainStarted:
		return join(quoted("task"), plain("orchestrator_pid"), plain("execution_id"), plain("continued_by"), plain("resumed_by"))
	case chain.EventChainResumed:
		return join(plain("resumed_by"), plain("continued_by"), plain("orchestrator_pid"), plain("execution_id"))
	case chain.EventStepStarted:
		return join(plain("role"), quoted("task"), plain("receipt_path"))
	case chain.EventStepOutput:
		line := strings.TrimSpace(fmt.Sprint(payload["line"]))
		if line == "" || line == "<nil>" || shouldSuppressEventOutput(line) {
			return ""
		}
		stream := strings.TrimSpace(fmt.Sprint(payload["stream"]))
		if stream == "" || stream == "<nil>" {
			stream = "stdout"
		}
		return fmt.Sprintf("[%s] %s", stream, line)
	case chain.EventStepCompleted, chain.EventStepFailed:
		return join(plain("role"), plain("verdict"), plain("tokens_used"), plain("duration_secs"), plain("exit_code"), quoted("error"))
	case chain.EventResolverLoop:
		return join(plain("count"), quoted("task_context"))
	case chain.EventReindexStarted, chain.EventReindexCompleted, chain.EventSafetyLimitHit, chain.EventChainPaused, chain.EventChainCancelled, chain.EventChainCompleted:
		return join(plain("status"), quoted("summary"), plain("duration_secs"), plain("limit"), plain("role"), plain("exit_code"), plain("execution_id"), plain("finalized_from"))
	default:
		return ""
	}
}

func shouldSuppressEventOutput(line string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(line))
	for _, noisy := range []string{
		"provider registered",
		"registered provider",
		"provider failed ping() startup validation",
		"brain backend: mcp (in-process)",
		"status: waiting_for_llm",
		"status: executing_tools",
		"status: assembling_context",
	} {
		if strings.Contains(trimmed, noisy) {
			return true
		}
	}
	return false
}
