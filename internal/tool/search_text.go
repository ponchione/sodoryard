package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ponchione/sodoryard/internal/outputcap"
)

// SearchText implements the search_text tool — ripgrep-based text search
// across the project with structured output.
type SearchText struct{}

type searchTextInput struct {
	Pattern      string   `json:"pattern"`
	Path         string   `json:"path,omitempty"`
	Paths        []string `json:"paths,omitempty"`
	FileGlob     string   `json:"file_glob,omitempty"`
	ContextLines *int     `json:"context_lines,omitempty"`
	MaxResults   *int     `json:"max_results,omitempty"`
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
					"description": "Optional file or subdirectory to scope the search to (relative to project root). For multiple scopes, prefer paths."
				},
				"paths": {
					"type": "array",
					"items": {"type": "string"},
					"description": "Optional list of files or subdirectories to scope the search to (relative to project root)"
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

	searchDirs, skippedPaths, scopeErr := resolveSearchTextScopes(projectRoot, params)
	if scopeErr != nil {
		return scopeErr, nil
	}
	if len(searchDirs) == 0 {
		return &ToolResult{Success: true, Content: withSkippedSearchPaths(fmt.Sprintf("No matches found for pattern: '%s'", params.Pattern), skippedPaths)}, nil
	}
	args = append(args, searchDirs...)

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

	stderr := outputcap.NewBuffer(outputcap.DefaultLimit)
	cmd.Stderr = stderr

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
			return &ToolResult{Success: true, Content: withSkippedSearchPaths(fmt.Sprintf("No matches found for pattern: '%s'", params.Pattern), skippedPaths)}, nil
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

	return &ToolResult{Success: true, Content: withSkippedSearchPaths(formatted, skippedPaths)}, nil
}

func resolveSearchTextScopes(projectRoot string, params searchTextInput) ([]string, []string, *ToolResult) {
	rawScopes := append([]string(nil), params.Paths...)
	if strings.TrimSpace(params.Path) != "" {
		rawScopes = append(rawScopes, params.Path)
	}
	if len(rawScopes) == 0 {
		return []string{"."}, nil, nil
	}

	var scopes []string
	var skipped []string
	seen := map[string]struct{}{}
	for _, raw := range rawScopes {
		candidates, split := expandSearchTextScope(projectRoot, raw)
		for _, candidate := range candidates {
			resolved, err := resolvePath(projectRoot, candidate)
			if err != nil {
				return nil, nil, failureResult(err.Error(), err.Error())
			}
			if isHiddenStateSearchPath(candidate) {
				continue
			}
			if !fileExists(resolved) {
				if split || len(rawScopes) > 1 {
					skipped = append(skipped, candidate)
					continue
				}
				msg := fmt.Sprintf("search path not found: %s", candidate)
				return nil, nil, failureResult(msg, msg)
			}
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			scopes = append(scopes, candidate)
		}
	}
	if len(scopes) == 0 {
		return nil, skipped, nil
	}
	return scopes, skipped, nil
}

func expandSearchTextScope(projectRoot string, raw string) ([]string, bool) {
	scope := strings.TrimSpace(raw)
	if scope == "" {
		return nil, false
	}
	if resolved, err := resolvePath(projectRoot, scope); err == nil && fileExists(resolved) {
		return []string{scope}, false
	}
	fields := strings.Fields(scope)
	if len(fields) > 1 {
		return fields, true
	}
	return []string{scope}, false
}

func withSkippedSearchPaths(content string, skipped []string) string {
	if len(skipped) == 0 {
		return content
	}
	return fmt.Sprintf("Skipped missing search paths: %s\n\n%s", strings.Join(skipped, ", "), content)
}
