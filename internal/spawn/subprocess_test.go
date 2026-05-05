package spawn

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunCommandSuccessAndCapture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell test")
	}
	var stdout, stderr bytes.Buffer
	var pid int
	res := RunCommand(context.Background(), RunCommandInput{
		Name:    "/bin/sh",
		Args:    []string{"-c", "printf ok; printf warn >&2"},
		Stdout:  &stdout,
		Stderr:  &stderr,
		Timeout: 5 * time.Second,
		OnStart: func(startedPID int) {
			pid = startedPID
		},
	})
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("result = %+v", res)
	}
	if pid <= 0 {
		t.Fatalf("pid = %d, want process pid", pid)
	}
	if stdout.String() != "ok" || stderr.String() != "warn" {
		t.Fatalf("stdout/stderr = %q/%q", stdout.String(), stderr.String())
	}
}

func TestRunCommandMergesEnvOverridesWithParentEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell test")
	}
	t.Setenv("SODORYARD_PARENT_ENV_TEST", "parent")

	var stdout bytes.Buffer
	res := RunCommand(context.Background(), RunCommandInput{
		Name:    "/bin/sh",
		Args:    []string{"-c", "printf '%s:%s' \"$SODORYARD_PARENT_ENV_TEST\" \"$SODORYARD_CHILD_ENV_TEST\""},
		Stdout:  &stdout,
		Env:     []string{"SODORYARD_CHILD_ENV_TEST=child"},
		Timeout: 5 * time.Second,
	})
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("result = %+v", res)
	}
	if got := stdout.String(); got != "parent:child" {
		t.Fatalf("stdout = %q, want parent:child", got)
	}
}

func TestRunCommandNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell test")
	}
	res := RunCommand(context.Background(), RunCommandInput{Name: "/bin/sh", Args: []string{"-c", "exit 7"}, Timeout: 5 * time.Second})
	if res.Err != nil || res.ExitCode != 7 {
		t.Fatalf("result = %+v, want exit 7", res)
	}
}

func TestRunCommandTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell test")
	}
	res := RunCommand(context.Background(), RunCommandInput{Name: "/bin/sh", Args: []string{"-c", "sleep 5"}, Timeout: 100 * time.Millisecond, TerminateGracePeriod: 100 * time.Millisecond})
	if res.ExitCode == 0 {
		t.Fatalf("result = %+v, want non-zero exit", res)
	}
}

func TestRunCommandEmitsStdoutAndStderrLines(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell test")
	}
	var stdout, stderr bytes.Buffer
	var stdoutLines, stderrLines []string
	res := RunCommand(context.Background(), RunCommandInput{
		Name:    "/bin/sh",
		Args:    []string{"-c", "printf 'one\ntwo\n'; printf 'warn1\nwarn2\n' >&2"},
		Stdout:  &stdout,
		Stderr:  &stderr,
		Timeout: 5 * time.Second,
		OnStdoutLine: func(line string) {
			stdoutLines = append(stdoutLines, line)
		},
		OnStderrLine: func(line string) {
			stderrLines = append(stderrLines, line)
		},
	})
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("result = %+v", res)
	}
	if got, want := stdout.String(), "one\ntwo\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "warn1\nwarn2\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if got, want := strings.Join(stdoutLines, "|"), "one|two"; got != want {
		t.Fatalf("stdout lines = %q, want %q", got, want)
	}
	if got, want := strings.Join(stderrLines, "|"), "warn1|warn2"; got != want {
		t.Fatalf("stderr lines = %q, want %q", got, want)
	}
}

func TestLineEmitterCapsPendingLineWithoutNewline(t *testing.T) {
	var lines []string
	emitter := &lineEmitter{onLine: func(line string) {
		lines = append(lines, line)
	}}

	payload := strings.Repeat("x", maxLineEmitterPendingBytes+10)
	n, err := emitter.Write([]byte(payload))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("Write count = %d, want %d", n, len(payload))
	}
	if len(emitter.pending) != 0 {
		t.Fatalf("pending bytes = %d, want 0", len(emitter.pending))
	}
	if len(lines) != 1 {
		t.Fatalf("emitted lines = %d, want 1", len(lines))
	}
	if !strings.Contains(lines[0], "line truncated") {
		t.Fatalf("emitted line = %q, want truncation marker", lines[0])
	}
}
