package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ponchione/sirtopham/internal/brain"
	"github.com/ponchione/sirtopham/internal/config"
)

// BrainSearch implements the brain_search tool — keyword search against the
// project brain via the Obsidian REST API.
type BrainSearch struct {
	client *brain.ObsidianClient
	config config.BrainConfig
}

// NewBrainSearch creates a brain_search tool backed by the given Obsidian client.
func NewBrainSearch(client *brain.ObsidianClient, cfg config.BrainConfig) *BrainSearch {
	return &BrainSearch{client: client, config: cfg}
}

type brainSearchInput struct {
	Query      string   `json:"query"`
	Mode       string   `json:"mode,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	MaxResults *int     `json:"max_results,omitempty"`
}

func (b *BrainSearch) Name() string        { return "brain_search" }
func (b *BrainSearch) Description() string { return "Search the project brain (Obsidian vault) by keyword" }
func (b *BrainSearch) ToolPurity() Purity  { return Pure }

func (b *BrainSearch) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "brain_search",
		"description": "Search the project brain (Obsidian knowledge vault) for documents by keyword. Returns matching document paths, titles, and relevant snippets. Use this to find architectural decisions, debugging journals, conventions, and other project knowledge.",
		"input_schema": {
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "The search query — keywords, tag names, or concepts to find"
				},
				"mode": {
					"type": "string",
					"description": "Search mode: 'keyword' (default). 'semantic' and 'auto' are coming in v0.2.",
					"enum": ["keyword", "semantic", "auto"],
					"default": "keyword"
				},
				"tags": {
					"type": "array",
					"items": {"type": "string"},
					"description": "Optional tag filters to narrow results (e.g., ['architecture', 'debugging'])"
				},
				"max_results": {
					"type": "integer",
					"description": "Maximum number of results to return (default: 10)"
				}
			},
			"required": ["query"]
		}
	}`)
}

func (b *BrainSearch) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	if !b.config.Enabled {
		return &ToolResult{
			Success: false,
			Content: "Project brain is not configured. See sirtopham.yaml brain section.",
		}, nil
	}

	var params brainSearchInput
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Invalid input: %v", err),
			Error:   err.Error(),
		}, nil
	}

	if params.Query == "" {
		return &ToolResult{
			Success: false,
			Content: "query is required",
			Error:   "empty query",
		}, nil
	}

	// Semantic/auto mode falls through to keyword search with a notice.
	semanticNotice := ""
	mode := strings.ToLower(params.Mode)
	if mode == "semantic" || mode == "auto" {
		semanticNotice = "Semantic search is not yet available (coming in v0.2). Using keyword search instead.\n\n"
	}

	// Append tags to query if provided.
	query := params.Query
	if len(params.Tags) > 0 {
		for _, tag := range params.Tags {
			if !strings.HasPrefix(tag, "#") {
				tag = "#" + tag
			}
			query += " " + tag
		}
	}

	hits, err := b.client.SearchKeyword(ctx, query)
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Brain search failed: %v", err),
			Error:   err.Error(),
		}, nil
	}

	maxResults := 10
	if params.MaxResults != nil && *params.MaxResults > 0 {
		maxResults = *params.MaxResults
	}
	if len(hits) > maxResults {
		hits = hits[:maxResults]
	}

	if len(hits) == 0 {
		return &ToolResult{
			Success: true,
			Content: semanticNotice + fmt.Sprintf("No brain documents found for query: '%s'", params.Query),
		}, nil
	}

	var sb strings.Builder
	if semanticNotice != "" {
		sb.WriteString(semanticNotice)
	}
	sb.WriteString(fmt.Sprintf("Found %d results for: '%s'\n\n", len(hits), params.Query))
	for i, hit := range hits {
		title := titleFromPath(hit.Path)
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, hit.Path))
		sb.WriteString(fmt.Sprintf("   Title: %s\n", title))
		if hit.Snippet != "" {
			sb.WriteString(fmt.Sprintf("   Snippet: %s\n", strings.TrimSpace(hit.Snippet)))
		}
		sb.WriteString(fmt.Sprintf("   Score: %.2f\n", hit.Score))
		if i < len(hits)-1 {
			sb.WriteString("\n")
		}
	}

	return &ToolResult{
		Success: true,
		Content: sb.String(),
	}, nil
}

// titleFromPath extracts a human-readable title from a vault path.
func titleFromPath(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	// Convert hyphens and underscores to spaces, title-case.
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return titleCase(name)
}

// titleCase capitalizes the first letter of each word.
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
