package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// GitStatus implements the git_status tool — returns a structured snapshot
// of the repository state (branch, dirty files, recent commits).
type GitStatus struct{}

type gitStatusInput struct {
	RecentCommits *int `json:"recent_commits,omitempty"`
}

func (GitStatus) Name() string        { return "git_status" }
func (GitStatus) Description() string { return "Show current branch, dirty files, and recent commits" }
func (GitStatus) ToolPurity() Purity  { return Pure }

func (GitStatus) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "git_status",
		"description": "Returns the current branch name, dirty files with status codes, and N most recent commit summaries.",
		"input_schema": {
			"type": "object",
			"properties": {
				"recent_commits": {
					"type": "integer",
					"description": "Number of recent commits to show (default: 5)"
				}
			}
		}
	}`)
}

func (GitStatus) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	var params gitStatusInput
	if len(input) > 0 {
		json.Unmarshal(input, &params)
	}

	recentCommits := 5
	if params.RecentCommits != nil && *params.RecentCommits > 0 {
		recentCommits = *params.RecentCommits
	}

	// Check if git is available.
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: "git is required but not found in PATH",
			Error:   "git not found",
		}, nil
	}

	// Get current branch.
	branch, err := runGitCommand(ctx, gitPath, projectRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not a git repository") || strings.Contains(errMsg, "fatal") {
			return &ToolResult{
				Success: false,
				Content: "Not a git repository (or any parent up to filesystem root)",
				Error:   "not a git repo",
			}, nil
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to get branch: %v", err),
			Error:   err.Error(),
		}, nil
	}

	// Get status.
	status, _ := runGitCommand(ctx, gitPath, projectRoot, "status", "--porcelain")

	// Get recent commits.
	commits, _ := runGitCommand(ctx, gitPath, projectRoot, "log", "--oneline", fmt.Sprintf("-%d", recentCommits))

	// Format output.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Branch: %s\n\n", strings.TrimSpace(branch)))

	sb.WriteString("Status:\n")
	if strings.TrimSpace(status) == "" {
		sb.WriteString("Working tree clean\n")
	} else {
		sb.WriteString(strings.TrimRight(status, "\n"))
		sb.WriteString("\n")
	}

	if strings.TrimSpace(commits) != "" {
		sb.WriteString(fmt.Sprintf("\nRecent commits:\n%s\n", strings.TrimRight(commits, "\n")))
	}

	return &ToolResult{
		Success: true,
		Content: sb.String(),
	}, nil
}

// runGitCommand executes a git command and returns its stdout output.
func runGitCommand(ctx context.Context, gitPath, workDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, gitPath, args...)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errOutput := stderr.String()
		if errOutput != "" {
			return "", fmt.Errorf("%s", strings.TrimSpace(errOutput))
		}
		return "", err
	}

	return stdout.String(), nil
}
