package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/config"
	appcontext "github.com/ponchione/sodoryard/internal/context"
)

// BrainSearch implements the brain_search tool — keyword search against the
// project brain backend.
type BrainRuntimeSearcher interface {
	Search(ctx context.Context, request appcontext.BrainSearchRequest) ([]appcontext.BrainSearchResult, error)
}

type BrainSearch struct {
	client  brain.Backend
	runtime BrainRuntimeSearcher
	config  config.BrainConfig
}

// NewBrainSearch creates a brain_search tool backed by the given brain backend.
func NewBrainSearch(client brain.Backend, cfg config.BrainConfig) *BrainSearch {
	return &BrainSearch{client: client, config: cfg}
}

// NewBrainSearchWithRuntime creates a brain_search tool with an optional hybrid
// runtime searcher used for semantic/auto modes.
func NewBrainSearchWithRuntime(client brain.Backend, runtime BrainRuntimeSearcher, cfg config.BrainConfig) *BrainSearch {
	return &BrainSearch{client: client, runtime: runtime, config: cfg}
}

type brainSearchInput struct {
	Query      string   `json:"query"`
	Mode       string   `json:"mode,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	MaxResults *int     `json:"max_results,omitempty"`
}

func (b *BrainSearch) Name() string { return "brain_search" }
func (b *BrainSearch) Description() string {
	return "Search Shunter project brain documents by keyword"
}
func (b *BrainSearch) ToolPurity() Purity {
	if b.config.LogBrainQueries {
		return Mutating
	}
	return Pure
}

func (b *BrainSearch) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "brain_search",
		"description": "Search Shunter project brain documents and derived knowledge matches. Use this when the prompt refers to brain notes like 'notes/...md', or when search_text found nothing but the content may live in the brain. Prefer brain_search/brain_read over search_text/file_read for brain note paths, and do not double-check a successful brain hit with repo search tools. Returns matching document paths, titles, and relevant snippets. Use this to find architectural decisions, debugging journals, conventions, and other project knowledge. Keyword mode stays lexical-only; semantic and auto modes use the landed runtime search path when available and may also include graph/backlink expansion from derived brain links.",
		"input_schema": {
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "The search query — keywords, tag names, or concepts to find"
				},
				"mode": {
					"type": "string",
					"description": "Search mode: 'keyword' (default) for deterministic lexical search, 'semantic' for runtime semantic search when available, or 'auto' for hybrid runtime search that can combine keyword, semantic, and derived graph/backlink signals.",
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
			Content: "Project brain is not configured. See the project's YAML config brain section.",
			Error:   "brain not configured",
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

	params.Query = strings.Join(strings.Fields(params.Query), " ")
	normalizedTags := normalizeBrainSearchTags(params.Tags)
	if params.Query == "" && len(normalizedTags) == 0 {
		return &ToolResult{
			Success: false,
			Content: "query is required",
			Error:   "empty query",
		}, nil
	}

	mode := strings.ToLower(strings.TrimSpace(params.Mode))
	if mode == "" {
		mode = "keyword"
	}

	maxResults := 10
	if params.MaxResults != nil && *params.MaxResults > 0 {
		maxResults = *params.MaxResults
	}

	semanticNotice := ""
	formatted, err := b.searchFormattedHits(ctx, params.Query, mode, normalizedTags, maxResults)
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Brain search failed: %v", err),
			Error:   err.Error(),
		}, nil
	}
	if (mode == "semantic" || mode == "auto") && b.runtime == nil {
		semanticNotice = "Semantic/index-backed brain search is not a landed runtime path yet. Using keyword search instead.\n\n"
	}

	queryLabel := describeBrainSearchQuery(params.Query, normalizedTags)
	if len(formatted) == 0 {
		content := semanticNotice + fmt.Sprintf("No brain documents found for query: '%s'", queryLabel)
		if err := b.appendQueryLog(ctx, queryLabel, 0); err != nil {
			return &ToolResult{Success: false, Content: fmt.Sprintf("Brain search completed but failed to append query log: %v", err), Error: err.Error()}, nil
		}
		return &ToolResult{
			Success: true,
			Content: content,
		}, nil
	}

	content := formatBrainSearchResult(queryLabel, formatted)
	if semanticNotice != "" {
		content = semanticNotice + content
	}
	if err := b.appendQueryLog(ctx, queryLabel, len(formatted)); err != nil {
		return &ToolResult{Success: false, Content: fmt.Sprintf("Brain search completed but failed to append query log: %v", err), Error: err.Error()}, nil
	}

	return &ToolResult{
		Success: true,
		Content: content,
	}, nil
}

func (b *BrainSearch) searchHits(ctx context.Context, query string, maxResults int) ([]brain.SearchHit, error) {
	if query != "" {
		var (
			hits []brain.SearchHit
			err  error
		)
		if limited, ok := b.client.(brain.LimitedKeywordSearcher); ok {
			hits, err = limited.SearchKeywordLimit(ctx, query, maxResults)
		} else {
			hits, err = b.client.SearchKeyword(ctx, query)
		}
		if err != nil {
			return nil, err
		}
		filtered := make([]brain.SearchHit, 0, len(hits))
		for _, hit := range hits {
			if !brain.IsOperationalDocument(hit.Path) {
				filtered = append(filtered, hit)
			}
		}
		return filtered, nil
	}

	paths, err := b.client.ListDocuments(ctx, "")
	if err != nil {
		return nil, err
	}
	results := make([]brain.SearchHit, 0, len(paths))
	for _, path := range paths {
		if brain.IsOperationalDocument(path) {
			continue
		}
		content, err := b.client.ReadDocument(ctx, path)
		if err != nil {
			return nil, err
		}
		results = append(results, brain.SearchHit{
			Path:    path,
			Snippet: content[:min(100, len(content))],
			Score:   0,
		})
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Path < results[j].Path
	})
	return results, nil
}

func (b *BrainSearch) searchFormattedHits(ctx context.Context, query string, mode string, tags []string, maxResults int) ([]formattedSearchHit, error) {
	if (mode == "semantic" || mode == "auto") && b.runtime != nil && strings.TrimSpace(query) != "" {
		results, err := b.runtime.Search(ctx, appcontext.BrainSearchRequest{
			Query:            query,
			Mode:             mode,
			MaxResults:       maxResults,
			IncludeGraphHops: b.config.IncludeGraphHops,
			GraphHopDepth:    b.config.GraphHopDepth,
		})
		if err != nil {
			return nil, err
		}
		if len(tags) > 0 {
			results, err = b.filterRuntimeResultsByTags(ctx, results, tags)
			if err != nil {
				return nil, err
			}
		}
		if len(results) > maxResults {
			results = results[:maxResults]
		}
		return formatRuntimeBrainSearchHits(results), nil
	}

	hits, err := b.searchHits(ctx, query, maxResults)
	if err != nil {
		return nil, err
	}
	if len(tags) > 0 {
		hits, err = b.filterHitsByTags(ctx, hits, tags)
		if err != nil {
			return nil, err
		}
		if len(hits) == 0 && query != "" {
			hits, err = b.searchTaggedDocsByLooseQuery(ctx, query, tags)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(hits) > maxResults {
		hits = hits[:maxResults]
	}
	formatted := make([]formattedSearchHit, 0, len(hits))
	for _, hit := range hits {
		formatted = append(formatted, formattedSearchHit{Path: hit.Path, Title: titleFromPath(hit.Path), Snippet: compactSnippet(hit.Snippet)})
	}
	return formatted, nil
}

func (b *BrainSearch) filterHitsByTags(ctx context.Context, hits []brain.SearchHit, tags []string) ([]brain.SearchHit, error) {
	filtered := make([]brain.SearchHit, 0, len(hits))
	for _, hit := range hits {
		if brain.IsOperationalDocument(hit.Path) {
			continue
		}
		content, err := b.client.ReadDocument(ctx, hit.Path)
		if err != nil {
			return nil, err
		}
		if brainDocumentHasAllTags(content, tags) {
			filtered = append(filtered, hit)
		}
	}
	return filtered, nil
}

func (b *BrainSearch) filterRuntimeResultsByTags(ctx context.Context, hits []appcontext.BrainSearchResult, tags []string) ([]appcontext.BrainSearchResult, error) {
	filtered := make([]appcontext.BrainSearchResult, 0, len(hits))
	for _, hit := range hits {
		if len(hit.Tags) > 0 && stringSliceHasAllFolded(hit.Tags, tags) {
			filtered = append(filtered, hit)
			continue
		}
		content, err := b.client.ReadDocument(ctx, hit.DocumentPath)
		if err != nil {
			return nil, err
		}
		if brainDocumentHasAllTags(content, tags) {
			filtered = append(filtered, hit)
		}
	}
	return filtered, nil
}

func (b *BrainSearch) searchTaggedDocsByLooseQuery(ctx context.Context, query string, tags []string) ([]brain.SearchHit, error) {
	paths, err := b.client.ListDocuments(ctx, "")
	if err != nil {
		return nil, err
	}
	queryTokens := strings.Fields(normalizeBrainSearchText(query))
	if len(queryTokens) == 0 {
		return nil, nil
	}
	results := make([]brain.SearchHit, 0, len(paths))
	for _, path := range paths {
		if brain.IsOperationalDocument(path) {
			continue
		}
		content, err := b.client.ReadDocument(ctx, path)
		if err != nil {
			return nil, err
		}
		if !brainDocumentHasAllTags(content, tags) {
			continue
		}
		normalizedHaystack := normalizeBrainSearchText(path + "\n" + content)
		matched := 0
		for _, token := range queryTokens {
			if strings.Contains(normalizedHaystack, token) {
				matched++
			}
		}
		if matched != len(queryTokens) {
			continue
		}
		results = append(results, brain.SearchHit{
			Path:    path,
			Snippet: content[:min(100, len(content))],
			Score:   float64(matched),
		})
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Path < results[j].Path
	})
	return results, nil
}

func (b *BrainSearch) appendQueryLog(ctx context.Context, query string, resultCount int) error {
	if !b.config.LogBrainQueries {
		return nil
	}
	summary := fmt.Sprintf("Returned %d %s via keyword search.", resultCount, pluralizeBrainSearchResults(resultCount))
	return appendBrainLog(ctx, b.client, BrainLogEntry{
		Timestamp: time.Now().UTC(),
		Operation: "query",
		Target:    strings.Join(strings.Fields(query), " "),
		Summary:   summary,
		Session:   sessionIDFromContext(ctx),
	})
}
