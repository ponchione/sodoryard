package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
)

var wikilinkRegexp = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// BrainRead implements the brain_read tool — read a specific brain document
// by its vault-relative path.
type BrainRead struct {
	client    brain.Backend
	config    config.BrainConfig
	queries   *appdb.Queries
	projectID string
}

// NewBrainRead creates a brain_read tool backed by the given brain backend.
func NewBrainRead(client brain.Backend, cfg config.BrainConfig) *BrainRead {
	return &BrainRead{client: client, config: cfg}
}

// NewBrainReadWithIndex creates a brain_read tool with optional derived graph metadata.
func NewBrainReadWithIndex(client brain.Backend, cfg config.BrainConfig, queries *appdb.Queries, projectID string) *BrainRead {
	return &BrainRead{client: client, config: cfg, queries: queries, projectID: strings.TrimSpace(projectID)}
}

type brainReadInput struct {
	Path             string `json:"path"`
	IncludeBacklinks bool   `json:"include_backlinks,omitempty"`
}

func (b *BrainRead) Name() string { return "brain_read" }
func (b *BrainRead) Description() string {
	return "Read a brain document by path from the Obsidian vault"
}
func (b *BrainRead) ToolPurity() Purity { return Pure }

func (b *BrainRead) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "brain_read",
		"description": "Read a specific brain document from the Obsidian vault by its vault-relative path. Use this for brain notes like 'notes/...md' or '.brain/notes/...md', not repo-root files. Prefer brain_read instead of file_read for vault-relative note paths, and never use file_read for .brain paths. Returns the markdown content, extracted YAML frontmatter, and outgoing wikilinks.",
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
			Content: "Project brain is not configured. See the project's YAML config brain section.",
			Error:   "brain not configured",
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
		if result := brainDocumentNotFoundResult(ctx, b.client, params.Path, errMsg); result != nil {
			return result, nil
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

	backlinks := []string{}
	if params.IncludeBacklinks {
		backlinks = b.lookupBacklinks(ctx, params.Path)
	}

	contentOut := formatBrainReadDocument(params.Path, frontmatter, wikilinks, bodyContent)
	if len(backlinks) > 0 {
		contentOut += "\n\nReferenced by:\n" + formatHeadingList(backlinks)
	}

	return &ToolResult{
		Success: true,
		Content: contentOut,
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

func (b *BrainRead) lookupBacklinks(ctx context.Context, path string) []string {
	if backlinks := b.lookupIndexedBacklinks(ctx, path); len(backlinks) > 0 {
		return backlinks
	}
	return b.lookupHeuristicBacklinks(ctx, path)
}

func (b *BrainRead) lookupIndexedBacklinks(ctx context.Context, path string) []string {
	if b == nil || b.queries == nil || strings.TrimSpace(b.projectID) == "" || strings.TrimSpace(path) == "" {
		return nil
	}
	links, err := b.queries.ListBrainLinksByTarget(ctx, appdb.ListBrainLinksByTargetParams{
		ProjectID:  b.projectID,
		TargetPath: path,
	})
	if err != nil || len(links) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	backlinks := make([]string, 0, len(links))
	for _, link := range links {
		sourcePath := strings.TrimSpace(link.SourcePath)
		if sourcePath == "" || sourcePath == path {
			continue
		}
		if _, ok := seen[sourcePath]; ok {
			continue
		}
		seen[sourcePath] = struct{}{}
		backlinks = append(backlinks, sourcePath)
	}
	sort.Strings(backlinks)
	return backlinks
}

func (b *BrainRead) lookupHeuristicBacklinks(ctx context.Context, path string) []string {
	if b == nil || b.client == nil {
		return nil
	}
	basename := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	hits, err := b.client.SearchKeyword(ctx, basename)
	if err != nil || len(hits) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	backlinks := make([]string, 0, len(hits))
	for _, hit := range hits {
		hitPath := strings.TrimSpace(hit.Path)
		if hitPath == "" || hitPath == path {
			continue
		}
		if _, ok := seen[hitPath]; ok {
			continue
		}
		seen[hitPath] = struct{}{}
		backlinks = append(backlinks, hitPath)
	}
	sort.Strings(backlinks)
	return backlinks
}
