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
