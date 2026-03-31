package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestShellSuccess(t *testing.T) {
	s := NewShell(ShellConfig{})
	result, err := s.Execute(context.Background(), t.TempDir(),
		json.RawMessage(`{"command":"echo hello world"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Exit code: 0") {
		t.Fatalf("expected exit code 0, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "hello world") {
		t.Fatalf("expected 'hello world' in output, got:\n%s", result.Content)
	}
}

func TestShellNonZeroExitCode(t *testing.T) {
	s := NewShell(ShellConfig{})
	result, err := s.Execute(context.Background(), t.TempDir(),
		json.RawMessage(`{"command":"sh -c 'echo error >&2; exit 1'"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Non-zero exit codes are still Success=true.
	if !result.Success {
		t.Fatalf("expected success=true for non-zero exit, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Exit code: 1") {
		t.Fatalf("expected exit code 1, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "STDERR") {
		t.Fatalf("expected STDERR section, got:\n%s", result.Content)
	}
}

func TestShellTimeout(t *testing.T) {
	s := NewShell(ShellConfig{})
	result, err := s.Execute(context.Background(), t.TempDir(),
		json.RawMessage(`{"command":"sleep 30","timeout_seconds":1}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success=true even on timeout, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "timed out") {
		t.Fatalf("expected timeout message, got:\n%s", result.Content)
	}
}

func TestShellDenylist(t *testing.T) {
	s := NewShell(ShellConfig{
		Denylist: []string{"rm -rf /", "git push --force"},
	})

	tests := []struct {
		cmd     string
		blocked bool
	}{
		{"rm -rf /", true},
		{"git push --force origin main", true},
		{"echo hello", false},
		{"rm file.txt", false},
	}

	for _, tt := range tests {
		input, _ := json.Marshal(shellInput{Command: tt.cmd})
		result, err := s.Execute(context.Background(), t.TempDir(), input)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", tt.cmd, err)
		}
		if tt.blocked && result.Success {
			t.Fatalf("expected blocked for %q, but got success", tt.cmd)
		}
		if tt.blocked && !strings.Contains(result.Content, "denylist") {
			t.Fatalf("expected denylist message for %q, got: %s", tt.cmd, result.Content)
		}
		if !tt.blocked && !result.Success {
			t.Fatalf("expected success for %q, got: %s", tt.cmd, result.Content)
		}
	}
}

func TestShellCancellation(t *testing.T) {
	s := NewShell(ShellConfig{})
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := s.Execute(ctx, t.TempDir(),
		json.RawMessage(`{"command":"sleep 30"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should still be success=true (infra didn't fail).
	if !result.Success {
		t.Fatalf("expected success=true on cancel, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "cancelled") {
		t.Fatalf("expected cancelled message, got:\n%s", result.Content)
	}
}

func TestShellWorkingDir(t *testing.T) {
	dir := t.TempDir()
	s := NewShell(ShellConfig{})

	// pwd should return the project root.
	result, err := s.Execute(context.Background(), dir,
		json.RawMessage(`{"command":"pwd"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, dir) {
		t.Fatalf("expected working dir %s in output, got:\n%s", dir, result.Content)
	}
}

func TestShellWorkingDirTraversal(t *testing.T) {
	s := NewShell(ShellConfig{})
	result, err := s.Execute(context.Background(), t.TempDir(),
		json.RawMessage(`{"command":"echo hi","working_dir":"../../etc"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for path traversal")
	}
	if !strings.Contains(result.Content, "escapes project root") {
		t.Fatalf("expected path traversal error, got: %s", result.Content)
	}
}

func TestShellEmptyCommand(t *testing.T) {
	s := NewShell(ShellConfig{})
	result, err := s.Execute(context.Background(), t.TempDir(),
		json.RawMessage(`{"command":""}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for empty command")
	}
}

func TestShellSchema(t *testing.T) {
	s := NewShell(ShellConfig{})
	schema := s.Schema()
	if !json.Valid(schema) {
		t.Fatal("Schema() is not valid JSON")
	}
	if !strings.Contains(string(schema), "shell") {
		t.Fatal("Schema() does not contain tool name")
	}
}

func TestRegisterShellTool(t *testing.T) {
	reg := NewRegistry()
	RegisterShellTool(reg, ShellConfig{})
	if _, ok := reg.Get("shell"); !ok {
		t.Fatal("shell not registered")
	}
}
