package initializer

import (
	"strings"
	"testing"
)

func TestEmbeddedTemplatesContainsYardYaml(t *testing.T) {
	content, err := readEmbeddedFile("templates/init/yard.yaml.example")
	if err != nil {
		t.Fatalf("readEmbeddedFile: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "{{PROJECT_ROOT}}") {
		t.Fatalf("expected embedded yard.yaml.example to contain {{PROJECT_ROOT}} placeholder")
	}
	if !strings.Contains(text, "agent_roles:") {
		t.Fatalf("expected embedded yard.yaml.example to contain agent_roles section")
	}
	if !strings.Contains(text, "orchestrator:") {
		t.Fatalf("expected embedded yard.yaml.example to contain orchestrator role")
	}
	if !strings.Contains(text, "builtin:orchestrator") {
		t.Fatalf("expected embedded yard.yaml.example to contain builtin orchestrator prompt marker")
	}
	if strings.Contains(text, "{{SODORYARD_AGENTS_DIR}}") {
		t.Fatalf("expected embedded yard.yaml.example to avoid {{SODORYARD_AGENTS_DIR}} placeholder")
	}
	for _, want := range []string{"yard index --config yard.yaml", "yard chain start --config yard.yaml"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected embedded yard.yaml.example to contain %q", want)
		}
	}
	for _, stale := range []string{"tidmouth index", "sirtopham chain"} {
		if strings.Contains(text, stale) {
			t.Fatalf("expected embedded yard.yaml.example to avoid stale command %q", stale)
		}
	}
}
