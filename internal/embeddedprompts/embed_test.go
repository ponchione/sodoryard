package embeddedprompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/promptmeta"
)

func TestKeysIncludesAllBuiltInRoles(t *testing.T) {
	want := []string{
		"coder",
		"correctness-auditor",
		"docs-arbiter",
		"epic-decomposer",
		"integration-auditor",
		"orchestrator",
		"performance-auditor",
		"planner",
		"quality-auditor",
		"resolver",
		"security-auditor",
		"task-decomposer",
		"test-writer",
	}
	got := Keys()
	if len(got) != len(want) {
		t.Fatalf("Keys() returned %d keys, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("Keys()[%d] = %q, want %q (all=%v)", i, got[i], w, got)
		}
		if !Has(w) {
			t.Fatalf("Has(%q) = false, want true", w)
		}
		content, ok := Get(w)
		if !ok {
			t.Fatalf("Get(%q) ok = false, want true", w)
		}
		if content == "" {
			t.Fatalf("Get(%q) returned empty content", w)
		}
	}
}

func TestGetUnknownRoleReturnsFalse(t *testing.T) {
	if got, ok := Get("not-a-role"); ok || got != "" {
		t.Fatalf("Get(unknown) = (%q, %t), want (\"\", false)", got, ok)
	}
	if Has("not-a-role") {
		t.Fatal("Has(unknown) = true, want false")
	}
}

func TestPersonaAliasesIncludePromptPersona(t *testing.T) {
	aliases := PersonaAliases("coder")
	if !strings.Contains(strings.Join(aliases, ","), "Thomas") {
		t.Fatalf("PersonaAliases(coder) = %v, want Thomas", aliases)
	}
}

func TestEmbeddedPromptsMatchRepoRootAgents(t *testing.T) {
	for role, filename := range roleToAsset {
		embedded, ok := Get(role)
		if !ok {
			t.Fatalf("Get(%q) ok = false", role)
		}
		repoPath := filepath.Join("..", "..", "agents", filename)
		data, err := os.ReadFile(repoPath)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", repoPath, err)
		}
		if embedded != string(data) {
			t.Fatalf("embedded prompt %q does not match %s", role, repoPath)
		}
	}
}

func TestEmbeddedPromptsUseRuntimeToolNamesAndCleanMarkdown(t *testing.T) {
	for role := range roleToAsset {
		content, ok := Get(role)
		if !ok {
			t.Fatalf("Get(%q) ok = false", role)
		}
		parsed := promptmeta.Parse(content)
		body := strings.TrimSpace(parsed.Body)
		if !strings.HasPrefix(body, "# ") {
			t.Fatalf("prompt %q body starts with %q, want markdown heading", role, body[:min(len(body), 20)])
		}
		if strings.Contains(content, "spawn_engine") {
			t.Fatalf("prompt %q references obsolete spawn_engine tool name", role)
		}
	}
}

func TestEmbeddedPromptsCarryRoleMetadata(t *testing.T) {
	type wantMetadata struct {
		persona                    string
		expectedTools              []string
		recommendedMaxTurns        int
		requiresStructuredFindings bool
	}
	want := map[string]wantMetadata{
		"orchestrator":        {persona: "Sir Topham Hatt", expectedTools: []string{"brain"}, recommendedMaxTurns: 50},
		"planner":             {persona: "Gordon", expectedTools: []string{"brain", "search"}, recommendedMaxTurns: 30},
		"epic-decomposer":     {persona: "Edward", expectedTools: []string{"brain"}, recommendedMaxTurns: 20},
		"task-decomposer":     {persona: "Emily", expectedTools: []string{"brain"}, recommendedMaxTurns: 20},
		"coder":               {persona: "Thomas", expectedTools: []string{"brain", "file", "git", "search", "shell"}, recommendedMaxTurns: 100},
		"correctness-auditor": {persona: "Percy", expectedTools: []string{"brain", "file:read", "git"}, recommendedMaxTurns: 30, requiresStructuredFindings: true},
		"quality-auditor":     {persona: "James", expectedTools: []string{"brain", "file:read", "git"}, recommendedMaxTurns: 30, requiresStructuredFindings: true},
		"performance-auditor": {persona: "Spencer", expectedTools: []string{"brain", "file:read", "git"}, recommendedMaxTurns: 20, requiresStructuredFindings: true},
		"security-auditor":    {persona: "Diesel", expectedTools: []string{"brain", "file:read", "git"}, recommendedMaxTurns: 20, requiresStructuredFindings: true},
		"integration-auditor": {persona: "Toby", expectedTools: []string{"brain", "file:read", "git"}, recommendedMaxTurns: 20, requiresStructuredFindings: true},
		"test-writer":         {persona: "Rosie", expectedTools: []string{"brain", "file", "git", "search", "shell"}, recommendedMaxTurns: 50},
		"resolver":            {persona: "Victor", expectedTools: []string{"brain", "file", "git", "search", "shell"}, recommendedMaxTurns: 50},
		"docs-arbiter":        {persona: "Harold", expectedTools: []string{"brain"}, recommendedMaxTurns: 20},
	}

	for role, expected := range want {
		content, ok := Get(role)
		if !ok {
			t.Fatalf("Get(%q) ok = false", role)
		}
		parsed := promptmeta.Parse(content)
		if !parsed.HasFrontmatter {
			t.Fatalf("prompt %q has no metadata frontmatter", role)
		}
		if len(parsed.Warnings) > 0 {
			t.Fatalf("prompt %q metadata warnings = %v", role, parsed.Warnings)
		}
		meta := parsed.Metadata
		if meta.RoleKey != role || meta.Persona != expected.persona || meta.ReceiptSchema != promptmeta.ReceiptSchemaV1 {
			t.Fatalf("prompt %q metadata = %+v, want role/persona/schema", role, meta)
		}
		if !equalStringSlices(meta.ExpectedTools, expected.expectedTools) {
			t.Fatalf("prompt %q expected tools = %v, want %v", role, meta.ExpectedTools, expected.expectedTools)
		}
		if meta.RecommendedMaxTurns != expected.recommendedMaxTurns || meta.RequiresStructuredFindings != expected.requiresStructuredFindings {
			t.Fatalf("prompt %q runtime hints = max_turns %d structured %t, want %d/%t", role, meta.RecommendedMaxTurns, meta.RequiresStructuredFindings, expected.recommendedMaxTurns, expected.requiresStructuredFindings)
		}
	}
}

func equalStringSlices(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
