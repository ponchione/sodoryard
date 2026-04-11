package spawn

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"
)

type RunCommandInput struct {
	Name                 string
	Args                 []string
	Stdin                io.Reader
	Stdout               io.Writer
	Stderr               io.Writer
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
	cmd.Stdout = in.Stdout
	cmd.Stderr = in.Stderr
	cmd.Env = in.Env
	cmd.Dir = in.Dir
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = in.TerminateGracePeriod
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return RunResult{ExitCode: exitErr.ExitCode()}
		}
		return RunResult{ExitCode: -1, Err: fmt.Errorf("run command: %w", err)}
	}
	return RunResult{ExitCode: 0}
}
