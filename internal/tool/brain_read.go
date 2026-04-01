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

var wikilinkRegexp = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// BrainRead implements the brain_read tool — read a specific brain document
// by its vault-relative path.
type BrainRead struct {
	client *brain.ObsidianClient
	config config.BrainConfig
}

// NewBrainRead creates a brain_read tool backed by the given Obsidian client.
func NewBrainRead(client *brain.ObsidianClient, cfg config.BrainConfig) *BrainRead {
	return &BrainRead{client: client, config: cfg}
}

type brainReadInput struct {
	Path             string `json:"path"`
	IncludeBacklinks bool   `json:"include_backlinks,omitempty"`
}

func (b *BrainRead) Name() string        { return "brain_read" }
func (b *BrainRead) Description() string { return "Read a brain document by path from the Obsidian vault" }
func (b *BrainRead) ToolPurity() Purity  { return Pure }

func (b *BrainRead) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "brain_read",
		"description": "Read a specific brain document from the Obsidian vault by its vault-relative path. Returns the markdown content, extracted YAML frontmatter, and outgoing wikilinks.",
		"input_schema": {
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "Vault-relative path to the document (e.g., 'architecture/provider-design.md')"
				},
				"include_backlinks": {
					"type": "boolean",
					"description": "If true, search for documents that reference this one (default: false)"
				}
			},
			"required": ["path"]
		}
	}`)
}

func (b *BrainRead) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	if !b.config.Enabled {
		return &ToolResult{
			Success: false,
			Content: "Project brain is not configured. See sirtopham.yaml brain section.",
		}, nil
	}

	var params brainReadInput
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

	content, err := b.client.ReadDocument(ctx, params.Path)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "Document not found") {
			// Enrich with directory listing.
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

	// Parse frontmatter and wikilinks.
	frontmatter, bodyContent := extractFrontmatter(content)
	wikilinks := extractWikilinks(content)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Path: %s\n", params.Path))

	if frontmatter != "" {
		sb.WriteString("\nFrontmatter:\n")
		for _, line := range strings.Split(frontmatter, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				sb.WriteString("  " + line + "\n")
			}
		}
	}

	if len(wikilinks) > 0 {
		linkStrs := make([]string, len(wikilinks))
		for i, link := range wikilinks {
			linkStrs[i] = "[[" + link + "]]"
		}
		sb.WriteString(fmt.Sprintf("\nOutgoing links: %s\n", strings.Join(linkStrs, ", ")))
	}

	// Backlinks via heuristic keyword search.
	if params.IncludeBacklinks {
		basename := strings.TrimSuffix(filepath.Base(params.Path), filepath.Ext(params.Path))
		hits, searchErr := b.client.SearchKeyword(ctx, basename)
		if searchErr == nil && len(hits) > 0 {
			sb.WriteString("\nReferenced by:\n")
			for _, hit := range hits {
				if hit.Path != params.Path {
					sb.WriteString("  " + hit.Path + "\n")
				}
			}
		}
	}

	sb.WriteString("\nContent:\n")
	sb.WriteString(bodyContent)

	return &ToolResult{
		Success: true,
		Content: sb.String(),
	}, nil
}

// extractFrontmatter splits YAML frontmatter from the body.
// Returns ("", fullContent) if no frontmatter is present.
func extractFrontmatter(content string) (string, string) {
	if !strings.HasPrefix(content, "---") {
		return "", content
	}
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", content
	}
	fm := strings.TrimSpace(rest[:idx])
	body := strings.TrimLeft(rest[idx+4:], "\n")
	return fm, body
}

// extractWikilinks finds all [[wikilink]] references in the content.
func extractWikilinks(content string) []string {
	matches := wikilinkRegexp.FindAllStringSubmatch(content, -1)
	seen := make(map[string]struct{})
	var links []string
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		link := match[1]
		// Handle display text: [[target|display]] → target
		if idx := strings.Index(link, "|"); idx >= 0 {
			link = link[:idx]
		}
		link = strings.TrimSpace(link)
		if _, ok := seen[link]; ok {
			continue
		}
		seen[link] = struct{}{}
		links = append(links, link)
	}
	return links
}
