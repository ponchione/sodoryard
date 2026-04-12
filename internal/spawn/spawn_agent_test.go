//go:build sqlite_fts5
// +build sqlite_fts5

package spawn

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
)

func TestSpawnAgentRejectsUnknownRole(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	tool := NewSpawnAgentTool(SpawnAgentDeps{Store: store, Backend: &fakeBrainBackend{docs: map[string]string{}}, Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{}}, ChainID: chainID, ProjectRoot: t.TempDir()})
	_, err := tool.Execute(ctx, ".", []byte(`{"role":"missing","task":"do work"}`))
	if err == nil || !strings.Contains(err.Error(), "not defined in config") {
		t.Fatalf("error = %v, want unknown role", err)
	}
}

func TestSpawnAgentRunsSubprocessAndStoresReceipt(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{Store: store, Backend: backend, Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {}}}, ChainID: chainID, EngineBinary: "tidmouth", ProjectRoot: t.TempDir()})
	var gotArgs []string
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		gotArgs = append([]string(nil), in.Args...)
		backend.docs["receipts/coder/"+chainID+"-step-001.md"] = `---
agent: coder
chain_id: ` + chainID + `
step: 1
verdict: completed
timestamp: 2026-04-11T00:00:00Z
turns_used: 2
tokens_used: 33
duration_seconds: 5
---

Done.
`
		return RunResult{ExitCode: 0}
	}
	tool.now = func() time.Time { return time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC) }
	result, err := tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil || !result.Success || !strings.Contains(result.Content, "Done.") {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(gotArgs) == 0 || gotArgs[0] != "run" || !strings.Contains(strings.Join(gotArgs, " "), "--receipt-path receipts/coder/"+chainID+"-step-001.md") {
		t.Fatalf("unexpected args: %v", gotArgs)
	}
	steps, err := store.ListSteps(ctx, chainID)
	if err != nil {
		t.Fatalf("ListSteps returned error: %v", err)
	}
	if len(steps) != 1 || steps[0].Status != "completed" || steps[0].TokensUsed != 33 {
		t.Fatalf("unexpected steps: %+v", steps)
	}
	ch, err := store.GetChain(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if ch.TotalSteps != 1 || ch.TotalTokens != 33 {
		t.Fatalf("unexpected chain metrics: %+v", ch)
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected events, got %+v", events)
	}
}

func TestSpawnAgentRunsReindexBeforeWhenRequested(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{Store: store, Backend: backend, Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {}}}, ChainID: chainID, EngineBinary: "tidmouth", ProjectRoot: t.TempDir()})
	var calls [][]string
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		calls = append(calls, append([]string(nil), in.Args...))
		if len(calls) == 2 {
			backend.docs["receipts/coder/"+chainID+"-step-001.md"] = `---
agent: coder
chain_id: ` + chainID + `
step: 1
verdict: completed
timestamp: 2026-04-11T00:00:00Z
turns_used: 1
tokens_used: 1
duration_seconds: 1
---

Done.
`
		}
		return RunResult{ExitCode: 0}
	}
	_, err := tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work","reindex_before":true}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(calls) != 2 || calls[0][0] != "index" || calls[1][0] != "run" {
		t.Fatalf("unexpected calls: %+v", calls)
	}
}

func TestSpawnAgentFailsWhenReceiptMissing(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	tool := NewSpawnAgentTool(SpawnAgentDeps{Store: store, Backend: &fakeBrainBackend{docs: map[string]string{}}, Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {}}}, ChainID: chainID, EngineBinary: "tidmouth", ProjectRoot: t.TempDir()})
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult { return RunResult{ExitCode: 1} }
	_, err := tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work"}`))
	if err == nil || !strings.Contains(err.Error(), "missing receipt") {
		t.Fatalf("error = %v, want missing receipt", err)
	}
	steps, _ := store.ListSteps(ctx, chainID)
	if len(steps) != 1 || steps[0].Status != "failed" {
		t.Fatalf("unexpected failed step: %+v", steps)
	}
}

func TestSpawnAgentRejectsStepLimit(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 1, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err := store.UpdateChainMetrics(ctx, chainID, chain.ChainMetrics{TotalSteps: 1}); err != nil {
		t.Fatalf("UpdateChainMetrics returned error: %v", err)
	}
	tool := NewSpawnAgentTool(SpawnAgentDeps{Store: store, Backend: &fakeBrainBackend{docs: map[string]string{}}, Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {}}}, ChainID: chainID, EngineBinary: "tidmouth", ProjectRoot: t.TempDir()})
	_, err := tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work"}`))
	if err == nil || !strings.Contains(err.Error(), "max_steps exceeded") {
		t.Fatalf("error = %v, want max_steps exceeded", err)
	}
}

func TestSpawnAgentStopsCleanlyWhenChainPaused(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err := store.SetChainStatus(ctx, chainID, "paused"); err != nil {
		t.Fatalf("SetChainStatus returned error: %v", err)
	}
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{Store: store, Backend: backend, Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {}}}, ChainID: chainID, EngineBinary: "tidmouth", ProjectRoot: t.TempDir()})
	runCalled := false
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		runCalled = true
		return RunResult{ExitCode: 0}
	}
	_, err := tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work"}`))
	if err == nil || err.Error() != "tool: chain complete" {
		t.Fatalf("error = %v, want tool.ErrChainComplete", err)
	}
	if runCalled {
		t.Fatal("runCommand called unexpectedly for paused chain")
	}
	steps, err := store.ListSteps(ctx, chainID)
	if err != nil {
		t.Fatalf("ListSteps returned error: %v", err)
	}
	if len(steps) != 0 {
		t.Fatalf("steps = %+v, want none", steps)
	}
}

func TestSpawnAgentStopsCleanlyWhenChainCancelled(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err := store.SetChainStatus(ctx, chainID, "cancelled"); err != nil {
		t.Fatalf("SetChainStatus returned error: %v", err)
	}
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{Store: store, Backend: backend, Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {}}}, ChainID: chainID, EngineBinary: "tidmouth", ProjectRoot: t.TempDir()})
	runCalled := false
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		runCalled = true
		return RunResult{ExitCode: 0}
	}
	_, err := tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work"}`))
	if err == nil || err.Error() != "tool: chain complete" {
		t.Fatalf("error = %v, want tool.ErrChainComplete", err)
	}
	if runCalled {
		t.Fatal("runCommand called unexpectedly for cancelled chain")
	}
	steps, err := store.ListSteps(ctx, chainID)
	if err != nil {
		t.Fatalf("ListSteps returned error: %v", err)
	}
	if len(steps) != 0 {
		t.Fatalf("steps = %+v, want none", steps)
	}
}
