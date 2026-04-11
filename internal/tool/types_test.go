package tool

import (
	"encoding/json"
	"testing"

	"github.com/ponchione/sodoryard/internal/provider"
)

func TestPurityString(t *testing.T) {
	tests := []struct {
		p    Purity
		want string
	}{
		{Pure, "pure"},
		{Mutating, "mutating"},
		{Purity(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.p.String(); got != tt.want {
			t.Errorf("Purity(%d).String() = %q, want %q", tt.p, got, tt.want)
		}
	}
}

func TestToolCallFromProvider(t *testing.T) {
	pc := provider.ToolCall{
		ID:    "toolu_123",
		Name:  "file_read",
		Input: json.RawMessage(`{"path":"main.go"}`),
	}
	tc := ToolCallFromProvider(pc)

	if tc.ID != "toolu_123" {
		t.Fatalf("ID = %q, want toolu_123", tc.ID)
	}
	if tc.Name != "file_read" {
		t.Fatalf("Name = %q, want file_read", tc.Name)
	}
	if string(tc.Arguments) != `{"path":"main.go"}` {
		t.Fatalf("Arguments = %s, want {\"path\":\"main.go\"}", tc.Arguments)
	}
}

func TestToolResultToProvider(t *testing.T) {
	// Success case.
	tr := ToolResult{
		CallID:     "toolu_123",
		Content:    "file contents",
		Success:    true,
		DurationMs: 42,
	}
	pr := tr.ToProvider()
	if pr.ToolUseID != "toolu_123" {
		t.Fatalf("ToolUseID = %q, want toolu_123", pr.ToolUseID)
	}
	if pr.Content != "file contents" {
		t.Fatalf("Content = %q, want 'file contents'", pr.Content)
	}
	if pr.IsError {
		t.Fatal("IsError = true, want false for successful result")
	}

	// Failure case.
	tr2 := ToolResult{
		CallID:  "toolu_456",
		Content: "file not found",
		Success: false,
		Error:   "not found",
	}
	pr2 := tr2.ToProvider()
	if !pr2.IsError {
		t.Fatal("IsError = false, want true for failed result")
	}
}
