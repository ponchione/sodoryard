package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/conversation"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/role"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/ponchione/sodoryard/internal/tool"
)

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

func TestReadTaskFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task.txt")
	if err := os.WriteFile(path, []byte("implement the feature\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	got, err := readTask("", path)
	if err != nil {
		t.Fatalf("readTask returned error: %v", err)
	}
	if got != "implement the feature" {
		t.Fatalf("readTask = %q, want trimmed file content", got)
	}
}

func TestResolveReceiptPathDefaultAndOverride(t *testing.T) {
	if got := resolveReceiptPath("coder", "chain-1", ""); got != "receipts/coder/chain-1.md" {
		t.Fatalf("resolveReceiptPath(default) = %q", got)
	}
	if got := resolveReceiptPath("coder", "chain-1", "custom/out.md"); got != "custom/out.md" {
		t.Fatalf("resolveReceiptPath(override) = %q", got)
	}
}

func TestLoadRoleSystemPromptResolvesRelativeToProjectRoot(t *testing.T) {
	projectRoot := t.TempDir()
	promptDir := filepath.Join(projectRoot, "agents")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	promptPath := filepath.Join(promptDir, "coder.md")
	if err := os.WriteFile(promptPath, []byte("you are coder"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	got, err := rtpkg.LoadRoleSystemPrompt(projectRoot, "agents/coder.md")
	if err != nil {
		t.Fatalf("loadRoleSystemPrompt returned error: %v", err)
	}
	if got != "you are coder" {
		t.Fatalf("LoadRoleSystemPrompt = %q, want prompt contents", got)
	}
}

func TestResolveModelContextLimitUsesConfiguredAndFallbackValues(t *testing.T) {
	cfg := &appconfig.Config{Providers: map[string]appconfig.ProviderConfig{
		"codex": {Type: "codex", ContextLength: 111},
		"local": {Type: "openai-compatible"},
	}}
	if got, err := rtpkg.ResolveModelContextLimit(cfg, "codex"); err != nil || got != 111 {
		t.Fatalf("ResolveModelContextLimit(codex) = (%d, %v), want (111, nil)", got, err)
	}
	if got, err := rtpkg.ResolveModelContextLimit(cfg, "local"); err != nil || got != 32768 {
		t.Fatalf("ResolveModelContextLimit(local) = (%d, %v), want (32768, nil)", got, err)
	}
}

func TestExceededMaxTokens(t *testing.T) {
	result := &agent.TurnResult{}
	result.TotalUsage.InputTokens = 10
	result.TotalUsage.OutputTokens = 5
	if !exceededMaxTokens(result, 15) {
		t.Fatal("expected max token threshold to trigger")
	}
	if exceededMaxTokens(result, 16) {
		t.Fatal("did not expect threshold above total usage to trigger")
	}
}

func TestRunProgressSinkFormatsKeyEvents(t *testing.T) {
	sink := newRunProgressSink(&strings.Builder{})
	if got := sink.format(agent.StatusEvent{State: agent.StateAssemblingContext}); !strings.Contains(got, "status:") {
		t.Fatalf("status format = %q", got)
	}
	if got := sink.format(agent.ToolCallStartEvent{ToolName: "file_read"}); !strings.Contains(got, "tool: start file_read") {
		t.Fatalf("tool start format = %q", got)
	}
	if got := sink.format(agent.TurnCompleteEvent{IterationCount: 2, Duration: time.Second}); !strings.Contains(got, "complete: iterations=2") {
		t.Fatalf("turn complete format = %q", got)
	}
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
