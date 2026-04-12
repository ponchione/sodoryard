package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ListDirectory implements the list_directory tool — tree-style directory
// listing with configurable depth, hidden-file control, and standard exclusions.
type ListDirectory struct{}

type listDirectoryInput struct {
	Path          string `json:"path,omitempty"`
	Depth         *int   `json:"depth,omitempty"`
	IncludeHidden bool   `json:"include_hidden,omitempty"`
}

// defaultDirExcludes is the set of directory names always skipped during
// directory listing (and will be shared with the find_files tool).
var defaultDirExcludes = map[string]struct{}{
	".git":        {},
	".yard":       {},
	".sirtopham":  {},
	".sodoryard":  {},
	".brain":      {},
	".obsidian":   {},
	"vendor":      {},
	"node_modules": {},
	".venv":       {},
	"__pycache__": {},
	".idea":       {},
	".vscode":     {},
}

func (ListDirectory) Name() string        { return "list_directory" }
func (ListDirectory) Description() string { return "List directory contents as a tree with configurable depth" }
func (ListDirectory) ToolPurity() Purity  { return Pure }

func (ListDirectory) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "list_directory",
		"description": "List directory contents in a tree-style format with configurable depth. Skips build artifacts and hidden state directories (node_modules, .git, .venv, etc.).",
		"input_schema": {
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "Directory to list, relative to project root (default: \".\")"
				},
				"depth": {
					"type": "integer",
					"description": "Maximum recursion depth, 1-10 (default: 3)"
				},
				"include_hidden": {
					"type": "boolean",
					"description": "Show hidden files/directories (names starting with '.'); default false. Always skips defaultDirExcludes regardless."
				}
			}
		}
	}`)
}

func (ListDirectory) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	var params listDirectoryInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &params); err != nil {
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("Invalid input: %v", err),
				Error:   err.Error(),
			}, nil
		}
	}

	// Resolve path — default to project root.
	targetPath := params.Path
	if targetPath == "" {
		targetPath = "."
	}

	absPath, err := resolvePath(projectRoot, targetPath)
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: err.Error(),
			Error:   err.Error(),
		}, nil
	}

	// Validate target exists and is a directory.
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("Path not found: %s", targetPath),
				Error:   "path not found",
			}, nil
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Cannot access path: %v", err),
			Error:   err.Error(),
		}, nil
	}
	if !info.IsDir() {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Not a directory: %s", targetPath),
			Error:   "not a directory",
		}, nil
	}

	// Determine depth.
	depth := 3
	if params.Depth != nil {
		depth = *params.Depth
		if depth < 1 {
			depth = 1
		}
		if depth > 10 {
			depth = 10
		}
	}

	var sb strings.Builder
	var dirCount, fileCount int

	walkDir(&sb, absPath, 0, depth, params.IncludeHidden, &dirCount, &fileCount)

	sb.WriteString(fmt.Sprintf("(%d directories, %d files)", dirCount, fileCount))

	return &ToolResult{
		Success: true,
		Content: sb.String(),
	}, nil
}

// walkDir recursively walks absDir, writing tree lines to sb.
// currentDepth is the current recursion level (0 = target root).
// maxDepth is the maximum depth to recurse into.
func walkDir(sb *strings.Builder, absDir string, currentDepth, maxDepth int, includeHidden bool, dirCount, fileCount *int) {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return
	}

	// Separate dirs and files, apply filters.
	var dirs, files []os.DirEntry
	for _, e := range entries {
		name := e.Name()

		// Always exclude defaultDirExcludes.
		if e.IsDir() {
			if _, excluded := defaultDirExcludes[name]; excluded {
				continue
			}
		}

		// Skip hidden entries unless include_hidden is set.
		// (defaultDirExcludes are already handled above for dirs;
		//  hidden files starting with '.' are controlled by include_hidden.)
		if strings.HasPrefix(name, ".") && !includeHidden {
			continue
		}

		if e.IsDir() {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}

	// Sort each group alphabetically.
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })

	indent := strings.Repeat("  ", currentDepth)

	// Write directories first.
	for _, d := range dirs {
		*dirCount++
		sb.WriteString(fmt.Sprintf("%s%s/\n", indent, d.Name()))
		if currentDepth < maxDepth {
			walkDir(sb, absDir+"/"+d.Name(), currentDepth+1, maxDepth, includeHidden, dirCount, fileCount)
		}
	}

	// Write files.
	for _, f := range files {
		*fileCount++
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, f.Name()))
	}
}
