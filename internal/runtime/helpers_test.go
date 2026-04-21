package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/ponchione/sodoryard/internal/config"
)

func TestLoadRoleSystemPromptSupportsFileOverrideAndBuiltIns(t *testing.T) {
	projectRoot := t.TempDir()
	promptDir := filepath.Join(projectRoot, "prompts")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	promptPath := filepath.Join(promptDir, "coder.md")
	if err := os.WriteFile(promptPath, []byte("custom coder prompt"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	got, source, err := LoadRoleSystemPrompt("coder", projectRoot, "prompts/coder.md")
	if err != nil {
		t.Fatalf("LoadRoleSystemPrompt(file override) returned error: %v", err)
	}
	if got != "custom coder prompt" {
		t.Fatalf("LoadRoleSystemPrompt(file override) = %q, want custom prompt", got)
	}
	if source != "file:"+promptPath {
		t.Fatalf("LoadRoleSystemPrompt(file override) source = %q, want %q", source, "file:"+promptPath)
	}

	builtin, builtinSource, err := LoadRoleSystemPrompt("coder", projectRoot, "builtin:coder")
	if err != nil {
		t.Fatalf("LoadRoleSystemPrompt(explicit builtin) returned error: %v", err)
	}
	if builtin == "" {
		t.Fatal("LoadRoleSystemPrompt(explicit builtin) returned empty prompt")
	}
	if builtinSource != "embedded:coder" {
		t.Fatalf("LoadRoleSystemPrompt(explicit builtin) source = %q, want embedded:coder", builtinSource)
	}

	defaultBuiltin, defaultSource, err := LoadRoleSystemPrompt("coder", projectRoot, "")
	if err != nil {
		t.Fatalf("LoadRoleSystemPrompt(default builtin) returned error: %v", err)
	}
	if defaultBuiltin == "" {
		t.Fatal("LoadRoleSystemPrompt(default builtin) returned empty prompt")
	}
	if defaultSource != "embedded:coder" {
		t.Fatalf("LoadRoleSystemPrompt(default builtin) source = %q, want embedded:coder", defaultSource)
	}
}

func TestLoadRoleSystemPromptRejectsMissingOverridesAndUnknownBuiltIns(t *testing.T) {
	projectRoot := t.TempDir()

	if _, _, err := LoadRoleSystemPrompt("coder", projectRoot, "prompts/missing.md"); err == nil || !strings.Contains(err.Error(), "missing role system prompt override") {
		t.Fatalf("LoadRoleSystemPrompt(missing override) error = %v, want missing override error", err)
	}

	if _, _, err := LoadRoleSystemPrompt("coder", projectRoot, "builtin:not-a-role"); err == nil || !strings.Contains(err.Error(), "unknown built-in role system prompt") {
		t.Fatalf("LoadRoleSystemPrompt(unknown builtin) error = %v, want unknown builtin error", err)
	}

	if _, _, err := LoadRoleSystemPrompt("custom-role", projectRoot, ""); err == nil || !strings.Contains(err.Error(), "no built-in role system prompt") {
		t.Fatalf("LoadRoleSystemPrompt(empty custom role) error = %v, want no built-in error", err)
	}
}

func TestResolveModelContextLimitUsesConfiguredAndFallbackValues(t *testing.T) {
	cfg := &appconfig.Config{Providers: map[string]appconfig.ProviderConfig{
		"codex": {Type: "codex", ContextLength: 111},
		"local": {Type: "openai-compatible"},
	}}
	if got, err := ResolveModelContextLimit(cfg, "codex"); err != nil || got != 111 {
		t.Fatalf("ResolveModelContextLimit(codex) = (%d, %v), want (111, nil)", got, err)
	}
	if got, err := ResolveModelContextLimit(cfg, "local"); err != nil || got != 32768 {
		t.Fatalf("ResolveModelContextLimit(local) = (%d, %v), want (32768, nil)", got, err)
	}
}
