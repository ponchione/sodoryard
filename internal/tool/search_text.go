package tool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
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
var defaultExcludes = []string{".git", ".yard", ".brain", ".obsidian", "vendor", "node_modules", ".venv", "__pycache__", ".idea", ".vscode"}

var hiddenStateSearchExcludes = map[string]struct{}{
	".git":       {},
	".yard":      {},
	".brain":     {},
	".obsidian":  {},
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
	for _, excl := range defaultExcludes {
		args = append(args, "--glob", fmt.Sprintf("!%s/", excl))
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

func isHiddenStateSearchPath(path string) bool {
	cleaned := filepath.Clean(path)
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if _, excluded := hiddenStateSearchExcludes[part]; excluded {
			return true
		}
	}
	return false
}

func formatRipgrepStream(r io.Reader, pattern string, maxResults int, stop func()) (formatted string, matchCount int, stoppedEarly bool, err error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var sb strings.Builder
	currentFile := ""
	printedCurrentFileHeader := false

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg rgMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "match":
			if maxResults > 0 && matchCount >= maxResults {
				if stop != nil {
					stop()
				}
				return finalizeRipgrepFormat(&sb, pattern, matchCount), matchCount, true, nil
			}
			var m rgMatch
			if json.Unmarshal(msg.Data, &m) != nil {
				continue
			}
			if m.Path.Text != currentFile {
				if currentFile != "" {
					sb.WriteString("\n")
				}
				currentFile = m.Path.Text
				printedCurrentFileHeader = true
				sb.WriteString(fmt.Sprintf("%s\n", currentFile))
			} else if !printedCurrentFileHeader {
				printedCurrentFileHeader = true
				sb.WriteString(fmt.Sprintf("%s\n", currentFile))
			}
			matchCount++
			text := strings.TrimRight(m.Lines.Text, "\n\r")
			sb.WriteString(fmt.Sprintf("> %4d  %s\n", m.LineNumber, text))
			if maxResults > 0 && matchCount >= maxResults {
				if stop != nil {
					stop()
				}
				return finalizeRipgrepFormat(&sb, pattern, matchCount), matchCount, true, nil
			}

		case "context":
			if maxResults > 0 && matchCount >= maxResults {
				continue
			}
			var c rgContext
			if json.Unmarshal(msg.Data, &c) != nil {
				continue
			}
			if c.Path.Text != currentFile {
				if currentFile != "" {
					sb.WriteString("\n")
				}
				currentFile = c.Path.Text
				printedCurrentFileHeader = true
				sb.WriteString(fmt.Sprintf("%s\n", currentFile))
			} else if !printedCurrentFileHeader {
				printedCurrentFileHeader = true
				sb.WriteString(fmt.Sprintf("%s\n", currentFile))
			}
			text := strings.TrimRight(c.Lines.Text, "\n\r")
			sb.WriteString(fmt.Sprintf("  %4d  %s\n", c.LineNumber, text))
		}
	}

	if err := scanner.Err(); err != nil {
		return "", matchCount, stoppedEarly, err
	}
	return finalizeRipgrepFormat(&sb, pattern, matchCount), matchCount, false, nil
}

func finalizeRipgrepFormat(sb *strings.Builder, pattern string, matchCount int) string {
	if matchCount == 0 {
		return fmt.Sprintf("No matches found for pattern: '%s'", pattern)
	}
	sb.WriteString(fmt.Sprintf("\n(%d matches)", matchCount))
	return sb.String()
}
