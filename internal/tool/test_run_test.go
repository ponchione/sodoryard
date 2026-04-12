package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireGo skips the test if `go` is not available in PATH.
func requireGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not found in PATH, skipping Go test_run tests")
	}
}

// setupGoModule creates a minimal Go module in dir.
func setupGoModule(t *testing.T, dir, moduleName string) {
	t.Helper()
	gomod := fmt.Sprintf("module %s\n\ngo 1.21\n", moduleName)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
}

// writeGoFile writes content to path inside dir.
func writeGoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// --- Integration tests ---

func TestTestRunGoPassingTests(t *testing.T) {
	requireGo(t)
	dir := t.TempDir()
	setupGoModule(t, dir, "example.com/passing")

	writeGoFile(t, dir, "add.go", `package passing

func Add(a, b int) int { return a + b }
`)
	writeGoFile(t, dir, "add_test.go", `package passing

import "testing"

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("Add(1,2) != 3")
	}
}

func TestAddZero(t *testing.T) {
	if Add(0, 0) != 0 {
		t.Fatal("Add(0,0) != 0")
	}
}
`)

	tool := TestRun{}
	result, err := tool.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "GO PASS") {
		t.Errorf("expected GO PASS in output, got:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "FAILURES") {
		t.Errorf("expected no FAILURES section for passing tests, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "2 passed") {
		t.Errorf("expected '2 passed' in output, got:\n%s", result.Content)
	}
}

func TestTestRunGoFailingTest(t *testing.T) {
	requireGo(t)
	dir := t.TempDir()
	setupGoModule(t, dir, "example.com/failing")

	writeGoFile(t, dir, "math.go", `package failing

func Double(n int) int { return n * 2 }
`)
	writeGoFile(t, dir, "math_test.go", `package failing

import "testing"

func TestDoublePass(t *testing.T) {
	if Double(3) != 6 {
		t.Fatal("Double(3) should be 6")
	}
}

func TestDoubleWrong(t *testing.T) {
	// Intentionally wrong expectation.
	if Double(2) != 999 {
		t.Errorf("got %d, want 999", Double(2))
	}
}
`)

	tool := TestRun{}
	result, err := tool.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		// tool.Success=true even on test failures (infra didn't fail)
		t.Fatalf("expected Success=true even when tests fail, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "GO FAIL") {
		t.Errorf("expected GO FAIL in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "FAILURES") {
		t.Errorf("expected FAILURES section, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "TestDoubleWrong") {
		t.Errorf("expected TestDoubleWrong in failures, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "1 passed") {
		t.Errorf("expected '1 passed' in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "1 failed") {
		t.Errorf("expected '1 failed' in output, got:\n%s", result.Content)
	}
}

func TestTestRunEcosystemDetection(t *testing.T) {
	t.Run("go_mod", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/x\ngo 1.21\n"), 0o644)
		eco := detectTestEcosystem(dir, dir)
		if eco != "go" {
			t.Errorf("expected 'go', got %q", eco)
		}
	})

	t.Run("pyproject_toml", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[tool.pytest]\n"), 0o644)
		eco := detectTestEcosystem(dir, dir)
		if eco != "python" {
			t.Errorf("expected 'python', got %q", eco)
		}
	})

	t.Run("package_json", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test"}`), 0o644)
		eco := detectTestEcosystem(dir, dir)
		if eco != "typescript" {
			t.Errorf("expected 'typescript', got %q", eco)
		}
	})

	t.Run("walk_up", func(t *testing.T) {
		root := t.TempDir()
		sub := filepath.Join(root, "subpkg")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		// Put go.mod at root, detect from subdir.
		os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/x\ngo 1.21\n"), 0o644)
		eco := detectTestEcosystem(sub, root)
		if eco != "go" {
			t.Errorf("expected 'go' (walk up), got %q", eco)
		}
	})

	t.Run("none", func(t *testing.T) {
		dir := t.TempDir()
		eco := detectTestEcosystem(dir, dir)
		if eco != "" {
			t.Errorf("expected empty, got %q", eco)
		}
	})
}

func TestTestRunSchema(t *testing.T) {
	schema := TestRun{}.Schema()
	if !json.Valid(schema) {
		t.Fatal("Schema() is not valid JSON")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("failed to parse schema: %v", err)
	}
	if parsed["name"] != "test_run" {
		t.Errorf("expected name 'test_run', got %v", parsed["name"])
	}
}

func TestTestRunPythonStub(t *testing.T) {
	dir := t.TempDir()
	// Add a pyproject.toml so ecosystem detection finds python.
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[tool.pytest]\n"), 0o644)

	tool := TestRun{}
	result, err := tool.Execute(context.Background(), dir, json.RawMessage(`{"ecosystem":"python"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for python stub")
	}
	if !strings.Contains(result.Content, "Python test runner not yet available") {
		t.Errorf("expected stub message, got: %s", result.Content)
	}
}

func TestTestRunTypescriptStub(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test"}`), 0o644)

	tool := TestRun{}
	result, err := tool.Execute(context.Background(), dir, json.RawMessage(`{"ecosystem":"typescript"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for typescript stub")
	}
	if !strings.Contains(result.Content, "TypeScript test runner not yet available") {
		t.Errorf("expected stub message, got: %s", result.Content)
	}
}

func TestTestRunNoEcosystem(t *testing.T) {
	dir := t.TempDir() // no marker files
	tool := TestRun{}
	result, err := tool.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure when no ecosystem detected")
	}
	if !strings.Contains(result.Content, "detect") {
		t.Errorf("expected detection error message, got: %s", result.Content)
	}
}
