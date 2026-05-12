package spawn

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const maxLineEmitterPendingBytes = 64 * 1024

type RunCommandInput struct {
	Name                 string
	Args                 []string
	Stdin                io.Reader
	Stdout               io.Writer
	Stderr               io.Writer
	OnStdoutLine         func(string)
	OnStderrLine         func(string)
	OnStart              func(pid int)
	Env                  []string
	Dir                  string
	Timeout              time.Duration
	TerminateGracePeriod time.Duration
}

type RunResult struct {
	ExitCode int
	Err      error
}

func RunCommand(ctx context.Context, in RunCommandInput) RunResult {
	if in.TerminateGracePeriod <= 0 {
		in.TerminateGracePeriod = 10 * time.Second
	}
	if in.Timeout <= 0 {
		in.Timeout = 30 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, in.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, in.Name, in.Args...)
	cmd.Stdin = in.Stdin
	stdout, flushStdout := composeOutputWriter(in.Stdout, in.OnStdoutLine)
	stderr, flushStderr := composeOutputWriter(in.Stderr, in.OnStderrLine)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if len(in.Env) > 0 {
		cmd.Env = append(os.Environ(), in.Env...)
	}
	cmd.Dir = in.Dir
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = in.TerminateGracePeriod
	if err := cmd.Start(); err != nil {
		return RunResult{ExitCode: -1, Err: fmt.Errorf("start command: %w", err)}
	}
	if in.OnStart != nil && cmd.Process != nil {
		in.OnStart(cmd.Process.Pid)
	}
	err := cmd.Wait()
	flushStdout()
	flushStderr()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return RunResult{ExitCode: exitErr.ExitCode()}
		}
		return RunResult{ExitCode: -1, Err: fmt.Errorf("run command: %w", err)}
	}
	return RunResult{ExitCode: 0}
}

func composeOutputWriter(base io.Writer, onLine func(string)) (io.Writer, func()) {
	if onLine == nil {
		return base, func() {}
	}
	lineWriter := &lineEmitter{onLine: onLine}
	if base == nil {
		return lineWriter, lineWriter.Flush
	}
	return io.MultiWriter(base, lineWriter), lineWriter.Flush
}

type lineEmitter struct {
	pending []byte
	onLine  func(string)
}

func (w *lineEmitter) Write(p []byte) (int, error) {
	remaining := append(w.pending, p...)
	for {
		idx := -1
		for i, b := range remaining {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		line := string(remaining[:idx])
		if w.onLine != nil {
			w.onLine(line)
		}
		remaining = remaining[idx+1:]
	}
	if len(remaining) > maxLineEmitterPendingBytes {
		if w.onLine != nil {
			w.onLine(string(remaining[:maxLineEmitterPendingBytes]) + " [line truncated]")
		}
		remaining = remaining[:0]
	}
	w.pending = append(w.pending[:0], remaining...)
	return len(p), nil
}

func (w *lineEmitter) Flush() {
	if len(w.pending) == 0 {
		return
	}
	if w.onLine != nil {
		w.onLine(string(w.pending))
	}
	w.pending = w.pending[:0]
}
