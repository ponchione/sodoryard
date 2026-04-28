package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

	store := mutableFileStore(f.store)
	state, result := loadMutableFileState(ctx, projectRoot, store, absPath, params.Path, "file_edit")
	if result != nil {
		return result, nil
	}
	if result := verifyMutableFileSnapshotFresh(ctx, store, projectRoot, state, params.Path, "file_edit"); result != nil {
		return result, nil
	}

	oldContent := string(state.data)
	info := state.info
	if params.OldStr == params.NewStr {
		return &ToolResult{
			Success: false,
			Content: "file_edit new_str is identical to old_str. Provide a different replacement string or skip the edit.",
			Error:   "old_equals_new",
		}, nil
	}
	analysis := analyzeFileEditMatches(oldContent, params.OldStr)

	switch analysis.count {
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
			Content: fmt.Sprintf("String appears %d times in the file. Provide a longer, more unique search string that includes surrounding context from the full file_read.\nCandidate lines: %s\nCandidate snippets: %s", analysis.count, analysis.candidateLines, analysis.candidateSnippets),
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
	if result := verifyMutableFileSnapshotFresh(ctx, store, projectRoot, state, params.Path, "file_edit"); result != nil {
		return result, nil
	}
	if err := os.WriteFile(absPath, []byte(newContent), perm); err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to write file: %v", err),
			Error:   err.Error(),
		}, nil
	}
	clearMutableFileSnapshot(ctx, store, state.scopeKey, state.absPath)

	// Generate diff.
	diff := unifiedDiff("a/"+params.Path, "b/"+params.Path, oldContent, newContent, 3)
	details := map[string]any{
		"operation":       "edit",
		"path":            params.Path,
		"created":         false,
		"changed":         diff != "",
		"diff_format":     "unified",
		"diff_line_count": detailLineCount(diff),
		"diff_truncated":  false,
		"bytes_before":    len(oldContent),
		"bytes_after":     len(newContent),
	}
	if line := firstChangedLine(oldContent, params.OldStr); line > 0 {
		details["first_changed_line"] = line
	}
	if diff == "" {
		return &ToolResult{
			Success: true,
			Content: fmt.Sprintf("File edited: %s (replacement identical to original)", params.Path),
			Details: newFileMutationDetails(details),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Content: diff,
		Details: newFileMutationDetails(details),
	}, nil
}
