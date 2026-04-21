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
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Invalid input: %v", err),
			Error:   err.Error(),
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

	store := f.store
	if store == nil {
		store = defaultReadStateStore
	}
	scopeKey := readScopeKey(ctx)

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
		if result := ensureFreshFullRead(ctx, store, scopeKey, absPath, params.Path, "file_write", []byte(oldContent)); result != nil {
			return result, nil
		}
	}

	// Write atomically: write to temp file in same dir, then rename.
	tmpFile, err := os.CreateTemp(dir, ".yard-write-*")
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to create temp file: %v", err),
			Error:   err.Error(),
		}, nil
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(params.Content); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to write file: %v", err),
			Error:   err.Error(),
		}, nil
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to close temp file: %v", err),
			Error:   err.Error(),
		}, nil
	}

	// Preserve permissions if overwriting.
	if !isNew {
		if info, err := os.Stat(absPath); err == nil {
			if err := os.Chmod(tmpPath, info.Mode()); err != nil {
				os.Remove(tmpPath)
				return &ToolResult{
					Success: false,
					Content: fmt.Sprintf("Failed to preserve file permissions: %v", err),
					Error:   err.Error(),
				}, nil
			}
		}
	}

	if requiresFreshRead {
		if result := ensureFreshFullRead(ctx, store, scopeKey, absPath, params.Path, "file_write", nil); result != nil {
			os.Remove(tmpPath)
			return result, nil
		}
	}

	if err := os.Rename(tmpPath, absPath); err != nil {
		os.Remove(tmpPath)
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to write file: %v", err),
			Error:   err.Error(),
		}, nil
	}
	if !isNew {
		_ = store.Clear(ctx, scopeKey, absPath)
	}

	// Generate response.
	if isNew {
		return &ToolResult{
			Success: true,
			Content: fmt.Sprintf("[new file created] %s (%d bytes)", params.Path, len(params.Content)),
		}, nil
	}

	// Generate diff for overwrites.
	diff := unifiedDiff("a/"+params.Path, "b/"+params.Path, oldContent, params.Content, 3)
	if diff == "" {
		return &ToolResult{
			Success: true,
			Content: fmt.Sprintf("File written: %s (no changes detected)", params.Path),
		}, nil
	}

	// Truncate diff if too long.
	diffLines := strings.Split(diff, "\n")
	if len(diffLines) > diffTruncateLines {
		truncated := strings.Join(diffLines[:diffTruncateLines], "\n")
		return &ToolResult{
			Success: true,
			Content: fmt.Sprintf("%s\n[diff truncated — showing %d of %d lines]", truncated, diffTruncateLines, len(diffLines)),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Content: diff,
	}, nil
}

func ensureFreshFullRead(ctx context.Context, store readStateStore, scopeKey, absPath, displayPath, toolName string, currentData []byte) *ToolResult {
	snapshot, ok, err := store.Get(ctx, scopeKey, absPath)
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to load read state: %v", err),
			Error:   err.Error(),
		}
	}
	if !ok || snapshot.Kind != readKindFull {
		return notReadFirstForToolError(toolName, displayPath)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ToolResult{Success: false, Content: fmt.Sprintf("File not found: %s", displayPath), Error: "file not found"}
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Error stating file: %v", err),
			Error:   err.Error(),
		}
	}
	data := currentData
	if data == nil {
		data, err = os.ReadFile(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return &ToolResult{Success: false, Content: fmt.Sprintf("File not found: %s", displayPath), Error: "file not found"}
			}
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("Error reading file: %v", err),
				Error:   err.Error(),
			}
		}
	}
	if snapshot.Fingerprint != fileFingerprint(data) || snapshot.SizeBytes != info.Size() || !snapshot.MTime.Equal(info.ModTime()) {
		_ = store.Clear(ctx, scopeKey, absPath)
		return staleWriteReadError(toolName, displayPath)
	}
	return nil
}
