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
