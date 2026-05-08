package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/chain"
)

func renderYardChainEvents(out io.Writer, events []chain.Event, opts chainRenderOptions) int64 {
	var lastID int64
	for _, event := range events {
		_, _ = fmt.Fprint(out, formatChainEvent(event, opts))
		lastID = event.ID
	}
	return lastID
}

func formatChainEvent(event chain.Event, opts chainRenderOptions) string {
	formatted := formatKnownChainEvent(event, opts)
	if formatted == "" {
		if event.EventType == chain.EventStepOutput {
			return ""
		}
		formatted = event.EventData
		if formatted == "" {
			return ""
		}
	}
	return fmt.Sprintf("%d\t%s\t%s\t%s\n", event.ID, event.CreatedAt.Format(time.RFC3339), event.EventType, formatted)
}

func shouldSuppressStepOutput(line string, opts chainRenderOptions) bool {
	if normalizeChainVerbosity(opts.Verbosity) == chainVerbosityDebug {
		return false
	}
	trimmed := strings.TrimSpace(strings.ToLower(line))
	for _, noisy := range []string{
		"provider registered",
		"registered provider",
		"provider failed ping() startup validation",
		"brain backend: mcp (in-process)",
		"brain backend: shunter",
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

func normalizeChainVerbosity(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), chainVerbosityDebug) {
		return chainVerbosityDebug
	}
	return chainVerbosityNormal
}

func formatKnownChainEvent(event chain.Event, opts chainRenderOptions) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil {
		return ""
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
		if line == "" || line == "<nil>" || shouldSuppressStepOutput(line, opts) {
			return ""
		}
		stream := strings.TrimSpace(fmt.Sprint(payload["stream"]))
		if stream == "" || stream == "<nil>" {
			stream = "stdout"
		}
		return fmt.Sprintf("[%s] %s", stream, line)
	case chain.EventStepChangedFiles:
		return join(plain("count"), quoted("paths"), quoted("error"))
	case chain.EventStepGuardrailFacts:
		return join(plain("role"), plain("sequence"), plain("receipt_valid"), plain("receipt_schema_valid"), plain("receipt_sections_valid"), plain("changed_file_count"), plain("changed_file_claim_present"), plain("changed_file_claim_matches_manifest"), quoted("claimed_changed_files"), quoted("changed_file_claim_extra"), quoted("changed_file_manifest_unclaimed"), plain("code_index_dirty"), quoted("code_index_dirty_reason"), plain("brain_index_dirty"), quoted("brain_index_dirty_reason"), plain("source_writer_lock_released"), plain("open_finding_count"), plain("addressed_finding_count"), quoted("receipt_error"), quoted("suspicious_verdict_finding_reason"))
	case chain.EventStepCompleted, chain.EventStepFailed:
		return join(plain("role"), plain("verdict"), plain("tokens_used"), plain("duration_secs"), plain("exit_code"), quoted("error"))
	case chain.EventReceiptValidation:
		return join(plain("role"), plain("receipt_path"), quoted("warning"), quoted("error"))
	case chain.EventReceiptFindings:
		return join(plain("role"), plain("verdict"), plain("finding_count"), plain("open_count"), plain("closed_count"), plain("addressed_count"), quoted("open_finding_ids"), quoted("closed_finding_ids"), quoted("addressed_ids"))
	case chain.EventFindingLifecycleFacts:
		return join(plain("role"), plain("verdict"), quoted("receipt_path"), quoted("facts"))
	case chain.EventSourceWriterBlocked:
		return join(plain("requested_role"), plain("lock_name"), plain("owner_role"), plain("owner_step_id"), plain("owner_chain_id"), quoted("error"))
	case chain.EventSourceWriterLockAcquired, chain.EventSourceWriterLockReleased, chain.EventSourceWriterLockForceReleased, chain.EventSourceWriterLockReleaseFailed, chain.EventSourceWriterLockHeartbeatFailed, chain.EventSourceWriterLockStaleReplaced:
		return join(plain("lock_name"), plain("owner_role"), plain("owner_step_id"), plain("owner_chain_id"), plain("expires_at"), quoted("error"), quoted("reason"))
	case chain.EventResolverLoop:
		return join(plain("count"), quoted("task_context"))
	case chain.EventReindexStarted, chain.EventReindexCompleted, chain.EventSafetyLimitHit, chain.EventChainPaused, chain.EventChainCancelled, chain.EventChainCompleted:
		parts := make([]string, 0, len(payload))
		for _, key := range []string{"status", "summary", "duration_secs", "limit", "role", "exit_code", "execution_id", "finalized_from"} {
			if key == "summary" {
				parts = append(parts, quoted(key))
				continue
			}
			parts = append(parts, plain(key))
		}
		return join(parts...)
	default:
		return ""
	}
}
