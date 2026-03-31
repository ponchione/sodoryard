package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	defaultShellTimeout = 120 * time.Second
	killGracePeriod     = 5 * time.Second
)

// ShellConfig configures shell tool behavior.
type ShellConfig struct {
	TimeoutSeconds int
	Denylist       []string
}

// Shell implements the shell tool — arbitrary command execution with safety
// guardrails, timeout management, and process group lifecycle control.
type Shell struct {
	config ShellConfig
}

// NewShell creates a shell tool with the given configuration.
func NewShell(config ShellConfig) *Shell {
	return &Shell{config: config}
}

type shellInput struct {
	Command        string `json:"command"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty"`
	WorkingDir     string `json:"working_dir,omitempty"`
}

func (s *Shell) Name() string        { return "shell" }
func (s *Shell) Description() string { return "Execute a shell command in the project directory" }
func (s *Shell) ToolPurity() Purity  { return Mutating }

func (s *Shell) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "shell",
		"description": "Execute a shell command. Non-zero exit codes are reported but not treated as errors. Dangerous commands (rm -rf /, git push --force) are blocked. Default timeout: 120s.",
		"input_schema": {
			"type": "object",
			"properties": {
				"command": {
					"type": "string",
					"description": "Shell command to execute (run via sh -c)"
				},
				"timeout_seconds": {
					"type": "integer",
					"description": "Override timeout in seconds (default: 120)"
				},
				"working_dir": {
					"type": "string",
					"description": "Subdirectory within project root to use as working directory"
				}
			},
			"required": ["command"]
		}
	}`)
}

func (s *Shell) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	var params shellInput
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Invalid input: %v", err),
			Error:   err.Error(),
		}, nil
	}

	if params.Command == "" {
		return &ToolResult{
			Success: false,
			Content: "command is required",
			Error:   "empty command",
		}, nil
	}

	// Denylist check.
	for _, pattern := range s.config.Denylist {
		if strings.Contains(params.Command, pattern) {
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("Command rejected by safety denylist: matches pattern '%s'. This is a safeguard against catastrophic mistakes.", pattern),
				Error:   "denylist match",
			}, nil
		}
	}

	// Resolve working directory.
	workDir := projectRoot
	if params.WorkingDir != "" {
		resolved, err := resolvePath(projectRoot, params.WorkingDir)
		if err != nil {
			return &ToolResult{
				Success: false,
				Content: err.Error(),
				Error:   err.Error(),
			}, nil
		}
		workDir = resolved
	}

	// Determine timeout.
	timeout := defaultShellTimeout
	if s.config.TimeoutSeconds > 0 {
		timeout = time.Duration(s.config.TimeoutSeconds) * time.Second
	}
	if params.TimeoutSeconds != nil && *params.TimeoutSeconds > 0 {
		timeout = time.Duration(*params.TimeoutSeconds) * time.Second
	}

	// Create command with timeout context.
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.Command("sh", "-c", params.Command)
	cmd.Dir = workDir

	// Set process group for cleanup.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Start()
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to start command: %v", err),
			Error:   err.Error(),
		}, nil
	}

	// Wait for completion.
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	var waitErr error
	select {
	case waitErr = <-waitDone:
		// Process finished normally.
	case <-cmdCtx.Done():
		// Timeout or parent cancellation — kill process group.
		if cmd.Process != nil {
			pgid, pgErr := syscall.Getpgid(cmd.Process.Pid)
			if pgErr == nil {
				syscall.Kill(-pgid, syscall.SIGTERM)
			}
			// Give grace period then SIGKILL.
			timer := time.NewTimer(killGracePeriod)
			select {
			case waitErr = <-waitDone:
				timer.Stop()
			case <-timer.C:
				if pgErr == nil {
					syscall.Kill(-pgid, syscall.SIGKILL)
				}
				waitErr = <-waitDone
			}
		}
	}

	exitCode := 0
	timedOut := cmdCtx.Err() == context.DeadlineExceeded
	cancelled := ctx.Err() == context.Canceled

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	// Format output.
	content := formatShellOutput(exitCode, stdout.String(), stderr.String(), timedOut, cancelled, timeout)

	// Non-zero exit codes are NOT failures from the tool's perspective.
	// Only infrastructure issues (can't start process) are failures.
	return &ToolResult{
		Success: true,
		Content: content,
	}, nil
}

func formatShellOutput(exitCode int, stdout, stderr string, timedOut, cancelled bool, timeout time.Duration) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Exit code: %d\n", exitCode))

	if strings.TrimSpace(stdout) != "" {
		sb.WriteString(fmt.Sprintf("\nSTDOUT:\n%s\n", strings.TrimRight(stdout, "\n")))
	}

	if strings.TrimSpace(stderr) != "" {
		sb.WriteString(fmt.Sprintf("\nSTDERR:\n%s\n", strings.TrimRight(stderr, "\n")))
	}

	if timedOut {
		sb.WriteString(fmt.Sprintf("\n[Command timed out after %ds. Process killed. Partial output above.]", int(timeout.Seconds())))
	} else if cancelled {
		sb.WriteString("\n[Command cancelled by user. Partial output above.]")
	}

	return sb.String()
}
