package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestShellDetails(t *testing.T) {
	s := &Shell{config: ShellConfig{}}
	result, err := s.Execute(context.Background(), t.TempDir(),
		json.RawMessage(`{"command":"printf out; printf err >&2; exit 7"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success=true for non-zero exit, got: %s", result.Content)
	}

	details := decodeToolResultDetails(t, result.Details)
	if details["kind"] != "shell" {
		t.Fatalf("kind = %#v, want shell", details["kind"])
	}
	if details["command"] != "printf out; printf err >&2; exit 7" {
		t.Fatalf("command = %#v", details["command"])
	}
	if details["working_dir"] != "." {
		t.Fatalf("working_dir = %#v, want .", details["working_dir"])
	}
	if got := detailInt(t, details, "exit_code"); got != 7 {
		t.Fatalf("exit_code = %d, want 7", got)
	}
	if got := detailInt(t, details, "stdout_bytes"); got != 3 {
		t.Fatalf("stdout_bytes = %d, want 3", got)
	}
	if got := detailInt(t, details, "stderr_bytes"); got != 3 {
		t.Fatalf("stderr_bytes = %d, want 3", got)
	}
	if got := detailInt(t, details, "output_bytes"); got != len(result.Content) {
		t.Fatalf("output_bytes = %d, want %d", got, len(result.Content))
	}
	if details["timed_out"] != false || details["cancelled"] != false {
		t.Fatalf("timeout/cancel details = %#v/%#v, want false/false", details["timed_out"], details["cancelled"])
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
		{"sh -c 'git push --force origin main'", true},
		{"echo safe && git push --force origin main", true},
		{"true; git push --force origin main", true},
		{"false || git push --force origin main", true},
		{"echo hello", false},
		{"rm file.txt", false},
		{"printf 'git push --force'", false},
		{"echo rm -rf /", false},
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

func TestShellTokenize(t *testing.T) {
	tokens := shellTokenize(`sh -c 'git push --force origin main'`)
	joined := strings.Join(tokens, "|")
	if joined != "sh|-c|git push --force origin main" {
		t.Fatalf("tokens = %q", joined)
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

func TestShellRTKPrefix(t *testing.T) {
	got := applyRTKPrefix("git status", "/usr/local/bin/rtk")
	want := "'/usr/local/bin/rtk' git status"
	if got != want {
		t.Fatalf("applyRTKPrefix(%q, %q) = %q; want %q", "git status", "/usr/local/bin/rtk", got, want)
	}
}

func TestShellRTKPrefixQuotesResolvedPath(t *testing.T) {
	got := applyRTKPrefix("git status", "/tmp/fake tools/fake rtk")
	want := "'/tmp/fake tools/fake rtk' git status"
	if got != want {
		t.Fatalf("applyRTKPrefix quoted path = %q; want %q", got, want)
	}
}

func TestShellRTKPrefixQuotesSingleQuotes(t *testing.T) {
	got := applyRTKPrefix("git status", "/tmp/fake 'tools'/rtk")
	want := `'/tmp/fake '\''tools'\''/rtk' git status`
	if got != want {
		t.Fatalf("applyRTKPrefix single-quote path = %q; want %q", got, want)
	}
}

func TestShellRTKPrefixSkipsWhenUnavailable(t *testing.T) {
	got := applyRTKPrefix("git status", "")
	want := "git status"
	if got != want {
		t.Fatalf("applyRTKPrefix(%q, empty path) = %q; want %q", "git status", got, want)
	}
}

func TestShellRTKPrefixSkipsRTKCommands(t *testing.T) {
	got := applyRTKPrefix("rtk git status", "/usr/local/bin/rtk")
	want := "rtk git status"
	if got != want {
		t.Fatalf("applyRTKPrefix(%q, path) = %q; want %q", "rtk git status", got, want)
	}
}

func TestShellRTKPrefixSkipsShellBuiltins(t *testing.T) {
	got := applyRTKPrefix("cd /tmp && ls", "/usr/local/bin/rtk")
	want := "cd /tmp && ls"
	if got != want {
		t.Fatalf("applyRTKPrefix(%q, path) = %q; want %q", "cd /tmp && ls", got, want)
	}
}

func TestShellRTKPrefixSkipsExport(t *testing.T) {
	got := applyRTKPrefix("export FOO=bar", "/usr/local/bin/rtk")
	want := "export FOO=bar"
	if got != want {
		t.Fatalf("applyRTKPrefix(%q, path) = %q; want %q", "export FOO=bar", got, want)
	}
}

func TestShellRTKPrefixSkipsSource(t *testing.T) {
	got := applyRTKPrefix("source .env", "/usr/local/bin/rtk")
	want := "source .env"
	if got != want {
		t.Fatalf("applyRTKPrefix(%q, path) = %q; want %q", "source .env", got, want)
	}
}

func TestShellRTKPrefixSkipsBareBuiltins(t *testing.T) {
	cases := []struct {
		cmd  string
		want string
	}{
		{"cd", "cd"},
		{"export", "export"},
		{"eval", "eval"},
		{"source", "source"},
	}
	for _, tc := range cases {
		got := applyRTKPrefix(tc.cmd, "/usr/local/bin/rtk")
		if got != tc.want {
			t.Fatalf("applyRTKPrefix(%q, path) = %q; want %q", tc.cmd, got, tc.want)
		}
	}
}

func TestDetectRTKPath(t *testing.T) {
	dir := t.TempDir()
	rtkPath := filepath.Join(dir, "rtk")
	if err := os.WriteFile(rtkPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake rtk: %v", err)
	}
	t.Setenv("PATH", dir)

	got := detectRTKPath()
	if got != rtkPath {
		t.Fatalf("detectRTKPath() = %q; want %q", got, rtkPath)
	}
}

func TestDetectRTKPathUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	got := detectRTKPath()
	if got != "" {
		t.Fatalf("detectRTKPath() = %q; want empty path", got)
	}
}
