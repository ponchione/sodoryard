package agent

import (
	"encoding/json"
	"testing"

	"github.com/ponchione/sodoryard/internal/provider"
)

// --- estimateRequestChars ---

func TestEstimateRequestCharsNilRequest(t *testing.T) {
	if got := estimateRequestChars(nil); got != 0 {
		t.Fatalf("expected 0 for nil request, got %d", got)
	}
}

func TestEstimateRequestCharsEmpty(t *testing.T) {
	req := &provider.Request{}
	if got := estimateRequestChars(req); got != 0 {
		t.Fatalf("expected 0 for empty request, got %d", got)
	}
}

func TestEstimateRequestCharsCountsSystemBlocks(t *testing.T) {
	req := &provider.Request{
		SystemBlocks: []provider.SystemBlock{
			{Text: "Hello world"},       // 11
			{Text: "Another prompt"},     // 14
		},
	}
	got := estimateRequestChars(req)
	if got != 25 {
		t.Fatalf("expected 25, got %d", got)
	}
}

func TestEstimateRequestCharsCountsMessages(t *testing.T) {
	req := &provider.Request{
		Messages: []provider.Message{
			{
				Role:    provider.RoleUser,
				Content: json.RawMessage(`"hello"`), // 7
			},
			{
				Role:      provider.RoleTool,
				Content:   json.RawMessage(`"result"`), // 8
				ToolUseID: "tool-123",                  // 8
				ToolName:  "file_read",                 // 9
			},
		},
	}
	got := estimateRequestChars(req)
	expected := 7 + 8 + 8 + 9
	if got != expected {
		t.Fatalf("expected %d, got %d", expected, got)
	}
}

func TestEstimateRequestCharsCountsTools(t *testing.T) {
	name := "file_read"
	desc := "Read a file from the project"
	schema := json.RawMessage(`{"type":"object"}`)
	req := &provider.Request{
		Tools: []provider.ToolDefinition{
			{Name: name, Description: desc, InputSchema: schema},
		},
	}
	got := estimateRequestChars(req)
	expected := len(name) + len(desc) + len(schema)
	if got != expected {
		t.Fatalf("expected %d, got %d", expected, got)
	}
}

func TestEstimateRequestCharsCountsAll(t *testing.T) {
	req := &provider.Request{
		SystemBlocks: []provider.SystemBlock{
			{Text: "sys"}, // 3
		},
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: json.RawMessage(`"hi"`)}, // 4
		},
		Tools: []provider.ToolDefinition{
			{Name: "t", Description: "d", InputSchema: json.RawMessage(`{}`)}, // 1 + 1 + 2
		},
	}
	got := estimateRequestChars(req)
	expected := 3 + 4 + 1 + 1 + 2
	if got != expected {
		t.Fatalf("expected %d, got %d", expected, got)
	}
}

// --- analyzeToolCalls ---

func TestAnalyzeToolCallsEmpty(t *testing.T) {
	usedSearch, readFiles := analyzeToolCalls(nil)
	if usedSearch {
		t.Fatal("expected usedSearch=false for empty calls")
	}
	if len(readFiles) != 0 {
		t.Fatal("expected no readFiles for empty calls")
	}
}

func TestAnalyzeToolCallsDetectsSearchSemantic(t *testing.T) {
	calls := []completedToolCall{
		{ToolName: "search_semantic", Arguments: json.RawMessage(`{"query":"auth"}`)},
	}
	usedSearch, readFiles := analyzeToolCalls(calls)
	if !usedSearch {
		t.Fatal("expected usedSearch=true for search_semantic")
	}
	if len(readFiles) != 0 {
		t.Fatal("expected no readFiles")
	}
}

func TestAnalyzeToolCallsDetectsSearchText(t *testing.T) {
	calls := []completedToolCall{
		{ToolName: "search_text", Arguments: json.RawMessage(`{"pattern":"TODO"}`)},
	}
	usedSearch, _ := analyzeToolCalls(calls)
	if !usedSearch {
		t.Fatal("expected usedSearch=true for search_text")
	}
}

func TestAnalyzeToolCallsDetectsSearchRegex(t *testing.T) {
	calls := []completedToolCall{
		{ToolName: "search_regex", Arguments: json.RawMessage(`{"pattern":"TODO.*FIX"}`)},
	}
	usedSearch, _ := analyzeToolCalls(calls)
	if !usedSearch {
		t.Fatal("expected usedSearch=true for search_regex")
	}
}

func TestAnalyzeToolCallsDetectsBrainSearch(t *testing.T) {
	calls := []completedToolCall{
		{ToolName: "brain_search", Arguments: json.RawMessage(`{"query":"runtime proof"}`)},
	}
	usedSearch, readFiles := analyzeToolCalls(calls)
	if !usedSearch {
		t.Fatal("expected usedSearch=true for brain_search")
	}
	if len(readFiles) != 0 {
		t.Fatal("expected no readFiles for brain_search")
	}
}

func TestAnalyzeToolCallsExtractsFileReadPaths(t *testing.T) {
	calls := []completedToolCall{
		{ToolName: "file_read", Arguments: json.RawMessage(`{"path":"internal/auth/service.go"}`)},
		{ToolName: "file_read", Arguments: json.RawMessage(`{"path":"internal/auth/handler.go"}`)},
	}
	usedSearch, readFiles := analyzeToolCalls(calls)
	if usedSearch {
		t.Fatal("expected usedSearch=false")
	}
	if len(readFiles) != 2 {
		t.Fatalf("expected 2 readFiles, got %d", len(readFiles))
	}
	if readFiles[0] != "internal/auth/service.go" {
		t.Fatalf("expected first readFile to be internal/auth/service.go, got %s", readFiles[0])
	}
	if readFiles[1] != "internal/auth/handler.go" {
		t.Fatalf("expected second readFile to be internal/auth/handler.go, got %s", readFiles[1])
	}
}

func TestAnalyzeToolCallsMixedTools(t *testing.T) {
	calls := []completedToolCall{
		{ToolName: "file_read", Arguments: json.RawMessage(`{"path":"README.md"}`)},
		{ToolName: "search_semantic", Arguments: json.RawMessage(`{"query":"auth"}`)},
		{ToolName: "brain_read", Arguments: json.RawMessage(`{"path":"notes/runtime.md"}`)},
		{ToolName: "shell", Arguments: json.RawMessage(`{"command":"ls"}`)},
		{ToolName: "file_read", Arguments: json.RawMessage(`{"path":"go.mod"}`)},
	}
	usedSearch, readFiles := analyzeToolCalls(calls)
	if !usedSearch {
		t.Fatal("expected usedSearch=true")
	}
	if len(readFiles) != 3 {
		t.Fatalf("expected 3 readFiles, got %d", len(readFiles))
	}
	if readFiles[1] != "notes/runtime.md" {
		t.Fatalf("expected brain_read path to be captured, got %v", readFiles)
	}
}

func TestAnalyzeToolCallsSkipsFileReadWithoutPath(t *testing.T) {
	calls := []completedToolCall{
		{ToolName: "file_read", Arguments: json.RawMessage(`{"offset":10}`)},
		{ToolName: "file_read", Arguments: json.RawMessage(`{}`)},
		{ToolName: "file_read", Arguments: json.RawMessage(`invalid json`)},
		{ToolName: "file_read", Arguments: nil},
	}
	_, readFiles := analyzeToolCalls(calls)
	if len(readFiles) != 0 {
		t.Fatalf("expected 0 readFiles for file_read without path, got %d", len(readFiles))
	}
}

// --- extractStringArg ---

func TestExtractStringArgValid(t *testing.T) {
	raw := json.RawMessage(`{"path":"internal/auth/service.go","offset":10}`)
	got := extractStringArg(raw, "path")
	if got != "internal/auth/service.go" {
		t.Fatalf("expected 'internal/auth/service.go', got %q", got)
	}
}

func TestExtractStringArgMissingKey(t *testing.T) {
	raw := json.RawMessage(`{"offset":10}`)
	got := extractStringArg(raw, "path")
	if got != "" {
		t.Fatalf("expected empty string for missing key, got %q", got)
	}
}

func TestExtractStringArgNotString(t *testing.T) {
	raw := json.RawMessage(`{"path":42}`)
	got := extractStringArg(raw, "path")
	if got != "" {
		t.Fatalf("expected empty string for non-string value, got %q", got)
	}
}

func TestExtractStringArgInvalidJSON(t *testing.T) {
	raw := json.RawMessage(`not valid json`)
	got := extractStringArg(raw, "path")
	if got != "" {
		t.Fatalf("expected empty string for invalid JSON, got %q", got)
	}
}

func TestExtractStringArgEmpty(t *testing.T) {
	got := extractStringArg(nil, "path")
	if got != "" {
		t.Fatalf("expected empty string for nil input, got %q", got)
	}
}
