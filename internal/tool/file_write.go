package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const diffTruncateLines = 50

// FileWrite implements the file_write tool — writes or overwrites a file
// with provided content, creating parent directories as needed.
type FileWrite struct {
	store readStateStore
}

func NewFileWrite(store readStateStore) FileWrite {
	return FileWrite{store: store}
}

type fileWriteInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (FileWrite) Name() string        { return "file_write" }
func (FileWrite) Description() string { return "Write or overwrite a file with provided content" }
func (FileWrite) ToolPurity() Purity  { return Mutating }

func (FileWrite) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "file_write",
		"description": "Write or overwrite a file. Creates parent directories if needed. Returns a diff for overwrites.",
		"input_schema": {
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "File path relative to the project root"
				},
				"content": {
					"type": "string",
					"description": "Complete file content to write"
				}
			},
			"required": ["path", "content"]
		}
	}`)
}

func (f FileWrite) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	var params fileWriteInput
	if err := json.Unmarshal(input, &params); err != nil {
		return invalidInputResult(err), nil
	}

	absPath, result := resolvePathResult(projectRoot, params.Path)
	if result != nil {
		return result, nil
	}

	store := mutableFileStore(f.store)

	// Create parent directories.
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to create directories: %v", err),
			Error:   err.Error(),
		}, nil
	}

	// Check if file exists for diff generation and stale-write protection.
	var oldContent string
	var mutationState mutableFileState
	isNew := true
	requiresFreshRead := false
	if existing, err := os.ReadFile(absPath); err == nil {
		oldContent = string(existing)
		isNew = false
		requiresFreshRead = len(existing) > 0
	} else if !os.IsNotExist(err) {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Error reading file: %v", err),
			Error:   err.Error(),
		}, nil
	}

	if requiresFreshRead {
		var result *ToolResult
		mutationState, result = loadFreshMutableFileState(ctx, projectRoot, store, absPath, params.Path, "file_write")
		if result != nil {
			return result, nil
		}
		oldContent = string(mutationState.data)
	}

	beforeRename := func() *ToolResult {
		if !requiresFreshRead {
			return nil
		}
		return verifyMutableFileSnapshotFresh(ctx, store, projectRoot, mutationState, params.Path, "file_write")
	}
	if result := writeStringAtomically(absPath, params.Content, !isNew, beforeRename); result != nil {
		return result, nil
	}
	if !isNew {
		clearMutableFileSnapshot(ctx, store, mutationState.scopeKey, mutationState.absPath)
	}

	// Generate response.
	if isNew {
		return &ToolResult{
			Success: true,
			Content: fmt.Sprintf("[new file created] %s (%d bytes)", params.Path, len(params.Content)),
			Details: newFileMutationDetails(fileMutationDetailFields("write", params.Path, true, true, "", 0, len(params.Content))),
		}, nil
	}

	return fileWriteOverwriteResult(params.Path, oldContent, params.Content), nil
}

func writeStringAtomically(absPath, content string, preservePermissions bool, beforeRename func() *ToolResult) *ToolResult {
	dir := filepath.Dir(absPath)
	tmpFile, err := os.CreateTemp(dir, ".yard-write-*")
	if err != nil {
		return failureResult(fmt.Sprintf("Failed to create temp file: %v", err), err.Error())
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return failureResult(fmt.Sprintf("Failed to write file: %v", err), err.Error())
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return failureResult(fmt.Sprintf("Failed to close temp file: %v", err), err.Error())
	}

	if preservePermissions {
		if info, err := os.Stat(absPath); err == nil {
			if err := os.Chmod(tmpPath, info.Mode()); err != nil {
				os.Remove(tmpPath)
				return failureResult(fmt.Sprintf("Failed to preserve file permissions: %v", err), err.Error())
			}
		}
	}

	if beforeRename != nil {
		if result := beforeRename(); result != nil {
			os.Remove(tmpPath)
			return result
		}
	}

	if err := os.Rename(tmpPath, absPath); err != nil {
		os.Remove(tmpPath)
		return failureResult(fmt.Sprintf("Failed to write file: %v", err), err.Error())
	}
	return nil
}

func fileWriteOverwriteResult(path, oldContent, newContent string) *ToolResult {
	diff := unifiedDiff("a/"+path, "b/"+path, oldContent, newContent, 3)
	details := fileMutationDetailFields("write", path, false, diff != "", diff, len(oldContent), len(newContent))
	if diff == "" {
		return &ToolResult{
			Success: true,
			Content: fmt.Sprintf("File written: %s (no changes detected)", path),
			Details: newFileMutationDetails(details),
		}
	}

	diffLines := strings.Split(diff, "\n")
	if len(diffLines) > diffTruncateLines {
		truncated := strings.Join(diffLines[:diffTruncateLines], "\n")
		details["diff_truncated"] = true
		return &ToolResult{
			Success: true,
			Content: fmt.Sprintf("%s\n[diff truncated — showing %d of %d lines]", truncated, diffTruncateLines, len(diffLines)),
			Details: newFileMutationDetails(details),
		}
	}

	return &ToolResult{
		Success: true,
		Content: diff,
		Details: newFileMutationDetails(details),
	}
}
