package role

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ponchione/sodoryard/internal/codeintel"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/tool"
	"github.com/ponchione/sodoryard/internal/toolgroup"
)

type fakeSemanticSearcher struct{}

func (fakeSemanticSearcher) Search(ctx context.Context, queries []string, opts codeintel.SearchOptions) ([]codeintel.SearchResult, error) {
	return nil, nil
}

type fakeCustomTool struct{ name string }

func (f fakeCustomTool) Name() string            { return f.name }
func (f fakeCustomTool) Description() string     { return "fake custom tool" }
func (f fakeCustomTool) ToolPurity() tool.Purity { return tool.Mutating }
func (f fakeCustomTool) Execute(context.Context, string, json.RawMessage) (*tool.ToolResult, error) {
	return &tool.ToolResult{Success: true, Content: "ok"}, nil
}
func (f fakeCustomTool) Schema() json.RawMessage {
	return json.RawMessage(`{"name":"` + f.name + `","description":"fake","input_schema":{"type":"object","properties":{}}}`)
}

func TestKnownToolGroupsHaveRegistrars(t *testing.T) {
	for _, name := range toolgroup.Names() {
		if _, ok := toolGroupRegistrars[name]; !ok {
			t.Fatalf("tool group %q is known to config but has no role registrar", name)
		}
	}
	if len(toolGroupRegistrars) != len(toolgroup.Names()) {
		t.Fatalf("registrar count = %d, known group count = %d", len(toolGroupRegistrars), len(toolgroup.Names()))
	}
}

func TestBuildRegistryIncludesOnlyRequestedToolGroups(t *testing.T) {
	cfg := &appconfig.Config{}
	cfg.Agent.ShellTimeoutSeconds = 45
	cfg.Agent.ShellDenylist = []string{"rm -rf /"}

	registry, _, err := BuildRegistry(cfg, appconfig.AgentRoleConfig{Tools: []string{"file", "git"}}, BuilderDeps{})
	if err != nil {
		t.Fatalf("BuildRegistry returned error: %v", err)
	}

	got := registry.Names()
	want := []string{"file_edit", "file_read", "file_write", "git_diff", "git_status"}
	if len(got) != len(want) {
		t.Fatalf("Names() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names()[%d] = %q, want %q (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestBuildRegistryFileReadGroupRegistersOnlyReadTool(t *testing.T) {
	cfg := &appconfig.Config{}

	registry, _, err := BuildRegistry(cfg, appconfig.AgentRoleConfig{Tools: []string{"file:read"}}, BuilderDeps{})
	if err != nil {
		t.Fatalf("BuildRegistry returned error: %v", err)
	}

	got := registry.Names()
	want := []string{"file_read"}
	if len(got) != len(want) {
		t.Fatalf("Names() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names()[%d] = %q, want %q (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestBuildRegistryFileReadRegistersReadOnlyFileTools(t *testing.T) {
	cfg := &appconfig.Config{}

	registry, _, err := BuildRegistry(cfg, appconfig.AgentRoleConfig{Tools: []string{"file:read", "git"}}, BuilderDeps{})
	if err != nil {
		t.Fatalf("BuildRegistry returned error: %v", err)
	}

	got := registry.Names()
	want := []string{"file_read", "git_diff", "git_status"}
	if len(got) != len(want) {
		t.Fatalf("Names() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names()[%d] = %q, want %q (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestBuildRegistryBrainToolsCarryScopedBrainPolicy(t *testing.T) {
	cfg := &appconfig.Config{}
	cfg.Brain = appconfig.BrainConfig{Enabled: true}

	registry, scopedBrainCfg, err := BuildRegistry(cfg, appconfig.AgentRoleConfig{
		Tools:           []string{"brain"},
		BrainWritePaths: []string{"receipts/**"},
		BrainDenyPaths:  []string{"receipts/private/**"},
	}, BuilderDeps{ProjectID: "/tmp/project"})
	if err != nil {
		t.Fatalf("BuildRegistry returned error: %v", err)
	}

	if got := registry.Names(); len(got) != 5 {
		t.Fatalf("Names() len = %d, want 5 (%v)", len(got), got)
	}
	if len(scopedBrainCfg.BrainWritePaths) != 1 || scopedBrainCfg.BrainWritePaths[0] != "receipts/**" {
		t.Fatalf("BrainWritePaths = %#v, want [receipts/**]", scopedBrainCfg.BrainWritePaths)
	}
	if len(scopedBrainCfg.BrainDenyPaths) != 1 || scopedBrainCfg.BrainDenyPaths[0] != "receipts/private/**" {
		t.Fatalf("BrainDenyPaths = %#v, want [receipts/private/**]", scopedBrainCfg.BrainDenyPaths)
	}
	if len(cfg.Brain.BrainWritePaths) != 0 || len(cfg.Brain.BrainDenyPaths) != 0 {
		t.Fatalf("base config brain policy was mutated: %#v", cfg.Brain)
	}
}

func TestBuildRegistryOmitsBrainToolsWhenBrainDisabled(t *testing.T) {
	cfg := &appconfig.Config{}
	cfg.Brain = appconfig.BrainConfig{Enabled: false}

	registry, scopedBrainCfg, err := BuildRegistry(cfg, appconfig.AgentRoleConfig{
		Tools:           []string{"brain"},
		BrainWritePaths: []string{"receipts/**"},
		BrainDenyPaths:  []string{"receipts/private/**"},
	}, BuilderDeps{ProjectID: "/tmp/project"})
	if err != nil {
		t.Fatalf("BuildRegistry returned error: %v", err)
	}
	if got := registry.Names(); len(got) != 0 {
		t.Fatalf("Names() len = %d, want 0 when brain disabled (%v)", len(got), got)
	}
	if len(scopedBrainCfg.BrainWritePaths) != 1 || scopedBrainCfg.BrainWritePaths[0] != "receipts/**" {
		t.Fatalf("BrainWritePaths = %#v, want [receipts/**]", scopedBrainCfg.BrainWritePaths)
	}
	if len(scopedBrainCfg.BrainDenyPaths) != 1 || scopedBrainCfg.BrainDenyPaths[0] != "receipts/private/**" {
		t.Fatalf("BrainDenyPaths = %#v, want [receipts/private/**]", scopedBrainCfg.BrainDenyPaths)
	}
}

func TestBuildRegistryRejectsCustomToolsWithoutFactory(t *testing.T) {
	cfg := &appconfig.Config{}
	_, _, err := BuildRegistry(cfg, appconfig.AgentRoleConfig{CustomTools: []string{"external.runner"}}, BuilderDeps{})
	if err == nil {
		t.Fatal("expected error for custom_tools, got nil")
	}
}

func TestBuildRegistryRegistersCustomToolsFromFactory(t *testing.T) {
	cfg := &appconfig.Config{}
	registry, _, err := BuildRegistry(cfg, appconfig.AgentRoleConfig{CustomTools: []string{"spawn_agent"}}, BuilderDeps{CustomToolFactory: map[string]func() tool.Tool{"spawn_agent": func() tool.Tool { return fakeCustomTool{name: "spawn_agent"} }}})
	if err != nil {
		t.Fatalf("BuildRegistry returned error: %v", err)
	}
	got := registry.Names()
	if len(got) != 1 || got[0] != "spawn_agent" {
		t.Fatalf("Names() = %v, want [spawn_agent]", got)
	}
}

func TestBuildRegistryRejectsMissingFactoryTool(t *testing.T) {
	cfg := &appconfig.Config{}
	_, _, err := BuildRegistry(cfg, appconfig.AgentRoleConfig{CustomTools: []string{"spawn_agent"}}, BuilderDeps{CustomToolFactory: map[string]func() tool.Tool{}})
	if err == nil {
		t.Fatal("expected missing factory tool error, got nil")
	}
}

func TestBuildRegistryIgnoresUnusedFactoryWhenNoCustomTools(t *testing.T) {
	cfg := &appconfig.Config{}
	registry, _, err := BuildRegistry(cfg, appconfig.AgentRoleConfig{Tools: []string{"search"}}, BuilderDeps{SemanticSearcher: fakeSemanticSearcher{}, CustomToolFactory: map[string]func() tool.Tool{"spawn_agent": func() tool.Tool { return fakeCustomTool{name: "spawn_agent"} }}})
	if err != nil {
		t.Fatalf("BuildRegistry returned error: %v", err)
	}
	got := registry.Names()
	want := []string{"search_semantic", "search_text"}
	if len(got) != len(want) {
		t.Fatalf("Names() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names()[%d] = %q, want %q (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestBuildRegistrySearchGroupRegistersSemanticAndTextTools(t *testing.T) {
	cfg := &appconfig.Config{}
	registry, _, err := BuildRegistry(cfg, appconfig.AgentRoleConfig{Tools: []string{"search"}}, BuilderDeps{SemanticSearcher: fakeSemanticSearcher{}})
	if err != nil {
		t.Fatalf("BuildRegistry returned error: %v", err)
	}

	got := registry.Names()
	want := []string{"search_semantic", "search_text"}
	if len(got) != len(want) {
		t.Fatalf("Names() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names()[%d] = %q, want %q (all=%v)", i, got[i], want[i], got)
		}
	}
}
