package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type ToolResultStore interface {
	PersistToolResult(ctx context.Context, toolUseID, toolName, content string) (string, error)
}

type FileToolResultStore struct {
	rootDir string
}

func NewFileToolResultStore(rootDir string) *FileToolResultStore {
	if rootDir == "" {
		rootDir = filepath.Join(os.TempDir(), "sirtopham-tool-results")
	}
	return &FileToolResultStore{rootDir: rootDir}
}

var unsafeToolResultFileChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func (s *FileToolResultStore) PersistToolResult(_ context.Context, toolUseID, toolName, content string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("tool result store is nil")
	}
	if err := os.MkdirAll(s.rootDir, 0o755); err != nil {
		return "", fmt.Errorf("create tool result store dir: %w", err)
	}
	fileName := fmt.Sprintf("%s-%s.txt", sanitizeToolResultFilePart(toolName), sanitizeToolResultFilePart(toolUseID))
	fullPath := filepath.Join(s.rootDir, fileName)
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write persisted tool result: %w", err)
	}
	return fullPath, nil
}

func sanitizeToolResultFilePart(part string) string {
	if part == "" {
		return "unknown"
	}
	safe := unsafeToolResultFileChars.ReplaceAllString(part, "-")
	if safe == "" {
		return "unknown"
	}
	return safe
}
