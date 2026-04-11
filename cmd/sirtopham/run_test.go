package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/agent"
	appconfig "github.com/ponchione/sodoryard/internal/config"
)

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
	got, err := loadRoleSystemPrompt(projectRoot, "agents/coder.md")
	if err != nil {
		t.Fatalf("loadRoleSystemPrompt returned error: %v", err)
	}
	if got != "you are coder" {
		t.Fatalf("loadRoleSystemPrompt = %q, want prompt contents", got)
	}
}

func TestResolveModelContextLimitUsesConfiguredAndFallbackValues(t *testing.T) {
	cfg := &appconfig.Config{Providers: map[string]appconfig.ProviderConfig{
		"codex": {Type: "codex", ContextLength: 111},
		"local": {Type: "openai-compatible"},
	}}
	if got, err := resolveModelContextLimit(cfg, "codex"); err != nil || got != 111 {
		t.Fatalf("resolveModelContextLimit(codex) = (%d, %v), want (111, nil)", got, err)
	}
	if got, err := resolveModelContextLimit(cfg, "local"); err != nil || got != 32768 {
		t.Fatalf("resolveModelContextLimit(local) = (%d, %v), want (32768, nil)", got, err)
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
