package agent

import (
	stdctx "context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/provider"
)

func TestBuildPersistedToolResultMessageIncludesStructuredReferenceAndPreview(t *testing.T) {
	ref := "/tmp/persisted/search_text-tc-1.txt"
	content := strings.Repeat("SEARCH-RESULT-LINE\n", 20)

	got := buildPersistedToolResultMessage(ref, "tc-1", "search_text", content, 220)

	for _, want := range []string{
		"[persisted_tool_result]",
		"path=/tmp/persisted/search_text-tc-1.txt",
		"tool=search_text",
		"tool_use_id=tc-1",
		"preview=",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("message missing %q: %q", want, got)
		}
	}
	if !strings.Contains(got, "SEARCH-RESULT-LINE") {
		t.Fatalf("message missing preview body: %q", got)
	}
}

func TestBuildPersistedToolResultMessageFallsBackToBarePathForTinyBudget(t *testing.T) {
	ref := "/tmp/persisted/search_text-tc-1.txt"

	got := buildPersistedToolResultMessage(ref, "tc-1", "search_text", "preview", len(ref))

	if got != ref {
		t.Fatalf("message = %q, want bare path %q", got, ref)
	}
}

func TestBuildPersistedToolResultMessageUsesTailPreviewForShellOutput(t *testing.T) {
	ref := "/tmp/persisted/shell-tc-1.txt"
	content := strings.Join([]string{
		"go test ./...",
		"running package a",
		"running package b",
		"running package c",
		"FAIL: final assertion exploded",
		"stacktrace line 1",
		"stacktrace line 2",
	}, "\n")

	got := buildPersistedToolResultMessage(ref, "tc-1", "shell", content, 180)

	if !strings.Contains(got, "preview=") {
		t.Fatalf("message missing preview header: %q", got)
	}
	if !strings.Contains(got, "FAIL: final assertion exploded") {
		t.Fatalf("shell preview should preserve tail error context, got: %q", got)
	}
	if strings.Contains(got, "go test ./...") {
		t.Fatalf("shell preview should prefer tail over head for constrained budgets, got: %q", got)
	}
}

func TestApplyAggregateToolResultBudgetReportsPersistenceAndSavings(t *testing.T) {
	fullOutput := strings.Repeat("SEARCH-RESULT-LINE\n", 40)
	store := &toolResultStoreStub{}

	budgeted, report := applyAggregateToolResultBudget(
		stdctx.Background(),
		store,
		[]provider.ToolResult{{ToolUseID: "tc-1", Content: fullOutput, Details: json.RawMessage(`{"version":1,"kind":"search","original_size":760,"normalized_size":760}`)}},
		[]provider.ToolCall{{ID: "tc-1", Name: "search_text"}},
		120,
	)

	if len(budgeted) != 1 {
		t.Fatalf("budgeted result count = %d, want 1", len(budgeted))
	}
	if report.OriginalChars != len(fullOutput) {
		t.Fatalf("report.OriginalChars = %d, want %d", report.OriginalChars, len(fullOutput))
	}
	if report.FinalChars != len(budgeted[0].Content) {
		t.Fatalf("report.FinalChars = %d, want %d", report.FinalChars, len(budgeted[0].Content))
	}
	if report.PersistedResults != 1 {
		t.Fatalf("report.PersistedResults = %d, want 1", report.PersistedResults)
	}
	if report.InlineShrunkResults != 0 {
		t.Fatalf("report.InlineShrunkResults = %d, want 0", report.InlineShrunkResults)
	}
	if report.ReplacedResults != 1 {
		t.Fatalf("report.ReplacedResults = %d, want 1", report.ReplacedResults)
	}
	if report.CharsSaved <= 0 {
		t.Fatalf("report.CharsSaved = %d, want > 0", report.CharsSaved)
	}
	details := decodeAgentToolResultDetails(t, budgeted[0].Details)
	if details["kind"] != "search" {
		t.Fatalf("details kind = %#v, want search", details["kind"])
	}
	if details["truncated"] != true {
		t.Fatalf("details truncated = %#v, want true", details["truncated"])
	}
	if details["persisted_path"] != "/tmp/persisted/search_text-tc-1.txt" {
		t.Fatalf("persisted_path = %#v", details["persisted_path"])
	}
	if got := int(details["returned_size"].(float64)); got != len(budgeted[0].Content) {
		t.Fatalf("returned_size = %d, want %d", got, len(budgeted[0].Content))
	}
}

func TestApplyAggregateToolResultBudgetReportsInlineShrinkWhenPersistenceUnavailable(t *testing.T) {
	fullOutput := strings.Repeat("SEARCH-RESULT-LINE\n", 40)
	store := &toolResultStoreStub{err: stdctx.Canceled}

	budgeted, report := applyAggregateToolResultBudget(
		stdctx.Background(),
		store,
		[]provider.ToolResult{{ToolUseID: "tc-1", Content: fullOutput}},
		[]provider.ToolCall{{ID: "tc-1", Name: "search_text"}},
		120,
	)

	if len(budgeted) != 1 {
		t.Fatalf("budgeted result count = %d, want 1", len(budgeted))
	}
	if report.PersistedResults != 0 {
		t.Fatalf("report.PersistedResults = %d, want 0", report.PersistedResults)
	}
	if report.InlineShrunkResults != 1 {
		t.Fatalf("report.InlineShrunkResults = %d, want 1", report.InlineShrunkResults)
	}
	if report.ReplacedResults != 1 {
		t.Fatalf("report.ReplacedResults = %d, want 1", report.ReplacedResults)
	}
	if report.CharsSaved <= 0 {
		t.Fatalf("report.CharsSaved = %d, want > 0", report.CharsSaved)
	}
}

func TestToolOutputManagerApplyAggregateBudgetReturnsBudgetedResultsAndReport(t *testing.T) {
	fullOutput := strings.Repeat("SEARCH-RESULT-LINE\n", 40)
	store := &toolResultStoreStub{}
	manager := NewToolOutputManager(store)

	managed := manager.ApplyAggregateBudget(
		stdctx.Background(),
		[]provider.ToolResult{{ToolUseID: "tc-1", Content: fullOutput}},
		[]provider.ToolCall{{ID: "tc-1", Name: "search_text"}},
		120,
	)

	if len(managed.Results) != 1 {
		t.Fatalf("managed result count = %d, want 1", len(managed.Results))
	}
	if managed.Report.PersistedResults != 1 {
		t.Fatalf("managed.Report.PersistedResults = %d, want 1", managed.Report.PersistedResults)
	}
	if managed.Report.ReplacedResults != 1 {
		t.Fatalf("managed.Report.ReplacedResults = %d, want 1", managed.Report.ReplacedResults)
	}
	if managed.Report.CharsSaved <= 0 {
		t.Fatalf("managed.Report.CharsSaved = %d, want > 0", managed.Report.CharsSaved)
	}
	if !strings.Contains(managed.Results[0].Content, "[persisted_tool_result]") {
		t.Fatalf("managed result should contain persisted-tool-result marker, got: %q", managed.Results[0].Content)
	}
}

func TestToolOutputManagerApplyAggregateBudgetPrioritizesShellThenOtherPersistableResultsBeforeFileRead(t *testing.T) {
	store := &toolResultStoreStub{}
	manager := NewToolOutputManager(store)

	shellOutput := strings.Join([]string{
		"go test ./...",
		"package a ok",
		"package b ok",
		"FAIL: final assertion exploded",
		"stacktrace line 1",
		"stacktrace line 2",
	}, "\n") + strings.Repeat("\nextra failure context", 6)
	searchOutput := strings.Repeat("SEARCH-RESULT-LINE\n", 24)
	brainOutput := strings.Repeat("BRAIN-RESULT-LINE\n", 8)
	fileReadOutput := strings.Repeat("120|package main\n", 8)

	managed := manager.ApplyAggregateBudget(
		stdctx.Background(),
		[]provider.ToolResult{
			{ToolUseID: "tc-shell", Content: shellOutput},
			{ToolUseID: "tc-search", Content: searchOutput},
			{ToolUseID: "tc-brain", Content: brainOutput},
			{ToolUseID: "tc-file", Content: fileReadOutput},
		},
		[]provider.ToolCall{
			{ID: "tc-shell", Name: "shell"},
			{ID: "tc-search", Name: "search_text"},
			{ID: "tc-brain", Name: "brain_search"},
			{ID: "tc-file", Name: "file_read"},
		},
		580,
	)

	if got, want := strings.Join(store.callOrder, ","), "tc-shell,tc-search"; got != want {
		t.Fatalf("persist call order = %q, want %q", got, want)
	}
	if managed.Report.PersistedResults != 2 {
		t.Fatalf("managed.Report.PersistedResults = %d, want 2", managed.Report.PersistedResults)
	}
	if managed.Report.InlineShrunkResults != 0 {
		t.Fatalf("managed.Report.InlineShrunkResults = %d, want 0", managed.Report.InlineShrunkResults)
	}
	if managed.Report.ReplacedResults != 2 {
		t.Fatalf("managed.Report.ReplacedResults = %d, want 2", managed.Report.ReplacedResults)
	}
	if !strings.Contains(managed.Results[0].Content, "[persisted_tool_result]") {
		t.Fatalf("shell result should be persisted, got: %q", managed.Results[0].Content)
	}
	if !strings.Contains(managed.Results[0].Content, "FAIL: final assertion exploded") {
		t.Fatalf("shell persisted preview should keep tail failure context, got: %q", managed.Results[0].Content)
	}
	if !strings.Contains(managed.Results[1].Content, "[persisted_tool_result]") {
		t.Fatalf("search result should be persisted, got: %q", managed.Results[1].Content)
	}
	if strings.Contains(managed.Results[2].Content, "[persisted_tool_result]") {
		t.Fatalf("brain result should remain inline once budget is satisfied, got: %q", managed.Results[2].Content)
	}
	if strings.Contains(managed.Results[3].Content, "[persisted_tool_result]") {
		t.Fatalf("file_read should not be persisted ahead of other tool classes, got: %q", managed.Results[3].Content)
	}
}

func decodeAgentToolResultDetails(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var details map[string]any
	if err := json.Unmarshal(raw, &details); err != nil {
		t.Fatalf("unmarshal details: %v", err)
	}
	return details
}
