package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ponchione/sodoryard/internal/codeintel"
)

// SemanticSearcher is the narrow interface the search_semantic tool needs
// from the Layer 1 Searcher. The codeintel.Searcher satisfies this directly.
type SemanticSearcher interface {
	Search(ctx context.Context, queries []string, opts codeintel.SearchOptions) ([]codeintel.SearchResult, error)
}

// SearchSemantic implements the search_semantic tool — RAG-based semantic
// search against the code intelligence layer.
type SearchSemantic struct {
	searcher SemanticSearcher
}

// NewSearchSemantic creates a search_semantic tool backed by the given searcher.
func NewSearchSemantic(searcher SemanticSearcher) *SearchSemantic {
	return &SearchSemantic{searcher: searcher}
}

type searchSemanticInput struct {
	Query      string `json:"query"`
	Language   string `json:"language,omitempty"`
	ChunkType  string `json:"chunk_type,omitempty"`
	MaxResults *int   `json:"max_results,omitempty"`
}

func (s *SearchSemantic) Name() string        { return "search_semantic" }
func (s *SearchSemantic) Description() string { return "Semantic search across the codebase using RAG" }
func (s *SearchSemantic) ToolPurity() Purity  { return Pure }

func (s *SearchSemantic) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "search_semantic",
		"description": "Semantic search across the codebase. Uses embeddings to find code by meaning, not just text matching. Best for 'find the authentication logic' or 'where is error handling implemented'.",
		"input_schema": {
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "Natural language query describing what you're looking for"
				},
				"language": {
					"type": "string",
					"description": "Optional language filter (e.g., 'go', 'python')"
				},
				"chunk_type": {
					"type": "string",
					"description": "Optional chunk type filter (e.g., 'function', 'type', 'method')"
				},
				"max_results": {
					"type": "integer",
					"description": "Maximum number of results (default: 15)"
				}
			},
			"required": ["query"]
		}
	}`)
}

func (s *SearchSemantic) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	var params searchSemanticInput
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

	if s.searcher == nil {
		return &ToolResult{
			Success: false,
			Content: "Code index is empty or not built. Run 'sirtopham index' to build the code intelligence index.",
			Error:   "no searcher",
		}, nil
	}

	maxResults := 15
	if params.MaxResults != nil && *params.MaxResults > 0 {
		maxResults = *params.MaxResults
	}

	opts := codeintel.SearchOptions{
		MaxResults: maxResults,
		TopK:       maxResults * 3, // overfetch for dedup
	}

	if params.Language != "" {
		opts.Filter.Language = params.Language
	}
	if params.ChunkType != "" {
		opts.Filter.ChunkType = codeintel.ChunkType(params.ChunkType)
	}

	results, err := s.searcher.Search(ctx, []string{params.Query}, opts)
	if err != nil {
		// Check for common "not initialized" errors.
		errMsg := err.Error()
		if strings.Contains(errMsg, "not initialized") ||
			strings.Contains(errMsg, "not found") ||
			strings.Contains(errMsg, "no such table") ||
			strings.Contains(errMsg, "does not exist") {
			return &ToolResult{
				Success: false,
				Content: "Code index is empty or not built. Run 'sirtopham index' to build the code intelligence index.",
				Error:   err.Error(),
			}, nil
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Semantic search failed: %v", err),
			Error:   err.Error(),
		}, nil
	}

	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Content: fmt.Sprintf("No semantically relevant code found for query: '%s'. Try rephrasing or use search_text for exact string matching.", params.Query),
		}, nil
	}

	// Format results.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d results for: '%s'\n\n", len(results), params.Query))

	for i, r := range results {
		chunk := r.Chunk
		lineRange := ""
		if chunk.LineStart > 0 && chunk.LineEnd > 0 {
			lineRange = fmt.Sprintf(":%d-%d", chunk.LineStart, chunk.LineEnd)
		}

		sb.WriteString(fmt.Sprintf("─── Result %d (score: %.2f) ───\n", i+1, r.Score))
		sb.WriteString(fmt.Sprintf("  File: %s%s\n", chunk.FilePath, lineRange))

		if chunk.Name != "" {
			kindLabel := string(chunk.ChunkType)
			if kindLabel == "" {
				kindLabel = "symbol"
			}
			sb.WriteString(fmt.Sprintf("  %s: %s\n", kindLabel, chunk.Name))
		}

		if chunk.Description != "" {
			sb.WriteString(fmt.Sprintf("  Description: %s\n", chunk.Description))
		}

		if chunk.Signature != "" {
			sb.WriteString(fmt.Sprintf("  Signature: %s\n", chunk.Signature))
		}

		sb.WriteString("\n")
	}

	return &ToolResult{
		Success: true,
		Content: sb.String(),
	}, nil
}
