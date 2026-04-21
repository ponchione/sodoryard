package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/brain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/role"
	"github.com/ponchione/sodoryard/internal/tool"
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

func (f *fakeReceiptBackend) PatchDocument(ctx context.Context, path string, operation string, content string) error {
	return fmt.Errorf("unsupported")
}

func (f *fakeReceiptBackend) SearchKeyword(ctx context.Context, query string) ([]brain.SearchHit, error) {
	return nil, nil
}

func (f *fakeReceiptBackend) ListDocuments(ctx context.Context, directory string) ([]string, error) {
	return nil, nil
}

func writeRunConfig(t *testing.T, projectRoot string, roleYAML string) string {
	t.Helper()
	brainRoot := filepath.Join(projectRoot, ".brain")
	if err := os.MkdirAll(brainRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	content := strings.Join([]string{
		"project_root: " + projectRoot,
		"log_level: info",
		"log_format: text",
		"brain:",
		"  enabled: true",
		"  vault_path: .brain",
		"routing:",
		"  default:",
		"    provider: codex",
		"    model: gpt-5.4-mini",
		"providers:",
		"  codex:",
		"    type: codex",
		"    model: gpt-5.4-mini",
		"agent_roles:",
		roleYAML,
	}, "\n") + "\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	return configPath
}

func stubRunRuntime(t *testing.T, runtime *appRuntime) {
	t.Helper()
	prev := buildRunRuntime
	buildRunRuntime = func(ctx context.Context, cfg *appconfig.Config) (*appRuntime, error) {
		return runtime, nil
	}
	t.Cleanup(func() { buildRunRuntime = prev })
}

func stubRunRegistry(t *testing.T, registry *tool.Registry, brainCfg appconfig.BrainConfig, err error) {
	t.Helper()
	prev := buildRunRoleRegistry
	buildRunRoleRegistry = func(cfg *appconfig.Config, roleCfg appconfig.AgentRoleConfig, deps role.BuilderDeps) (*tool.Registry, appconfig.BrainConfig, error) {
		return registry, brainCfg, err
	}
	t.Cleanup(func() { buildRunRoleRegistry = prev })
}

func stubRunConversation(t *testing.T, conv *conversation.Conversation, err error) {
	t.Helper()
	prev := createRunConversation
	createRunConversation = func(ctx context.Context, manager *conversation.Manager, projectRoot string, opts ...conversation.CreateOption) (*conversation.Conversation, error) {
		return conv, err
	}
	t.Cleanup(func() { createRunConversation = prev })
}

type fakeRunLoop struct {
	result *agent.TurnResult
	err    error
}

func (f *fakeRunLoop) RunTurn(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
	return f.result, f.err
}

func (f *fakeRunLoop) Close() {}

func stubRunLoop(t *testing.T, loop runAgentLoop) {
	t.Helper()
	prev := newRunAgentLoop
	newRunAgentLoop = func(deps agent.AgentLoopDeps) runAgentLoop { return loop }
	t.Cleanup(func() { newRunAgentLoop = prev })
}

func TestRunHeadlessRejectsMutuallyExclusiveTaskSources(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetContext(context.Background())
	_, err := runHeadless(cmd, filepath.Join(t.TempDir(), "sirtopham.yaml"), runFlags{
		Role:     "coder",
		Task:     "inline",
		TaskFile: filepath.Join(t.TempDir(), "task.txt"),
		Timeout:  time.Minute,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var exitErr runExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error = %T, want runExitError", err)
	}
	if exitErr.ExitCode() != runExitInfrastructure {
		t.Fatalf("exit code = %d, want %d", exitErr.ExitCode(), runExitInfrastructure)
	}
}

func TestRunHeadlessRejectsCustomToolsRoleBeforeLoopStart(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := writeRunConfig(t, projectRoot, strings.Join([]string{
		"  orchestrator:",
		"    system_prompt: agents/orchestrator.md",
		"    tools:",
		"      - brain",
		"    custom_tools:",
		"      - spawn_agent",
		"    brain_write_paths:",
		"      - receipts/orchestrator/**",
	}, "\n"))
	promptDir := filepath.Join(projectRoot, "agents")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptDir, "orchestrator.md"), []byte("prompt"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	stubRunRuntime(t, &appRuntime{Cleanup: func() {}})

	cmd := newRootCmd()
	cmd.SetContext(context.Background())
	_, err := runHeadless(cmd, configPath, runFlags{Role: "orchestrator", Task: "plan", Timeout: time.Minute})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "custom_tools are not implemented") {
		t.Fatalf("error = %v, want custom_tools rejection", err)
	}
}

func TestRunHeadlessSupportsTaskFileAndWritesFallbackReceipt(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := writeRunConfig(t, projectRoot, strings.Join([]string{
		"  coder:",
		"    system_prompt: agents/coder.md",
		"    tools:",
		"      - brain",
		"    brain_write_paths:",
		"      - receipts/coder/**",
	}, "\n"))
	promptDir := filepath.Join(projectRoot, "agents")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptDir, "coder.md"), []byte("prompt"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	taskPath := filepath.Join(t.TempDir(), "task.txt")
	if err := os.WriteFile(taskPath, []byte("do the work\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	backend := &fakeReceiptBackend{docs: map[string]string{}}
	stubRunRuntime(t, &appRuntime{BrainBackend: backend, Logger: slog.Default(), Cleanup: func() {}})
	stubRunRegistry(t, tool.NewRegistry(), appconfig.BrainConfig{Enabled: true, BrainWritePaths: []string{"receipts/coder/**"}}, nil)
	stubRunConversation(t, &conversation.Conversation{ID: "conv-1"}, nil)
	stubRunLoop(t, &fakeRunLoop{result: &agent.TurnResult{FinalText: "done"}})

	cmd := newRootCmd()
	cmd.SetContext(context.Background())
	result, err := runHeadless(cmd, configPath, runFlags{Role: "coder", TaskFile: taskPath, ChainID: "chain-1", Timeout: time.Minute})
	if err != nil {
		t.Fatalf("runHeadless returned error: %v", err)
	}
	if result == nil || result.ExitCode != runExitOK {
		t.Fatalf("result = %#v, want exit 0", result)
	}
	if result.ReceiptPath != "receipts/coder/chain-1.md" {
		t.Fatalf("receipt path = %q, want receipts/coder/chain-1.md", result.ReceiptPath)
	}
	if !strings.Contains(backend.docs[result.ReceiptPath], "done") {
		t.Fatalf("receipt content = %q, want final text", backend.docs[result.ReceiptPath])
	}
}

func TestRunHeadlessReturnsSafetyLimitExitAndReceiptPath(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := writeRunConfig(t, projectRoot, strings.Join([]string{
		"  coder:",
		"    system_prompt: agents/coder.md",
		"    tools:",
		"      - brain",
		"    brain_write_paths:",
		"      - receipts/coder/**",
		"    max_tokens: 10",
	}, "\n"))
	promptDir := filepath.Join(projectRoot, "agents")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptDir, "coder.md"), []byte("prompt"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	backend := &fakeReceiptBackend{docs: map[string]string{}}
	stubRunRuntime(t, &appRuntime{BrainBackend: backend, Logger: slog.Default(), Cleanup: func() {}})
	stubRunRegistry(t, tool.NewRegistry(), appconfig.BrainConfig{Enabled: true, BrainWritePaths: []string{"receipts/coder/**"}}, nil)
	stubRunConversation(t, &conversation.Conversation{ID: "conv-1"}, nil)
	loopResult := &agent.TurnResult{FinalText: "stopped by limit"}
	loopResult.TotalUsage.InputTokens = 6
	loopResult.TotalUsage.OutputTokens = 5
	stubRunLoop(t, &fakeRunLoop{result: loopResult})

	cmd := newRootCmd()
	cmd.SetContext(context.Background())
	result, err := runHeadless(cmd, configPath, runFlags{Role: "coder", Task: "work", ChainID: "chain-2", Timeout: time.Minute})
	if err != nil {
		t.Fatalf("runHeadless returned error: %v", err)
	}
	if result == nil || result.ExitCode != runExitSafetyLimit {
		t.Fatalf("result=%#v, want safety-limit exit 2", result)
	}
	if !strings.Contains(backend.docs[result.ReceiptPath], "verdict: safety_limit") {
		t.Fatalf("receipt content = %q, want safety_limit verdict", backend.docs[result.ReceiptPath])
	}
}

func TestRunHeadlessReturnsEscalationExitWhenReceiptSaysEscalate(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := writeRunConfig(t, projectRoot, strings.Join([]string{
		"  coder:",
		"    system_prompt: agents/coder.md",
		"    tools:",
		"      - brain",
		"    brain_write_paths:",
		"      - receipts/coder/**",
	}, "\n"))
	promptDir := filepath.Join(projectRoot, "agents")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptDir, "coder.md"), []byte("prompt"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	backend := &fakeReceiptBackend{docs: map[string]string{
		"receipts/coder/chain-3.md": `---
agent: coder
chain_id: chain-3
step: 1
verdict: escalate
timestamp: 2026-04-11T00:00:00Z
turns_used: 1
tokens_used: 1
duration_seconds: 0
---

## Summary
Escalate.`,
	}}
	stubRunRuntime(t, &appRuntime{BrainBackend: backend, Logger: slog.Default(), Cleanup: func() {}})
	stubRunRegistry(t, tool.NewRegistry(), appconfig.BrainConfig{Enabled: true, BrainWritePaths: []string{"receipts/coder/**"}}, nil)
	stubRunConversation(t, &conversation.Conversation{ID: "conv-1"}, nil)
	stubRunLoop(t, &fakeRunLoop{result: &agent.TurnResult{FinalText: "done"}})

	cmd := newRootCmd()
	cmd.SetContext(context.Background())
	result, err := runHeadless(cmd, configPath, runFlags{Role: "coder", Task: "work", ChainID: "chain-3", Timeout: time.Minute})
	if err != nil {
		t.Fatalf("runHeadless returned error: %v", err)
	}
	if result == nil || result.ExitCode != runExitEscalation {
		t.Fatalf("result=%#v, want escalation exit 3", result)
	}
}
