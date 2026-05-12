package contractgen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratedProjectMemoryBindingsAreCurrent(t *testing.T) {
	contractJSON, bindings, err := Generate(DefaultTypeScriptRuntimeImport)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	assertGeneratedFile(t, filepath.Join("..", "..", "..", "web", "src", "generated", "yard-project-memory.contract.json"), contractJSON)
	assertGeneratedFile(t, filepath.Join("..", "..", "..", "web", "src", "generated", "yard-project-memory.ts"), bindings)
}

func assertGeneratedFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated file %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("generated file %s is stale; run make projectmemory-bindings", path)
	}
}
