package headless

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/agent"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/role"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/ponchione/sodoryard/internal/tool"
)

func writeRunSessionConfig(t *testing.T, projectRoot string, roleYAML string) string {
	t.Helper()
	brainRoot := filepath.Join(projectRoot, ".brain")
	if err := os.MkdirAll(brainRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "yard.yaml")
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
		"    model: gpt-5.5",
		"providers:",
		"  codex:",
		"    type: codex",
		"    model: gpt-5.5",
		"agent_roles:",
		roleYAML,
	}, "\n") + "\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	return configPath
}

type fakeRunSessionLoop struct {
	result *agent.TurnResult
	err    error
}

func (f *fakeRunSessionLoop) RunTurn(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
	return f.result, f.err
}

func (f *fakeRunSessionLoop) Close() {}

func stubRunSessionDeps(rt *rtpkg.EngineRuntime, registry *tool.Registry, brainCfg appconfig.BrainConfig, conv *conversation.Conversation, loop AgentLoop) Deps {
	return Deps{
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.EngineRuntime, error) {
			return rt, nil
		},
		BuildRegistry: func(cfg *appconfig.Config, roleCfg appconfig.AgentRoleConfig, deps role.BuilderDeps) (*tool.Registry, appconfig.BrainConfig, error) {
			return registry, brainCfg, nil
		},
		CreateConversation: func(ctx context.Context, manager *conversation.Manager, projectRoot string, opts ...conversation.CreateOption) (*conversation.Conversation, error) {
			return conv, nil
		},
		NewAgentLoop: func(deps agent.AgentLoopDeps) AgentLoop {
			return loop
		},
		NewProgressSink: func(out io.Writer) agent.EventSink {
			return nil
		},
		NewChainID: func() string {
			return "generated-chain-id"
		},
	}
}

func TestRunSessionRejectsMutuallyExclusiveTaskSources(t *testing.T) {
	_, err := RunSession(context.Background(), nil, filepath.Join(t.TempDir(), "yard.yaml"), RunRequest{
		Role:     "coder",
		Task:     "inline",
		TaskFile: filepath.Join(t.TempDir(), "task.txt"),
		Timeout:  time.Minute,
	}, Deps{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "exactly one of --task or --task-file is required") {
		t.Fatalf("error = %v, want mutually exclusive task-source validation", err)
	}
}

func TestRunSessionRejectsCustomToolsRoleBeforeLoopStart(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := writeRunSessionConfig(t, projectRoot, strings.Join([]string{
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

	_, err := RunSession(context.Background(), nil, configPath, RunRequest{Role: "orchestrator", Task: "plan", Timeout: time.Minute}, Deps{
		BuildRuntime: func(ctx context.Context, cfg *appconfig.Config) (*rtpkg.EngineRuntime, error) {
			return &rtpkg.EngineRuntime{Cleanup: func() {}}, nil
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "custom_tools are not implemented") {
		t.Fatalf("error = %v, want custom_tools rejection", err)
	}
}

func TestRunSessionSupportsTaskFileAndWritesFallbackReceipt(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := writeRunSessionConfig(t, projectRoot, strings.Join([]string{
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
	deps := stubRunSessionDeps(
		&rtpkg.EngineRuntime{BrainBackend: backend, Logger: slog.Default(), Cleanup: func() {}},
		tool.NewRegistry(),
		appconfig.BrainConfig{Enabled: true, BrainWritePaths: []string{"receipts/coder/**"}},
		&conversation.Conversation{ID: "conv-1"},
		&fakeRunSessionLoop{result: &agent.TurnResult{FinalText: "done"}},
	)

	result, err := RunSession(context.Background(), nil, configPath, RunRequest{Role: "coder", TaskFile: taskPath, ChainID: "chain-1", Timeout: time.Minute}, deps)
	if err != nil {
		t.Fatalf("RunSession returned error: %v", err)
	}
	if result == nil || result.ExitCode != ExitOK {
		t.Fatalf("result = %#v, want exit 0", result)
	}
	if result.ReceiptPath != "receipts/coder/chain-1.md" {
		t.Fatalf("receipt path = %q, want receipts/coder/chain-1.md", result.ReceiptPath)
	}
	if !strings.Contains(backend.docs[result.ReceiptPath], "done") {
		t.Fatalf("receipt content = %q, want final text", backend.docs[result.ReceiptPath])
	}
}

func TestRunSessionReturnsSafetyLimitExitAndReceiptPath(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := writeRunSessionConfig(t, projectRoot, strings.Join([]string{
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
	loopResult := &agent.TurnResult{FinalText: "stopped by limit"}
	loopResult.TotalUsage.InputTokens = 6
	loopResult.TotalUsage.OutputTokens = 5
	deps := stubRunSessionDeps(
		&rtpkg.EngineRuntime{BrainBackend: backend, Logger: slog.Default(), Cleanup: func() {}},
		tool.NewRegistry(),
		appconfig.BrainConfig{Enabled: true, BrainWritePaths: []string{"receipts/coder/**"}},
		&conversation.Conversation{ID: "conv-1"},
		&fakeRunSessionLoop{result: loopResult},
	)

	result, err := RunSession(context.Background(), nil, configPath, RunRequest{Role: "coder", Task: "work", ChainID: "chain-2", Timeout: time.Minute}, deps)
	if err != nil {
		t.Fatalf("RunSession returned error: %v", err)
	}
	if result == nil || result.ExitCode != ExitSafetyLimit {
		t.Fatalf("result=%#v, want safety-limit exit 2", result)
	}
	if !strings.Contains(backend.docs[result.ReceiptPath], "verdict: safety_limit") {
		t.Fatalf("receipt content = %q, want safety_limit verdict", backend.docs[result.ReceiptPath])
	}
}

func TestRunSessionReturnsEscalationExitWhenReceiptSaysEscalate(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := writeRunSessionConfig(t, projectRoot, strings.Join([]string{
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
	deps := stubRunSessionDeps(
		&rtpkg.EngineRuntime{BrainBackend: backend, Logger: slog.Default(), Cleanup: func() {}},
		tool.NewRegistry(),
		appconfig.BrainConfig{Enabled: true, BrainWritePaths: []string{"receipts/coder/**"}},
		&conversation.Conversation{ID: "conv-1"},
		&fakeRunSessionLoop{result: &agent.TurnResult{FinalText: "done"}},
	)

	result, err := RunSession(context.Background(), nil, configPath, RunRequest{Role: "coder", Task: "work", ChainID: "chain-3", Timeout: time.Minute}, deps)
	if err != nil {
		t.Fatalf("RunSession returned error: %v", err)
	}
	if result == nil || result.ExitCode != ExitEscalation {
		t.Fatalf("result=%#v, want escalation exit 3", result)
	}
}

func TestRunSessionReturnsSafetyLimitOnDeadlineCancellation(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := writeRunSessionConfig(t, projectRoot, strings.Join([]string{
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
	backend := &fakeReceiptBackend{docs: map[string]string{}}
	deps := stubRunSessionDeps(
		&rtpkg.EngineRuntime{BrainBackend: backend, Logger: slog.Default(), Cleanup: func() {}},
		tool.NewRegistry(),
		appconfig.BrainConfig{Enabled: true, BrainWritePaths: []string{"receipts/coder/**"}},
		&conversation.Conversation{ID: "conv-1"},
		&fakeRunSessionLoop{err: agent.ErrTurnCancelled},
	)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	result, err := RunSession(ctx, nil, configPath, RunRequest{Role: "coder", Task: "work", ChainID: "chain-4", Timeout: time.Minute}, deps)
	if err != nil {
		t.Fatalf("RunSession returned error: %v", err)
	}
	if result == nil || result.ExitCode != ExitSafetyLimit {
		t.Fatalf("result=%#v, want safety-limit exit 2", result)
	}
	if !strings.Contains(backend.docs[result.ReceiptPath], "verdict: safety_limit") {
		t.Fatalf("receipt content = %q, want safety_limit verdict", backend.docs[result.ReceiptPath])
	}
}

func TestDefaultDepsUsesFallbackChainIDGenerator(t *testing.T) {
	deps := withDefaultDeps(Deps{})
	if deps.NewChainID == nil {
		t.Fatal("expected default chain ID generator")
	}
	if strings.TrimSpace(deps.NewChainID()) == "" {
		t.Fatal("expected generated chain ID")
	}
}

func TestProgressSinkFactoryAcceptsWriter(t *testing.T) {
	deps := withDefaultDeps(Deps{})
	sink := deps.NewProgressSink(os.Stderr)
	if sink == nil {
		t.Fatal("expected non-nil progress sink")
	}
}

func TestResolveRunLimitsNeverLoosensRoleLimits(t *testing.T) {
	cfg := &appconfig.Config{Agent: appconfig.AgentConfig{MaxIterationsPerTurn: 50}}
	roleCfg := appconfig.AgentRoleConfig{MaxTurns: 20, MaxTokens: 200}

	gotTurns, gotTokens := resolveRunLimits(cfg, roleCfg, RunRequest{MaxTurns: 100, MaxTokens: 1000})
	if gotTurns != 20 || gotTokens != 200 {
		t.Fatalf("resolveRunLimits() = (%d, %d), want role caps (20, 200)", gotTurns, gotTokens)
	}

	gotTurns, gotTokens = resolveRunLimits(cfg, roleCfg, RunRequest{MaxTurns: 10, MaxTokens: 100})
	if gotTurns != 10 || gotTokens != 100 {
		t.Fatalf("resolveRunLimits() = (%d, %d), want tightened CLI caps (10, 100)", gotTurns, gotTokens)
	}

	gotTurns, gotTokens = resolveRunLimits(cfg, appconfig.AgentRoleConfig{}, RunRequest{MaxTurns: 75, MaxTokens: 300})
	if gotTurns != 50 || gotTokens != 300 {
		t.Fatalf("resolveRunLimits() = (%d, %d), want global turn cap and CLI token cap (50, 300)", gotTurns, gotTokens)
	}
}

func TestResolveRunTimeoutUsesRoleCapAndAllowsTightening(t *testing.T) {
	roleCfg := appconfig.AgentRoleConfig{Timeout: appconfig.Duration(20 * time.Minute)}

	if got := resolveRunTimeout(roleCfg, 45*time.Minute); got != 20*time.Minute {
		t.Fatalf("resolveRunTimeout() = %s, want role cap 20m", got)
	}
	if got := resolveRunTimeout(roleCfg, 5*time.Minute); got != 5*time.Minute {
		t.Fatalf("resolveRunTimeout() = %s, want tightened CLI cap 5m", got)
	}
	if got := resolveRunTimeout(appconfig.AgentRoleConfig{}, 0); got != defaultRunTimeout {
		t.Fatalf("resolveRunTimeout() = %s, want default %s", got, defaultRunTimeout)
	}
}

func TestRunSessionWrapsConversationCreationFailure(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := writeRunSessionConfig(t, projectRoot, strings.Join([]string{
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
	deps := stubRunSessionDeps(
		&rtpkg.EngineRuntime{Logger: slog.Default(), Cleanup: func() {}},
		tool.NewRegistry(),
		appconfig.BrainConfig{},
		nil,
		&fakeRunSessionLoop{},
	)
	deps.CreateConversation = func(ctx context.Context, manager *conversation.Manager, projectRoot string, opts ...conversation.CreateOption) (*conversation.Conversation, error) {
		return nil, fmt.Errorf("boom")
	}

	_, err := RunSession(context.Background(), nil, configPath, RunRequest{Role: "coder", Task: "work", ChainID: "chain-5", Timeout: time.Minute}, deps)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "create conversation: boom") {
		t.Fatalf("error = %v, want wrapped conversation creation error", err)
	}
}
