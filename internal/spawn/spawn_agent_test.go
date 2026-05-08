//go:build sqlite_fts5
// +build sqlite_fts5

package spawn

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
		backend.docs["receipts/coder/"+chainID+"-step-001.md"] = testReceiptContent("coder", chainID, 1, receipt.VerdictCompleted, 33, testReceiptBody("Done."))
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
		!strings.Contains(gotTask, "Receipt path: receipts/coder/"+chainID+"-step-001.md") ||
		!strings.Contains(gotTask, "timestamp: <current UTC time in RFC3339 format>") ||
		!strings.Contains(gotTask, "Do not use created_at") ||
		!strings.Contains(gotTask, "put the answer in the Summary section") {
		t.Fatalf("spawn task missing harness context: %q", gotTask)
	}
	if strings.Contains(joinedArgs, "--quiet") {
		t.Fatalf("spawn args unexpectedly include --quiet: %v", gotArgs)
	}
	if strings.Contains(joinedArgs, "--max-turns") || strings.Contains(joinedArgs, "--max-tokens") {
		t.Fatalf("spawn args unexpectedly include optional limits: %v", gotArgs)
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

func TestSpawnAgentCapturesChangedFilesForSourceWriter(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	if output, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Skipf("git init failed: %v: %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(repo, "new-file.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{Store: store, Backend: backend, Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {}}}, ChainID: chainID, EngineBinary: "tidmouth", ProjectRoot: repo})
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

## Summary
Done.

## Changes
Created new-file.txt.

## Validation
- rtk make test

## Concerns
None.

## Next Steps
Audit.
`
		return RunResult{ExitCode: 0}
	}

	if _, err := tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work"}`)); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	var manifestSeen bool
	var facts postStepGuardrailFacts
	var factsSeen bool
	for _, event := range events {
		if event.EventType == chain.EventStepChangedFiles &&
			strings.Contains(event.EventData, `"count":1`) &&
			strings.Contains(event.EventData, `"new-file.txt"`) {
			manifestSeen = true
		}
		if event.EventType == chain.EventStepGuardrailFacts {
			factsSeen = true
			if err := json.Unmarshal([]byte(event.EventData), &facts); err != nil {
				t.Fatalf("decode guardrail facts: %v", err)
			}
		}
	}
	if !manifestSeen {
		t.Fatalf("events = %+v, want changed-file manifest for new-file.txt", events)
	}
	if !factsSeen {
		t.Fatalf("events = %+v, want post-step guardrail facts", events)
	}
	if !facts.ReceiptValid || !facts.ReceiptSchemaValid || !facts.ReceiptSectionsValid {
		t.Fatalf("guardrail facts receipt validity = %+v, want valid receipt", facts)
	}
	if !facts.SourceMutating || !facts.ChangedFileManifestPresent || facts.ChangedFileCount != 1 || len(facts.ChangedFiles) != 1 || facts.ChangedFiles[0] != "new-file.txt" {
		t.Fatalf("guardrail facts changed files = %+v, want new-file.txt manifest", facts)
	}
	if !facts.SourceWriterLockReleaseAttempted || !facts.SourceWriterLockReleased {
		t.Fatalf("guardrail facts lock release = %+v, want released source writer lock", facts)
	}
	if len(facts.ClaimedValidationCommands) != 1 || facts.ClaimedValidationCommands[0] != "rtk make test" {
		t.Fatalf("guardrail facts validation commands = %+v, want rtk make test", facts.ClaimedValidationCommands)
	}
}

func TestParseGitStatusChangedFiles(t *testing.T) {
	got := parseGitStatusChangedFiles(` M internal/foo.go
?? "docs/new file.md"
R  old.go -> internal/new.go
`)
	want := []string{"docs/new file.md", "internal/foo.go", "internal/new.go", "old.go"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("parseGitStatusChangedFiles = %#v, want %#v", got, want)
	}
}

func TestBuildReceiptFindingFactsIncludesLifecycleFacts(t *testing.T) {
	parsed, err := receipt.Parse([]byte(testReceiptContent("correctness-auditor", "chain-1", 1, receipt.VerdictFixRequired, 1, `## Summary
Audit.

## Changes
Reviewed.

## Validation
Not run.

## Concerns
None.

## Next Steps
Resolve.

## Findings

### FIND-correctness-001
Severity: high
Status: reopened
Evidence: internal/example.go:42
Summary: The nil case can still panic.
Required fix: Guard before dereferencing.
`)))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	facts, ok := buildReceiptFindingFacts("correctness-auditor", parsed)
	if !ok || len(facts.LifecycleFacts) != 1 {
		t.Fatalf("facts = %+v ok=%t, want one lifecycle fact", facts, ok)
	}
	got := facts.LifecycleFacts[0]
	if got.ID != "FIND-correctness-001" || got.SourceRole != "correctness-auditor" || got.Action != "reopened" || got.Status != "open" || got.Severity != "high" || got.Evidence != "internal/example.go:42" || got.RequiredFix == "" {
		t.Fatalf("lifecycle fact = %+v, want reopened finding details", got)
	}

	resolverReceipt, err := receipt.Parse([]byte(testReceiptContent("resolver", "chain-1", 2, receipt.VerdictCompleted, 1, `## Summary
Fixed.

## Changes
Patched internal/example.go.

## Validation
- rtk make test

## Concerns
None.

## Next Steps
Re-audit.

## Findings Addressed

### FIND-correctness-001
Resolution: fixed
Files changed:
- internal/example.go
Validation:
- rtk make test
`)))
	if err != nil {
		t.Fatalf("Parse resolver returned error: %v", err)
	}
	resolverFacts, ok := buildReceiptFindingFacts("resolver", resolverReceipt)
	if !ok || len(resolverFacts.LifecycleFacts) != 1 {
		t.Fatalf("resolver facts = %+v ok=%t, want one lifecycle fact", resolverFacts, ok)
	}
	resolved := resolverFacts.LifecycleFacts[0]
	if resolved.ID != "FIND-correctness-001" || resolved.Action != "addressed" || resolved.Status != "addressed" || resolved.Resolution != "fixed" {
		t.Fatalf("resolver lifecycle fact = %+v, want addressed fixed fact", resolved)
	}
	if len(resolved.FilesChanged) != 1 || resolved.FilesChanged[0] != "internal/example.go" || len(resolved.Validation) != 1 || resolved.Validation[0] != "rtk make test" {
		t.Fatalf("resolver lifecycle details = %+v/%+v, want changed file and validation", resolved.FilesChanged, resolved.Validation)
	}
}

func TestSpawnAgentLogsFindingLifecycleFactsForAuditor(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{Store: store, Backend: backend, Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"correctness-auditor": {}}}, ChainID: chainID, EngineBinary: "tidmouth", ProjectRoot: t.TempDir()})
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		backend.docs["receipts/correctness-auditor/"+chainID+"-step-001.md"] = testReceiptContent("correctness-auditor", chainID, 1, receipt.VerdictFixRequired, 1, `## Summary
Audit found one issue.

## Changes
Reviewed.

## Validation
Not run.

## Concerns
None.

## Next Steps
Resolver should fix the finding.

## Findings

### FIND-correctness-001
Severity: high
Status: open
Evidence: internal/example.go:42
Summary: The nil case can panic.
Required fix: Guard before dereferencing.
`)
		return RunResult{ExitCode: 0}
	}

	if _, err := tool.Execute(ctx, ".", []byte(`{"role":"correctness-auditor","task":"audit"}`)); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	var payload chain.FindingLifecycleFactsPayload
	var found bool
	for _, event := range events {
		if event.EventType != chain.EventFindingLifecycleFacts {
			continue
		}
		found = true
		if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil {
			t.Fatalf("decode lifecycle facts: %v", err)
		}
	}
	if !found || payload.Role != "correctness-auditor" || len(payload.Facts) != 1 {
		t.Fatalf("lifecycle payload = %+v found=%t, want auditor fact event", payload, found)
	}
	if got := payload.Facts[0]; got.ID != "FIND-correctness-001" || got.Action != "opened" || got.Status != "open" || got.Severity != "high" {
		t.Fatalf("lifecycle fact = %+v, want opened high finding", got)
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
	receiptContent := testReceiptContent("coder", chainID, 1, receipt.VerdictCompleted, 44, testReceiptBody("Done through Shunter atomic receipt completion."))
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

func TestSpawnAgentPassesHeadlessRunLimits(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 1000})
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{Store: store, Backend: backend, Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {}}}, ChainID: chainID, EngineBinary: "tidmouth", ProjectRoot: t.TempDir()})
	var gotArgs []string
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		gotArgs = append([]string(nil), in.Args...)
		backend.docs["receipts/coder/"+chainID+"-step-001.md"] = testReceiptContent("coder", chainID, 1, receipt.VerdictCompleted, 100, testReceiptBody("Done."))
		return RunResult{ExitCode: 0}
	}

	_, _, err := tool.RunStep(ctx, AgentStepInput{Role: "coder", Task: "do work", MaxTurns: 4, MaxTokens: 50000})
	if err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}
	if got := argValue(gotArgs, "--max-turns"); got != "4" {
		t.Fatalf("--max-turns = %q, want 4 (args=%v)", got, gotArgs)
	}
	if got := argValue(gotArgs, "--max-tokens"); got != "50000" {
		t.Fatalf("--max-tokens = %q, want 50000 (args=%v)", got, gotArgs)
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
		backend.docs["receipts/coder/"+chainID+"-step-001.md"] = testReceiptContent("coder", chainID, 1, receipt.VerdictCompleted, 1, testReceiptBody("Done."))
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

func TestSpawnAgentRejectsSecondSourceWriterAcrossProject(t *testing.T) {
	for _, tc := range []struct {
		name          string
		requestedRole string
	}{
		{name: "coder blocks coder", requestedRole: "coder"},
		{name: "coder blocks resolver", requestedRole: "resolver"},
		{name: "coder blocks test-writer", requestedRole: "test-writer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: t.TempDir(), DurableAck: true})
			if err != nil {
				t.Fatalf("OpenBrainBackend: %v", err)
			}
			defer backend.Close()
			store := chain.NewProjectMemoryStore(backend)
			if _, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "active-writer-chain", MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100}); err != nil {
				t.Fatalf("StartChain active returned error: %v", err)
			}
			if _, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "blocked-writer-chain", MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100}); err != nil {
				t.Fatalf("StartChain blocked returned error: %v", err)
			}
			if _, err := store.AcquireProjectLock(ctx, chain.AcquireProjectLockParams{
				LockName:     chain.SourceWriterLockName,
				OwnerChainID: "active-writer-chain",
				OwnerStepID:  "active-step",
				OwnerRole:    "coder",
				ExpiresAt:    time.Now().Add(time.Hour),
			}); err != nil {
				t.Fatalf("AcquireProjectLock active writer returned error: %v", err)
			}
			tool := NewSpawnAgentTool(SpawnAgentDeps{
				Store:   store,
				Backend: backend,
				Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{
					"coder":       {SystemPrompt: "builtin:coder"},
					"resolver":    {SystemPrompt: "builtin:resolver"},
					"test-writer": {SystemPrompt: "builtin:test-writer"},
				}},
				ChainID:      "blocked-writer-chain",
				EngineBinary: "tidmouth",
				ProjectRoot:  t.TempDir(),
			})
			runCalled := false
			tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
				runCalled = true
				return RunResult{ExitCode: 0}
			}

			_, err = tool.Execute(ctx, ".", []byte(`{"role":"`+tc.requestedRole+`","task":"do work"}`))
			if err == nil || !strings.Contains(err.Error(), "source writer guard") {
				t.Fatalf("error = %v, want source writer guard rejection", err)
			}
			if runCalled {
				t.Fatal("runCommand called despite active source writer")
			}
			steps, err := store.ListSteps(ctx, "blocked-writer-chain")
			if err != nil {
				t.Fatalf("ListSteps returned error: %v", err)
			}
			if len(steps) != 0 {
				t.Fatalf("steps = %+v, want no blocked step row", steps)
			}
			events, err := store.ListEvents(ctx, "blocked-writer-chain")
			if err != nil {
				t.Fatalf("ListEvents returned error: %v", err)
			}
			var blockedEvent bool
			for _, event := range events {
				if event.EventType == chain.EventSourceWriterBlocked &&
					strings.Contains(event.EventData, `"requested_role":"`+tc.requestedRole+`"`) &&
					strings.Contains(event.EventData, `"lock_name":"source_writer"`) &&
					strings.Contains(event.EventData, `"owner_role":"coder"`) {
					blockedEvent = true
				}
			}
			if !blockedEvent {
				t.Fatalf("events = %+v, want source writer guard blocked event", events)
			}
		})
	}
}

func TestSpawnAgentAllowsReadOnlyRoleWhileSourceWriterRuns(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	activeChainID, _ := store.StartChain(ctx, chain.ChainSpec{ChainID: "active-writer-chain", MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	activeStepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: activeChainID, SequenceNum: 1, Role: "coder", Task: "do work"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	if err := store.StepRunning(ctx, activeStepID); err != nil {
		t.Fatalf("StepRunning returned error: %v", err)
	}
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{ChainID: "readonly-chain", MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{
		Store:   store,
		Backend: backend,
		Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{
			"coder":               {SystemPrompt: "builtin:coder"},
			"correctness-auditor": {SystemPrompt: "builtin:correctness-auditor"},
		}},
		ChainID:      chainID,
		EngineBinary: "tidmouth",
		ProjectRoot:  t.TempDir(),
	})
	runCalled := false
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		runCalled = true
		backend.docs["receipts/correctness-auditor/"+chainID+"-step-001.md"] = testReceiptContent("correctness-auditor", chainID, 1, receipt.VerdictCompleted, 1, testAuditorReceiptBody("Audit complete."))
		return RunResult{ExitCode: 0}
	}

	result, err := tool.Execute(ctx, ".", []byte(`{"role":"correctness-auditor","task":"audit work"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !runCalled || result == nil || !result.Success {
		t.Fatalf("runCalled=%t result=%#v, want successful read-only spawn", runCalled, result)
	}
}

func TestSpawnAgentInjectsPreStepBriefing(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{ChainID: "briefing-chain", SourceTask: "audit the change", MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	stepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "do work"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	if err := store.CompleteStep(ctx, chain.CompleteStepParams{StepID: stepID, Status: "completed", Verdict: "completed", ReceiptPath: "receipts/coder/briefing-chain-step-001.md", TokensUsed: 3, TurnsUsed: 1}); err != nil {
		t.Fatalf("CompleteStep returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepID, chain.EventStepChangedFiles, map[string]any{"paths": []string{"internal/guard.go"}, "count": 1}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{
		Store:   store,
		Backend: backend,
		Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{
			"coder":               {SystemPrompt: "builtin:coder"},
			"correctness-auditor": {SystemPrompt: "builtin:correctness-auditor"},
		}},
		ChainID:      chainID,
		EngineBinary: "tidmouth",
		ProjectRoot:  t.TempDir(),
	})
	var gotTask string
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		gotTask = argValue(in.Args, "--task")
		backend.docs["receipts/correctness-auditor/"+chainID+"-step-002.md"] = `---
agent: correctness-auditor
chain_id: briefing-chain
step: 2
verdict: completed
timestamp: 2026-04-11T00:00:00Z
turns_used: 1
tokens_used: 1
duration_seconds: 1
---

## Summary
Audit complete.

## Changes
Only this receipt.

## Validation
Reviewed changed files.

## Concerns
None.

## Next Steps
Done.

## Findings
None.
`
		return RunResult{ExitCode: 0}
	}

	if _, err := tool.Execute(ctx, ".", []byte(`{"role":"correctness-auditor","task":"audit work"}`)); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, want := range []string{
		"Current chain briefing:",
		"Launch task: audit the change",
		"Current step: 2 correctness-auditor",
		"step 1 coder status=completed verdict=completed receipt=receipts/coder/briefing-chain-step-001.md",
		"internal/guard.go",
	} {
		if !strings.Contains(gotTask, want) {
			t.Fatalf("spawn task missing %q:\n%s", want, gotTask)
		}
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
		backend.docs["receipts/coder/"+chainID+"-step-001.md"] = testReceiptContent("coder", chainID, 1, receipt.VerdictCompleted, 1, testReceiptBody("Done."))
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
		backend.docs["receipts/coder/"+chainID+"-step-001.md"] = testReceiptContent("coder", chainID, 1, receipt.VerdictCompleted, 1, testReceiptBody("Done."))
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
		backend.docs["receipts/coder/"+chainID+"-step-001.md"] = testReceiptContent("coder", chainID, 1, receipt.VerdictCompleted, 11, testReceiptBody("Done."))
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
		backend.docs["receipts/coder/"+chainID+"-step-001.md"] = testReceiptContent("coder", chainID, 1, receipt.VerdictCompleted, 1, testReceiptBody("Done."))
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
			backend.docs["receipts/coder/"+chainID+"-step-001.md"] = testReceiptContent("coder", chainID, 1, receipt.VerdictCompleted, 1, testReceiptBody("Done."))
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
	if steps[0].ReceiptPath != "receipts/coder/"+chainID+"-step-001.md" {
		t.Fatalf("ReceiptPath = %q, want synthetic safety receipt path", steps[0].ReceiptPath)
	}
	safetyReceipt := backend.docs["receipts/coder/"+chainID+"-step-001.md"]
	if !strings.Contains(safetyReceipt, "verdict: safety_limit") || !strings.Contains(safetyReceipt, "missing receipt") {
		t.Fatalf("safety receipt = %q, want safety_limit receipt explaining missing receipt", safetyReceipt)
	}
}

func TestSpawnAgentReleasesSourceWriterLockOnFailurePaths(t *testing.T) {
	for _, tc := range []struct {
		name        string
		run         func(context.Context, *projectmemory.BrainBackend, string) RunResult
		wantErrText string
	}{
		{
			name: "missing receipt",
			run: func(context.Context, *projectmemory.BrainBackend, string) RunResult {
				return RunResult{ExitCode: 1}
			},
			wantErrText: "missing receipt",
		},
		{
			name: "invalid receipt contract",
			run: func(ctx context.Context, backend *projectmemory.BrainBackend, chainID string) RunResult {
				_ = backend.WriteDocument(ctx, "receipts/coder/"+chainID+"-step-001.md", `---
agent: resolver
chain_id: `+chainID+`
step: 1
verdict: completed
timestamp: 2026-04-11T00:00:00Z
turns_used: 1
tokens_used: 1
duration_seconds: 1
---

## Summary
Wrong role.

## Changes
None.

## Validation
Not run.

## Concerns
Invalid agent.

## Next Steps
Fix receipt.
`)
				return RunResult{ExitCode: 0}
			},
			wantErrText: "invalid field: agent",
		},
		{
			name: "infrastructure exit with receipt",
			run: func(ctx context.Context, backend *projectmemory.BrainBackend, chainID string) RunResult {
				_ = backend.WriteDocument(ctx, "receipts/coder/"+chainID+"-step-001.md", `---
agent: coder
chain_id: `+chainID+`
step: 1
verdict: completed
timestamp: 2026-04-11T00:00:00Z
turns_used: 1
tokens_used: 1
duration_seconds: 1
---

## Summary
Done.

## Changes
None.

## Validation
Not run.

## Concerns
Engine exited non-zero.

## Next Steps
Inspect infrastructure failure.
`)
				return RunResult{ExitCode: 1}
			},
			wantErrText: "engine exited 1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: t.TempDir(), DurableAck: true})
			if err != nil {
				t.Fatalf("OpenBrainBackend: %v", err)
			}
			defer backend.Close()
			store := chain.NewProjectMemoryStore(backend)
			chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "chain-lock-release", MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
			if err != nil {
				t.Fatalf("StartChain returned error: %v", err)
			}
			tool := NewSpawnAgentTool(SpawnAgentDeps{
				Store:        store,
				Backend:      backend,
				Config:       &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {SystemPrompt: "builtin:coder"}}},
				ChainID:      chainID,
				EngineBinary: "tidmouth",
				ProjectRoot:  t.TempDir(),
			})
			tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
				return tc.run(ctx, backend, chainID)
			}

			_, err = tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work"}`))
			if err == nil || !strings.Contains(err.Error(), tc.wantErrText) {
				t.Fatalf("error = %v, want %q", err, tc.wantErrText)
			}
			if lock, found, err := store.GetProjectLock(ctx, chain.SourceWriterLockName); err != nil || found {
				t.Fatalf("source writer lock after failure = %+v found=%t err=%v, want released", lock, found, err)
			}
			events, err := store.ListEvents(ctx, chainID)
			if err != nil {
				t.Fatalf("ListEvents returned error: %v", err)
			}
			var acquired, released bool
			for _, event := range events {
				if event.EventType == chain.EventSourceWriterLockAcquired {
					acquired = true
				}
				if event.EventType == chain.EventSourceWriterLockReleased {
					released = true
				}
			}
			if !acquired || !released {
				t.Fatalf("events = %+v, want lock acquired and released events", events)
			}
		})
	}
}

func TestSpawnAgentFailsWhenReceiptDoesNotMatchStep(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{Store: store, Backend: backend, Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {}}}, ChainID: chainID, EngineBinary: "tidmouth", ProjectRoot: t.TempDir()})
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		backend.docs["receipts/coder/"+chainID+"-step-001.md"] = `---
agent: resolver
chain_id: ` + chainID + `
step: 1
verdict: completed
timestamp: 2026-04-11T00:00:00Z
turns_used: 1
tokens_used: 1
duration_seconds: 1
---

## Summary
Done.

## Changes
None.

## Validation
Not run.

## Concerns
None.

## Next Steps
None.
`
		return RunResult{ExitCode: 0}
	}

	_, err := tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work"}`))
	if err == nil || !strings.Contains(err.Error(), "invalid field: agent") {
		t.Fatalf("error = %v, want agent validation failure", err)
	}
	steps, _ := store.ListSteps(ctx, chainID)
	if len(steps) != 1 || steps[0].Status != "failed" || !strings.Contains(steps[0].ErrorMessage, "validate receipt") {
		t.Fatalf("unexpected failed step: %+v", steps)
	}
}

func TestSpawnAgentFailsWhenReceiptSectionsAreMissing(t *testing.T) {
	ctx := context.Background()
	store := chain.NewStore(newSpawnTestDB(t))
	chainID, _ := store.StartChain(ctx, chain.ChainSpec{MaxSteps: 10, MaxResolverLoops: 1, MaxDuration: time.Hour, TokenBudget: 100})
	backend := &fakeBrainBackend{docs: map[string]string{}}
	tool := NewSpawnAgentTool(SpawnAgentDeps{Store: store, Backend: backend, Config: &appconfig.Config{AgentRoles: map[string]appconfig.AgentRoleConfig{"coder": {}}}, ChainID: chainID, EngineBinary: "tidmouth", ProjectRoot: t.TempDir()})
	tool.runCommand = func(ctx context.Context, in RunCommandInput) RunResult {
		backend.docs["receipts/coder/"+chainID+"-step-001.md"] = testReceiptContent("coder", chainID, 1, receipt.VerdictCompleted, 1, `## Summary
Done.`)
		return RunResult{ExitCode: 0}
	}

	_, err := tool.Execute(ctx, ".", []byte(`{"role":"coder","task":"do work"}`))
	if err == nil || !strings.Contains(err.Error(), "missing required section") {
		t.Fatalf("error = %v, want missing section failure", err)
	}
	steps, _ := store.ListSteps(ctx, chainID)
	if len(steps) != 1 || steps[0].Status != "failed" || !strings.Contains(steps[0].ErrorMessage, "validate receipt sections") {
		t.Fatalf("unexpected failed step: %+v", steps)
	}
	events, _ := store.ListEvents(ctx, chainID)
	var validationEvent bool
	var facts postStepGuardrailFacts
	var factsSeen bool
	for _, event := range events {
		if event.EventType == chain.EventReceiptValidation && strings.Contains(event.EventData, `"error":"receipt: missing required section`) {
			validationEvent = true
		}
		if event.EventType == chain.EventStepGuardrailFacts {
			factsSeen = true
			if err := json.Unmarshal([]byte(event.EventData), &facts); err != nil {
				t.Fatalf("decode guardrail facts: %v", err)
			}
		}
	}
	if !validationEvent {
		t.Fatalf("events = %+v, want receipt validation error event", events)
	}
	if !factsSeen || !facts.ReceiptSchemaValid || !facts.ReceiptStepValid || facts.ReceiptSectionsValid || facts.ReceiptValid || !strings.Contains(facts.ReceiptError, "missing required section") {
		t.Fatalf("guardrail facts = %+v, want invalid section facts", facts)
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

func testReceiptContent(role string, chainID string, step int, verdict receipt.Verdict, tokens int, body string) string {
	return `---
agent: ` + role + `
chain_id: ` + chainID + `
step: ` + strconv.Itoa(step) + `
verdict: ` + string(verdict) + `
timestamp: 2026-04-11T00:00:00Z
turns_used: 1
tokens_used: ` + strconv.Itoa(tokens) + `
duration_seconds: 1
---

` + strings.TrimSpace(body) + `
`
}

func testReceiptBody(summary string) string {
	return `## Summary
` + summary + `

## Changes
None.

## Validation
Not run.

## Concerns
None.

## Next Steps
None.`
}

func testAuditorReceiptBody(summary string) string {
	return testReceiptBody(summary) + `

## Findings
None.`
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
