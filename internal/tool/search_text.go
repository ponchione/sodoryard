package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// SearchText implements the search_text tool — ripgrep-based text search
// across the project with structured output.
type SearchText struct{}

type searchTextInput struct {
	Pattern      string `json:"pattern"`
	Path         string `json:"path,omitempty"`
	FileGlob     string `json:"file_glob,omitempty"`
	ContextLines *int   `json:"context_lines,omitempty"`
	MaxResults   *int   `json:"max_results,omitempty"`
}

func (SearchText) Name() string { return "search_text" }
func (SearchText) Description() string {
	return "Search for text patterns across project files using ripgrep"
}
func (SearchText) ToolPurity() Purity { return Pure }

func (SearchText) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "search_text",
		"description": "Search for text patterns (string or regex) across project files using ripgrep. Returns matches with file paths, line numbers, and surrounding context.",
		"input_schema": {
			"type": "object",
			"properties": {
				"pattern": {
					"type": "string",
					"description": "Search pattern (string or regex)"
				},
				"path": {
					"type": "string",
					"description": "Optional subdirectory to scope the search to (relative to project root)"
				},
				"file_glob": {
					"type": "string",
					"description": "Optional file glob filter (e.g., '*.go', '*.py')"
				},
				"context_lines": {
					"type": "integer",
					"description": "Number of surrounding context lines (default: 2)"
				},
				"max_results": {
					"type": "integer",
					"description": "Maximum number of matching lines to return (default: 50)"
				}
			},
			"required": ["pattern"]
		}
	}`)
}

func (SearchText) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	var params searchTextInput
	if err := json.Unmarshal(input, &params); err != nil {
		return invalidInputResult(err), nil
	}

	if params.Pattern == "" {
		return requiredFieldResult("pattern"), nil
	}

	rgPath, err := lookupCommandPath("rg")
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: "ripgrep (rg) is required but not found in PATH. Install it: https://github.com/BurntSushi/ripgrep#installation",
			Error:   "rg not found",
		}, nil
	}

	contextLines := 2
	if params.ContextLines != nil {
		contextLines = *params.ContextLines
	}
	maxResults := 50
	if params.MaxResults != nil && *params.MaxResults > 0 {
		maxResults = *params.MaxResults
	}

	args := []string{"--json"}
	if contextLines > 0 {
		args = append(args, fmt.Sprintf("-C%d", contextLines))
	}
	if params.FileGlob != "" {
		args = append(args, "--glob", params.FileGlob)
	}
	for _, glob := range defaultExcludedDirGlobs() {
		args = append(args, "--glob", glob)
	}
	args = append(args, "--", params.Pattern)

	searchDir := "."
	if params.Path != "" {
		if _, err := resolvePath(projectRoot, params.Path); err != nil {
			return &ToolResult{
				Success: false,
				Content: err.Error(),
				Error:   err.Error(),
			}, nil
		}
		if isHiddenStateSearchPath(params.Path) {
			return &ToolResult{Success: true, Content: fmt.Sprintf("No matches found for pattern: '%s'", params.Pattern)}, nil
		}
		searchDir = params.Path
	}
	args = append(args, searchDir)

	searchCtx, stopSearch := context.WithCancel(ctx)
	defer stopSearch()

	cmd := exec.CommandContext(searchCtx, rgPath, args...)
	cmd.Dir = projectRoot

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to set up ripgrep output: %v", err),
			Error:   err.Error(),
		}, nil
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to run ripgrep: %v", err),
			Error:   err.Error(),
		}, nil
	}

	formatted, matches, stoppedEarly, parseErr := formatRipgrepStream(stdout, params.Pattern, maxResults, stopSearch)
	waitErr := cmd.Wait()

	if parseErr != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to parse ripgrep output: %v", parseErr),
			Error:   parseErr.Error(),
		}, nil
	}

	if waitErr != nil {
		if ctx.Err() != nil {
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("ripgrep search cancelled: %v", ctx.Err()),
				Error:   ctx.Err().Error(),
			}, nil
		}
		if stoppedEarly {
			return &ToolResult{Success: true, Content: formatted}, nil
		}
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) && exitErr.ExitCode() == 1 && matches == 0 {
			return &ToolResult{Success: true, Content: fmt.Sprintf("No matches found for pattern: '%s'", params.Pattern)}, nil
		}
		if stderr.Len() > 0 {
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("ripgrep error: %s", strings.TrimSpace(stderr.String())),
				Error:   waitErr.Error(),
			}, nil
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to run ripgrep: %v", waitErr),
			Error:   waitErr.Error(),
		}, nil
	}

	return &ToolResult{Success: true, Content: formatted}, nil
}
