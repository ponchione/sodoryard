package approval

import (
	"encoding/json"
	"testing"
)

func TestPayloadFromToolResultDetails(t *testing.T) {
	details := json.RawMessage(`{"version":1,"kind":"approval_required","approval_id":"approval-tc-1","tool_name":"shell","status":"pending","reason":"matched policy","risk_level":"high","chain_id":"chain-1","step_id":"step-1"}`)

	payload, ok := PayloadFromToolResultDetails(details)
	if !ok {
		t.Fatal("PayloadFromToolResultDetails returned ok=false")
	}
	if payload["approval_id"] != "approval-tc-1" || payload["tool_name"] != "shell" || payload["reason"] != "matched policy" {
		t.Fatalf("payload = %+v, want approval fields", payload)
	}
	if _, ok := payload["version"]; ok {
		t.Fatalf("payload = %+v, did not expect details envelope fields", payload)
	}
}

func TestProgressLineRoundTrip(t *testing.T) {
	want := map[string]any{
		"approval_id": "approval-tc-1",
		"tool_name":   "shell",
		"status":      "pending",
	}
	line := EncodeProgressLine(want)
	if line == "" {
		t.Fatal("EncodeProgressLine returned empty line")
	}

	got, ok := PayloadFromProgressLine(line)
	if !ok {
		t.Fatalf("PayloadFromProgressLine(%q) returned ok=false", line)
	}
	if got["approval_id"] != want["approval_id"] || got["tool_name"] != want["tool_name"] || got["status"] != want["status"] {
		t.Fatalf("payload = %+v, want %+v", got, want)
	}
}

func TestDecisionEnvRoundTripFiltersInvalidDecisions(t *testing.T) {
	raw := EncodeDecisionEnv([]Decision{
		{
			ID:        "approval-tc-1",
			ToolName:  "shell",
			ToolInput: json.RawMessage(`{"command":"git push --force"}`),
			Status:    StatusApproved,
			Reason:    "reviewed",
		},
		{ID: "pending", ToolName: "shell", Status: "pending"},
	})
	if raw == "" {
		t.Fatal("EncodeDecisionEnv returned empty value")
	}

	decisions := DecodeDecisionEnv(raw)
	if len(decisions) != 1 {
		t.Fatalf("decisions = %+v, want one valid decision", decisions)
	}
	got := decisions[0]
	if got.ID != "approval-tc-1" || got.ToolName != "shell" || got.Status != StatusApproved || got.Reason != "reviewed" || string(got.ToolInput) != `{"command":"git push --force"}` {
		t.Fatalf("decision = %+v, want approved shell decision with input", got)
	}
}
