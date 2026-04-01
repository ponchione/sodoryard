package tool

import (
	"bytes"
	"context"
	"encoding/json"
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

// Default exclude patterns for project search.
var defaultExcludes = []string{".git", "vendor", "node_modules", ".venv", "__pycache__", ".idea", ".vscode"}

func (SearchText) Name() string        { return "search_text" }
func (SearchText) Description() string { return "Search for text patterns across project files using ripgrep" }
func (SearchText) ToolPurity() Purity  { return Pure }

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
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Invalid input: %v", err),
			Error:   err.Error(),
		}, nil
	}

	if params.Pattern == "" {
		return &ToolResult{
			Success: false,
			Content: "pattern is required",
			Error:   "empty pattern",
		}, nil
	}

	// Check if rg is available.
	rgPath, err := exec.LookPath("rg")
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

	// Build rg command.
	args := []string{
		"--json",
		fmt.Sprintf("--max-count=%d", maxResults),
	}
	if contextLines > 0 {
		args = append(args, fmt.Sprintf("-C%d", contextLines))
	}

	// Apply default excludes.
	for _, excl := range defaultExcludes {
		args = append(args, "--glob", fmt.Sprintf("!%s/", excl))
	}

	// Apply file glob filter.
	if params.FileGlob != "" {
		args = append(args, "--glob", params.FileGlob)
	}

	args = append(args, "--", params.Pattern)

	// Determine search directory — default to project root ("."), or
	// scope to a validated subdirectory if params.Path is set.
	searchDir := "."
	if params.Path != "" {
		if _, err := resolvePath(projectRoot, params.Path); err != nil {
			return &ToolResult{
				Success: false,
				Content: err.Error(),
				Error:   err.Error(),
			}, nil
		}
		searchDir = params.Path
	}
	args = append(args, searchDir)

	cmd := exec.CommandContext(ctx, rgPath, args...)
	cmd.Dir = projectRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	// rg exits with code 1 for "no matches" — that's not an error.
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return &ToolResult{
					Success: true,
					Content: fmt.Sprintf("No matches found for pattern: '%s'", params.Pattern),
				}, nil
			}
			// Exit code 2+ means actual error.
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("ripgrep error: %s", stderr.String()),
				Error:   err.Error(),
			}, nil
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to run ripgrep: %v", err),
			Error:   err.Error(),
		}, nil
	}

	// Parse JSON output and format.
	formatted := formatRipgrepJSON(stdout.Bytes(), params.Pattern)
	return &ToolResult{
		Success: true,
		Content: formatted,
	}, nil
}

// ripgrep JSON types for parsing --json output.
type rgMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type rgMatch struct {
	Path       rgPath       `json:"path"`
	Lines      rgText       `json:"lines"`
	LineNumber int          `json:"line_number"`
	Submatches []rgSubmatch `json:"submatches"`
}

type rgContext struct {
	Path       rgPath `json:"path"`
	Lines      rgText `json:"lines"`
	LineNumber int    `json:"line_number"`
}

type rgPath struct {
	Text string `json:"text"`
}

type rgText struct {
	Text string `json:"text"`
}

type rgSubmatch struct {
	Match rgText `json:"match"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// formatRipgrepJSON parses rg --json output into human-readable format.
func formatRipgrepJSON(data []byte, pattern string) string {
	lines := bytes.Split(data, []byte("\n"))

	var sb strings.Builder
	currentFile := ""
	matchCount := 0

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		var msg rgMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "match":
			var m rgMatch
			if json.Unmarshal(msg.Data, &m) != nil {
				continue
			}
			// Print file header if new file.
			if m.Path.Text != currentFile {
				if currentFile != "" {
					sb.WriteString("\n")
				}
				currentFile = m.Path.Text
				sb.WriteString(fmt.Sprintf("%s\n", currentFile))
			}
			matchCount++
			text := strings.TrimRight(m.Lines.Text, "\n\r")
			sb.WriteString(fmt.Sprintf("> %4d  %s\n", m.LineNumber, text))

		case "context":
			var c rgContext
			if json.Unmarshal(msg.Data, &c) != nil {
				continue
			}
			if c.Path.Text != currentFile {
				if currentFile != "" {
					sb.WriteString("\n")
				}
				currentFile = c.Path.Text
				sb.WriteString(fmt.Sprintf("%s\n", currentFile))
			}
			text := strings.TrimRight(c.Lines.Text, "\n\r")
			sb.WriteString(fmt.Sprintf("  %4d  %s\n", c.LineNumber, text))

		case "summary":
			// Skip summary line.
		}
	}

	if matchCount == 0 {
		return fmt.Sprintf("No matches found for pattern: '%s'", pattern)
	}

	sb.WriteString(fmt.Sprintf("\n(%d matches)", matchCount))
	return sb.String()
}
