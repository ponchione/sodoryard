package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/brain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
)

type fakeYardReceiptBackend struct {
	docs map[string]string
}

func (f *fakeYardReceiptBackend) ReadDocument(ctx context.Context, path string) (string, error) {
	if content, ok := f.docs[path]; ok {
		return content, nil
	}
	return "", fmt.Errorf("Document not found: %s", path)
}

func (f *fakeYardReceiptBackend) WriteDocument(ctx context.Context, path string, content string) error {
	if f.docs == nil {
		f.docs = map[string]string{}
	}
	f.docs[path] = content
	return nil
}
func (f *fakeYardReceiptBackend) PatchDocument(context.Context, string, string, string) error {
	return fmt.Errorf("unsupported")
}
func (f *fakeYardReceiptBackend) SearchKeyword(context.Context, string) ([]brain.SearchHit, error) {
	return nil, nil
}
func (f *fakeYardReceiptBackend) ListDocuments(context.Context, string) ([]string, error) {
	return nil, nil
}

func TestYardEnsureReceiptWritesFallbackWhenMissing(t *testing.T) {
	backend := &fakeYardReceiptBackend{docs: map[string]string{}}
	turnResult := &agent.TurnResult{FinalText: "finished work", IterationCount: 3, Duration: 2 * time.Second}
	turnResult.TotalUsage.InputTokens = 10
	turnResult.TotalUsage.OutputTokens = 5

	path, receipt, err := yardEnsureReceipt(context.Background(), backend, appconfig.BrainConfig{Enabled: true, BrainWritePaths: []string{"receipts/**"}}, "coder", "chain-1", "receipts/coder/chain-1.md", "completed_no_receipt", turnResult.FinalText, turnResult)
	if err != nil {
		t.Fatalf("yardEnsureReceipt returned error: %v", err)
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

func TestYardEnsureReceiptFallbackInfersStepFromReceiptPath(t *testing.T) {
	backend := &fakeYardReceiptBackend{docs: map[string]string{}}
	path, receipt, err := yardEnsureReceipt(context.Background(), backend, appconfig.BrainConfig{Enabled: true, BrainWritePaths: []string{"receipts/**"}}, "coder", "chain-1", "receipts/coder/chain-1-step-004.md", "completed_no_receipt", "done", nil)
	if err != nil {
		t.Fatalf("yardEnsureReceipt returned error: %v", err)
	}
	if path != "receipts/coder/chain-1-step-004.md" {
		t.Fatalf("path = %q, want step path", path)
	}
	if receipt == nil || receipt.Step != 4 {
		t.Fatalf("receipt.Step = %#v, want 4", receipt)
	}
	if !strings.Contains(backend.docs[path], "step: 4") {
		t.Fatalf("fallback receipt content = %q, want step 4", backend.docs[path])
	}
}
