package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ponchione/sirtopham/internal/brain"
	"github.com/ponchione/sirtopham/internal/config"
)

// BrainWrite implements the brain_write tool — create or overwrite a brain
// document in the Obsidian vault.
type BrainWrite struct {
	client brain.Backend
	config config.BrainConfig
}

// NewBrainWrite creates a brain_write tool backed by the given brain backend.
func NewBrainWrite(client brain.Backend, cfg config.BrainConfig) *BrainWrite {
	return &BrainWrite{client: client, config: cfg}
}

type brainWriteInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (b *BrainWrite) Name() string { return "brain_write" }
func (b *BrainWrite) Description() string {
	return "Create or overwrite a brain document in the Obsidian vault"
}
func (b *BrainWrite) ToolPurity() Purity { return Mutating }

func (b *BrainWrite) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "brain_write",
		"description": "Create a new document or overwrite an existing one in the project brain (Obsidian vault). Content should be Obsidian-native markdown with YAML frontmatter, [[wikilinks]], and #tags.",
		"input_schema": {
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "Vault-relative path for the document (e.g., 'debugging/auth-race.md')"
				},
				"content": {
					"type": "string",
					"description": "Full markdown content including YAML frontmatter. Should start with '---' frontmatter block."
				}
			},
			"required": ["path", "content"]
		}
	}`)
}

func (b *BrainWrite) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	if !b.config.Enabled {
		return &ToolResult{
			Success: false,
			Content: "Project brain is not configured. See the project's YAML config brain section.",
		}, nil
	}

	var params brainWriteInput
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Invalid input: %v", err),
			Error:   err.Error(),
		}, nil
	}

	if params.Path == "" {
		return &ToolResult{
			Success: false,
			Content: "path is required",
			Error:   "empty path",
		}, nil
	}
	if params.Content == "" {
		return &ToolResult{
			Success: false,
			Content: "content is required",
			Error:   "empty content",
		}, nil
	}

	// Warn if no frontmatter.
	if !strings.HasPrefix(strings.TrimSpace(params.Content), "---") {
		slog.Warn("brain_write: document written without YAML frontmatter", "path", params.Path)
	}

	if err := b.client.WriteDocument(ctx, params.Path, params.Content); err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to write brain document: %v", err),
			Error:   err.Error(),
		}, nil
	}

	if b.config.LogBrainOperations {
		if err := appendBrainLog(ctx, b.client, BrainLogEntry{
			Timestamp: time.Now().UTC(),
			Operation: "write",
			Target:    params.Path,
			Summary:   fmt.Sprintf("Wrote brain document: %s", params.Path),
			Session:   sessionIDFromContext(ctx),
		}); err != nil {
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("Brain document written but failed to append operation log: %v", err),
				Error:   err.Error(),
			}, nil
		}
	}

	return &ToolResult{
		Success: true,
		Content: fmt.Sprintf("Wrote brain document: %s\n\n%s", params.Path, brainIndexStaleReminder()),
	}, nil
}
