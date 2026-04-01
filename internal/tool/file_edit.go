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
			Content: "old_str cannot be empty",
			Error:   "empty old_str",
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
			Content: "String not found in file. Check for typos or whitespace differences.",
			Error:   "no match",
		}, nil
	case 1:
		// Exactly one match — proceed with replacement.
	default:
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("String appears %d times in the file. Provide a longer, more unique search string that includes surrounding context.", count),
			Error:   "multiple matches",
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
