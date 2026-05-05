package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	brainindexstate "github.com/ponchione/sodoryard/internal/brain/indexstate"
	"github.com/ponchione/sodoryard/internal/config"
)

// BrainUpdate implements the brain_update tool — append, prepend, or replace
// a section of an existing brain document.
type BrainUpdate struct {
	client brain.Backend
	config config.BrainConfig
}

// NewBrainUpdate creates a brain_update tool backed by the given brain backend.
func NewBrainUpdate(client brain.Backend, cfg config.BrainConfig) *BrainUpdate {
	return &BrainUpdate{client: client, config: cfg}
}

type brainUpdateInput struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Content   string `json:"content"`
	Section   string `json:"section,omitempty"`
}

func (b *BrainUpdate) Name() string { return "brain_update" }
func (b *BrainUpdate) Description() string {
	return "Update an existing brain document: append, prepend, or replace a section"
}
func (b *BrainUpdate) ToolPurity() Purity { return Mutating }

func (b *BrainUpdate) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "brain_update",
		"description": "Modify an existing brain document by appending, prepending, or replacing a specific section. For replace_section, the heading level determines the scope — content under sub-headings within the section is included in the replacement.",
		"input_schema": {
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "Vault-relative path to the document"
				},
				"operation": {
					"type": "string",
					"enum": ["append", "prepend", "replace_section"],
					"description": "Operation to perform: 'append' adds to end, 'prepend' inserts after frontmatter, 'replace_section' replaces a heading's content"
				},
				"content": {
					"type": "string",
					"description": "Content to add or replace with"
				},
				"section": {
					"type": "string",
					"description": "Target heading for replace_section (e.g., '## Workaround'). Required when operation is 'replace_section'."
				}
			},
			"required": ["path", "operation", "content"]
		}
	}`)
}

func (b *BrainUpdate) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	if !b.config.Enabled {
		return brainDisabledResult(), nil
	}

	var params brainUpdateInput
	if err := json.Unmarshal(input, &params); err != nil {
		return invalidInputResult(err), nil
	}

	if result := validateBrainPath(params.Path); result != nil {
		return result, nil
	}
	if result := validateBrainContent(params.Content); result != nil {
		return result, nil
	}

	switch params.Operation {
	case "append", "prepend", "replace_section":
		// valid
	default:
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Invalid operation: '%s'. Must be one of: append, prepend, replace_section.", params.Operation),
			Error:   "invalid operation",
		}, nil
	}

	normalizedPath, err := ensureBrainWriteAllowed(b.config, params.Path)
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Invalid brain write path: %v", err),
			Error:   err.Error(),
		}, nil
	}
	params.Path = normalizedPath

	patchContent := params.Content
	if params.Operation == "replace_section" {
		if params.Section == "" {
			return &ToolResult{
				Success: false,
				Content: "The 'section' parameter is required for replace_section operation.",
				Error:   "missing section",
			}, nil
		}
		patchContent = params.Section + "\n\n" + params.Content
	}

	if err := b.client.PatchDocument(ctx, params.Path, params.Operation, patchContent); err != nil {
		errMsg := err.Error()
		if result := brainDocumentNotFoundResult(ctx, b.client, params.Path, errMsg); result != nil {
			return result, nil
		}
		if strings.Contains(errMsg, "Section '") && strings.Contains(errMsg, "Available headings:") {
			prefix := "Section '"
			sectionStart := strings.Index(errMsg, prefix)
			sectionEnd := strings.Index(errMsg, "' not found")
			headingListStart := strings.Index(errMsg, "Available headings:")
			if sectionStart >= 0 && sectionEnd > sectionStart && headingListStart >= 0 {
				section := errMsg[sectionStart+len(prefix) : sectionEnd]
				headingsRaw := strings.TrimSpace(errMsg[headingListStart+len("Available headings:"):])
				headings := []string{}
				if headingsRaw != "" {
					for _, heading := range strings.Split(headingsRaw, ",") {
						headings = append(headings, strings.TrimSpace(heading))
					}
				}
				return &ToolResult{
					Success: false,
					Content: fmt.Sprintf("Section not found in %s: %s\n\nAvailable headings:\n%s", params.Path, section, formatHeadingList(headings)),
					Error:   errMsg,
				}, nil
			}
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to update brain document: %v", err),
			Error:   errMsg,
		}, nil
	}

	updated, err := b.client.ReadDocument(ctx, params.Path)
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Brain document updated, but failed to read back final content: %v", err),
			Error:   err.Error(),
		}, nil
	}
	if b.config.Backend != "shunter" {
		if err := brainindexstate.MarkStale(projectRoot, "brain_update", time.Now().UTC()); err != nil {
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("Brain document updated but failed to record stale brain index state: %v", err),
				Error:   err.Error(),
			}, nil
		}
	}

	if b.config.LogBrainOperations {
		if err := appendBrainLog(ctx, b.client, BrainLogEntry{
			Timestamp: time.Now().UTC(),
			Operation: "update",
			Target:    params.Path,
			Summary:   fmt.Sprintf("Updated brain document via %s.", params.Operation),
			Session:   sessionIDFromContext(ctx),
		}); err != nil {
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("Brain document updated but failed to append operation log: %v", err),
				Error:   err.Error(),
			}, nil
		}
	}

	return &ToolResult{
		Success: true,
		Content: fmt.Sprintf("Updated brain document: %s (%s)\n\n%s\n\nContent preview:\n%s", params.Path, params.Operation, brainIndexStaleReminder(), formatBrainDocumentPreview(updated, 100)),
	}, nil
}
