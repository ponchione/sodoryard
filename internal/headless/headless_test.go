package headless

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/brain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
)

type fakeReceiptBackend struct {
	docs map[string]string
}

func (f *fakeReceiptBackend) ReadDocument(ctx context.Context, path string) (string, error) {
	if content, ok := f.docs[path]; ok {
		return content, nil
	}
	return "", fmt.Errorf("Document not found: %s", path)
}

func (f *fakeReceiptBackend) WriteDocument(ctx context.Context, path string, content string) error {
	if f.docs == nil {
		f.docs = map[string]string{}
	}
	f.docs[path] = content
	return nil
}

func (f *fakeReceiptBackend) PatchDocument(context.Context, string, string, string) error {
	return fmt.Errorf("unsupported")
}

func (f *fakeReceiptBackend) SearchKeyword(context.Context, string) ([]brain.SearchHit, error) {
	return nil, nil
}

func (f *fakeReceiptBackend) ListDocuments(context.Context, string) ([]string, error) {
	return nil, nil
}

func TestReadTaskFromFile(t *testing.T) {
	path := t.TempDir() + "/task.txt"
	content := "implement the feature\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("osWriteFile returned error: %v", err)
	}
	got, err := ReadTask("", path)
	if err != nil {
		t.Fatalf("ReadTask returned error: %v", err)
	}
	if got != "implement the feature" {
		t.Fatalf("ReadTask = %q, want trimmed file content", got)
	}
}

func TestValidateReceiptContent(t *testing.T) {
	content := `---
agent: coder
chain_id: chain-1
step: 1
verdict: completed
timestamp: 2026-04-11T00:00:00Z
turns_used: 2
tokens_used: 42
duration_seconds: 5
---

## Summary
Done.`
	receipt, err := ValidateReceiptContent(content)
	if err != nil {
		t.Fatalf("ValidateReceiptContent returned error: %v", err)
	}
	if receipt.Verdict != "completed" {
		t.Fatalf("receipt verdict = %q, want completed", receipt.Verdict)
	}
}

func TestEnsureReceiptWritesFallbackWhenMissing(t *testing.T) {
	backend := &fakeReceiptBackend{docs: map[string]string{}}
	turnResult := &agent.TurnResult{FinalText: "finished work", IterationCount: 3, Duration: 2 * time.Second}
	turnResult.TotalUsage.InputTokens = 10
	turnResult.TotalUsage.OutputTokens = 5

	path, receipt, err := EnsureReceipt(context.Background(), backend, appconfig.BrainConfig{Enabled: true, BrainWritePaths: []string{"receipts/**"}}, "coder", "chain-1", "receipts/coder/chain-1.md", "completed_no_receipt", turnResult.FinalText, turnResult)
	if err != nil {
		t.Fatalf("EnsureReceipt returned error: %v", err)
	}
	if path != "receipts/coder/chain-1.md" {
		t.Fatalf("receipt path = %q, want receipts/coder/chain-1.md", path)
	}
	if receipt == nil || receipt.Verdict != "completed_no_receipt" {
		t.Fatalf("receipt = %#v, want fallback completed_no_receipt", receipt)
	}
	if !strings.Contains(backend.docs[path], "finished work") {
		t.Fatalf("fallback receipt content = %q, want final text", backend.docs[path])
	}
}

func TestEnsureReceiptUsesExistingValidReceipt(t *testing.T) {
	backend := &fakeReceiptBackend{docs: map[string]string{
		"receipts/coder/chain-1.md": `---
agent: coder
chain_id: chain-1
step: 1
verdict: escalate
timestamp: 2026-04-11T00:00:00Z
turns_used: 2
tokens_used: 42
duration_seconds: 5
---

## Summary
Escalate.`,
	}}
	path, receipt, err := EnsureReceipt(context.Background(), backend, appconfig.BrainConfig{Enabled: true, BrainWritePaths: []string{"receipts/**"}}, "coder", "chain-1", "receipts/coder/chain-1.md", "completed_no_receipt", "ignored", nil)
	if err != nil {
		t.Fatalf("EnsureReceipt returned error: %v", err)
	}
	if path != "receipts/coder/chain-1.md" || receipt == nil || receipt.Verdict != "escalate" {
		t.Fatalf("got (%q, %#v), want existing escalate receipt", path, receipt)
	}
}

func TestEnsureReceiptRejectsDisallowedFallbackPath(t *testing.T) {
	backend := &fakeReceiptBackend{docs: map[string]string{}}
	_, _, err := EnsureReceipt(context.Background(), backend, appconfig.BrainConfig{Enabled: true, BrainWritePaths: []string{"receipts/coder/**"}}, "coder", "chain-1", "receipts/auditor/chain-1.md", "completed_no_receipt", "done", nil)
	if err == nil {
		t.Fatal("expected disallowed receipt path error, got nil")
	}
	if !strings.Contains(err.Error(), "receipt path policy") {
		t.Fatalf("error = %q, want receipt path policy message", err)
	}
}

func TestEnsureReceiptFallbackInfersStepFromReceiptPath(t *testing.T) {
	backend := &fakeReceiptBackend{docs: map[string]string{}}
	path, receipt, err := EnsureReceipt(context.Background(), backend, appconfig.BrainConfig{Enabled: true, BrainWritePaths: []string{"receipts/**"}}, "coder", "chain-1", "receipts/coder/chain-1-step-003.md", "completed_no_receipt", "done", nil)
	if err != nil {
		t.Fatalf("EnsureReceipt returned error: %v", err)
	}
	if path != "receipts/coder/chain-1-step-003.md" {
		t.Fatalf("path = %q, want step path", path)
	}
	if receipt == nil || receipt.Step != 3 {
		t.Fatalf("receipt.Step = %#v, want 3", receipt)
	}
}

func TestExceededMaxTokens(t *testing.T) {
	result := &agent.TurnResult{}
	result.TotalUsage.InputTokens = 10
	result.TotalUsage.OutputTokens = 5
	if !ExceededMaxTokens(result, 15) {
		t.Fatal("expected max token threshold to trigger")
	}
	if ExceededMaxTokens(result, 16) {
		t.Fatal("did not expect threshold above total usage to trigger")
	}
}

func TestFormatEventFormatsKeyEvents(t *testing.T) {
	if got := FormatEvent(agent.StatusEvent{State: agent.StateAssemblingContext}); !strings.Contains(got, "status:") {
		t.Fatalf("status format = %q", got)
	}
	if got := FormatEvent(agent.ToolCallStartEvent{ToolName: "file_read"}); !strings.Contains(got, "tool: start file_read") {
		t.Fatalf("tool start format = %q", got)
	}
	if got := FormatEvent(agent.TurnCompleteEvent{IterationCount: 2, Duration: time.Second}); !strings.Contains(got, "complete: iterations=2") {
		t.Fatalf("turn complete format = %q", got)
	}
}
