package tool

import (
	"strings"
	"testing"
)

func TestTruncateResultBelowLimit(t *testing.T) {
	result := &ToolResult{
		CallID:  "tc-1",
		Content: "short content",
		Success: true,
	}
	original := result.Content
	truncateResult(result, 1000, "file_read")
	if result.Content != original {
		t.Fatalf("content was modified even though below limit: %q", result.Content)
	}
}

func TestTruncateResultAboveLimit(t *testing.T) {
	// Create content that exceeds the limit.
	// maxTokens=10 → maxChars=40
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "abcdefghij" // 10 chars per line
	}
	content := strings.Join(lines, "\n") // ~210 chars total

	result := &ToolResult{
		CallID:  "tc-1",
		Content: content,
		Success: true,
	}
	truncateResult(result, 10, "file_read")

	if len(result.Content) >= len(content) {
		t.Fatalf("content was not truncated: len=%d, original=%d", len(result.Content), len(content))
	}
	if !strings.Contains(result.Content, "[Output truncated") {
		t.Fatalf("truncation notice missing from content: %s", result.Content)
	}
}

func TestTruncateResultToolSpecificNotice(t *testing.T) {
	// Use a small limit so truncation triggers.
	makeContent := func() string {
		lines := make([]string, 20)
		for i := range lines {
			lines[i] = "abcdefghij"
		}
		return strings.Join(lines, "\n")
	}

	tests := []struct {
		tool     string
		contains string
	}{
		{"file_read", "file_read with line_start/line_end"},
		{"search_text", "more specific query"},
		{"search_semantic", "more specific query"},
		{"git_diff", "path filter"},
		{"shell", "piping to head/tail"},
		{"unknown_tool", "Output truncated"},
	}

	for _, tt := range tests {
		result := &ToolResult{CallID: "tc-1", Content: makeContent(), Success: true}
		truncateResult(result, 10, tt.tool)
		if !strings.Contains(result.Content, tt.contains) {
			t.Errorf("tool=%s: expected notice containing %q, got: %s", tt.tool, tt.contains, result.Content)
		}
	}
}

func TestTruncateResultZeroLimitUsesDefault(t *testing.T) {
	// With maxTokens=0, the default (50000) should be used.
	// Content with 100 chars should not be truncated.
	result := &ToolResult{
		CallID:  "tc-1",
		Content: strings.Repeat("a", 100),
		Success: true,
	}
	original := result.Content
	truncateResult(result, 0, "file_read")
	if result.Content != original {
		t.Fatal("content was truncated even though below default limit")
	}
}
