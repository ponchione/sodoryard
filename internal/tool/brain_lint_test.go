package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

type mockContradictionProvider struct {
	response      *provider.Response
	err           error
	completeCalls int
	lastRequest   *provider.Request
}

func (m *mockContradictionProvider) Complete(ctx context.Context, req *provider.Request) (*provider.Response, error) {
	m.completeCalls++
	m.lastRequest = req
	if m.err != nil {
		return nil, m.err
	}
	if m.response != nil {
		return m.response, nil
	}
	return &provider.Response{Content: []provider.ContentBlock{provider.NewTextBlock(`{"contradictions":[]}`)}}, nil
}

func (m *mockContradictionProvider) Stream(ctx context.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockContradictionProvider) Models(ctx context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "mock-model", Name: "mock-model"}}, nil
}

func (m *mockContradictionProvider) Name() string { return "mock" }

func TestFormatBrainLogEntry(t *testing.T) {
	ts := time.Date(2026, 4, 6, 14, 32, 0, 0, time.UTC)
	got := formatBrainLogEntry(BrainLogEntry{
		Timestamp: ts,
		Operation: "write",
		Target:    "auth-architecture.md",
		Summary:   "Agent created decision record after designing auth middleware.",
		Session:   "abc123",
	})

	want := "## [2026-04-06T14:32:00Z] write | auth-architecture.md\n" +
		"Agent created decision record after designing auth middleware.\n" +
		"Session: abc123\n"

	if got != want {
		t.Fatalf("unexpected log entry:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatBrainLogEntryOmitsEmptySession(t *testing.T) {
	ts := time.Date(2026, 4, 6, 14, 32, 0, 0, time.UTC)
	got := formatBrainLogEntry(BrainLogEntry{
		Timestamp: ts,
		Operation: "lint",
		Target:    "full",
		Summary:   "Found 1 orphan document.",
	})

	if strings.Contains(got, "Session:") {
		t.Fatalf("did not expect session line in %q", got)
	}
}

func TestSessionIDFromContext(t *testing.T) {
	ctx := ContextWithExecutionMeta(context.Background(), ExecutionMeta{
		ConversationID: "conv-123",
		TurnNumber:     1,
		Iteration:      1,
	})
	if got := sessionIDFromContext(ctx); got != "conv-123" {
		t.Fatalf("sessionIDFromContext() = %q, want conv-123", got)
	}
}

func TestAppendBrainLogPreservesExistingEntries(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"_log.md": "## [2026-04-05T12:00:00Z] write | old.md\nOld entry\n",
	})

	err := appendBrainLog(context.Background(), backend, BrainLogEntry{
		Timestamp: time.Date(2026, 4, 6, 14, 32, 0, 0, time.UTC),
		Operation: "write",
		Target:    "new.md",
		Summary:   "Created new note.",
	})
	if err != nil {
		t.Fatalf("appendBrainLog: %v", err)
	}

	got := backend.docs["_log.md"]
	if !strings.Contains(got, "old.md") || !strings.Contains(got, "new.md") {
		t.Fatalf("expected both old and new entries, got:\n%s", got)
	}
}

func TestBrainLintToolDisabled(t *testing.T) {
	tool := NewBrainLint(nil, brainConfig(false))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for disabled brain")
	}
}

func TestBrainLintToolReturnsStructuredSummary(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md": "# A\n[[notes/missing]]",
	})
	tool := NewBrainLint(backend, brainConfig(true))

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"scope":"full","checks":["dead_links"]}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "dead links") {
		t.Fatalf("expected dead link summary, got %q", result.Content)
	}
}

func TestBrainLintRejectsUnknownCheck(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md": "# A",
	})
	tool := NewBrainLint(backend, brainConfig(true))

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"checks":["typo_check"]}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for unknown check")
	}
}

func TestBrainLintSupportsCombinedScope(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/arch.md": `---
tags: [architecture]
updated_at: 2025-12-01T10:00:00Z
---
# Arch
[[shared/reference]]`,
		"notes/debug.md": `---
tags: [debugging]
---
# Debug`,
		"shared/reference.md": `---
updated_at: 2026-04-06T10:00:00Z
---
# Reference`,
		"notes/inbound.md": "# Inbound\n[[notes/arch]]",
	})
	tool := NewBrainLint(backend, brainConfig(true))

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"scope":"notes/+#architecture","checks":["orphans","dead_links","stale_references"]}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	for _, want := range []string{"Brain lint report (notes/+#architecture)", "- documents: 1", "- orphans: 0", "- dead links: 0", "- stale references: 1"} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("content = %q, want substring %q", result.Content, want)
		}
	}
}

func TestBrainLintRejectsInvalidCombinedScope(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md": "# A",
	})
	tool := NewBrainLint(backend, brainConfig(true))

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"scope":"notes/+#architecture+#debugging"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for invalid scope")
	}
	if !strings.Contains(result.Content, "Invalid input") {
		t.Fatalf("content = %q, want invalid input message", result.Content)
	}
	if !strings.Contains(result.Content, "invalid brain lint scope") {
		t.Fatalf("content = %q, want scope validation detail", result.Content)
	}
}

func TestBrainLintReportsMissingPages(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md": "# A\n[[concepts/shared-gap]]\n[[concepts/single-gap]]",
		"notes/b.md": "# B\n[[concepts/shared-gap]]",
	})
	tool := NewBrainLint(backend, brainConfig(true))

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"scope":"full","checks":["missing_pages"]}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	for _, want := range []string{"- missing pages: 1", "Missing page suggestions:", "- concepts/shared-gap (referenced 2 times)"} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("content = %q, want substring %q", result.Content, want)
		}
	}
	if strings.Contains(result.Content, "single-gap") {
		t.Fatalf("single-use missing page should be suppressed: %q", result.Content)
	}
}

func TestBrainLintMissingPagesIsOptional(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md": "# A\n[[concepts/shared-gap]]",
		"notes/b.md": "# B\n[[concepts/shared-gap]]",
	})
	tool := NewBrainLint(backend, brainConfig(true))

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"scope":"full"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if strings.Contains(result.Content, "missing pages") {
		t.Fatalf("missing_pages should not run by default: %q", result.Content)
	}
}

func TestBrainLintAcceptsMissingPagesCheckName(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md": "# A",
	})
	tool := NewBrainLint(backend, brainConfig(true))

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"checks":["missing_pages"]}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
}

func TestBrainLintRejectsContradictionsWithoutAllowModelCalls(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md": "# A",
		"notes/b.md": "# B",
	})
	tool := NewBrainLintWithProvider(backend, brainConfig(true), &mockContradictionProvider{})

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"checks":["contradictions"]}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false when contradictions lacks explicit opt-in")
	}
	if !strings.Contains(result.Content, "allow_model_calls") {
		t.Fatalf("content = %q, want allow_model_calls guidance", result.Content)
	}
}

func TestBrainLintRejectsContradictionsWithoutProvider(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md": "---\ntags: [architecture]\n---\n# A\nStatus: auth enabled.",
		"notes/b.md": "---\ntags: [architecture]\n---\n# B\nStatus: auth disabled.",
	})
	tool := NewBrainLint(backend, brainConfig(true))

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"checks":["contradictions"],"allow_model_calls":true}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false when contradiction provider is unavailable")
	}
	if !strings.Contains(result.Content, "provider") {
		t.Fatalf("content = %q, want provider guidance", result.Content)
	}
}

func TestBrainLintReportsContradictionsWhenExplicitlyOptedIn(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md": "---\ntags: [architecture]\n---\n# Auth Plan\nStatus: auth enabled.",
		"notes/b.md": "---\ntags: [architecture]\n---\n# Auth Rollback\nStatus: auth disabled.",
	})
	mock := &mockContradictionProvider{
		response: &provider.Response{Content: []provider.ContentBlock{provider.NewTextBlock(`{"contradictions":[{"left":"notes/b.md","right":"notes/a.md","summary":"One note says auth is enabled while the other says auth is disabled.","confidence":"high"}]}`)}},
	}
	tool := NewBrainLintWithProvider(backend, brainConfig(true), mock)

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"checks":["contradictions"],"allow_model_calls":true}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	for _, want := range []string{"- contradictions: 1", "- contradiction pairs examined: 1", "Potential contradictions:", "- notes/a.md <> notes/b.md: One note says auth is enabled while the other says auth is disabled. [high]"} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("content = %q, want substring %q", result.Content, want)
		}
	}
	if mock.completeCalls != 1 {
		t.Fatalf("Complete calls = %d, want 1", mock.completeCalls)
	}
	if mock.lastRequest == nil || mock.lastRequest.Purpose != "brain_lint" {
		t.Fatalf("last request = %#v, want brain_lint purpose", mock.lastRequest)
	}
}

func TestBrainLintContradictionsSkipsProviderWhenNoCandidates(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md": "# A\nAlpha only.",
		"notes/b.md": "# B\nBeta only.",
	})
	mock := &mockContradictionProvider{}
	tool := NewBrainLintWithProvider(backend, brainConfig(true), mock)

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"checks":["contradictions"],"allow_model_calls":true}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if strings.Contains(result.Content, "Potential contradictions:") {
		t.Fatalf("unexpected contradictions section in %q", result.Content)
	}
	if !strings.Contains(result.Content, "- contradiction pairs examined: 0") {
		t.Fatalf("content = %q, want zero examined pairs", result.Content)
	}
	if mock.completeCalls != 0 {
		t.Fatalf("Complete calls = %d, want 0 when no candidates", mock.completeCalls)
	}
}

func TestParseContradictionResponseRejectsEmptyText(t *testing.T) {
	_, err := parseContradictionResponse(&provider.Response{Content: []provider.ContentBlock{provider.NewTextBlock("  ")}})
	if err == nil || !strings.Contains(err.Error(), "no text") {
		t.Fatalf("err = %v, want no text error", err)
	}
}

func TestParseContradictionResponseRejectsMalformedJSON(t *testing.T) {
	_, err := parseContradictionResponse(&provider.Response{Content: []provider.ContentBlock{provider.NewTextBlock(`{"contradictions":[`)}})
	if err == nil || !strings.Contains(err.Error(), "parse contradiction response") {
		t.Fatalf("err = %v, want parse error", err)
	}
}

func TestParseContradictionResponseNormalizesAndDeduplicatesPairs(t *testing.T) {
	findings, err := parseContradictionResponse(&provider.Response{Content: []provider.ContentBlock{provider.NewTextBlock(`{"contradictions":[{"left":"notes/b.md","right":"notes/a.md","summary":"Mismatch","confidence":"low"},{"left":"notes/a.md","right":"notes/b.md","summary":"Mismatch","confidence":"high"}]}`)}})
	if err != nil {
		t.Fatalf("parseContradictionResponse returned error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	if got := findings[0]; got.Left != "notes/a.md" || got.Right != "notes/b.md" || got.Confidence != "high" {
		t.Fatalf("unexpected normalized finding: %+v", got)
	}
}

func TestRegisterBrainToolsWithProviderWiresBrainLintLLM(t *testing.T) {
	reg := NewRegistry()
	mock := &mockContradictionProvider{}
	RegisterBrainToolsWithProvider(reg, newFakeBackend(map[string]string{}), brainConfig(true), mock)

	tool, ok := reg.Get("brain_lint")
	if !ok {
		t.Fatal("brain_lint not registered")
	}
	lintTool, ok := tool.(*BrainLint)
	if !ok {
		t.Fatalf("brain_lint type = %T, want *BrainLint", tool)
	}
	if lintTool.llm != mock {
		t.Fatalf("brain_lint llm = %#v, want mock provider", lintTool.llm)
	}
}

func TestBrainLintAppendsLintOperationLogWithSession(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md": "# A\n[[notes/missing]]",
	})
	tool := NewBrainLint(backend, brainConfig(true))

	ctx := ContextWithExecutionMeta(context.Background(), ExecutionMeta{
		ConversationID: "conv-lint",
		TurnNumber:     1,
		Iteration:      1,
	})
	result, err := tool.Execute(ctx, "/tmp", json.RawMessage(`{"scope":"full","checks":["dead_links"]}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}

	logDoc := backend.docs["_log.md"]
	if !strings.Contains(logDoc, "Session: conv-lint") {
		t.Fatalf("expected lint session log entry, got:\n%s", logDoc)
	}
}

func TestBrainLintAppendsLintOperationLog(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md": "# A\n[[notes/missing]]",
	})
	tool := NewBrainLint(backend, brainConfig(true))

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"scope":"full","checks":["dead_links"]}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}

	logDoc := backend.docs["_log.md"]
	if !strings.Contains(logDoc, "lint | full") {
		t.Fatalf("expected lint log entry, got:\n%s", logDoc)
	}
}
