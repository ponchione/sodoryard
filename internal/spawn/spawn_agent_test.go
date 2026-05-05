//go:build sqlite_fts5
// +build sqlite_fts5

package spawn

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	"github.com/ponchione/sodoryard/internal/receipt"
	toolpkg "github.com/ponchione/sodoryard/internal/tool"
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
		if in.OnStart != nil {
			in.OnStart(4321)
		}
		if in.Stdout != nil {
			_, _ = in.Stdout.Write([]byte("stdout line 1\nstdout line 2\n"))
		}
		if in.OnStdoutLine != nil {
			in.OnStdoutLine("stdout line 1")
			in.OnStdoutLine("stdout line 2")
		}
		if in.Stderr != nil {
			_, _ = in.Stderr.Write([]byte("stderr line 1\n"))
		}
		if in.OnStderrLine != nil {
			in.OnStderrLine("stderr line 1")
		}
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
	joinedArgs := strings.Join(gotArgs, " ")
	if len(gotArgs) == 0 || gotArgs[0] != "run" || !strings.Contains(joinedArgs, "--receipt-path receipts/coder/"+chainID+"-step-001.md") {
		t.Fatalf("unexpected args: %v", gotArgs)
	}
	gotTask := argValue(gotArgs, "--task")
	if !strings.Contains(gotTask, "do work") ||
		!strings.Contains(gotTask, "Chain ID: "+chainID) ||
		!strings.Contains(gotTask, "Step number: 1") ||
		!strings.Contains(gotTask, "Receipt path: receipts/coder/"+chainID+"-step-001.md") {
		t.Fatalf("spawn task missing harness context: %q", gotTask)
	}
	if strings.Contains(joinedArgs, "--quiet") {
		t.Fatalf("spawn args unexpectedly include --quiet: %v", gotArgs)
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
	if len(events) < 5 {
		t.Fatalf("expected step output events, got %+v", events)
	}
	var stdoutSeen, stderrSeen, processStartedSeen, processExitedSeen bool
	for _, event := range events {
		switch event.EventType {
		case chain.EventStepOutput:
			if strings.Contains(event.EventData, `"stream":"stdout"`) && strings.Contains(event.EventData, "stdout line 1") {
				stdoutSeen = true
			}
			if strings.Contains(event.EventData, `"stream":"stderr"`) && strings.Contains(event.EventData, "stderr line 1") {
				stderrSeen = true
			}
		case chain.EventStepProcessStarted:
			if strings.Contains(event.EventData, `"process_id":4321`) {
				processStartedSeen = true
			}
		case chain.EventStepProcessExited:
			if strings.Contains(event.EventData, `"process_id":4321`) && strings.Contains(event.EventData, `"exit_code":0`) {
				processExitedSeen = true
			}
		}
	}
	if !stdoutSeen || !stderrSeen {
		t.Fatalf("step output events missing stdout/stderr lines: %+v", events)
	}
	if !processStartedSeen || !processExitedSeen {
		t.Fatalf("step process events missing start/exit: %+v", events)
	}
}

func TestSpawnAgentUsesProjectMemoryCompleteStepWithReceipt(t *testing.T) {
	ctx := context.Background()
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: t.TempDir(), DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	defer backend.Close()
	store := chain.NewProjectMemoryStore(backend)
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "spawn-shunter-receipt", MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 1000})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	tool := NewSpawnAgentTool(SpawnAgentDeps{
		Store:        store,
		Backend:      backend,
		Config:       &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {}}},
		ChainID:      chainID,
		EngineBinary: "tidmouth",
		ProjectRoot:  t.TempDir(),
	})
	receiptPath := "receipts/coder/" + chainID + "-step-001.md"
	receiptContent := `---
agent: coder
chain_id: spawn-shunter-receipt
step: 1
verdict: completed
timestamp: 2026-05-06T14:00:00Z
turns_used: 2
tokens_used: 44
duration_seconds: 6
---

Done through Shunter atomic receipt completion.
`
	tool.now = func() time.Time { return time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC) }
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		if err := backend.WriteDocument(ctx, receiptPath, receiptContent); err != nil {
			t.Fatalf("WriteDocument receipt: %v", err)
		}
		return RunResult{ExitCode: 0}
	}
	result, err := tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil || !result.Success || !strings.Contains(result.Content, "Shunter atomic receipt") {
		t.Fatalf("unexpected result: %#v", result)
	}
	steps, err := store.ListSteps(ctx, chainID)
	if err != nil {
		t.Fatalf("ListSteps returned error: %v", err)
	}
	if len(steps) != 1 || steps[0].Status != "completed" || steps[0].ReceiptPath != receiptPath || steps[0].TokensUsed != 44 {
		t.Fatalf("steps = %+v, want Shunter-completed receipt step", steps)
	}
	ch, err := store.GetChain(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if ch.TotalSteps != 1 || ch.TotalTokens != 44 {
		t.Fatalf("chain = %+v, want updated Shunter metrics", ch)
	}
	receiptDoc, err := backend.ReadDocument(ctx, receiptPath)
	if err != nil {
		t.Fatalf("ReadDocument receipt: %v", err)
	}
	if receiptDoc != receiptContent {
		t.Fatalf("receipt doc = %q, want atomic receipt content", receiptDoc)
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	var completed bool
	for _, event := range events {
		if event.EventType == chain.EventStepCompleted && strings.Contains(event.EventData, `"tokens_used":44`) {
			completed = true
		}
	}
	if !completed {
		t.Fatalf("events = %+v, want step_completed from atomic reducer", events)
	}
	state, found, err := backend.ReadBrainIndexState(ctx)
	if err != nil {
		t.Fatalf("ReadBrainIndexState: %v", err)
	}
	if !found || !state.Dirty || state.DirtyReason != "complete_step_with_receipt" {
		t.Fatalf("brain index state = %+v found=%t, want complete_step_with_receipt dirty reason", state, found)
	}
}

func TestSpawnAgentAcceptsPersonaAliasAndUsesCanonicalRole(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{
		Store:        store,
		Backend:      backend,
		Config:       &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {SystemPrompt: "builtin:coder"}}},
		ChainID:      chainID,
		EngineBinary: "tidmouth",
		ProjectRoot:  t.TempDir(),
	})
	var gotArgs []string
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		gotArgs = append([]string(nil), in.Args...)
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
		return RunResult{ExitCode: 0}
	}

	if _, err := tool.Execute(ctx, ".", []byte(`{"role":"thomas","task":"do work"}`)); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := argValue(gotArgs, "--role"); got != "coder" {
		t.Fatalf("subprocess --role = %q, want canonical coder", got)
	}
	if got := argValue(gotArgs, "--receipt-path"); got != "receipts/coder/"+chainID+"-step-001.md" {
		t.Fatalf("subprocess --receipt-path = %q, want canonical coder receipt path", got)
	}
	steps, err := store.ListSteps(ctx, chainID)
	if err != nil {
		t.Fatalf("ListSteps returned error: %v", err)
	}
	if len(steps) != 1 || steps[0].Role != "coder" {
		t.Fatalf("steps = %+v, want canonical coder role", steps)
	}
}

func TestSpawnAgentTreatsSpecVerdictsAsCompletedStepExecutions(t *testing.T) {
	for _, verdict := range []string{
		"completed",
		"completed_with_concerns",
		"completed_no_receipt",
		"fix_required",
		"blocked",
		"escalate",
		"safety_limit",
	} {
		t.Run(verdict, func(t *testing.T) {
			if got := statusFromVerdict(receipt.Verdict(verdict)); got != "completed" {
				t.Fatalf("statusFromVerdict(%q) = %q, want completed", verdict, got)
			}
		})
	}
}

func TestSpawnAgentPassesRoleTimeoutToSubprocess(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{
		Store:        store,
		Backend:      backend,
		Config:       &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {Timeout: appconfig.Duration(45 * time.Minute)}}},
		ChainID:      chainID,
		EngineBinary: "tidmouth",
		ProjectRoot:  t.TempDir(),
	})
	var gotArgs []string
	var gotTimeout time.Duration
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		gotArgs = append([]string(nil), in.Args...)
		gotTimeout = in.Timeout
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
		return RunResult{ExitCode: 0}
	}

	if _, err := tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work"}`)); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	joinedArgs := strings.Join(gotArgs, " ")
	if !strings.Contains(joinedArgs, "--timeout 45m0s") {
		t.Fatalf("subprocess args = %v, want role timeout flag", gotArgs)
	}
	if gotTimeout <= 45*time.Minute {
		t.Fatalf("subprocess timeout = %s, want parent guard above role timeout", gotTimeout)
	}
}

func TestSpawnAgentCapsSubprocessTimeoutToRemainingChainDuration(t *testing.T) {
	ctx := context.Background()
	db := newSpawnTestDB(t)
	clockNow := time.Date(2026, 4, 11, 14, 0, 0, 0, time.UTC)
	store := chain.StoreWithClock(db, func() time.Time { return clockNow })
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Minute, TokenBudget: 100})
	if _, err := db.ExecContext(ctx, `UPDATE chains SET started_at = ? WHERE id = ?`, clockNow.Add(-50*time.Second).Format(time.RFC3339), chainID); err != nil {
		t.Fatalf("set started_at returned error: %v", err)
	}
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{
		Store:        store,
		Backend:      backend,
		Config:       &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {Timeout: appconfig.Duration(45 * time.Minute)}}},
		ChainID:      chainID,
		EngineBinary: "tidmouth",
		ProjectRoot:  t.TempDir(),
	})
	var gotArgs []string
	var gotTimeout time.Duration
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		gotArgs = append([]string(nil), in.Args...)
		gotTimeout = in.Timeout
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
		return RunResult{ExitCode: 0}
	}

	if _, err := tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work"}`)); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := argValue(gotArgs, "--timeout"); got != "10s" {
		t.Fatalf("subprocess --timeout = %q, want 10s (remaining chain duration)", got)
	}
	if gotTimeout != 20*time.Second {
		t.Fatalf("parent timeout = %s, want remaining chain duration plus grace", gotTimeout)
	}
}

func TestSpawnAgentFailsChainWhenStepExceedsTokenBudget(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 10})
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{
		Store:        store,
		Backend:      backend,
		Config:       &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {}}},
		ChainID:      chainID,
		EngineBinary: "tidmouth",
		ProjectRoot:  t.TempDir(),
	})
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		backend.docs["receipts/coder/"+chainID+"-step-001.md"] = `---
agent: coder
chain_id: ` + chainID + `
step: 1
verdict: completed
timestamp: 2026-04-11T00:00:00Z
turns_used: 1
tokens_used: 11
duration_seconds: 1
---

Done.
`
		return RunResult{ExitCode: 0}
	}

	result, err := tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work"}`))
	if !errors.Is(err, toolpkg.ErrChainComplete) {
		t.Fatalf("error = %v, want tool.ErrChainComplete after safety limit", err)
	}
	if result == nil || result.Success || !strings.Contains(result.Content, "token_budget exceeded") {
		t.Fatalf("result = %#v, want failed safety-limit result", result)
	}
	ch, err := store.GetChain(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChain returned error: %v", err)
	}
	if ch.Status != "failed" || !strings.Contains(ch.Summary, "token_budget exceeded") || ch.TotalTokens != 11 {
		t.Fatalf("chain = %+v, want failed status with exceeded token metrics", ch)
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	var sawSafetyLimit bool
	for _, event := range events {
		if event.EventType == chain.EventSafetyLimitHit && strings.Contains(event.EventData, "token_budget exceeded") {
			sawSafetyLimit = true
		}
	}
	if !sawSafetyLimit {
		t.Fatalf("events = %+v, want safety_limit_hit for token budget", events)
	}
}

func TestSpawnAgentReturnsErrorForInfrastructureExitWithReceipt(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{Store: store, Backend: backend, Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {}}}, ChainID: chainID, EngineBinary: "tidmouth", ProjectRoot: t.TempDir()})
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
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
		return RunResult{ExitCode: 1}
	}

	_, err := tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work"}`))
	if err == nil || !strings.Contains(err.Error(), "engine exited 1") {
		t.Fatalf("error = %v, want infrastructure exit error", err)
	}
	steps, _ := store.ListSteps(ctx, chainID)
	if len(steps) != 1 || steps[0].Status != "failed" || steps[0].ExitCode == nil || *steps[0].ExitCode != 1 {
		t.Fatalf("unexpected failed step: %+v", steps)
	}
}

func TestSpawnAgentRunsReindexBeforeWhenRequested(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	backend := &fakeBrainBackend{docs: map[string]string{}}
	expectedEnv := []string{"SODORYARD_MEMORY_ENDPOINT=unix:/tmp/memory.sock"}
	tool := NewSpawnAgentTool(SpawnAgentDeps{Store: store, Backend: backend, Config: &appconfig.Config{Brain: appconfig.BrainConfig{Enabled: true}, AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {}}}, ChainID: chainID, EngineBinary: "tidmouth", ProjectRoot: t.TempDir(), SubprocessEnv: expectedEnv})
	type commandCall struct {
		name string
		args []string
		env  []string
	}
	var calls []commandCall
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		calls = append(calls, commandCall{name: in.Name, args: append([]string(nil), in.Args...), env: append([]string(nil), in.Env...)})
		if len(calls) == 3 {
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
	if len(calls) != 3 ||
		calls[0].name != "tidmouth" || calls[0].args[0] != "index" ||
		calls[1].name != "yard" || strings.Join(calls[1].args, " ") != "brain index --config yard.yaml --quiet" ||
		calls[2].name != "tidmouth" || calls[2].args[0] != "run" {
		t.Fatalf("unexpected calls: %+v", calls)
	}
	if !strings.Contains(strings.Join(calls[0].args, " "), "--quiet") {
		t.Fatalf("code reindex args = %v, want --quiet", calls[0].args)
	}
	for _, call := range calls {
		if strings.Join(call.env, "\n") != strings.Join(expectedEnv, "\n") {
			t.Fatalf("%s env = %v, want %v", call.name, call.env, expectedEnv)
		}
	}
}

func TestSpawnAgentFailsWhenReceiptMissing(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{Store: store, Backend: backend, Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {}}}, ChainID: chainID, EngineBinary: "tidmouth", ProjectRoot: t.TempDir()})
	tool.now = func() time.Time { return time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC) }
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult { return RunResult{ExitCode: 1} }
	_, err := tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work"}`))
	if err == nil || !strings.Contains(err.Error(), "missing receipt") {
		t.Fatalf("error = %v, want missing receipt", err)
	}
	steps, _ := store.ListSteps(ctx, chainID)
	if len(steps) != 1 || steps[0].Status != "failed" {
		t.Fatalf("unexpected failed step: %+v", steps)
	}
	safetyReceipt := backend.docs["receipts/coder/"+chainID+"-step-001.md"]
	if !strings.Contains(safetyReceipt, "verdict: safety_limit") || !strings.Contains(safetyReceipt, "missing receipt") {
		t.Fatalf("safety receipt = %q, want safety_limit receipt explaining missing receipt", safetyReceipt)
	}
}

func argValue(args []string, name string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
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
	testSpawnAgentStopsCleanlyForChainStatus(t, "paused")
}

func TestSpawnAgentStopsCleanlyWhenPauseRequested(t *testing.T) {
	testSpawnAgentStopsCleanlyForChainStatus(t, "pause_requested")
}

func testSpawnAgentStopsCleanlyForChainStatus(t *testing.T, status string) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err := store.SetChainStatus(ctx, chainID, status); err != nil {
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
		t.Fatalf("runCommand called unexpectedly for %s chain", status)
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
	testSpawnAgentStopsCleanlyForCancellationStatus(t, "cancelled")
}

func TestSpawnAgentStopsCleanlyWhenCancelRequested(t *testing.T) {
	testSpawnAgentStopsCleanlyForCancellationStatus(t, "cancel_requested")
}

func testSpawnAgentStopsCleanlyForCancellationStatus(t *testing.T, status string) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	if err := store.SetChainStatus(ctx, chainID, status); err != nil {
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
		t.Fatalf("runCommand called unexpectedly for %s chain", status)
	}
	steps, err := store.ListSteps(ctx, chainID)
	if err != nil {
		t.Fatalf("ListSteps returned error: %v", err)
	}
	if len(steps) != 0 {
		t.Fatalf("steps = %+v, want none", steps)
	}
}
