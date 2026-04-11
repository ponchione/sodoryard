package role

import (
	"context"
	"testing"

	"github.com/ponchione/sodoryard/internal/codeintel"
	appconfig "github.com/ponchione/sodoryard/internal/config"
)

type fakeSemanticSearcher struct{}

func (fakeSemanticSearcher) Search(ctx context.Context, queries []string, opts codeintel.SearchOptions) ([]codeintel.SearchResult, error) {
	return nil, nil
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

func TestBuildRegistryRejectsCustomTools(t *testing.T) {
	cfg := &appconfig.Config{}
	_, _, err := BuildRegistry(cfg, appconfig.AgentRoleConfig{CustomTools: []string{"external.runner"}}, BuilderDeps{})
	if err == nil {
		t.Fatal("expected error for custom_tools, got nil")
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
