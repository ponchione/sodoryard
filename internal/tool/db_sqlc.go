package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const sqlcTimeout = 60 * time.Second

// DbSqlc implements the db_sqlc tool - runs sqlc generate/vet/diff with
// structured error output.
type DbSqlc struct{}

type dbSqlcInput struct {
	Action string `json:"action,omitempty"`
	Path   string `json:"path,omitempty"`
}

func (DbSqlc) Name() string        { return "db_sqlc" }
func (DbSqlc) Description() string { return "Run sqlc generate, vet, or diff in the project directory" }
func (DbSqlc) ToolPurity() Purity  { return Mutating }

func (DbSqlc) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "db_sqlc",
		"description": "Run sqlc generate, vet, or diff. Requires a sqlc.yaml/sqlc.yml/sqlc.json config in the working directory.",
		"input_schema": {
			"type": "object",
			"properties": {
				"action": {
					"type": "string",
					"enum": ["generate", "vet", "diff"],
					"description": "sqlc action to run (default: generate)"
				},
				"path": {
					"type": "string",
					"description": "Subdirectory within the project root containing sqlc.yaml"
				}
			}
		}
	}`)
}

func (DbSqlc) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	var params dbSqlcInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &params); err != nil {
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("Invalid input: %v", err),
				Error:   err.Error(),
			}, nil
		}
	}

	// Default action.
	action := params.Action
	if action == "" {
		action = "generate"
	}

	// Validate action.
	switch action {
	case "generate", "vet", "diff":
		// valid
	default:
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Invalid action %q: must be one of generate, vet, diff", action),
			Error:   "invalid_action",
		}, nil
	}

	// Resolve working directory.
	workDir := projectRoot
	if params.Path != "" {
		resolved, err := resolvePath(projectRoot, params.Path)
		if err != nil {
			return &ToolResult{
				Success: false,
				Content: err.Error(),
				Error:   err.Error(),
			}, nil
		}
		workDir = resolved
	}

	// Check for sqlc config file.
	configFound := fileExists(workDir+"/sqlc.yaml") ||
		fileExists(workDir+"/sqlc.yml") ||
		fileExists(workDir+"/sqlc.json")
	if !configFound {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("No sqlc configuration file found in %s. Create a sqlc.yaml, sqlc.yml, or sqlc.json before running sqlc.", workDir),
			Error:   "missing_sqlc_config",
		}, nil
	}

	// Look up sqlc binary after local validation so missing configs and unsafe
	// paths still produce the most actionable error when sqlc is unavailable.
	sqlcPath, err := lookupCommandPath("sqlc")
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: "sqlc is required but not found in PATH",
			Error:   "sqlc not found",
		}, nil
	}

	// Run sqlc with timeout.
	cmdCtx, cancel := context.WithTimeout(ctx, sqlcTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, sqlcPath, action)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	combined := strings.TrimRight(stdout.String()+stderr.String(), "\n")

	switch action {
	case "generate":
		if runErr == nil {
			content := "sqlc generate: success"
			if out := strings.TrimSpace(stdout.String()); out != "" {
				content += "\n" + out
			}
			return &ToolResult{Success: true, Content: content}, nil
		}
		return &ToolResult{
			Success: false,
			Content: "sqlc generate failed:\n" + combined,
			Error:   runErr.Error(),
		}, nil

	case "vet":
		if runErr == nil {
			return &ToolResult{Success: true, Content: "sqlc vet: no issues found"}, nil
		}
		return &ToolResult{
			Success: false,
			Content: "sqlc vet found issues:\n" + combined,
			Error:   runErr.Error(),
		}, nil

	case "diff":
		if runErr == nil {
			// Successful exit - check whether output is empty (in sync) or not.
			out := strings.TrimSpace(stdout.String())
			if out == "" {
				return &ToolResult{Success: true, Content: "sqlc diff: everything in sync"}, nil
			}
			return &ToolResult{Success: true, Content: "sqlc diff:\n" + out}, nil
		}
		// Non-zero exit from diff means differences were found - not a tool failure.
		return &ToolResult{
			Success: true,
			Content: "sqlc diff: out of sync\n" + combined,
		}, nil
	}

	// Unreachable.
	return &ToolResult{Success: false, Content: "unknown action"}, nil
}
