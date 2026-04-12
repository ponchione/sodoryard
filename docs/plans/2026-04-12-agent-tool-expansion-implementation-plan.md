# Agent Tool Expansion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add five new tools to the agent framework — `list_directory`, `find_files`, `test_run`, `db_sqlc` — and integrate RTK auto-prefixing into the existing `shell` tool. These close the gaps that force agents into exploratory bash usage.

**Architecture:** Each new tool follows the existing `Tool` interface pattern in `internal/tool/`. New tools are registered via `Register*` functions in `register.go` and wired in `cmd/tidmouth/serve.go` and `cmd/tidmouth/run.go`. The `shell` tool gets an RTK-aware wrapper that transparently prefixes commands when `rtk` is available in PATH.

**Tech Stack:** Go, `os` / `filepath` / `path` stdlib for directory tools, `go test -json` for Go test parsing, `pytest` / `jest --json` output parsing for Python/TS, `sqlc` CLI for database tool.

---

## File Structure

### New files (tools):
| File | Responsibility |
|------|---------------|
| `internal/tool/list_directory.go` | `list_directory` tool — tree-style directory listing with depth control |
| `internal/tool/list_directory_test.go` | Tests for list_directory |
| `internal/tool/find_files.go` | `find_files` tool — glob-based file discovery by name pattern |
| `internal/tool/find_files_test.go` | Tests for find_files |
| `internal/tool/test_run.go` | `test_run` tool — ecosystem-detecting test runner with structured output |
| `internal/tool/test_run_parse.go` | Parsers for go/pytest/jest JSON output (kept separate from dispatch) |
| `internal/tool/test_run_test.go` | Tests for test_run (uses fixture data, not real test suites) |
| `internal/tool/test_run_parse_test.go` | Tests for individual parsers |
| `internal/tool/db_sqlc.go` | `db_sqlc` tool — sqlc generate/vet/diff with structured output |
| `internal/tool/db_sqlc_test.go` | Tests for db_sqlc |

### Modified files:
| File | Change |
|------|--------|
| `internal/tool/register.go` | Add `RegisterDirectoryTools`, `RegisterTestTool`, `RegisterSqlcTool` |
| `internal/tool/shell.go` | Add RTK auto-prefix logic in `Execute` |
| `internal/tool/shell_test.go` | Add RTK prefix tests |
| `internal/tool/truncate.go` | Add truncation notices for new tools |
| `cmd/tidmouth/serve.go` | Wire new tool registrations |
| `cmd/tidmouth/run.go` | Wire new tool registrations (mirrors serve.go) |

---

### Task 1: `list_directory` Tool

**Files:**
- Create: `internal/tool/list_directory.go`
- Create: `internal/tool/list_directory_test.go`
- Modify: `internal/tool/register.go`

- [ ] **Step 1: Write the failing test — basic directory listing**

```go
// internal/tool/list_directory_test.go
package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListDirectoryBasic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "internal", "tool"), 0o755)
	os.WriteFile(filepath.Join(dir, "internal", "tool", "types.go"), []byte("package tool\n"), 0o644)

	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "main.go") {
		t.Fatalf("expected main.go in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "README.md") {
		t.Fatalf("expected README.md in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "internal/") {
		t.Fatalf("expected internal/ in output, got:\n%s", result.Content)
	}
}

func TestListDirectorySchema(t *testing.T) {
	schema := ListDirectory{}.Schema()
	if !json.Valid(schema) {
		t.Fatal("Schema() is not valid JSON")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run TestListDirectory -v`
Expected: FAIL — `ListDirectory` undefined

- [ ] **Step 3: Write minimal implementation**

```go
// internal/tool/list_directory.go
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ListDirectory implements the list_directory tool — returns a tree-style
// listing of files and directories with configurable depth.
type ListDirectory struct{}

type listDirectoryInput struct {
	Path          string `json:"path,omitempty"`
	Depth         *int   `json:"depth,omitempty"`
	IncludeHidden bool   `json:"include_hidden,omitempty"`
}

func (ListDirectory) Name() string        { return "list_directory" }
func (ListDirectory) Description() string { return "List files and directories in a tree structure" }
func (ListDirectory) ToolPurity() Purity  { return Pure }

func (ListDirectory) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "list_directory",
		"description": "List files and directories in a tree structure with configurable depth. Returns a hierarchical view of the project structure.",
		"input_schema": {
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "Directory path relative to project root (default: root)"
				},
				"depth": {
					"type": "integer",
					"description": "Maximum recursion depth (default: 3, max: 10)"
				},
				"include_hidden": {
					"type": "boolean",
					"description": "Include hidden files/directories (default: false)"
				}
			}
		}
	}`)
}

// defaultDirExcludes are directories that are always skipped in listings
// to avoid noise from dependency caches and tool state.
var defaultDirExcludes = map[string]struct{}{
	".git": {}, ".yard": {}, ".sirtopham": {}, ".sodoryard": {},
	".brain": {}, ".obsidian": {}, "vendor": {}, "node_modules": {},
	".venv": {}, "__pycache__": {}, ".idea": {}, ".vscode": {},
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

	startDir := projectRoot
	relPrefix := "."
	if params.Path != "" {
		resolved, err := resolvePath(projectRoot, params.Path)
		if err != nil {
			return &ToolResult{
				Success: false,
				Content: err.Error(),
				Error:   err.Error(),
			}, nil
		}
		startDir = resolved
		relPrefix = params.Path
	}

	info, err := os.Stat(startDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("Directory not found: %s", params.Path),
				Error:   "not found",
			}, nil
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Error accessing directory: %v", err),
			Error:   err.Error(),
		}, nil
	}
	if !info.IsDir() {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Not a directory: %s", params.Path),
			Error:   "not a directory",
		}, nil
	}

	var sb strings.Builder
	fileCount, dirCount := buildTree(&sb, startDir, relPrefix, depth, 0, params.IncludeHidden)
	sb.WriteString(fmt.Sprintf("\n(%d directories, %d files)", dirCount, fileCount))

	return &ToolResult{
		Success: true,
		Content: sb.String(),
	}, nil
}

func buildTree(sb *strings.Builder, absDir, relDir string, maxDepth, currentDepth int, includeHidden bool) (files, dirs int) {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return 0, 0
	}

	// Sort: directories first, then files, both alphabetically.
	sort.Slice(entries, func(i, j int) bool {
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di
		}
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files unless requested.
		if !includeHidden && strings.HasPrefix(name, ".") {
			continue
		}
		// Always skip excluded directories.
		if entry.IsDir() {
			if _, excluded := defaultDirExcludes[name]; excluded {
				continue
			}
		}

		indent := strings.Repeat("  ", currentDepth)
		entryPath := filepath.Join(relDir, name)

		if entry.IsDir() {
			sb.WriteString(fmt.Sprintf("%s%s/\n", indent, name))
			dirs++
			if currentDepth < maxDepth-1 {
				childFiles, childDirs := buildTree(sb, filepath.Join(absDir, name), entryPath, maxDepth, currentDepth+1, includeHidden)
				files += childFiles
				dirs += childDirs
			}
		} else {
			sb.WriteString(fmt.Sprintf("%s%s\n", indent, name))
			files++
		}
	}

	return files, dirs
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run TestListDirectory -v`
Expected: PASS

- [ ] **Step 5: Write additional tests — depth, hidden files, excludes, path traversal**

```go
// Append to internal/tool/list_directory_test.go

func TestListDirectoryDepthLimit(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a", "b", "c", "d"), 0o755)
	os.WriteFile(filepath.Join(dir, "a", "b", "c", "d", "deep.go"), []byte("package d\n"), 0o644)

	// Depth 2 should show a/ and a/b/ but not recurse into c/.
	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{"depth":2}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "b/") {
		t.Fatalf("expected b/ at depth 2, got:\n%s", result.Content)
	}
	// deep.go is at depth 4, should not appear.
	if strings.Contains(result.Content, "deep.go") {
		t.Fatalf("did NOT expect deep.go at depth 2, got:\n%s", result.Content)
	}
}

func TestListDirectoryExcludesNodeModules(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "node_modules", "react"), 0o755)
	os.WriteFile(filepath.Join(dir, "index.js"), []byte("// main\n"), 0o644)

	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "node_modules") {
		t.Fatalf("did NOT expect node_modules in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "index.js") {
		t.Fatalf("expected index.js in output, got:\n%s", result.Content)
	}
}

func TestListDirectoryHiddenFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=1\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)

	// Default: hidden files excluded.
	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result.Content, ".env") {
		t.Fatalf("did NOT expect .env with default settings, got:\n%s", result.Content)
	}

	// With include_hidden: .env appears.
	result, err = ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{"include_hidden":true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, ".env") {
		t.Fatalf("expected .env with include_hidden=true, got:\n%s", result.Content)
	}
}

func TestListDirectoryPathTraversal(t *testing.T) {
	dir := t.TempDir()

	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{"path":"../../etc"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for path traversal")
	}
	if !strings.Contains(result.Content, "escapes project root") {
		t.Fatalf("expected path escape error, got: %s", result.Content)
	}
}

func TestListDirectoryNotFound(t *testing.T) {
	dir := t.TempDir()

	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{"path":"nonexistent"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for nonexistent directory")
	}
}

func TestListDirectoryNotADirectory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content\n"), 0o644)

	result, err := ListDirectory{}.Execute(context.Background(), dir, json.RawMessage(`{"path":"file.txt"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure when path is a file")
	}
	if !strings.Contains(result.Content, "Not a directory") {
		t.Fatalf("expected 'Not a directory' error, got: %s", result.Content)
	}
}
```

- [ ] **Step 6: Run all list_directory tests**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run TestListDirectory -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
cd /home/gernsback/source/sodoryard
git add internal/tool/list_directory.go internal/tool/list_directory_test.go
git commit -m "feat(tool): add list_directory tool for tree-style directory listing"
```

---

### Task 2: `find_files` Tool

**Files:**
- Create: `internal/tool/find_files.go`
- Create: `internal/tool/find_files_test.go`
- Modify: `internal/tool/register.go`

- [ ] **Step 1: Write the failing test — basic glob matching**

```go
// internal/tool/find_files_test.go
package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindFilesBasic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "internal"), 0o755)
	os.WriteFile(filepath.Join(dir, "internal", "lib.go"), []byte("package internal\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644)

	result, err := FindFiles{}.Execute(context.Background(), dir, json.RawMessage(`{"pattern":"*.go"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "main.go") {
		t.Fatalf("expected main.go, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "lib.go") {
		t.Fatalf("expected lib.go, got:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "README.md") {
		t.Fatalf("did NOT expect README.md for *.go pattern, got:\n%s", result.Content)
	}
}

func TestFindFilesSchema(t *testing.T) {
	schema := FindFiles{}.Schema()
	if !json.Valid(schema) {
		t.Fatal("Schema() is not valid JSON")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run TestFindFiles -v`
Expected: FAIL — `FindFiles` undefined

- [ ] **Step 3: Write minimal implementation**

The Go stdlib `filepath.Glob` doesn't support `**` (recursive). Use `filepath.WalkDir` with `filepath.Match` per segment. For `**` support, match the pattern against the full relative path using a simple recursive glob matcher.

```go
// internal/tool/find_files.go
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FindFiles implements the find_files tool — glob-based file discovery
// across the project. Supports ** for recursive matching.
type FindFiles struct{}

type findFilesInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	MaxResults *int   `json:"max_results,omitempty"`
}

func (FindFiles) Name() string        { return "find_files" }
func (FindFiles) Description() string { return "Find files by name pattern using glob matching" }
func (FindFiles) ToolPurity() Purity  { return Pure }

func (FindFiles) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "find_files",
		"description": "Find files matching a glob pattern. Supports ** for recursive directory matching (e.g., '**/*_test.go', 'cmd/**/*.go', 'sqlc.yaml'). Returns file paths relative to the project root.",
		"input_schema": {
			"type": "object",
			"properties": {
				"pattern": {
					"type": "string",
					"description": "Glob pattern to match file names (e.g., '*.go', '**/*_test.py', 'Makefile', '**/sqlc.yaml')"
				},
				"path": {
					"type": "string",
					"description": "Optional subdirectory to scope the search to (relative to project root)"
				},
				"max_results": {
					"type": "integer",
					"description": "Maximum number of results to return (default: 100)"
				}
			},
			"required": ["pattern"]
		}
	}`)
}

func (FindFiles) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	var params findFilesInput
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

	maxResults := 100
	if params.MaxResults != nil && *params.MaxResults > 0 {
		maxResults = *params.MaxResults
	}

	searchRoot := projectRoot
	pathPrefix := ""
	if params.Path != "" {
		resolved, err := resolvePath(projectRoot, params.Path)
		if err != nil {
			return &ToolResult{
				Success: false,
				Content: err.Error(),
				Error:   err.Error(),
			}, nil
		}
		searchRoot = resolved
		pathPrefix = params.Path
	}

	var matches []string
	err := filepath.WalkDir(searchRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		name := d.Name()

		// Skip excluded directories.
		if d.IsDir() {
			if _, excluded := defaultDirExcludes[name]; excluded {
				return filepath.SkipDir
			}
			if !params.includeHiddenDirs() && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Get path relative to searchRoot.
		rel, err := filepath.Rel(searchRoot, path)
		if err != nil {
			return nil
		}

		if globMatch(params.Pattern, rel) {
			displayPath := rel
			if pathPrefix != "" {
				displayPath = filepath.Join(pathPrefix, rel)
			}
			matches = append(matches, displayPath)
			if len(matches) >= maxResults {
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll && ctx.Err() == nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Error searching files: %v", err),
			Error:   err.Error(),
		}, nil
	}

	if len(matches) == 0 {
		return &ToolResult{
			Success: true,
			Content: fmt.Sprintf("No files found matching pattern: '%s'", params.Pattern),
		}, nil
	}

	var sb strings.Builder
	for _, m := range matches {
		sb.WriteString(m)
		sb.WriteString("\n")
	}
	sb.WriteString(fmt.Sprintf("\n(%d files)", len(matches)))

	return &ToolResult{
		Success: true,
		Content: sb.String(),
	}, nil
}

// includeHiddenDirs returns false — hidden directories are skipped during
// find_files walks. This is a method so the struct remains zero-value usable.
func (findFilesInput) includeHiddenDirs() bool { return false }

// globMatch matches a pattern against a path, supporting ** for recursive
// directory matching. Uses filepath.Match for individual segments.
func globMatch(pattern, path string) bool {
	// Simple case: no ** in pattern, match the basename only.
	if !strings.Contains(pattern, "/") && !strings.Contains(pattern, "**") {
		matched, _ := filepath.Match(pattern, filepath.Base(path))
		return matched
	}

	// Split pattern and path into segments.
	patParts := splitPath(pattern)
	pathParts := splitPath(path)

	return globMatchParts(patParts, pathParts)
}

func splitPath(p string) []string {
	p = filepath.ToSlash(p)
	parts := strings.Split(p, "/")
	var result []string
	for _, part := range parts {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func globMatchParts(pattern, path []string) bool {
	if len(pattern) == 0 {
		return len(path) == 0
	}

	if pattern[0] == "**" {
		// ** matches zero or more directory segments.
		rest := pattern[1:]
		for i := 0; i <= len(path); i++ {
			if globMatchParts(rest, path[i:]) {
				return true
			}
		}
		return false
	}

	if len(path) == 0 {
		return false
	}

	matched, _ := filepath.Match(pattern[0], path[0])
	if !matched {
		return false
	}

	return globMatchParts(pattern[1:], path[1:])
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run TestFindFiles -v`
Expected: PASS

- [ ] **Step 5: Write additional tests — recursive glob, scoped path, no results, max_results, path traversal**

```go
// Append to internal/tool/find_files_test.go

func TestFindFilesRecursiveGlob(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "cmd", "app"), 0o755)
	os.MkdirAll(filepath.Join(dir, "internal", "tool"), 0o755)
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "cmd", "app", "app_test.go"), []byte("package app\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "internal", "tool", "tool_test.go"), []byte("package tool\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "internal", "tool", "types.go"), []byte("package tool\n"), 0o644)

	result, err := FindFiles{}.Execute(context.Background(), dir, json.RawMessage(`{"pattern":"**/*_test.go"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "main_test.go") {
		t.Fatalf("expected main_test.go, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "app_test.go") {
		t.Fatalf("expected app_test.go, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "tool_test.go") {
		t.Fatalf("expected tool_test.go, got:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "types.go") {
		t.Fatalf("did NOT expect types.go for *_test.go pattern, got:\n%s", result.Content)
	}
}

func TestFindFilesScopedPath(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "cmd"), 0o755)
	os.MkdirAll(filepath.Join(dir, "internal"), 0o755)
	os.WriteFile(filepath.Join(dir, "cmd", "main.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "internal", "lib.go"), []byte("package internal\n"), 0o644)

	result, err := FindFiles{}.Execute(context.Background(), dir, json.RawMessage(`{"pattern":"*.go","path":"cmd"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "cmd/main.go") {
		t.Fatalf("expected cmd/main.go, got:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "lib.go") {
		t.Fatalf("did NOT expect lib.go in scoped results, got:\n%s", result.Content)
	}
}

func TestFindFilesNoResults(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)

	result, err := FindFiles{}.Execute(context.Background(), dir, json.RawMessage(`{"pattern":"*.rs"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success=true for no matches")
	}
	if !strings.Contains(result.Content, "No files found") {
		t.Fatalf("expected 'No files found', got: %s", result.Content)
	}
}

func TestFindFilesMaxResults(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%d.go", i)), []byte("package main\n"), 0o644)
	}

	result, err := FindFiles{}.Execute(context.Background(), dir, json.RawMessage(`{"pattern":"*.go","max_results":3}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "(3 files)") {
		t.Fatalf("expected exactly 3 files, got:\n%s", result.Content)
	}
}

func TestFindFilesExcludesNodeModules(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "node_modules", "react"), 0o755)
	os.WriteFile(filepath.Join(dir, "node_modules", "react", "index.js"), []byte("// react\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("// app\n"), 0o644)

	result, err := FindFiles{}.Execute(context.Background(), dir, json.RawMessage(`{"pattern":"*.js"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "node_modules") {
		t.Fatalf("did NOT expect node_modules in results, got:\n%s", result.Content)
	}
}

func TestFindFilesPathTraversal(t *testing.T) {
	dir := t.TempDir()

	result, err := FindFiles{}.Execute(context.Background(), dir, json.RawMessage(`{"pattern":"*","path":"../../etc"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for path traversal")
	}
}

func TestFindFilesEmptyPattern(t *testing.T) {
	dir := t.TempDir()
	result, err := FindFiles{}.Execute(context.Background(), dir, json.RawMessage(`{"pattern":""}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for empty pattern")
	}
}
```

- [ ] **Step 6: Run all find_files tests**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run TestFindFiles -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
cd /home/gernsback/source/sodoryard
git add internal/tool/find_files.go internal/tool/find_files_test.go
git commit -m "feat(tool): add find_files tool for glob-based file discovery"
```

---

### Task 3: Register Directory Tools

**Files:**
- Modify: `internal/tool/register.go`

- [ ] **Step 1: Write the failing test**

```go
// Add to an existing test file, or create internal/tool/register_directory_test.go

// In internal/tool/register_test.go or new file:
func TestRegisterDirectoryTools(t *testing.T) {
	reg := NewRegistry()
	RegisterDirectoryTools(reg)

	if _, ok := reg.Get("list_directory"); !ok {
		t.Fatal("list_directory not registered")
	}
	if _, ok := reg.Get("find_files"); !ok {
		t.Fatal("find_files not registered")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run TestRegisterDirectoryTools -v`
Expected: FAIL — `RegisterDirectoryTools` undefined

- [ ] **Step 3: Add registration function to register.go**

Add this after the existing `RegisterSearchTools` function in `internal/tool/register.go`:

```go
// RegisterDirectoryTools registers all directory navigation tools
// (list_directory, find_files) in the given registry.
func RegisterDirectoryTools(r *Registry) {
	r.Register(ListDirectory{})
	r.Register(FindFiles{})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run TestRegisterDirectoryTools -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/gernsback/source/sodoryard
git add internal/tool/register.go
git commit -m "feat(tool): add RegisterDirectoryTools registration function"
```

---

### Task 4: `test_run` Tool — Go Parser

**Files:**
- Create: `internal/tool/test_run.go`
- Create: `internal/tool/test_run_parse.go`
- Create: `internal/tool/test_run_parse_test.go`
- Create: `internal/tool/test_run_test.go`

This task builds the core `test_run` tool with Go ecosystem support. Python and TypeScript parsers are added in Tasks 5 and 6.

- [ ] **Step 1: Write the failing test — Go test JSON parser**

```go
// internal/tool/test_run_parse_test.go
package tool

import (
	"strings"
	"testing"
)

func TestParseGoTestJSON_AllPass(t *testing.T) {
	input := strings.Join([]string{
		`{"Time":"2026-04-12T10:00:00Z","Action":"run","Package":"./internal/tool","Test":"TestA"}`,
		`{"Time":"2026-04-12T10:00:00Z","Action":"output","Package":"./internal/tool","Test":"TestA","Output":"--- PASS: TestA (0.00s)\n"}`,
		`{"Time":"2026-04-12T10:00:00Z","Action":"pass","Package":"./internal/tool","Test":"TestA","Elapsed":0.001}`,
		`{"Time":"2026-04-12T10:00:00Z","Action":"pass","Package":"./internal/tool","Elapsed":0.5}`,
	}, "\n")

	result := parseGoTestJSON(input)
	if result.Passed != 1 {
		t.Fatalf("expected 1 passed, got %d", result.Passed)
	}
	if result.Failed != 0 {
		t.Fatalf("expected 0 failed, got %d", result.Failed)
	}
	if len(result.Failures) != 0 {
		t.Fatalf("expected no failures, got %d", len(result.Failures))
	}
}

func TestParseGoTestJSON_WithFailure(t *testing.T) {
	input := strings.Join([]string{
		`{"Time":"2026-04-12T10:00:00Z","Action":"run","Package":"./internal/tool","Test":"TestA"}`,
		`{"Time":"2026-04-12T10:00:00Z","Action":"output","Package":"./internal/tool","Test":"TestA","Output":"    tool_test.go:42: expected 3, got 0\n"}`,
		`{"Time":"2026-04-12T10:00:00Z","Action":"fail","Package":"./internal/tool","Test":"TestA","Elapsed":0.001}`,
		`{"Time":"2026-04-12T10:00:00Z","Action":"run","Package":"./internal/tool","Test":"TestB"}`,
		`{"Time":"2026-04-12T10:00:00Z","Action":"pass","Package":"./internal/tool","Test":"TestB","Elapsed":0.002}`,
		`{"Time":"2026-04-12T10:00:00Z","Action":"fail","Package":"./internal/tool","Elapsed":0.5}`,
	}, "\n")

	result := parseGoTestJSON(input)
	if result.Passed != 1 {
		t.Fatalf("expected 1 passed, got %d", result.Passed)
	}
	if result.Failed != 1 {
		t.Fatalf("expected 1 failed, got %d", result.Failed)
	}
	if len(result.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(result.Failures))
	}
	f := result.Failures[0]
	if f.Test != "TestA" {
		t.Fatalf("expected failure test TestA, got %s", f.Test)
	}
	if f.Package != "./internal/tool" {
		t.Fatalf("expected package ./internal/tool, got %s", f.Package)
	}
	if !strings.Contains(f.Output, "expected 3, got 0") {
		t.Fatalf("expected failure output to contain error message, got: %s", f.Output)
	}
}

func TestParseGoTestJSON_BuildError(t *testing.T) {
	input := strings.Join([]string{
		`{"Time":"2026-04-12T10:00:00Z","Action":"output","Package":"./internal/tool","Output":"# ./internal/tool\n"}`,
		`{"Time":"2026-04-12T10:00:00Z","Action":"output","Package":"./internal/tool","Output":"./tool.go:15:2: undefined: badFunc\n"}`,
		`{"Time":"2026-04-12T10:00:00Z","Action":"fail","Package":"./internal/tool","Elapsed":0}`,
	}, "\n")

	result := parseGoTestJSON(input)
	if len(result.BuildErrors) == 0 {
		t.Fatal("expected build errors")
	}
	if !strings.Contains(result.BuildErrors[0], "undefined: badFunc") {
		t.Fatalf("expected build error content, got: %s", result.BuildErrors[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run TestParseGoTest -v`
Expected: FAIL — `parseGoTestJSON` undefined

- [ ] **Step 3: Write the test result types and Go parser**

```go
// internal/tool/test_run_parse.go
package tool

import (
	"encoding/json"
	"strings"
)

// testRunResult is the structured output from a test run, regardless of ecosystem.
type testRunResult struct {
	Ecosystem   string        `json:"ecosystem"`
	Passed      int           `json:"passed"`
	Failed      int           `json:"failed"`
	Skipped     int           `json:"skipped"`
	Failures    []testFailure `json:"failures,omitempty"`
	BuildErrors []string      `json:"build_errors,omitempty"`
	Summary     string        `json:"summary"`
}

type testFailure struct {
	Test    string `json:"test"`
	Package string `json:"package,omitempty"`
	File    string `json:"file,omitempty"`
	Output  string `json:"output"`
}

// formatTestResult produces the agent-facing output string from a testRunResult.
func formatTestResult(r testRunResult) string {
	var sb strings.Builder

	if len(r.BuildErrors) > 0 {
		sb.WriteString("BUILD ERRORS:\n")
		for _, e := range r.BuildErrors {
			sb.WriteString(e)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(r.Summary)
	sb.WriteString("\n")

	if len(r.Failures) > 0 {
		sb.WriteString("\nFAILURES:\n")
		for _, f := range r.Failures {
			sb.WriteString("--- ")
			if f.Package != "" {
				sb.WriteString(f.Package)
				sb.WriteString("/")
			}
			sb.WriteString(f.Test)
			sb.WriteString("\n")
			sb.WriteString(strings.TrimRight(f.Output, "\n"))
			sb.WriteString("\n\n")
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// --- Go parser ---

type goTestEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

func parseGoTestJSON(raw string) testRunResult {
	result := testRunResult{Ecosystem: "go"}

	// Track per-test output for failures.
	testOutputs := make(map[string][]string) // key: "package/test"
	packageOutputs := make(map[string][]string) // package-level output (no Test field) for build errors

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var event goTestEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		key := event.Package + "/" + event.Test

		switch event.Action {
		case "output":
			if event.Test == "" {
				// Package-level output — may be build errors.
				packageOutputs[event.Package] = append(packageOutputs[event.Package], event.Output)
			} else {
				testOutputs[key] = append(testOutputs[key], event.Output)
			}

		case "pass":
			if event.Test != "" {
				result.Passed++
			}

		case "fail":
			if event.Test != "" {
				result.Failed++
				output := strings.Join(testOutputs[key], "")
				result.Failures = append(result.Failures, testFailure{
					Test:    event.Test,
					Package: event.Package,
					Output:  strings.TrimSpace(output),
				})
			} else {
				// Package failed — check if this was a build error (no tests ran).
				if result.Passed == 0 && result.Failed == 0 {
					if outputs, ok := packageOutputs[event.Package]; ok {
						for _, o := range outputs {
							trimmed := strings.TrimSpace(o)
							if trimmed != "" && !strings.HasPrefix(trimmed, "ok") && !strings.HasPrefix(trimmed, "FAIL") {
								result.BuildErrors = append(result.BuildErrors, trimmed)
							}
						}
					}
				}
			}

		case "skip":
			if event.Test != "" {
				result.Skipped++
			}
		}
	}

	total := result.Passed + result.Failed + result.Skipped
	result.Summary = formatTestSummary("go", total, result.Passed, result.Failed, result.Skipped)
	return result
}

func formatTestSummary(ecosystem string, total, passed, failed, skipped int) string {
	parts := []string{}
	parts = append(parts, strings.ToUpper(ecosystem))
	if failed > 0 {
		parts = append(parts, "FAIL")
	} else {
		parts = append(parts, "PASS")
	}
	parts = append(parts, "-")
	parts = append(parts, strings.Join([]string{
		intStr(passed) + " passed",
		intStr(failed) + " failed",
		intStr(skipped) + " skipped",
		intStr(total) + " total",
	}, ", "))
	return strings.Join(parts, " ")
}

func intStr(n int) string {
	return strings.TrimRight(strings.TrimRight(
		strings.Replace(
			strings.Replace(
				strings.Replace(
					string(rune('0'+n%10)),
					string(rune('0')), "0", 1),
				"", "", 0),
			"", "", 0),
		""), "")
}
```

Wait — that `intStr` is unnecessarily clever. Use `strconv.Itoa` or `fmt.Sprintf`:

```go
// Replace the intStr function with:
import "strconv"

// In formatTestSummary, just use:
func formatTestSummary(ecosystem string, total, passed, failed, skipped int) string {
	status := "PASS"
	if failed > 0 {
		status = "FAIL"
	}
	return fmt.Sprintf("%s %s — %d passed, %d failed, %d skipped, %d total",
		strings.ToUpper(ecosystem), status, passed, failed, skipped, total)
}
```

- [ ] **Step 4: Run parser tests to verify they pass**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run TestParseGoTest -v`
Expected: PASS

- [ ] **Step 5: Write the test_run tool with ecosystem detection**

```go
// internal/tool/test_run.go
package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// TestRun implements the test_run tool — auto-detecting test runner with
// structured output for Go, Python, and TypeScript ecosystems.
type TestRun struct{}

type testRunInput struct {
	Ecosystem string `json:"ecosystem,omitempty"`
	Path      string `json:"path,omitempty"`
	Filter    string `json:"filter,omitempty"`
	Verbose   bool   `json:"verbose,omitempty"`
	Timeout   *int   `json:"timeout_seconds,omitempty"`
}

func (TestRun) Name() string        { return "test_run" }
func (TestRun) Description() string { return "Run tests with structured output (auto-detects Go/Python/TypeScript)" }
func (TestRun) ToolPurity() Purity  { return Mutating }

func (TestRun) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "test_run",
		"description": "Run tests with structured, failures-only output. Auto-detects ecosystem (Go, Python, TypeScript) from project files, or accepts an explicit override. Returns pass/fail counts and detailed failure output only.",
		"input_schema": {
			"type": "object",
			"properties": {
				"ecosystem": {
					"type": "string",
					"enum": ["go", "python", "typescript"],
					"description": "Override auto-detection. If omitted, detects from go.mod/pyproject.toml/package.json."
				},
				"path": {
					"type": "string",
					"description": "Subdirectory or file to test (relative to project root). For Go: package path like './internal/tool/...'. For Python: path like 'tests/'. For TypeScript: path like 'src/components/'."
				},
				"filter": {
					"type": "string",
					"description": "Test name filter. Maps to: Go -run, Python -k, TypeScript --testNamePattern."
				},
				"verbose": {
					"type": "boolean",
					"description": "Include passing test output (default: false — only failures shown)"
				},
				"timeout_seconds": {
					"type": "integer",
					"description": "Test timeout in seconds (default: 300)"
				}
			}
		}
	}`)
}

func (TestRun) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	var params testRunInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &params); err != nil {
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("Invalid input: %v", err),
				Error:   err.Error(),
			}, nil
		}
	}

	// Resolve scope directory for ecosystem detection.
	detectDir := projectRoot
	if params.Path != "" {
		resolved, err := resolvePath(projectRoot, params.Path)
		if err != nil {
			return &ToolResult{
				Success: false,
				Content: err.Error(),
				Error:   err.Error(),
			}, nil
		}
		// If it's a file, use its directory for detection.
		info, statErr := os.Stat(resolved)
		if statErr == nil && !info.IsDir() {
			detectDir = filepath.Dir(resolved)
		} else {
			detectDir = resolved
		}
	}

	ecosystem := params.Ecosystem
	if ecosystem == "" {
		ecosystem = detectTestEcosystem(detectDir, projectRoot)
	}
	if ecosystem == "" {
		return &ToolResult{
			Success: false,
			Content: "Could not detect test ecosystem. No go.mod, pyproject.toml/setup.py, or package.json found. Use the ecosystem parameter to specify manually.",
			Error:   "no ecosystem detected",
		}, nil
	}

	timeout := 300 * time.Second
	if params.Timeout != nil && *params.Timeout > 0 {
		timeout = time.Duration(*params.Timeout) * time.Second
	}

	switch ecosystem {
	case "go":
		return runGoTests(ctx, projectRoot, params, timeout)
	case "python":
		return runPythonTests(ctx, projectRoot, params, timeout)
	case "typescript":
		return runTypeScriptTests(ctx, projectRoot, params, timeout)
	default:
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Unsupported ecosystem: %s. Supported: go, python, typescript.", ecosystem),
			Error:   "unsupported ecosystem",
		}, nil
	}
}

// detectTestEcosystem walks from detectDir up to projectRoot looking for
// ecosystem markers: go.mod, pyproject.toml/setup.py/setup.cfg, package.json.
func detectTestEcosystem(detectDir, projectRoot string) string {
	dir := detectDir
	for {
		if fileExists(filepath.Join(dir, "go.mod")) {
			return "go"
		}
		if fileExists(filepath.Join(dir, "pyproject.toml")) || fileExists(filepath.Join(dir, "setup.py")) || fileExists(filepath.Join(dir, "setup.cfg")) {
			return "python"
		}
		if fileExists(filepath.Join(dir, "package.json")) {
			return "typescript"
		}
		if dir == projectRoot {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func runGoTests(ctx context.Context, projectRoot string, params testRunInput, timeout time.Duration) (*ToolResult, error) {
	goPath, err := lookupCommandPath("go")
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: "go is required but not found in PATH",
			Error:   "go not found",
		}, nil
	}

	args := []string{"test", "-json"}
	if params.Filter != "" {
		args = append(args, "-run", params.Filter)
	}
	args = append(args, fmt.Sprintf("-timeout=%ds", int(timeout.Seconds())))

	if params.Path != "" {
		args = append(args, params.Path)
	} else {
		args = append(args, "./...")
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout+10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, goPath, args...)
	cmd.Dir = projectRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run() // Ignore exit code — go test returns non-zero on failures, which is expected.

	result := parseGoTestJSON(stdout.String())
	if stderr.Len() > 0 && len(result.BuildErrors) == 0 {
		// Stderr may have build errors not captured in JSON.
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			result.BuildErrors = append(result.BuildErrors, stderrStr)
		}
	}

	return &ToolResult{
		Success: true,
		Content: formatTestResult(result),
	}, nil
}

// runPythonTests and runTypeScriptTests are implemented in Tasks 5 and 6.
// For now, they return "not yet implemented" to keep the tool compilable.
func runPythonTests(ctx context.Context, projectRoot string, params testRunInput, timeout time.Duration) (*ToolResult, error) {
	return &ToolResult{
		Success: false,
		Content: "Python test runner not yet available. Use shell to run pytest directly.",
		Error:   "not implemented",
	}, nil
}

func runTypeScriptTests(ctx context.Context, projectRoot string, params testRunInput, timeout time.Duration) (*ToolResult, error) {
	return &ToolResult{
		Success: false,
		Content: "TypeScript test runner not yet available. Use shell to run jest/vitest directly.",
		Error:   "not implemented",
	}, nil
}
```

- [ ] **Step 6: Write integration-style test for the Go runner**

```go
// internal/tool/test_run_test.go
package tool

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not found in PATH, skipping test_run tests")
	}
}

func TestTestRunGoPassingTests(t *testing.T) {
	requireGo(t)
	dir := t.TempDir()

	// Create a minimal Go module with a passing test.
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(`package main

import "testing"

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("expected 3")
	}
}
`), 0o644)

	result, err := TestRun{}.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "1 passed") {
		t.Fatalf("expected '1 passed' in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "0 failed") {
		t.Fatalf("expected '0 failed' in output, got:\n%s", result.Content)
	}
}

func TestTestRunGoFailingTest(t *testing.T) {
	requireGo(t)
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(`package main

import "testing"

func TestAddWrong(t *testing.T) {
	if Add(1, 2) != 5 {
		t.Fatal("expected 5, got 3")
	}
}

func TestAddCorrect(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("expected 3")
	}
}
`), 0o644)

	result, err := TestRun{}.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success (tool always succeeds), got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "1 failed") {
		t.Fatalf("expected '1 failed' in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "TestAddWrong") {
		t.Fatalf("expected TestAddWrong in failures, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "expected 5, got 3") {
		t.Fatalf("expected failure message in output, got:\n%s", result.Content)
	}
}

func TestTestRunEcosystemDetection(t *testing.T) {
	t.Run("detects go.mod", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)
		eco := detectTestEcosystem(dir, dir)
		if eco != "go" {
			t.Fatalf("expected 'go', got %q", eco)
		}
	})

	t.Run("detects pyproject.toml", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\n"), 0o644)
		eco := detectTestEcosystem(dir, dir)
		if eco != "python" {
			t.Fatalf("expected 'python', got %q", eco)
		}
	})

	t.Run("detects package.json", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644)
		eco := detectTestEcosystem(dir, dir)
		if eco != "typescript" {
			t.Fatalf("expected 'typescript', got %q", eco)
		}
	})

	t.Run("walks up to project root", func(t *testing.T) {
		dir := t.TempDir()
		sub := filepath.Join(dir, "internal", "tool")
		os.MkdirAll(sub, 0o755)
		os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)
		eco := detectTestEcosystem(sub, dir)
		if eco != "go" {
			t.Fatalf("expected 'go' from subdirectory, got %q", eco)
		}
	})

	t.Run("no ecosystem found", func(t *testing.T) {
		dir := t.TempDir()
		eco := detectTestEcosystem(dir, dir)
		if eco != "" {
			t.Fatalf("expected empty string, got %q", eco)
		}
	})
}

func TestTestRunSchema(t *testing.T) {
	schema := TestRun{}.Schema()
	if !json.Valid(schema) {
		t.Fatal("Schema() is not valid JSON")
	}
}
```

- [ ] **Step 7: Run all test_run tests**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run "TestTestRun|TestParseGoTest" -v -timeout 60s`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
cd /home/gernsback/source/sodoryard
git add internal/tool/test_run.go internal/tool/test_run_parse.go internal/tool/test_run_parse_test.go internal/tool/test_run_test.go
git commit -m "feat(tool): add test_run tool with Go ecosystem support and structured output"
```

---

### Task 5: `test_run` — Python Parser

**Files:**
- Modify: `internal/tool/test_run_parse.go`
- Modify: `internal/tool/test_run_parse_test.go`
- Modify: `internal/tool/test_run.go` (replace stub)

- [ ] **Step 1: Write the failing test — pytest JSON parser**

```go
// Append to internal/tool/test_run_parse_test.go

func TestParsePytestJSON_AllPass(t *testing.T) {
	input := `{
		"summary": {"passed": 3, "total": 3},
		"tests": [
			{"nodeid": "tests/test_auth.py::test_login", "outcome": "passed"},
			{"nodeid": "tests/test_auth.py::test_logout", "outcome": "passed"},
			{"nodeid": "tests/test_auth.py::test_signup", "outcome": "passed"}
		]
	}`

	result := parsePytestJSON(input)
	if result.Passed != 3 {
		t.Fatalf("expected 3 passed, got %d", result.Passed)
	}
	if result.Failed != 0 {
		t.Fatalf("expected 0 failed, got %d", result.Failed)
	}
}

func TestParsePytestJSON_WithFailure(t *testing.T) {
	input := `{
		"summary": {"passed": 1, "failed": 1, "total": 2},
		"tests": [
			{"nodeid": "tests/test_auth.py::test_login", "outcome": "passed"},
			{
				"nodeid": "tests/test_auth.py::test_signup",
				"outcome": "failed",
				"call": {"longrepr": "AssertionError: expected 200, got 401\n    assert response.status_code == 200"}
			}
		]
	}`

	result := parsePytestJSON(input)
	if result.Failed != 1 {
		t.Fatalf("expected 1 failed, got %d", result.Failed)
	}
	if len(result.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(result.Failures))
	}
	if !strings.Contains(result.Failures[0].Output, "expected 200, got 401") {
		t.Fatalf("expected failure output, got: %s", result.Failures[0].Output)
	}
}

func TestParsePytestShort_WithFailure(t *testing.T) {
	input := `FAILED tests/test_auth.py::test_signup - AssertionError: expected 200
1 passed, 1 failed in 0.34s`

	result := parsePytestShort(input)
	if result.Failed != 1 {
		t.Fatalf("expected 1 failed, got %d", result.Failed)
	}
	if result.Passed != 1 {
		t.Fatalf("expected 1 passed, got %d", result.Passed)
	}
	if len(result.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(result.Failures))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run "TestParsePytest" -v`
Expected: FAIL — `parsePytestJSON` undefined

- [ ] **Step 3: Write Python parsers**

Add to `internal/tool/test_run_parse.go`:

```go
// --- Python/pytest parsers ---

type pytestReport struct {
	Summary pytestSummary `json:"summary"`
	Tests   []pytestTest  `json:"tests"`
}

type pytestSummary struct {
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
	Total   int `json:"total"`
}

type pytestTest struct {
	NodeID  string     `json:"nodeid"`
	Outcome string     `json:"outcome"`
	Call    *pytestCall `json:"call,omitempty"`
}

type pytestCall struct {
	LongRepr string `json:"longrepr"`
}

func parsePytestJSON(raw string) testRunResult {
	result := testRunResult{Ecosystem: "python"}

	var report pytestReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		result.BuildErrors = append(result.BuildErrors, fmt.Sprintf("Failed to parse pytest JSON: %v", err))
		return result
	}

	result.Passed = report.Summary.Passed
	result.Failed = report.Summary.Failed
	result.Skipped = report.Summary.Skipped

	for _, test := range report.Tests {
		if test.Outcome == "failed" {
			output := ""
			if test.Call != nil {
				output = test.Call.LongRepr
			}
			result.Failures = append(result.Failures, testFailure{
				Test:   test.NodeID,
				Output: output,
			})
		}
	}

	total := result.Passed + result.Failed + result.Skipped
	result.Summary = formatTestSummary("python", total, result.Passed, result.Failed, result.Skipped)
	return result
}

// parsePytestShort parses the output of `pytest -q --tb=short` as a fallback
// when pytest-json-report is not installed.
func parsePytestShort(raw string) testRunResult {
	result := testRunResult{Ecosystem: "python"}

	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "FAILED ") {
			// "FAILED tests/test_auth.py::test_signup - AssertionError: expected 200"
			rest := strings.TrimPrefix(line, "FAILED ")
			parts := strings.SplitN(rest, " - ", 2)
			test := parts[0]
			output := ""
			if len(parts) > 1 {
				output = parts[1]
			}
			result.Failures = append(result.Failures, testFailure{
				Test:   test,
				Output: output,
			})
			result.Failed++
		}
	}

	// Parse summary line: "1 passed, 1 failed in 0.34s"
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, " passed") || strings.Contains(line, " failed") {
			if strings.Contains(line, " in ") {
				parsePytestSummaryLine(line, &result)
				break
			}
		}
	}

	total := result.Passed + result.Failed + result.Skipped
	result.Summary = formatTestSummary("python", total, result.Passed, result.Failed, result.Skipped)
	return result
}

func parsePytestSummaryLine(line string, result *testRunResult) {
	// Parse "1 passed, 2 failed, 3 skipped in 0.34s"
	parts := strings.Split(line, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		// Strip " in X.XXs" from last segment.
		if idx := strings.Index(part, " in "); idx >= 0 {
			part = part[:idx]
		}
		var count int
		var label string
		if n, err := fmt.Sscanf(part, "%d %s", &count, &label); n == 2 && err == nil {
			switch {
			case strings.HasPrefix(label, "passed"):
				result.Passed = count
			case strings.HasPrefix(label, "failed"):
				result.Failed = count
			case strings.HasPrefix(label, "skipped"):
				result.Skipped = count
			}
		}
	}
}
```

- [ ] **Step 4: Write the `runPythonTests` implementation**

Replace the stub in `internal/tool/test_run.go`:

```go
func runPythonTests(ctx context.Context, projectRoot string, params testRunInput, timeout time.Duration) (*ToolResult, error) {
	pytestPath, err := lookupCommandPath("pytest")
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: "pytest is required but not found in PATH. Install: pip install pytest",
			Error:   "pytest not found",
		}, nil
	}

	// Try pytest-json-report first for structured output.
	useJSON := pytestJSONReportAvailable(ctx, pytestPath, projectRoot)

	args := []string{}
	if useJSON {
		args = append(args, "--json-report", "--json-report-file=-", "-q")
	} else {
		args = append(args, "-q", "--tb=short", "--no-header")
	}

	if params.Filter != "" {
		args = append(args, "-k", params.Filter)
	}

	args = append(args, fmt.Sprintf("--timeout=%d", int(timeout.Seconds())))

	if params.Path != "" {
		args = append(args, params.Path)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout+10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, pytestPath, args...)
	cmd.Dir = projectRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run()

	var result testRunResult
	if useJSON {
		result = parsePytestJSON(stdout.String())
	} else {
		result = parsePytestShort(stdout.String())
	}

	if stderr.Len() > 0 && len(result.BuildErrors) == 0 && result.Passed == 0 && result.Failed == 0 {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			result.BuildErrors = append(result.BuildErrors, stderrStr)
		}
	}

	return &ToolResult{
		Success: true,
		Content: formatTestResult(result),
	}, nil
}

// pytestJSONReportAvailable checks if the pytest-json-report plugin is installed.
func pytestJSONReportAvailable(ctx context.Context, pytestPath, projectRoot string) bool {
	cmd := exec.CommandContext(ctx, pytestPath, "--co", "--json-report", "--json-report-file=/dev/null", "-q")
	cmd.Dir = projectRoot
	err := cmd.Run()
	return err == nil
}
```

- [ ] **Step 5: Run all Python parser tests**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run "TestParsePytest" -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
cd /home/gernsback/source/sodoryard
git add internal/tool/test_run_parse.go internal/tool/test_run_parse_test.go internal/tool/test_run.go
git commit -m "feat(tool): add Python/pytest parser for test_run tool"
```

---

### Task 6: `test_run` — TypeScript Parser

**Files:**
- Modify: `internal/tool/test_run_parse.go`
- Modify: `internal/tool/test_run_parse_test.go`
- Modify: `internal/tool/test_run.go` (replace stub)

- [ ] **Step 1: Write the failing test — Jest/Vitest JSON parser**

```go
// Append to internal/tool/test_run_parse_test.go

func TestParseJestJSON_AllPass(t *testing.T) {
	input := `{
		"numPassedTests": 5,
		"numFailedTests": 0,
		"numPendingTests": 1,
		"testResults": []
	}`

	result := parseJestJSON(input)
	if result.Passed != 5 {
		t.Fatalf("expected 5 passed, got %d", result.Passed)
	}
	if result.Failed != 0 {
		t.Fatalf("expected 0 failed, got %d", result.Failed)
	}
	if result.Skipped != 1 {
		t.Fatalf("expected 1 skipped, got %d", result.Skipped)
	}
}

func TestParseJestJSON_WithFailure(t *testing.T) {
	input := `{
		"numPassedTests": 2,
		"numFailedTests": 1,
		"numPendingTests": 0,
		"testResults": [
			{
				"testFilePath": "/app/src/components/Button.test.tsx",
				"testResults": [
					{
						"fullName": "Button renders correctly",
						"status": "passed"
					},
					{
						"fullName": "Button handles click",
						"status": "failed",
						"failureMessages": ["Expected: 1\nReceived: 0"]
					}
				]
			}
		]
	}`

	result := parseJestJSON(input)
	if result.Failed != 1 {
		t.Fatalf("expected 1 failed, got %d", result.Failed)
	}
	if len(result.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(result.Failures))
	}
	f := result.Failures[0]
	if f.Test != "Button handles click" {
		t.Fatalf("expected test name, got: %s", f.Test)
	}
	if !strings.Contains(f.Output, "Expected: 1") {
		t.Fatalf("expected failure output, got: %s", f.Output)
	}
	if !strings.Contains(f.File, "Button.test.tsx") {
		t.Fatalf("expected file path, got: %s", f.File)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run "TestParseJest" -v`
Expected: FAIL — `parseJestJSON` undefined

- [ ] **Step 3: Write TypeScript/Jest parser**

Add to `internal/tool/test_run_parse.go`:

```go
// --- TypeScript/Jest/Vitest parser ---

type jestReport struct {
	NumPassedTests  int              `json:"numPassedTests"`
	NumFailedTests  int              `json:"numFailedTests"`
	NumPendingTests int              `json:"numPendingTests"`
	TestResults     []jestTestSuite  `json:"testResults"`
}

type jestTestSuite struct {
	TestFilePath string           `json:"testFilePath"`
	TestResults  []jestTestResult `json:"testResults"`
}

type jestTestResult struct {
	FullName        string   `json:"fullName"`
	Status          string   `json:"status"`
	FailureMessages []string `json:"failureMessages"`
}

func parseJestJSON(raw string) testRunResult {
	result := testRunResult{Ecosystem: "typescript"}

	var report jestReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		result.BuildErrors = append(result.BuildErrors, fmt.Sprintf("Failed to parse Jest JSON: %v", err))
		return result
	}

	result.Passed = report.NumPassedTests
	result.Failed = report.NumFailedTests
	result.Skipped = report.NumPendingTests

	for _, suite := range report.TestResults {
		for _, test := range suite.TestResults {
			if test.Status == "failed" {
				output := strings.Join(test.FailureMessages, "\n")
				result.Failures = append(result.Failures, testFailure{
					Test:   test.FullName,
					File:   suite.TestFilePath,
					Output: output,
				})
			}
		}
	}

	total := result.Passed + result.Failed + result.Skipped
	result.Summary = formatTestSummary("typescript", total, result.Passed, result.Failed, result.Skipped)
	return result
}
```

- [ ] **Step 4: Write the `runTypeScriptTests` implementation**

Replace the stub in `internal/tool/test_run.go`:

```go
func runTypeScriptTests(ctx context.Context, projectRoot string, params testRunInput, timeout time.Duration) (*ToolResult, error) {
	// Detect test runner: vitest (check for vitest.config.*) or jest (default).
	runner, runnerArgs := detectTSTestRunner(projectRoot)

	npxPath, err := lookupCommandPath("npx")
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: "npx is required but not found in PATH. Install Node.js.",
			Error:   "npx not found",
		}, nil
	}

	args := append([]string{runner}, runnerArgs...)
	args = append(args, "--json")

	if runner == "vitest" {
		args = append(args, "run") // vitest needs "run" for non-watch mode
	}

	if params.Filter != "" {
		args = append(args, "--testNamePattern", params.Filter)
	}
	if params.Path != "" {
		args = append(args, params.Path)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout+10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, npxPath, args...)
	cmd.Dir = projectRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run()

	result := parseJestJSON(stdout.String())

	if stderr.Len() > 0 && len(result.BuildErrors) == 0 && result.Passed == 0 && result.Failed == 0 {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			result.BuildErrors = append(result.BuildErrors, stderrStr)
		}
	}

	return &ToolResult{
		Success: true,
		Content: formatTestResult(result),
	}, nil
}

// detectTSTestRunner checks for vitest config files, falls back to jest.
func detectTSTestRunner(projectRoot string) (runner string, args []string) {
	vitestConfigs := []string{"vitest.config.ts", "vitest.config.js", "vitest.config.mts"}
	for _, cfg := range vitestConfigs {
		if fileExists(filepath.Join(projectRoot, cfg)) {
			return "vitest", nil
		}
	}
	return "jest", []string{"--forceExit"}
}
```

- [ ] **Step 5: Run all TypeScript parser tests**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run "TestParseJest" -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
cd /home/gernsback/source/sodoryard
git add internal/tool/test_run_parse.go internal/tool/test_run_parse_test.go internal/tool/test_run.go
git commit -m "feat(tool): add TypeScript/Jest parser for test_run tool"
```

---

### Task 7: `db_sqlc` Tool

**Files:**
- Create: `internal/tool/db_sqlc.go`
- Create: `internal/tool/db_sqlc_test.go`

- [ ] **Step 1: Write the failing test — schema and basic structure**

```go
// internal/tool/db_sqlc_test.go
package tool

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireSqlc(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sqlc"); err != nil {
		t.Skip("sqlc not found in PATH, skipping db_sqlc tests")
	}
}

func TestDbSqlcSchema(t *testing.T) {
	schema := DbSqlc{}.Schema()
	if !json.Valid(schema) {
		t.Fatal("Schema() is not valid JSON")
	}
	if !strings.Contains(string(schema), "db_sqlc") {
		t.Fatal("Schema() does not contain tool name")
	}
}

func TestDbSqlcPurity(t *testing.T) {
	// generate is mutating, but vet and diff are pure.
	// Since tool purity is a single value, and generate mutates, tool is Mutating.
	if DbSqlc{}.ToolPurity() != Mutating {
		t.Fatal("expected Mutating purity")
	}
}

func TestDbSqlcNoSqlcYaml(t *testing.T) {
	dir := t.TempDir()

	result, err := DbSqlc{}.Execute(context.Background(), dir, json.RawMessage(`{"action":"vet"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure when no sqlc.yaml found")
	}
	if !strings.Contains(result.Content, "sqlc.yaml") && !strings.Contains(result.Content, "sqlc.yml") {
		t.Fatalf("expected helpful message about missing config, got: %s", result.Content)
	}
}

func TestDbSqlcVetSuccess(t *testing.T) {
	requireSqlc(t)
	dir := setupSqlcProject(t)

	result, err := DbSqlc{}.Execute(context.Background(), dir, json.RawMessage(`{"action":"vet"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
}

func TestDbSqlcDiff(t *testing.T) {
	requireSqlc(t)
	dir := setupSqlcProject(t)

	result, err := DbSqlc{}.Execute(context.Background(), dir, json.RawMessage(`{"action":"diff"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
}

// setupSqlcProject creates a minimal sqlc project with schema and query.
func setupSqlcProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// sqlc.yaml
	os.WriteFile(filepath.Join(dir, "sqlc.yaml"), []byte(`version: "2"
sql:
  - engine: "sqlite"
    queries: "query.sql"
    schema: "schema.sql"
    gen:
      go:
        package: "db"
        out: "db"
`), 0o644)

	// schema.sql
	os.WriteFile(filepath.Join(dir, "schema.sql"), []byte(`CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL
);
`), 0o644)

	// query.sql
	os.WriteFile(filepath.Join(dir, "query.sql"), []byte(`-- name: GetUser :one
SELECT id, name FROM users WHERE id = ?;
`), 0o644)

	// Run generate to create initial state.
	cmd := exec.Command("sqlc", "generate")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sqlc generate setup failed: %v\n%s", err, out)
	}

	return dir
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run TestDbSqlc -v`
Expected: FAIL — `DbSqlc` undefined

- [ ] **Step 3: Write implementation**

```go
// internal/tool/db_sqlc.go
package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DbSqlc implements the db_sqlc tool — runs sqlc generate, vet, or diff
// with structured error output.
type DbSqlc struct{}

type dbSqlcInput struct {
	Action string `json:"action,omitempty"`
	Path   string `json:"path,omitempty"`
}

func (DbSqlc) Name() string        { return "db_sqlc" }
func (DbSqlc) Description() string { return "Run sqlc generate, vet, or diff for SQL/Go codegen" }
func (DbSqlc) ToolPurity() Purity  { return Mutating }

func (DbSqlc) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "db_sqlc",
		"description": "Run sqlc operations for SQL-to-Go code generation. Actions: generate (regenerate Go from SQL), vet (lint queries against schema), diff (show what's out of sync).",
		"input_schema": {
			"type": "object",
			"properties": {
				"action": {
					"type": "string",
					"enum": ["generate", "vet", "diff"],
					"description": "Action to perform (default: generate)"
				},
				"path": {
					"type": "string",
					"description": "Subdirectory containing sqlc.yaml (if not project root)"
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

	action := params.Action
	if action == "" {
		action = "generate"
	}

	if action != "generate" && action != "vet" && action != "diff" {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Invalid action: %q. Supported: generate, vet, diff.", action),
			Error:   "invalid action",
		}, nil
	}

	sqlcPath, err := lookupCommandPath("sqlc")
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: "sqlc is required but not found in PATH. Install: https://docs.sqlc.dev/en/stable/overview/install.html",
			Error:   "sqlc not found",
		}, nil
	}

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

	// Check for sqlc config.
	if !fileExists(filepath.Join(workDir, "sqlc.yaml")) && !fileExists(filepath.Join(workDir, "sqlc.yml")) && !fileExists(filepath.Join(workDir, "sqlc.json")) {
		hint := workDir
		if params.Path != "" {
			hint = params.Path
		} else {
			hint = "project root"
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("No sqlc.yaml/sqlc.yml/sqlc.json found in %s. Create a sqlc config or use the path parameter to point to the directory containing it.", hint),
			Error:   "no sqlc config",
		}, nil
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, sqlcPath, action)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	output := strings.TrimSpace(stdout.String())
	errOutput := strings.TrimSpace(stderr.String())

	switch action {
	case "generate":
		if runErr != nil {
			combined := output
			if errOutput != "" {
				if combined != "" {
					combined += "\n"
				}
				combined += errOutput
			}
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("sqlc generate failed:\n%s", combined),
				Error:   "generate failed",
			}, nil
		}
		msg := "sqlc generate: success"
		if output != "" {
			msg += "\n" + output
		}
		return &ToolResult{Success: true, Content: msg}, nil

	case "vet":
		if runErr != nil {
			combined := output
			if errOutput != "" {
				if combined != "" {
					combined += "\n"
				}
				combined += errOutput
			}
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("sqlc vet found issues:\n%s", combined),
				Error:   "vet issues",
			}, nil
		}
		return &ToolResult{Success: true, Content: "sqlc vet: no issues found"}, nil

	case "diff":
		if runErr != nil {
			// diff exits non-zero when there are differences.
			combined := output
			if errOutput != "" {
				if combined != "" {
					combined += "\n"
				}
				combined += errOutput
			}
			if combined == "" {
				combined = "sqlc diff exited with an error but produced no output"
			}
			return &ToolResult{
				Success: true, // Differences aren't a tool failure.
				Content: fmt.Sprintf("sqlc diff: out of sync\n%s", combined),
			}, nil
		}
		if output == "" {
			return &ToolResult{Success: true, Content: "sqlc diff: everything in sync"}, nil
		}
		return &ToolResult{Success: true, Content: fmt.Sprintf("sqlc diff:\n%s", output)}, nil
	}

	return &ToolResult{Success: false, Content: "unreachable", Error: "unreachable"}, nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run TestDbSqlc -v`
Expected: All PASS (tests requiring sqlc will skip if not installed)

- [ ] **Step 5: Commit**

```bash
cd /home/gernsback/source/sodoryard
git add internal/tool/db_sqlc.go internal/tool/db_sqlc_test.go
git commit -m "feat(tool): add db_sqlc tool for sqlc generate/vet/diff"
```

---

### Task 8: RTK Shell Integration

**Files:**
- Modify: `internal/tool/shell.go`
- Modify: `internal/tool/shell_test.go`

- [ ] **Step 1: Write the failing test**

```go
// Append to internal/tool/shell_test.go

func TestShellRTKPrefix(t *testing.T) {
	// Test that when RTK is available, commands get prefixed.
	// We can't test the actual rtk binary, but we can test the prefix logic.
	result := applyRTKPrefix("git status", true)
	if result != "rtk git status" {
		t.Fatalf("expected 'rtk git status', got %q", result)
	}
}

func TestShellRTKPrefixSkipsWhenUnavailable(t *testing.T) {
	result := applyRTKPrefix("git status", false)
	if result != "git status" {
		t.Fatalf("expected unchanged 'git status', got %q", result)
	}
}

func TestShellRTKPrefixSkipsRTKCommands(t *testing.T) {
	// Don't double-prefix if the command already starts with rtk.
	result := applyRTKPrefix("rtk git status", true)
	if result != "rtk git status" {
		t.Fatalf("expected unchanged 'rtk git status', got %q", result)
	}
}

func TestShellRTKPrefixSkipsInternalCommands(t *testing.T) {
	// Some commands shouldn't go through rtk (e.g., cd, export, source).
	result := applyRTKPrefix("cd /tmp && ls", true)
	if result != "cd /tmp && ls" {
		t.Fatalf("expected unchanged command, got %q", result)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run TestShellRTK -v`
Expected: FAIL — `applyRTKPrefix` undefined

- [ ] **Step 3: Write RTK prefix logic**

Add to `internal/tool/shell.go`:

```go
// rtkSkipPrefixes are shell builtins and commands that should not be prefixed
// with rtk because they're shell-internal or rtk meta-commands.
var rtkSkipPrefixes = []string{
	"rtk", "cd ", "export ", "source ", ".", "eval ",
}

// applyRTKPrefix prepends "rtk " to a command when rtk is available and
// the command is suitable for proxying. This is transparent to the agent.
func applyRTKPrefix(command string, rtkAvailable bool) string {
	if !rtkAvailable {
		return command
	}
	trimmed := strings.TrimSpace(command)
	for _, prefix := range rtkSkipPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return command
		}
	}
	return "rtk " + command
}
```

- [ ] **Step 4: Integrate into Shell.Execute**

Modify `Shell.Execute` in `internal/tool/shell.go`. Add the RTK check after denylist check and before command execution. Add an `rtkAvailable` field computed once at construction:

Add to `Shell` struct:

```go
type Shell struct {
	config       ShellConfig
	rtkAvailable bool
}

func NewShell(config ShellConfig) *Shell {
	_, err := exec.LookPath("rtk")
	return &Shell{
		config:       config,
		rtkAvailable: err == nil,
	}
}
```

In `Execute`, right before the `cmd := exec.Command("sh", "-c", params.Command)` line, add:

```go
	// Apply RTK prefix for token compression when available.
	execCommand := applyRTKPrefix(params.Command, s.rtkAvailable)
```

Then change the exec line to use `execCommand`:

```go
	cmd := exec.Command("sh", "-c", execCommand)
```

- [ ] **Step 5: Run all shell tests (existing + new)**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run TestShell -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
cd /home/gernsback/source/sodoryard
git add internal/tool/shell.go internal/tool/shell_test.go
git commit -m "feat(tool): auto-prefix shell commands with rtk when available"
```

---

### Task 9: Registration Wiring and Truncation Notices

**Files:**
- Modify: `internal/tool/register.go`
- Modify: `internal/tool/truncate.go`
- Modify: `cmd/tidmouth/serve.go`
- Modify: `cmd/tidmouth/run.go`

- [ ] **Step 1: Add registration functions for test_run and db_sqlc**

Add to `internal/tool/register.go`:

```go
// RegisterTestTool registers the test_run tool in the given registry.
func RegisterTestTool(r *Registry) {
	r.Register(TestRun{})
}

// RegisterSqlcTool registers the db_sqlc tool in the given registry.
func RegisterSqlcTool(r *Registry) {
	r.Register(DbSqlc{})
}
```

- [ ] **Step 2: Add truncation notices for new tools**

Add cases to the `truncationNotice` function in `internal/tool/truncate.go`:

```go
	case "list_directory":
		return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Use a deeper path or reduce depth to narrow results.]", shownLines, totalLines)
	case "find_files":
		return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Use a more specific pattern or path to narrow results.]", shownLines, totalLines)
	case "test_run":
		return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Use a path or filter to run fewer tests.]", shownLines, totalLines)
	case "db_sqlc":
		return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Use the path parameter to scope to a specific sqlc project.]", shownLines, totalLines)
```

- [ ] **Step 3: Wire into serve.go**

In `cmd/tidmouth/serve.go`, after the existing `tool.RegisterSearchTools(registry, runtimeBundle.SemanticSearcher)` line (~line 83), add:

```go
	tool.RegisterDirectoryTools(registry)
	tool.RegisterTestTool(registry)
	tool.RegisterSqlcTool(registry)
```

- [ ] **Step 4: Wire into run.go**

Find the equivalent registration block in `cmd/tidmouth/run.go` and add the same three lines after `RegisterSearchTools`.

- [ ] **Step 5: Write registration tests**

```go
// Append to existing register tests or add to a test file

func TestRegisterTestTool(t *testing.T) {
	reg := NewRegistry()
	RegisterTestTool(reg)
	if _, ok := reg.Get("test_run"); !ok {
		t.Fatal("test_run not registered")
	}
}

func TestRegisterSqlcTool(t *testing.T) {
	reg := NewRegistry()
	RegisterSqlcTool(reg)
	if _, ok := reg.Get("db_sqlc"); !ok {
		t.Fatal("db_sqlc not registered")
	}
}
```

- [ ] **Step 6: Run full test suite to verify nothing is broken**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -v -timeout 120s`
Expected: All PASS

- [ ] **Step 7: Verify build compiles**

Run: `cd /home/gernsback/source/sodoryard && rtk go build ./cmd/tidmouth/`
Expected: Clean build, no errors

- [ ] **Step 8: Commit**

```bash
cd /home/gernsback/source/sodoryard
git add internal/tool/register.go internal/tool/truncate.go cmd/tidmouth/serve.go cmd/tidmouth/run.go
git commit -m "feat(tool): wire new tools into registry and command entry points"
```

---

### Task 10: Final Integration Verification

**Files:** None modified — verification only.

- [ ] **Step 1: Run full tool package tests**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -v -timeout 120s -count=1`
Expected: All PASS

- [ ] **Step 2: Run full project build**

Run: `cd /home/gernsback/source/sodoryard && rtk go build ./...`
Expected: Clean build

- [ ] **Step 3: Verify tool definitions are generated correctly**

Write a quick verification test or use an existing one to confirm all new tools appear in `registry.ToolDefinitions()` output:

```go
func TestAllNewToolsRegistered(t *testing.T) {
	reg := NewRegistry()
	RegisterFileTools(reg)
	RegisterGitTools(reg)
	RegisterShellTool(reg, ShellConfig{})
	RegisterSearchTools(reg, nil)
	RegisterDirectoryTools(reg)
	RegisterTestTool(reg)
	RegisterSqlcTool(reg)

	expected := []string{
		"list_directory", "find_files",
		"test_run", "db_sqlc",
	}
	for _, name := range expected {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("tool %q not registered", name)
		}
	}

	defs := reg.ToolDefinitions()
	defNames := make(map[string]bool)
	for _, d := range defs {
		defNames[d.Name] = true
	}
	for _, name := range expected {
		if !defNames[name] {
			t.Fatalf("tool %q not in ToolDefinitions()", name)
		}
	}
}
```

- [ ] **Step 4: Run verification test**

Run: `cd /home/gernsback/source/sodoryard && rtk go test ./internal/tool/ -run TestAllNewToolsRegistered -v`
Expected: PASS

- [ ] **Step 5: Final commit with verification test**

```bash
cd /home/gernsback/source/sodoryard
git add internal/tool/
git commit -m "test(tool): add integration verification for all new tools"
```
