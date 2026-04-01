package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ponchione/sirtopham/internal/brain"
	"github.com/ponchione/sirtopham/internal/config"
)

var headingRegexp = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)

// BrainUpdate implements the brain_update tool — append, prepend, or replace
// a section of an existing brain document.
type BrainUpdate struct {
	client *brain.ObsidianClient
	config config.BrainConfig
}

// NewBrainUpdate creates a brain_update tool backed by the given Obsidian client.
func NewBrainUpdate(client *brain.ObsidianClient, cfg config.BrainConfig) *BrainUpdate {
	return &BrainUpdate{client: client, config: cfg}
}

type brainUpdateInput struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Content   string `json:"content"`
	Section   string `json:"section,omitempty"`
}

func (b *BrainUpdate) Name() string        { return "brain_update" }
func (b *BrainUpdate) Description() string { return "Update an existing brain document: append, prepend, or replace a section" }
func (b *BrainUpdate) ToolPurity() Purity  { return Mutating }

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
		return &ToolResult{
			Success: false,
			Content: "Project brain is not configured. See sirtopham.yaml brain section.",
		}, nil
	}

	var params brainUpdateInput
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

	// Read current document.
	current, err := b.client.ReadDocument(ctx, params.Path)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "Document not found") {
			dir := filepath.Dir(params.Path)
			files, listErr := b.client.ListDocuments(ctx, dir)
			if listErr == nil && len(files) > 0 {
				return &ToolResult{
					Success: false,
					Content: fmt.Sprintf("Document not found: %s\n\nAvailable documents in %s/:\n  %s",
						params.Path, dir, strings.Join(files, "\n  ")),
					Error: errMsg,
				}, nil
			}
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to read brain document: %v", err),
			Error:   errMsg,
		}, nil
	}

	var updated string
	switch params.Operation {
	case "append":
		updated = appendContent(current, params.Content)
	case "prepend":
		updated = prependContent(current, params.Content)
	case "replace_section":
		if params.Section == "" {
			return &ToolResult{
				Success: false,
				Content: "The 'section' parameter is required for replace_section operation.",
				Error:   "missing section",
			}, nil
		}
		var replaceErr error
		updated, replaceErr = replaceSectionContent(current, params.Section, params.Content)
		if replaceErr != nil {
			return &ToolResult{
				Success: false,
				Content: replaceErr.Error(),
				Error:   "section not found",
			}, nil
		}
	}

	// Write back.
	if err := b.client.WriteDocument(ctx, params.Path, updated); err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to write updated document: %v", err),
			Error:   err.Error(),
		}, nil
	}

	// Return first 100 lines.
	lines := strings.Split(updated, "\n")
	content := updated
	if len(lines) > 100 {
		content = strings.Join(lines[:100], "\n") + fmt.Sprintf("\n\n[showing first 100 of %d lines]", len(lines))
	}

	return &ToolResult{
		Success: true,
		Content: fmt.Sprintf("Brain document updated (%s): %s\n\n%s", params.Operation, params.Path, content),
	}, nil
}

// appendContent appends new content at the end with a blank line separator.
func appendContent(current, addition string) string {
	if !strings.HasSuffix(current, "\n") {
		current += "\n"
	}
	return current + "\n" + addition
}

// prependContent inserts new content after YAML frontmatter (if present)
// or at the very start.
func prependContent(current, addition string) string {
	if !strings.HasPrefix(current, "---") {
		return addition + "\n\n" + current
	}
	// Find end of frontmatter.
	rest := current[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return addition + "\n\n" + current
	}
	fmEnd := 3 + idx + 4 // "---" + content up to "\n---" + len("\n---")
	fm := current[:fmEnd]
	body := strings.TrimLeft(current[fmEnd:], "\n")
	return fm + "\n\n" + addition + "\n\n" + body
}

// replaceSectionContent replaces a heading's content up to the next heading
// of equal or higher level.
func replaceSectionContent(current, section, newContent string) (string, error) {
	lines := strings.Split(current, "\n")

	// Parse the target heading level.
	targetLevel, targetText := parseHeading(section)
	if targetLevel == 0 {
		// Treat as plain text heading match.
		targetLevel = 0
		targetText = section
	}

	// Find the target heading line.
	targetIdx := -1
	for i, line := range lines {
		level, text := parseHeading(line)
		if level == 0 {
			continue
		}
		if targetLevel > 0 && level == targetLevel && strings.TrimSpace(text) == strings.TrimSpace(targetText) {
			targetIdx = i
			break
		}
		if targetLevel == 0 && strings.TrimSpace(text) == strings.TrimSpace(targetText) {
			targetIdx = i
			targetLevel = level
			break
		}
	}

	if targetIdx < 0 {
		// List available headings.
		headings := listHeadings(lines)
		if len(headings) > 0 {
			return "", fmt.Errorf("Section '%s' not found. Available headings: %s",
				section, strings.Join(headings, ", "))
		}
		return "", fmt.Errorf("Section '%s' not found. The document has no headings.", section)
	}

	// Find the end of the section (next heading of equal or higher level).
	endIdx := len(lines)
	for i := targetIdx + 1; i < len(lines); i++ {
		level, _ := parseHeading(lines[i])
		if level > 0 && level <= targetLevel {
			endIdx = i
			break
		}
	}

	// Reconstruct: before + target heading + new content + after.
	var parts []string
	parts = append(parts, lines[:targetIdx+1]...)
	parts = append(parts, newContent)
	if endIdx < len(lines) {
		parts = append(parts, lines[endIdx:]...)
	}

	return strings.Join(parts, "\n"), nil
}

// parseHeading extracts the level and text from a markdown heading line.
// Returns (0, "") if the line is not a heading.
func parseHeading(line string) (int, string) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return 0, ""
	}
	level := 0
	for _, ch := range trimmed {
		if ch == '#' {
			level++
		} else {
			break
		}
	}
	if level > 6 || level == 0 {
		return 0, ""
	}
	text := strings.TrimSpace(trimmed[level:])
	return level, text
}

// listHeadings returns all heading lines from the document.
func listHeadings(lines []string) []string {
	var headings []string
	for _, line := range lines {
		level, text := parseHeading(line)
		if level > 0 {
			headings = append(headings, strings.Repeat("#", level)+" "+text)
		}
	}
	return headings
}
