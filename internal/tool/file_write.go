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
type FileWrite struct{}

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

func (FileWrite) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
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

	// Create parent directories.
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to create directories: %v", err),
			Error:   err.Error(),
		}, nil
	}

	// Check if file exists for diff generation.
	var oldContent string
	isNew := true
	if existing, err := os.ReadFile(absPath); err == nil {
		oldContent = string(existing)
		isNew = false
	}

	// Write atomically: write to temp file in same dir, then rename.
	tmpFile, err := os.CreateTemp(dir, ".sirtopham-write-*")
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
			os.Chmod(tmpPath, info.Mode())
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
