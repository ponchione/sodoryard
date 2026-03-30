package context

import (
	stdctx "context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ponchione/sirtopham/internal/codeintel"
	"github.com/ponchione/sirtopham/internal/config"
)

const (
	defaultRetrievalTimeout     = 5 * time.Second
	defaultMaxExplicitFileBytes = 64 * 1024
	defaultRetrievalMaxResults  = 30
	defaultMaxExplicitFiles     = 5
	defaultRelevanceThreshold   = 0.35
	defaultStructuralHopDepth   = 1
	defaultStructuralHopBudget  = 10
	defaultGitContextDepth      = 5
)

type fileReaderFunc func(path string) ([]byte, error)
type gitRunnerFunc func(ctx stdctx.Context, workdir string, depth int) (string, error)

// RetrievalOrchestrator executes the v0.1 retrieval paths for assembled context.
//
// It coordinates semantic search, explicit file reads, structural graph lookups,
// conventions, and git context, then applies relevance filtering and cross-source
// deduplication before returning RetrievalResults.
type RetrievalOrchestrator struct {
	searcher             codeintel.Searcher
	graph                codeintel.GraphStore
	conventions          ConventionSource
	projectRoot          string
	fileReader           fileReaderFunc
	gitRunner            gitRunnerFunc
	timeout              time.Duration
	maxExplicitFileBytes int
}

// NewRetrievalOrchestrator constructs the concrete Slice 4 retriever.
func NewRetrievalOrchestrator(searcher codeintel.Searcher, graph codeintel.GraphStore, conventions ConventionSource, projectRoot string) *RetrievalOrchestrator {
	if conventions == nil {
		conventions = NoopConventionSource{}
	}
	return &RetrievalOrchestrator{
		searcher:             searcher,
		graph:                graph,
		conventions:          conventions,
		projectRoot:          projectRoot,
		fileReader:           os.ReadFile,
		gitRunner:            defaultGitRunner,
		timeout:              defaultRetrievalTimeout,
		maxExplicitFileBytes: defaultMaxExplicitFileBytes,
	}
}

// Retrieve runs all eligible v0.1 retrieval paths concurrently and returns the
// merged pre-budget retrieval payload.
func (o *RetrievalOrchestrator) Retrieve(ctx stdctx.Context, needs *ContextNeeds, queries []string, cfg config.ContextConfig) (*RetrievalResults, error) {
	if needs == nil {
		needs = &ContextNeeds{}
	}
	if o == nil {
		return &RetrievalResults{}, nil
	}

	var (
		ragHits        []RAGHit
		graphHits      []GraphHit
		fileResults    []FileResult
		conventionText string
		gitContext     string
	)

	var wg sync.WaitGroup

	if len(queries) > 0 && o.searcher != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pathCtx, cancel := o.pathContext(ctx)
			defer cancel()
			hits, err := o.retrieveSemanticSearch(pathCtx, queries, cfg)
			if err != nil {
				slog.Warn("context retrieval semantic search failed", "error", err)
				return
			}
			ragHits = hits
		}()
	}

	if len(needs.ExplicitFiles) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pathCtx, cancel := o.pathContext(ctx)
			defer cancel()
			results, err := o.retrieveExplicitFiles(pathCtx, needs.ExplicitFiles, cfg)
			if err != nil {
				slog.Warn("context retrieval explicit files failed", "error", err)
				return
			}
			fileResults = results
		}()
	}

	if len(needs.ExplicitSymbols) > 0 && o.graph != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pathCtx, cancel := o.pathContext(ctx)
			defer cancel()
			hits, err := o.retrieveStructuralGraph(pathCtx, needs.ExplicitSymbols, cfg)
			if err != nil {
				slog.Warn("context retrieval structural graph failed", "error", err)
				return
			}
			graphHits = hits
		}()
	}

	if needs.IncludeConventions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pathCtx, cancel := o.pathContext(ctx)
			defer cancel()
			text, err := o.retrieveConventions(pathCtx)
			if err != nil {
				slog.Warn("context retrieval conventions failed", "error", err)
				return
			}
			conventionText = text
		}()
	}

	if needs.IncludeGitContext {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pathCtx, cancel := o.pathContext(ctx)
			defer cancel()
			text, err := o.retrieveGitContext(pathCtx, needs.GitContextDepth)
			if err != nil {
				slog.Warn("context retrieval git context failed", "error", err)
				return
			}
			gitContext = text
		}()
	}

	wg.Wait()

	results := &RetrievalResults{
		RAGHits:        ragHits,
		BrainHits:      []BrainHit{},
		GraphHits:      graphHits,
		FileResults:    fileResults,
		ConventionText: conventionText,
		GitContext:     gitContext,
	}
	o.postProcess(results, cfg)
	return results, nil
}

func (o *RetrievalOrchestrator) pathContext(parent stdctx.Context) (stdctx.Context, stdctx.CancelFunc) {
	timeout := o.timeout
	if timeout <= 0 {
		timeout = defaultRetrievalTimeout
	}
	return stdctx.WithTimeout(parent, timeout)
}

func (o *RetrievalOrchestrator) retrieveSemanticSearch(ctx stdctx.Context, queries []string, cfg config.ContextConfig) ([]RAGHit, error) {
	results, err := o.searcher.Search(ctx, queries, codeintel.SearchOptions{
		TopK:               10,
		MaxResults:         retrievalMaxResults(cfg),
		EnableHopExpansion: true,
		HopBudgetFraction:  0.5,
		HopDepth:           structuralHopDepth(cfg),
	})
	if err != nil {
		return nil, err
	}

	hits := make([]RAGHit, 0, len(results))
	for _, result := range results {
		hits = append(hits, RAGHit{
			ChunkID:         result.Chunk.ID,
			FilePath:        result.Chunk.FilePath,
			Name:            result.Chunk.Name,
			Signature:       result.Chunk.Signature,
			Description:     result.Chunk.Description,
			Body:            result.Chunk.Body,
			SimilarityScore: result.Score,
			Language:        result.Chunk.Language,
			ChunkType:       result.Chunk.ChunkType,
			LineStart:       result.Chunk.LineStart,
			LineEnd:         result.Chunk.LineEnd,
			HitCount:        result.HitCount,
			FromHop:         result.FromHop,
			MatchedBy:       result.MatchedBy,
			Sources:         []string{"rag"},
		})
	}
	return hits, nil
}

func (o *RetrievalOrchestrator) retrieveExplicitFiles(ctx stdctx.Context, explicitFiles []string, cfg config.ContextConfig) ([]FileResult, error) {
	_ = ctx
	limit := cfg.MaxExplicitFiles
	if limit <= 0 {
		limit = defaultMaxExplicitFiles
	}
	maxBytes := o.maxExplicitFileBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxExplicitFileBytes
	}

	results := make([]FileResult, 0, min(limit, len(explicitFiles)))
	for i, file := range explicitFiles {
		if i >= limit {
			break
		}
		resolved, relative, ok := resolveProjectPath(o.projectRoot, file)
		if !ok {
			slog.Warn("context retrieval skipped path outside project root", "path", file)
			continue
		}
		content, err := o.fileReader(resolved)
		if err != nil {
			slog.Warn("context retrieval file read failed", "path", file, "error", err)
			continue
		}
		truncated := false
		if len(content) > maxBytes {
			content = content[:maxBytes]
			truncated = true
		}
		results = append(results, FileResult{
			FilePath:   relative,
			Content:    string(content),
			TokenCount: len(strings.Fields(string(content))),
			Truncated:  truncated,
		})
	}
	return results, nil
}

func (o *RetrievalOrchestrator) retrieveStructuralGraph(ctx stdctx.Context, symbols []string, cfg config.ContextConfig) ([]GraphHit, error) {
	seen := make(map[string]struct{})
	results := make([]GraphHit, 0)
	for _, symbol := range symbols {
		blast, err := o.graph.BlastRadius(ctx, codeintel.GraphQuery{
			Symbol:   symbol,
			MaxDepth: structuralHopDepth(cfg),
			MaxNodes: structuralHopBudget(cfg),
		})
		if err != nil {
			return nil, err
		}
		appendGraphNodes(&results, seen, blast.Upstream, "upstream")
		appendGraphNodes(&results, seen, blast.Downstream, "downstream")
		appendGraphNodes(&results, seen, blast.Interfaces, "interface")
	}
	return results, nil
}

func (o *RetrievalOrchestrator) retrieveConventions(ctx stdctx.Context) (string, error) {
	if o.conventions == nil {
		return "", nil
	}
	return o.conventions.Load(ctx)
}

func (o *RetrievalOrchestrator) retrieveGitContext(ctx stdctx.Context, depth int) (string, error) {
	if o.gitRunner == nil {
		return "", nil
	}
	if depth <= 0 {
		depth = defaultGitContextDepth
	}
	output, err := o.gitRunner(ctx, o.projectRoot, depth)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (o *RetrievalOrchestrator) postProcess(results *RetrievalResults, cfg config.ContextConfig) {
	results.RAGHits = filterAndDedupRAGHits(results.RAGHits, cfg)
	results.GraphHits = mergeGraphHitsIntoRAG(results.RAGHits, results.GraphHits)
}

func filterAndDedupRAGHits(hits []RAGHit, cfg config.ContextConfig) []RAGHit {
	threshold := cfg.RelevanceThreshold
	if threshold == 0 {
		threshold = defaultRelevanceThreshold
	}

	bestByChunk := make(map[string]RAGHit)
	order := make([]string, 0, len(hits))
	for _, hit := range hits {
		if hit.SimilarityScore < threshold {
			continue
		}
		if len(hit.Sources) == 0 {
			hit.Sources = []string{"rag"}
		}
		existing, ok := bestByChunk[hit.ChunkID]
		if !ok {
			bestByChunk[hit.ChunkID] = hit
			order = append(order, hit.ChunkID)
			continue
		}
		if hit.SimilarityScore > existing.SimilarityScore {
			for _, source := range existing.Sources {
				appendUnique(&hit.Sources, source)
			}
			bestByChunk[hit.ChunkID] = hit
			continue
		}
		for _, source := range hit.Sources {
			appendUnique(&existing.Sources, source)
		}
		bestByChunk[hit.ChunkID] = existing
	}

	filtered := make([]RAGHit, 0, len(order))
	for _, chunkID := range order {
		filtered = append(filtered, bestByChunk[chunkID])
	}
	return filtered
}

func mergeGraphHitsIntoRAG(ragHits []RAGHit, graphHits []GraphHit) []GraphHit {
	seenGraph := make(map[string]struct{})
	mergedGraphHits := make([]GraphHit, 0, len(graphHits))
	for _, graphHit := range graphHits {
		if idx := matchingRAGHitIndex(ragHits, graphHit); idx >= 0 {
			appendUnique(&ragHits[idx].Sources, "graph")
			continue
		}
		key := graphDedupKey(graphHit)
		if _, exists := seenGraph[key]; exists {
			continue
		}
		seenGraph[key] = struct{}{}
		mergedGraphHits = append(mergedGraphHits, graphHit)
	}
	return mergedGraphHits
}

func matchingRAGHitIndex(ragHits []RAGHit, graphHit GraphHit) int {
	for i := range ragHits {
		if ragHits[i].FilePath == graphHit.FilePath && ragHits[i].Name == graphHit.SymbolName {
			return i
		}
	}
	return -1
}

func appendGraphNodes(dst *[]GraphHit, seen map[string]struct{}, nodes []codeintel.GraphNode, relationship string) {
	for _, node := range nodes {
		hit := GraphHit{
			ChunkID:          graphNodeChunkID(node),
			SymbolName:       node.Symbol,
			FilePath:         node.FilePath,
			RelationshipType: relationship,
			Depth:            node.Depth,
			LineStart:        node.LineStart,
			LineEnd:          node.LineEnd,
		}
		key := graphDedupKey(hit)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		*dst = append(*dst, hit)
	}
}

func graphNodeChunkID(node codeintel.GraphNode) string {
	return fmt.Sprintf("%s#%s", node.FilePath, node.Symbol)
}

func graphDedupKey(hit GraphHit) string {
	return fmt.Sprintf("%s|%s|%s", hit.FilePath, hit.SymbolName, hit.RelationshipType)
}

func retrievalMaxResults(cfg config.ContextConfig) int {
	if cfg.MaxChunks > 0 {
		return cfg.MaxChunks
	}
	return defaultRetrievalMaxResults
}

func structuralHopDepth(cfg config.ContextConfig) int {
	if cfg.StructuralHopDepth > 0 {
		return cfg.StructuralHopDepth
	}
	return defaultStructuralHopDepth
}

func structuralHopBudget(cfg config.ContextConfig) int {
	if cfg.StructuralHopBudget > 0 {
		return cfg.StructuralHopBudget
	}
	return defaultStructuralHopBudget
}

func resolveProjectPath(projectRoot string, requested string) (string, string, bool) {
	if requested == "" {
		return "", "", false
	}
	cleanRequested := filepath.Clean(filepath.FromSlash(requested))
	resolved := cleanRequested
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(projectRoot, cleanRequested)
	}
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", "", false
	}
	absResolved, err := filepath.Abs(resolved)
	if err != nil {
		return "", "", false
	}
	rel, err := filepath.Rel(absRoot, absResolved)
	if err != nil {
		return "", "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", "", false
	}
	return absResolved, filepath.ToSlash(rel), true
}

func defaultGitRunner(ctx stdctx.Context, workdir string, depth int) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "log", "--oneline", fmt.Sprintf("-%d", depth))
	cmd.Dir = workdir
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
