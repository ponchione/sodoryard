package searcher

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/ponchione/sirtopham/internal/codeintel"
)

// Searcher executes multi-query semantic search with deduplication,
// re-ranking, and optional call-graph hop expansion.
type Searcher struct {
	store    codeintel.Store
	embedder codeintel.Embedder
}

// New creates a Searcher from the given store and embedder.
func New(store codeintel.Store, embedder codeintel.Embedder) *Searcher {
	return &Searcher{store: store, embedder: embedder}
}

// Search embeds each query, runs vector search with the provided options,
// deduplicates by chunk ID, re-ranks by hit count with best-score tie
// breaking, optionally expands one hop, and returns up to MaxResults.
func (s *Searcher) Search(ctx context.Context, queries []string, opts codeintel.SearchOptions) ([]codeintel.SearchResult, error) {
	if len(queries) == 0 {
		return nil, fmt.Errorf("searcher: at least one query is required")
	}

	topK := opts.TopK
	if topK == 0 {
		topK = 10
	}

	type scored struct {
		result   codeintel.SearchResult
		hitCount int
		best     float64
	}
	seen := make(map[string]*scored)
	var failCount int

	for _, q := range queries {
		vec, err := s.embedder.EmbedQuery(ctx, q)
		if err != nil {
			slog.Warn("embed query failed", "query", q, "error", err)
			failCount++
			continue
		}

		results, err := s.store.VectorSearch(ctx, vec, topK, opts.Filter)
		if err != nil {
			slog.Warn("vector search failed", "query", q, "error", err)
			failCount++
			continue
		}

		for _, r := range results {
			if existing, ok := seen[r.Chunk.ID]; ok {
				existing.hitCount++
				if r.Score > existing.best {
					existing.best = r.Score
					existing.result = r
				}
			} else {
				seen[r.Chunk.ID] = &scored{
					result:   r,
					hitCount: 1,
					best:     r.Score,
				}
			}
		}
	}

	if failCount == len(queries) {
		return nil, fmt.Errorf("searcher: all %d queries failed", len(queries))
	}

	directHits := make([]*scored, 0, len(seen))
	for _, s := range seen {
		directHits = append(directHits, s)
	}
	sort.Slice(directHits, func(i, j int) bool {
		if directHits[i].hitCount != directHits[j].hitCount {
			return directHits[i].hitCount > directHits[j].hitCount
		}
		return directHits[i].best > directHits[j].best
	})

	maxResults := opts.MaxResults
	if maxResults == 0 {
		maxResults = 30
	}

	var results []codeintel.SearchResult

	directBudget := len(directHits)
	hopBudget := 0
	if opts.EnableHopExpansion {
		hopFrac := opts.HopBudgetFraction
		if hopFrac == 0 {
			hopFrac = 0.4
		}
		directBudget = min(int(float64(maxResults)*(1-hopFrac)), len(directHits))
		hopBudget = maxResults - directBudget
	}

	seenIDs := make(map[string]bool)
	for i := 0; i < directBudget && i < len(directHits); i++ {
		h := directHits[i]
		h.result.HitCount = h.hitCount
		results = append(results, h.result)
		seenIDs[h.result.Chunk.ID] = true
	}

	if hopBudget > 0 {
		hopDepth := opts.HopDepth
		if hopDepth == 0 {
			hopDepth = 1
		}
		hops := s.expandHops(ctx, results, seenIDs, hopBudget, hopDepth)
		results = append(results, hops...)
	}

	if len(results) > maxResults {
		results = results[:maxResults]
	}

	return results, nil
}

// expandHops performs iterative hop expansion through the call graph.
// It runs up to depth rounds, where each round expands the results
// discovered in the previous round. All rounds share seenIDs for
// deduplication and are collectively capped by budget.
func (s *Searcher) expandHops(
	ctx context.Context,
	directHits []codeintel.SearchResult,
	seenIDs map[string]bool,
	budget int,
	depth int,
) []codeintel.SearchResult {
	var allHops []codeintel.SearchResult

	// The first round expands from directHits; subsequent rounds expand
	// from the results discovered in the previous round.
	frontier := directHits

	for round := 0; round < depth; round++ {
		var roundHops []codeintel.SearchResult

		for _, hit := range frontier {
			if len(allHops) >= budget {
				break
			}

			allRefs := make([]codeintel.FuncRef, 0, len(hit.Chunk.Calls)+len(hit.Chunk.CalledBy))
			allRefs = append(allRefs, hit.Chunk.Calls...)
			allRefs = append(allRefs, hit.Chunk.CalledBy...)

			for _, ref := range allRefs {
				if len(allHops) >= budget {
					break
				}
				chunks, err := s.store.GetByName(ctx, ref.Name)
				if err != nil {
					slog.Debug("hop lookup failed", "name", ref.Name, "error", err)
					continue
				}
				for _, c := range chunks {
					if len(allHops) >= budget {
						break
					}
					if seenIDs[c.ID] {
						continue
					}
					seenIDs[c.ID] = true
					r := codeintel.SearchResult{
						Chunk:   c,
						Score:   0,
						FromHop: true,
					}
					allHops = append(allHops, r)
					roundHops = append(roundHops, r)
				}
			}
		}

		// Next round expands from this round's discoveries.
		frontier = roundHops

		if len(allHops) >= budget || len(frontier) == 0 {
			break
		}
	}

	return allHops
}
