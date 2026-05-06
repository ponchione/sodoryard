package cmdutil

import (
	"bytes"
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
