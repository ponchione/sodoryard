package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/ponchione/sodoryard/internal/pathglob"
)

const maxFindFilesResults = 1000

// FindFiles implements the find_files tool — glob-based file discovery with
// recursive ** matching support.
type FindFiles struct{}

type findFilesInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	MaxResults *int   `json:"max_results,omitempty"`
}

func (FindFiles) Name() string        { return "find_files" }
func (FindFiles) Description() string { return "Find files matching a glob pattern in the project" }
func (FindFiles) ToolPurity() Purity  { return Pure }

func (FindFiles) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "find_files",
		"description": "Find files matching a glob pattern (supports ** for recursive matching). Returns relative file paths.",
		"input_schema": {
			"type": "object",
			"properties": {
				"pattern": {
					"type": "string",
					"description": "Glob pattern to match (e.g., '*.go', '**/*_test.py', 'Makefile')"
				},
				"path": {
					"type": "string",
					"description": "Optional subdirectory to scope the search to (relative to project root)"
				},
				"max_results": {
					"type": "integer",
					"description": "Maximum number of results to return (default: 100, max: 1000)"
				}
			},
			"required": ["pattern"]
		}
	}`)
}

func (FindFiles) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	var params findFilesInput
	if err := json.Unmarshal(input, &params); err != nil {
		return invalidInputResult(err), nil
	}

	if params.Pattern == "" {
		return requiredFieldResult("pattern"), nil
	}

	maxResults := boundedPositiveInt(params.MaxResults, 100, maxFindFilesResults)

	// Determine the search root.
	searchRoot := projectRoot
	pathPrefix := ""
	if params.Path != "" {
		resolved, result := resolvePathResult(projectRoot, params.Path)
		if result != nil {
			return result, nil
		}
		searchRoot = resolved
		pathPrefix = filepath.Clean(params.Path)
	}

	var results []string

	err := filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		if d.IsDir() {
			name := d.Name()
			if isDefaultExcludedDir(name) {
				return filepath.SkipDir
			}
			// Skip hidden dirs (but not the walk root itself).
			if path != searchRoot && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if len(results) >= maxResults {
			return filepath.SkipAll
		}

		// Compute the relative path from searchRoot.
		rel, err := filepath.Rel(searchRoot, path)
		if err != nil {
			return nil
		}
		// Use forward slashes for consistent glob matching.
		relSlash := filepath.ToSlash(rel)

		if matchesFindPattern(params.Pattern, relSlash, d.Name()) {
			if pathPrefix != "" {
				results = append(results, filepath.ToSlash(filepath.Join(pathPrefix, rel)))
			} else {
				results = append(results, relSlash)
			}
		}
		return nil
	})
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to walk directory: %v", err),
			Error:   err.Error(),
		}, nil
	}

	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Content: fmt.Sprintf("No files found matching pattern: '%s'", params.Pattern),
		}, nil
	}

	var sb strings.Builder
	for _, r := range results {
		sb.WriteString(r)
		sb.WriteByte('\n')
	}
	sb.WriteString(fmt.Sprintf("\n(%d files)", len(results)))

	return &ToolResult{Success: true, Content: sb.String()}, nil
}

func matchesFindPattern(pattern, relPath, name string) bool {
	if strings.Contains(pattern, "/") || strings.Contains(pattern, "**") {
		return pathglob.Match(pattern, relPath)
	}
	matched, _ := filepath.Match(pattern, name)
	return matched
}
