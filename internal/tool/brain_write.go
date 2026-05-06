package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/config"
)

// BrainWrite implements the brain_write tool: create or overwrite a Shunter
// brain document.
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
	return "Create or overwrite a brain document in Shunter project memory"
}
func (b *BrainWrite) ToolPurity() Purity { return Mutating }

func (b *BrainWrite) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "brain_write",
		"description": "Create a new document or overwrite an existing one in the project brain. Content should be markdown with YAML frontmatter, [[wikilinks]], and #tags.",
		"input_schema": {
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "Brain document path (e.g., 'debugging/auth-race.md')"
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
		return brainDisabledResult(), nil
	}

	var params brainWriteInput
	if err := json.Unmarshal(input, &params); err != nil {
		return invalidInputResult(err), nil
	}

	normalizedPath, result := validateBrainMutationInput(b.config, params.Path, params.Content)
	if result != nil {
		return result, nil
	}
	params.Path = normalizedPath

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

	if result := finishBrainMutation(ctx, b.client, b.config, projectRoot, "brain_write", "written", "write", params.Path, fmt.Sprintf("Wrote brain document: %s", params.Path)); result != nil {
		return result, nil
	}

	return &ToolResult{
		Success: true,
		Content: fmt.Sprintf("Wrote brain document: %s\n\n%s", params.Path, brainIndexStaleReminder()),
	}, nil
}
