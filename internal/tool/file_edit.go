package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// FileEdit implements the file_edit tool — search-and-replace within a file,
// validating that the search string appears exactly once.
type FileEdit struct {
	store readStateStore
}

func NewFileEdit(store readStateStore) FileEdit {
	return FileEdit{store: store}
}

type fileEditInput struct {
	Path   string `json:"path"`
	OldStr string `json:"old_str"`
	NewStr string `json:"new_str"`
}

func (FileEdit) Name() string        { return "file_edit" }
func (FileEdit) Description() string { return "Search and replace a unique string in a file" }
func (FileEdit) ToolPurity() Purity  { return Mutating }

func (FileEdit) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "file_edit",
		"description": "Search for a unique string in a file and replace it. The old_str must appear exactly once. Returns a unified diff of the change.",
		"input_schema": {
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "File path relative to the project root"
				},
				"old_str": {
					"type": "string",
					"description": "Exact string to find (must appear exactly once in the file)"
				},
				"new_str": {
					"type": "string",
					"description": "Replacement string"
				}
			},
			"required": ["path", "old_str", "new_str"]
		}
	}`)
}

func filePreview(content string, maxLines int) string {
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return "(empty file)"
	}
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

type candidateMatch struct {
	line    int
	snippet string
}

func snippetFromLineWindow(lines []string, startLine, maxLines int) string {
	if len(lines) == 0 {
		return ""
	}
	if startLine < 1 {
		startLine = 1
	}
	if maxLines <= 0 {
		maxLines = 1
	}
	startIdx := startLine - 1
	if startIdx >= len(lines) {
		startIdx = len(lines) - 1
	}
	endIdx := startIdx + maxLines
	if endIdx > len(lines) {
		endIdx = len(lines)
	}
	return strings.Join(lines[startIdx:endIdx], "\\n")
}

func candidateMatches(content, needle string) []candidateMatch {
	if needle == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	matches := make([]candidateMatch, 0)
	needleLineCount := strings.Count(needle, "\n") + 1
	for i, line := range lines {
		if strings.Contains(line, needle) {
			matches = append(matches, candidateMatch{line: i + 1, snippet: line})
		}
	}
	if len(matches) > 0 {
		return matches
	}
	for i := 0; i < len(content); {
		idx := strings.Index(content[i:], needle)
		if idx < 0 {
			break
		}
		pos := i + idx
		line := 1 + strings.Count(content[:pos], "\n")
		matches = append(matches, candidateMatch{line: line, snippet: snippetFromLineWindow(lines, line, min(needleLineCount+1, 3))})
		i = pos + len(needle)
	}
	return matches
}

func candidateLineSummary(content, needle string, maxCandidates int) string {
	matches := candidateMatches(content, needle)
	if len(matches) == 0 {
		return "unknown"
	}
	lines := make([]int, 0, len(matches))
	for _, match := range matches {
		lines = append(lines, match.line)
	}
	sort.Ints(lines)
	parts := make([]string, 0, min(len(lines), maxCandidates))
	for _, line := range lines {
		if len(parts) >= maxCandidates {
			break
		}
		parts = append(parts, fmt.Sprintf("line %d", line))
	}
	if len(lines) > maxCandidates {
		parts = append(parts, fmt.Sprintf("... (%d total)", len(lines)))
	}
	return strings.Join(parts, ", ")
}

func candidateSnippetSummary(content, needle string, maxCandidates int) string {
	matches := candidateMatches(content, needle)
	if len(matches) == 0 {
		return "unknown"
	}
	parts := make([]string, 0, min(len(matches), maxCandidates))
	for _, match := range matches {
		if len(parts) >= maxCandidates {
			break
		}
		parts = append(parts, fmt.Sprintf("line %d: %s", match.line, match.snippet))
	}
	if len(matches) > maxCandidates {
		parts = append(parts, fmt.Sprintf("... (%d total)", len(matches)))
	}
	return strings.Join(parts, " | ")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (f FileEdit) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	var params fileEditInput
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Invalid input: %v", err),
			Error:   err.Error(),
		}, nil
	}

	if params.OldStr == "" {
		return &ToolResult{
			Success: false,
			Content: "file_edit cannot create or append content with an empty old_str. Read the file first and provide an exact match to replace, or use file_write to create/overwrite the file.",
			Error:   "invalid_create_via_edit",
		}, nil
	}

	absPath, err := resolvePath(projectRoot, params.Path)
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: err.Error(),
			Error:   err.Error(),
		}, nil
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			msg := fileNotFoundError(projectRoot, params.Path)
			return &ToolResult{Success: false, Content: msg, Error: "file not found"}, nil
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Error reading file: %v", err),
			Error:   err.Error(),
		}, nil
	}

	store := f.store
	if store == nil {
		store = defaultReadStateStore
	}
	scopeKey := readScopeKey(ctx)
	snapshot, ok, err := store.Get(ctx, scopeKey, absPath)
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to load read state: %v", err),
			Error:   err.Error(),
		}, nil
	}
	if !ok || snapshot.Kind != readKindFull {
		return notReadFirstError(params.Path), nil
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			msg := fileNotFoundError(projectRoot, params.Path)
			return &ToolResult{Success: false, Content: msg, Error: "file not found"}, nil
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Error stating file: %v", err),
			Error:   err.Error(),
		}, nil
	}
	currentFingerprint := fileFingerprint(data)
	if snapshot.Fingerprint != currentFingerprint || snapshot.SizeBytes != info.Size() || !snapshot.MTime.Equal(info.ModTime()) {
		_ = store.Clear(ctx, scopeKey, absPath)
		return staleReadError(params.Path), nil
	}

	oldContent := string(data)
	count := strings.Count(oldContent, params.OldStr)

	switch count {
	case 0:
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("String not found in file. Check for typos, whitespace differences, or refresh with a full file_read before retrying file_edit.\nPreview:\n%s", filePreview(oldContent, 3)),
			Error:   "zero_match",
		}, nil
	case 1:
		// Exactly one match — proceed with replacement.
	default:
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("String appears %d times in the file. Provide a longer, more unique search string that includes surrounding context from the full file_read.\nCandidate lines: %s\nCandidate snippets: %s", count, candidateLineSummary(oldContent, params.OldStr, 5), candidateSnippetSummary(oldContent, params.OldStr, 3)),
			Error:   "multiple_matches",
		}, nil
	}

	// Perform the replacement.
	newContent := strings.Replace(oldContent, params.OldStr, params.NewStr, 1)

	// Write the file.
	perm := os.FileMode(0o644)
	if info != nil {
		perm = info.Mode()
	}
	latestInfo, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			msg := fileNotFoundError(projectRoot, params.Path)
			return &ToolResult{Success: false, Content: msg, Error: "file not found"}, nil
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Error stating file before write: %v", err),
			Error:   err.Error(),
		}, nil
	}
	latestData, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			msg := fileNotFoundError(projectRoot, params.Path)
			return &ToolResult{Success: false, Content: msg, Error: "file not found"}, nil
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Error reading file before write: %v", err),
			Error:   err.Error(),
		}, nil
	}
	if snapshot.Fingerprint != fileFingerprint(latestData) || snapshot.SizeBytes != latestInfo.Size() || !snapshot.MTime.Equal(latestInfo.ModTime()) {
		_ = store.Clear(ctx, scopeKey, absPath)
		return staleReadError(params.Path), nil
	}
	if err := os.WriteFile(absPath, []byte(newContent), perm); err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to write file: %v", err),
			Error:   err.Error(),
		}, nil
	}
	_ = store.Clear(ctx, scopeKey, absPath)

	// Generate diff.
	diff := unifiedDiff("a/"+params.Path, "b/"+params.Path, oldContent, newContent, 3)
	if diff == "" {
		return &ToolResult{
			Success: true,
			Content: fmt.Sprintf("File edited: %s (replacement identical to original)", params.Path),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Content: diff,
	}, nil
}
