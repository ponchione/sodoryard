package cmdutil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunConfigPrintsResolvedSummary(t *testing.T) {
	configPath := writeLLMConfig(t, strings.Join([]string{
		"  enabled: true",
		"  mode: auto",
		"  compose_file: ops/llm/docker-compose.yml",
		"  project_dir: ops/llm",
	}, "\n"))
	var out bytes.Buffer

	if err := RunConfig(&out, configPath); err != nil {
		t.Fatalf("RunConfig returned error: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"config: valid",
		"config_path: " + configPath,
		"default_provider: codex",
		"default_model: gpt-5.5",
		"default_reasoning_effort: medium",
		"default_context_window: 400000",
		"default_model_capabilities: tools,reasoning_effort",
		"database_path: <unused in shunter mode>",
		"brain_enabled: true",
		"local_services_enabled: true",
		"local_services_mode: auto",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, ".yard/yard.db") {
		t.Fatalf("output = %q, want Shunter mode not to advertise .yard/yard.db", got)
	}
}

func TestRunConfigPrintsPromptMetadataWarnings(t *testing.T) {
	projectRoot := t.TempDir()
	promptDir := filepath.Join(projectRoot, "prompts")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptDir, "coder.md"), []byte("---\nrole_key: reviewer\nexpected_tools: [brain]\nreceipt_schema: other.schema\nrecommended_max_turns: 12\n---\n# Prompt\n"), 0o644); err != nil {
		t.Fatalf("WriteFile prompt returned error: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "yard.yaml")
	config := fmt.Sprintf(`project_root: %q
routing:
  default:
    provider: codex
    model: gpt-5.5
providers:
  codex:
    type: codex
    model: gpt-5.5
local_services:
  enabled: false
  mode: off
agent_roles:
  coder:
    system_prompt: prompts/coder.md
    tools: [file]
    max_turns: 30
`, projectRoot)
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile config returned error: %v", err)
	}
	var out bytes.Buffer

	if err := RunConfig(&out, configPath); err != nil {
		t.Fatalf("RunConfig returned error: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		`prompt_warning: coder file:`,
		`role_key "reviewer" differs from configured role "coder"`,
		`expected_tools [brain] differ from configured tools [file]`,
		`receipt_schema "other.schema" is not yard.receipt.v1`,
		`recommended_max_turns 12 differs from configured max_turns 30`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q in %q", want, got)
		}
	}
}
