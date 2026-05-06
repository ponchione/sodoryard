package tool

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/config"
)

func brainDocumentNotFoundResult(ctx context.Context, client brain.Backend, path string, errMsg string) *ToolResult {
	if !strings.Contains(errMsg, "Document not found") {
		return nil
	}
	dir := filepath.Dir(path)
	files, listErr := client.ListDocuments(ctx, dir)
	if listErr != nil || len(files) == 0 {
		return nil
	}
	return &ToolResult{
		Success: false,
		Content: fmt.Sprintf("Document not found: %s\n\nAvailable documents in %s/:\n  %s", path, dir, strings.Join(files, "\n  ")),
		Error:   errMsg,
	}
}

func brainDisabledResult() *ToolResult {
	return failureResult(
		"Project brain is not configured. See the project's YAML config brain section.",
		"brain not configured",
	)
}

func validateBrainPath(path string) *ToolResult {
	if _, err := normalizeBrainDocumentPath(path); err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Invalid brain path: %v", err),
			Error:   err.Error(),
		}
	}
	return nil
}

func validateBrainContent(content string) *ToolResult {
	if content == "" {
		return requiredFieldResult("content")
	}
	return nil
}

func validateBrainMutationInput(cfg config.BrainConfig, path string, content string) (string, *ToolResult) {
	if result := validateBrainPath(path); result != nil {
		return "", result
	}
	if result := validateBrainContent(content); result != nil {
		return "", result
	}
	normalizedPath, err := ValidateBrainWritePath(cfg, path)
	if err != nil {
		return "", &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Invalid brain write path: %v", err),
			Error:   err.Error(),
		}
	}
	return normalizedPath, nil
}
