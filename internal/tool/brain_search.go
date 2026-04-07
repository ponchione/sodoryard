package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/ponchione/sirtopham/internal/brain"
	"github.com/ponchione/sirtopham/internal/config"
)

// BrainSearch implements the brain_search tool — keyword search against the
// project brain backend.
type BrainSearch struct {
	client brain.Backend
	config config.BrainConfig
}

// NewBrainSearch creates a brain_search tool backed by the given brain backend.
func NewBrainSearch(client brain.Backend, cfg config.BrainConfig) *BrainSearch {
	return &BrainSearch{client: client, config: cfg}
}

type brainSearchInput struct {
	Query      string   `json:"query"`
	Mode       string   `json:"mode,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	MaxResults *int     `json:"max_results,omitempty"`
}

func (b *BrainSearch) Name() string { return "brain_search" }
func (b *BrainSearch) Description() string {
	return "Search the project brain (Obsidian vault) by keyword"
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
		"description": "Search the project brain (Obsidian knowledge vault) for documents by keyword. Use this when the prompt refers to brain notes like 'notes/...md' or '.brain/notes/...md', or when search_text found nothing but the content may live in the brain. Prefer brain_search/brain_read over search_text/file_read for vault-relative note paths, never use search_text for .brain paths, and do not double-check a successful brain hit with repo search tools. Returns matching document paths, titles, and relevant snippets. Use this to find architectural decisions, debugging journals, conventions, and other project knowledge.",
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
			Content: "Project brain is not configured. See the project's YAML config brain section.",
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

	// Semantic/auto mode falls through to keyword search with a notice.
	semanticNotice := ""
	mode := strings.ToLower(params.Mode)
	if mode == "semantic" || mode == "auto" {
		semanticNotice = "Semantic search is not yet available (coming in v0.2). Using keyword search instead.\n\n"
	}

	hits, err := b.searchHits(ctx, params.Query)
	if err != nil {
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Brain search failed: %v", err),
			Error:   err.Error(),
		}, nil
	}
	if len(normalizedTags) > 0 {
		hits, err = b.filterHitsByTags(ctx, hits, normalizedTags)
		if err != nil {
			return &ToolResult{
				Success: false,
				Content: fmt.Sprintf("Brain search failed while filtering tags: %v", err),
				Error:   err.Error(),
			}, nil
		}
		if len(hits) == 0 && params.Query != "" {
			hits, err = b.searchTaggedDocsByLooseQuery(ctx, params.Query, normalizedTags)
			if err != nil {
				return &ToolResult{
					Success: false,
					Content: fmt.Sprintf("Brain search failed while retrying tagged fallback: %v", err),
					Error:   err.Error(),
				}, nil
			}
		}
	}

	maxResults := 10
	if params.MaxResults != nil && *params.MaxResults > 0 {
		maxResults = *params.MaxResults
	}
	if len(hits) > maxResults {
		hits = hits[:maxResults]
	}

	queryLabel := describeBrainSearchQuery(params.Query, normalizedTags)
	if len(hits) == 0 {
		content := semanticNotice + fmt.Sprintf("No brain documents found for query: '%s'", queryLabel)
		if err := b.appendQueryLog(ctx, queryLabel, 0); err != nil {
			return &ToolResult{Success: false, Content: fmt.Sprintf("Brain search completed but failed to append query log: %v", err), Error: err.Error()}, nil
		}
		return &ToolResult{
			Success: true,
			Content: content,
		}, nil
	}

	formatted := make([]formattedSearchHit, 0, len(hits))
	for _, hit := range hits {
		formatted = append(formatted, formattedSearchHit{
			Path:    hit.Path,
			Title:   titleFromPath(hit.Path),
			Snippet: compactSnippet(hit.Snippet),
		})
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

func (b *BrainSearch) searchHits(ctx context.Context, query string) ([]brain.SearchHit, error) {
	if query != "" {
		hits, err := b.client.SearchKeyword(ctx, query)
		if err != nil {
			return nil, err
		}
		return filterOperationalBrainSearchHits(hits), nil
	}

	paths, err := b.client.ListDocuments(ctx, "")
	if err != nil {
		return nil, err
	}
	results := make([]brain.SearchHit, 0, len(paths))
	for _, path := range paths {
		if isOperationalBrainDocumentPath(path) {
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

func (b *BrainSearch) filterHitsByTags(ctx context.Context, hits []brain.SearchHit, tags []string) ([]brain.SearchHit, error) {
	filtered := make([]brain.SearchHit, 0, len(hits))
	for _, hit := range hits {
		if isOperationalBrainDocumentPath(hit.Path) {
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

func (b *BrainSearch) searchTaggedDocsByLooseQuery(ctx context.Context, query string, tags []string) ([]brain.SearchHit, error) {
	paths, err := b.client.ListDocuments(ctx, "")
	if err != nil {
		return nil, err
	}
	queryTokens := strings.Fields(normalizeBrainSearchText(query))
	results := make([]brain.SearchHit, 0, len(paths))
	for _, path := range paths {
		if isOperationalBrainDocumentPath(path) {
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
		if matched != len(queryTokens) || matched == 0 {
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

func filterOperationalBrainSearchHits(hits []brain.SearchHit) []brain.SearchHit {
	filtered := make([]brain.SearchHit, 0, len(hits))
	for _, hit := range hits {
		if isOperationalBrainDocumentPath(hit.Path) {
			continue
		}
		filtered = append(filtered, hit)
	}
	return filtered
}

func isOperationalBrainDocumentPath(path string) bool {
	return filepath.Base(strings.Trim(filepath.ToSlash(strings.TrimSpace(path)), "/")) == "_log.md"
}

func normalizeBrainSearchTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		normalized := normalizeBrainTag(tag)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func normalizeBrainTag(tag string) string {
	tag = strings.TrimSpace(tag)
	tag = strings.TrimPrefix(tag, "#")
	return normalizeBrainSearchText(tag)
}

func normalizeBrainSearchText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastWasSep := true
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			lastWasSep = false
			continue
		}
		if !lastWasSep {
			b.WriteByte(' ')
			lastWasSep = true
		}
	}
	return strings.TrimSpace(b.String())
}

func brainDocumentHasAllTags(content string, tags []string) bool {
	if len(tags) == 0 {
		return true
	}
	frontmatterTags := parseBrainFrontmatterTags(content)
	metadataTags := parseBrainMetadataTags(content)
	inlineTags := extractBrainInlineTags(content)
	for _, tag := range tags {
		if _, ok := frontmatterTags[tag]; ok {
			continue
		}
		if _, ok := metadataTags[tag]; ok {
			continue
		}
		if _, ok := inlineTags[tag]; ok {
			continue
		}
		return false
	}
	return true
}

func parseBrainFrontmatterTags(content string) map[string]struct{} {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil
	}

	tags := map[string]struct{}{}
	inTagsList := false
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			break
		}
		if inTagsList {
			if strings.HasPrefix(line, "-") {
				if tag := normalizeBrainTag(strings.TrimSpace(strings.TrimPrefix(line, "-"))); tag != "" {
					tags[tag] = struct{}{}
				}
				continue
			}
			inTagsList = false
		}
		if !strings.HasPrefix(strings.ToLower(line), "tags:") {
			continue
		}
		rest := strings.TrimSpace(line[len("tags:"):])
		if rest == "" {
			inTagsList = true
			continue
		}
		for _, part := range strings.Split(strings.Trim(rest, "[]"), ",") {
			if tag := normalizeBrainTag(part); tag != "" {
				tags[tag] = struct{}{}
			}
		}
	}
	return tags
}

func parseBrainMetadataTags(content string) map[string]struct{} {
	tags := map[string]struct{}{}
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		lower := strings.ToLower(line)
		for _, prefix := range []string{"family:", "tag:", "tags:"} {
			if !strings.HasPrefix(lower, prefix) {
				continue
			}
			rest := strings.TrimSpace(line[len(prefix):])
			for _, part := range strings.Split(strings.Trim(rest, "[]"), ",") {
				if tag := normalizeBrainTag(part); tag != "" {
					tags[tag] = struct{}{}
				}
			}
		}
	}
	return tags
}

func extractBrainInlineTags(content string) map[string]struct{} {
	tags := map[string]struct{}{}
	var current strings.Builder
	capturing := false
	flush := func() {
		if !capturing {
			return
		}
		if tag := normalizeBrainTag(current.String()); tag != "" {
			tags[tag] = struct{}{}
		}
		current.Reset()
		capturing = false
	}
	for _, r := range content {
		switch {
		case r == '#':
			flush()
			capturing = true
		case capturing && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'):
			current.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return tags
}

func describeBrainSearchQuery(query string, tags []string) string {
	if len(tags) == 0 {
		return query
	}
	tagLabel := "tags: " + strings.Join(tags, ", ")
	if query == "" {
		return tagLabel
	}
	return query + " (" + tagLabel + ")"
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

func pluralizeBrainSearchResults(count int) string {
	if count == 1 {
		return "result"
	}
	return "results"
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
