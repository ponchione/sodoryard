package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	contextpkg "github.com/ponchione/sodoryard/internal/context"
)

var (
	_ Event = TokenEvent{}
	_ Event = ThinkingStartEvent{}
	_ Event = ThinkingDeltaEvent{}
	_ Event = ThinkingEndEvent{}
	_ Event = ToolCallStartEvent{}
	_ Event = ToolCallOutputEvent{}
	_ Event = ToolCallEndEvent{}
	_ Event = TurnCompleteEvent{}
	_ Event = TurnCancelledEvent{}
	_ Event = ErrorEvent{}
	_ Event = StatusEvent{}
	_ Event = ContextDebugEvent{}
)

func TestContextDebugEventReusesLayer3ReportAndTimestamp(t *testing.T) {
	now := time.Unix(1700000100, 0).UTC()
	report := &contextpkg.ContextAssemblyReport{TurnNumber: 2}
	event := ContextDebugEvent{Report: report, Time: now}

	if got := event.EventType(); got != "context_debug" {
		t.Fatalf("EventType() = %q, want context_debug", got)
	}
	if got := event.Timestamp(); !got.Equal(now) {
		t.Fatalf("Timestamp() = %v, want %v", got, now)
	}
	if event.Report != report {
		t.Fatal("Report pointer was not preserved")
	}

	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	if !strings.Contains(string(payload), `"turn_number":2`) {
		t.Fatalf("marshaled payload missing Layer 3 report content: %s", payload)
	}
}

func TestEventTypeDefaultsAndOverrides(t *testing.T) {
	now := time.Unix(1700000200, 0).UTC()
	defaultEvent := TokenEvent{Token: "x", Time: now}
	if got := defaultEvent.EventType(); got != "token" {
		t.Fatalf("default EventType() = %q, want token", got)
	}

	overridden := StatusEvent{Type: "custom_status", State: StateIdle, Time: now}
	if got := overridden.EventType(); got != "custom_status" {
		t.Fatalf("overridden EventType() = %q, want custom_status", got)
	}
}
