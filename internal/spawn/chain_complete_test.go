//go:build sqlite_fts5
// +build sqlite_fts5

package spawn

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/tool"
)

type fakeBrainBackend struct {
	docs     map[string]string
	writeErr error
	readErr  error
}

func (f *fakeBrainBackend) ReadDocument(ctx context.Context, path string) (string, error) {
	if f.readErr != nil {
		return "", f.readErr
	}
	if content, ok := f.docs[path]; ok {
		return content, nil
	}
	return "", fmt.Errorf("Document not found: %s", path)
}
func (f *fakeBrainBackend) WriteDocument(ctx context.Context, path string, content string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if f.docs == nil {
		f.docs = map[string]string{}
	}
	f.docs[path] = content
	return nil
}
func (f *fakeBrainBackend) PatchDocument(context.Context, string, string, string) error {
	return fmt.Errorf("unsupported")
}
func (f *fakeBrainBackend) SearchKeyword(context.Context, string) ([]brain.SearchHit, error) {
	return nil, nil
}
func (f *fakeBrainBackend) ListDocuments(context.Context, string) ([]string, error) { return nil, nil }

func TestChainCompleteHappyPathReturnsSentinel(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 2, MaxDuration: time.Hour, TokenBudget: 100})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	backend := &fakeBrainBackend{docs: map[string]string{}}
	completeTool := NewChainCompleteTool(store, backend, chainID)
	completeTool.Now = func() time.Time { return time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC) }

	result, err := completeTool.Execute(ctx, ".", []byte(`{"summary":"done","status":"success"}`))
	if !errors.Is(err, tool.ErrChainComplete) {
		t.Fatalf("error = %v, want ErrChainComplete", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("result = %#v, want success", result)
	}
	receiptPath := fmt.Sprintf("receipts/orchestrator/%s.md", chainID)
	if _, ok := backend.docs[receiptPath]; !ok {
		t.Fatalf("receipt not written: %#v", backend.docs)
	}
	if !strings.Contains(backend.docs[receiptPath], "verdict: completed") {
		t.Fatalf("receipt content = %q, want completed verdict", backend.docs[receiptPath])
	}
	stored, err2 := store.GetChain(ctx, chainID)
	if err2 != nil {
		t.Fatalf("GetChain returned error: %v", err2)
	}
	if stored.Status != "completed" || stored.Summary != "done" {
		t.Fatalf("unexpected chain: %+v", stored)
	}
}

func TestChainCompletePreservesPartialStatus(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, err := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 2, MaxDuration: time.Hour, TokenBudget: 100})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	backend := &fakeBrainBackend{docs: map[string]string{}}
	completeTool := NewChainCompleteTool(store, backend, chainID)
	completeTool.Now = func() time.Time { return time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC) }

	result, err := completeTool.Execute(ctx, ".", []byte(`{"summary":"done with concerns","status":"partial"}`))
	if !errors.Is(err, tool.ErrChainComplete) {
		t.Fatalf("error = %v, want ErrChainComplete", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("result = %#v, want success", result)
	}
	receiptPath := fmt.Sprintf("receipts/orchestrator/%s.md", chainID)
	if !strings.Contains(backend.docs[receiptPath], "Status: partial") {
		t.Fatalf("receipt content = %q, want partial status text", backend.docs[receiptPath])
	}
	stored, err2 := store.GetChain(ctx, chainID)
	if err2 != nil {
		t.Fatalf("GetChain returned error: %v", err2)
	}
	if stored.Status != "partial" || stored.Summary != "done with concerns" {
		t.Fatalf("unexpected chain: %+v", stored)
	}
}

func TestChainCompleteRejectsInvalidStatus(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 2, MaxDuration: time.Hour, TokenBudget: 100})
	backend := &fakeBrainBackend{docs: map[string]string{}}
	completeTool := NewChainCompleteTool(store, backend, chainID)
	_, err := completeTool.Execute(ctx, ".", []byte(`{"summary":"done","status":"wat"}`))
	if err == nil || strings.Contains(fmt.Sprint(err), "chain complete") {
		t.Fatalf("error = %v, want normal invalid status error", err)
	}
	if len(backend.docs) != 0 {
		t.Fatalf("unexpected docs: %#v", backend.docs)
	}
}

func TestChainCompletePropagatesBackendFailure(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 2, MaxDuration: time.Hour, TokenBudget: 100})
	backend := &fakeBrainBackend{writeErr: fmt.Errorf("boom")}
	completeTool := NewChainCompleteTool(store, backend, chainID)
	_, err := completeTool.Execute(ctx, ".", []byte(`{"summary":"done","status":"success"}`))
	if err == nil || errors.Is(err, tool.ErrChainComplete) {
		t.Fatalf("error = %v, want backend write error", err)
	}
}
