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
	res := RunCommand(context.Background(), RunCommandInput{Name: "/bin/sh", Args: []string{"-c", "printf ok; printf warn >&2"}, Stdout: &stdout, Stderr: &stderr, Timeout: 5 * time.Second})
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("result = %+v", res)
	}
	if stdout.String() != "ok" || stderr.String() != "warn" {
		t.Fatalf("stdout/stderr = %q/%q", stdout.String(), stderr.String())
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
