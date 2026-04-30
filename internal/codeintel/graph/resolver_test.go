package graph

import (
	"path/filepath"
	"testing"
)

func TestResolver_DetectsGo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\ngo 1.21\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")

	cfg := DefaultAnalyzerConfig()
	cfg.TypeScript.Enabled = false
	cfg.Python.Enabled = false

	resolver := NewResolver(dir, &cfg)
	result, err := resolver.Analyze()
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if len(result.Symbols) == 0 {
		t.Error("expected Go symbols, got none")
	}

	foundMain := false
	for _, s := range result.Symbols {
		if s.Name == "main" && s.Language == "go" {
			foundMain = true
		}
	}
	if !foundMain {
		t.Error("missing main function symbol")
	}
}

func TestResolver_DetectsPython(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.py"), "def hello():\n    return \"hi\"\n")

	cfg := DefaultAnalyzerConfig()
	cfg.Go.Enabled = false
	cfg.TypeScript.Enabled = false

	resolver := NewResolver(dir, &cfg)
	result, err := resolver.Analyze()
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if len(result.Symbols) == 0 {
		t.Error("expected Python symbols, got none")
	}
}

func TestResolver_AppliesIndexRulesToPythonGraph(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".brain", "ignored.py"), "def ignored():\n    return 'nope'\n")

	cfg := DefaultAnalyzerConfig()
	cfg.Go.Enabled = false
	cfg.TypeScript.Enabled = false

	resolver := NewResolverWithIndexRules(dir, &cfg, []string{"**/*.py"}, []string{"**/.brain/**"})
	result, err := resolver.Analyze()
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(result.Symbols) != 0 {
		t.Fatalf("symbols = %d, want 0 for excluded Python file", len(result.Symbols))
	}
}

func TestFilterAnalysisResultDropsExcludedSymbolsAndEdges(t *testing.T) {
	input := &AnalysisResult{
		Symbols: []Symbol{
			{ID: "keep", FilePath: "src/keep.ts"},
			{ID: "drop", FilePath: "dist/drop.ts"},
		},
		Edges: []Edge{
			{SourceID: "keep", TargetID: "drop"},
			{SourceID: "keep", TargetID: "external"},
			{SourceID: "drop", TargetID: "keep"},
		},
		BoundarySymbols: []BoundarySymbol{{ID: "external"}},
	}

	got := filterAnalysisResult(input, func(relPath string) bool {
		return relPath != "dist/drop.ts"
	})
	if len(got.Symbols) != 1 || got.Symbols[0].ID != "keep" {
		t.Fatalf("symbols = %#v, want only keep", got.Symbols)
	}
	if len(got.Edges) != 1 || got.Edges[0].TargetID != "external" {
		t.Fatalf("edges = %#v, want only edge to external boundary", got.Edges)
	}
}

func TestResolver_EmptyProject(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultAnalyzerConfig()
	resolver := NewResolver(dir, &cfg)
	result, err := resolver.Analyze()
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(result.Symbols) != 0 {
		t.Errorf("expected 0 symbols, got %d", len(result.Symbols))
	}
}

func TestResolver_DisabledAnalyzer(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\ngo 1.21\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")

	cfg := DefaultAnalyzerConfig()
	cfg.Go.Enabled = false
	cfg.TypeScript.Enabled = false
	cfg.Python.Enabled = false

	resolver := NewResolver(dir, &cfg)
	result, err := resolver.Analyze()
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(result.Symbols) != 0 {
		t.Errorf("expected 0 with all disabled, got %d", len(result.Symbols))
	}
}
