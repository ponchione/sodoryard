package context

import (
	stdctx "context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/codeintel"
	"github.com/ponchione/sodoryard/internal/config"
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

// brainKeywordStopwords filters function words out of proactive brain
// keyword-search queries so the stopword-stripped candidate falls closer to
// content tokens that are likely to appear verbatim in brain notes.
//
// Keep this list conservative: only very common English function words plus a
// small set of question/filler verbs. Every entry here also costs one
// candidate form, so low-signal words deserve the stopword treatment but
// domain nouns (e.g. "brain", "layout", "auth") must NOT be added.
var brainKeywordStopwords = map[string]struct{}{
	// articles / determiners
	"a": {}, "an": {}, "the": {},
	"this": {}, "that": {}, "these": {}, "those": {},
	"there": {}, "here": {},
	// common pronouns
	"i": {}, "me": {}, "my": {}, "mine": {},
	"we": {}, "us": {}, "our": {}, "ours": {},
	"you": {}, "your": {}, "yours": {},
	"they": {}, "them": {}, "their": {}, "theirs": {},
	"it": {}, "its": {},
	// auxiliaries / copula
	"is": {}, "am": {}, "are": {}, "was": {}, "were": {},
	"be": {}, "been": {}, "being": {},
	"have": {}, "has": {}, "had": {}, "having": {},
	"do": {}, "does": {}, "did": {}, "doing": {},
	"will": {}, "would": {}, "shall": {}, "should": {},
	"can": {}, "could": {}, "may": {}, "might": {}, "must": {},
	// prepositions
	"to": {}, "of": {}, "in": {}, "on": {}, "at": {}, "by": {},
	"for": {}, "from": {}, "with": {}, "into": {}, "onto": {},
	"about": {}, "through": {}, "between": {}, "among": {},
	"as": {}, "over": {}, "under": {},
	// coordinators / common adverbs
	"and": {}, "or": {}, "but": {}, "if": {}, "so": {},
	"not": {}, "no": {}, "yes": {},
	// question words
	"what": {}, "when": {}, "where": {}, "why": {}, "how": {},
	"which": {}, "who": {}, "whom": {}, "whose": {},
	// common request filler verbs
	"explain": {}, "walk": {}, "show": {}, "tell": {}, "describe": {},
	"summarize": {}, "summarise": {}, "cite": {},
	// legacy/historical entries kept for continuity
	"answer": {}, "any": {}, "help": {}, "only": {}, "phrase": {},
	"please": {}, "reply": {},
}

// brainKeywordMinFallbackWords gates the single-keyword fallback candidate:
// if the stopword-stripped query still contains at least this many content
// tokens, brainKeywordCandidates also emits a last-resort candidate that is
// just the longest (= most distinctive) content word. That keyword almost
// always still appears in any brain note that was written about the topic,
// even when prose punctuation or list formatting prevents a multi-word
// substring match against the underlying vault keyword backend.
//
// The threshold is intentionally high so the fallback only fires for long,
// prose-shaped turns; short specific queries like "auth middleware" keep
// using the existing two-candidate form without gaining a noisy single-word
// fallback that could pull in unrelated notes.
const brainKeywordMinFallbackWords = 4

// brainKeywordMinFallbackWordLen is the minimum length a single-token
// fallback candidate must reach before it is emitted. It keeps the fallback
// from collapsing onto short tokens like "api" or "dev" which would match
// too many unrelated notes.
const brainKeywordMinFallbackWordLen = 5

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
	brain                BrainSearcher
	brainCfg             config.BrainConfig
	projectRoot          string
	fileReader           fileReaderFunc
	gitRunner            gitRunnerFunc
	timeout              time.Duration
	maxExplicitFileBytes int
	logBrainQueries      bool
	brainQueryTrace      func(msg string, args ...any)
}

// NewRetrievalOrchestrator constructs the concrete retriever used by context assembly.
func NewRetrievalOrchestrator(searcher codeintel.Searcher, graph codeintel.GraphStore, conventions ConventionSource, brain BrainSearcher, projectRoot string) *RetrievalOrchestrator {
	if conventions == nil {
		conventions = NoopConventionSource{}
	}
	return &RetrievalOrchestrator{
		searcher:             searcher,
		graph:                graph,
		conventions:          conventions,
		brain:                brain,
		projectRoot:          projectRoot,
		fileReader:           os.ReadFile,
		gitRunner:            defaultGitRunner,
		timeout:              defaultRetrievalTimeout,
		maxExplicitFileBytes: defaultMaxExplicitFileBytes,
		brainQueryTrace:      slog.Debug,
	}
}

func (o *RetrievalOrchestrator) SetLogBrainQueries(enabled bool) {
	if o == nil {
		return
	}
	o.logBrainQueries = enabled
}

func (o *RetrievalOrchestrator) SetBrainConfig(cfg config.BrainConfig) {
	if o == nil {
		return
	}
	o.brainCfg = cfg
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
		brainHits      []BrainHit
		fileResults    []FileResult
		conventionText string
		gitContext     string
	)

	var wg sync.WaitGroup
	runPath := func(label string, enabled bool, fn func(stdctx.Context) error) {
		if !enabled {
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			pathCtx, cancel := o.pathContext(ctx)
			defer cancel()
			if err := fn(pathCtx); err != nil {
				slog.Warn("context retrieval "+label+" failed", "error", err)
			}
		}()
	}

	runPath("semantic search", shouldRunSemanticSearch(needs, queries) && o.searcher != nil, func(pathCtx stdctx.Context) error {
		hits, err := o.retrieveSemanticSearch(pathCtx, queries, cfg)
		if err == nil {
			ragHits = hits
		}
		return err
	})

	runPath("explicit files", len(needs.ExplicitFiles) > 0, func(pathCtx stdctx.Context) error {
		results, err := o.retrieveExplicitFiles(pathCtx, needs.ExplicitFiles, cfg)
		if err == nil {
			fileResults = results
		}
		return err
	})

	runPath("structural graph", len(needs.ExplicitSymbols) > 0 && o.graph != nil, func(pathCtx stdctx.Context) error {
		hits, err := o.retrieveStructuralGraph(pathCtx, needs.ExplicitSymbols, cfg)
		if err == nil {
			graphHits = hits
		}
		return err
	})

	runPath("brain search", len(queries) > 0 && o.brain != nil, func(pathCtx stdctx.Context) error {
		hits, err := o.retrieveBrainSearch(pathCtx, queries, cfg)
		if err == nil {
			brainHits = hits
		}
		return err
	})

	runPath("conventions", needs.IncludeConventions, func(pathCtx stdctx.Context) error {
		text, err := o.retrieveConventions(pathCtx)
		if err == nil {
			conventionText = text
		}
		return err
	})

	runPath("git context", needs.IncludeGitContext, func(pathCtx stdctx.Context) error {
		text, err := o.retrieveGitContext(pathCtx, needs.GitContextDepth)
		if err == nil {
			gitContext = text
		}
		return err
	})

	wg.Wait()

	results := &RetrievalResults{
		RAGHits:        ragHits,
		BrainHits:      brainHits,
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

func shouldRunSemanticSearch(needs *ContextNeeds, queries []string) bool {
	if len(queries) == 0 {
		return false
	}
	if needs == nil {
		return true
	}
	if !needs.PreferBrainContext {
		return true
	}
	if len(needs.ExplicitFiles) > 0 || len(needs.ExplicitSymbols) > 0 {
		return true
	}
	return false
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

func (o *RetrievalOrchestrator) retrieveBrainSearch(ctx stdctx.Context, queries []string, cfg config.ContextConfig) ([]BrainHit, error) {
	if o.brain == nil || len(queries) == 0 {
		return nil, nil
	}

	seen := make(map[string]BrainHit)
	for _, query := range queries {
		candidates := brainKeywordCandidates(query)
		o.traceBrainQuery("proactive brain search", "raw_query", query, "candidates", candidates)
		if len(candidates) == 0 {
			continue
		}
		for _, candidateQuery := range candidates {
			hits, err := o.brain.Search(ctx, BrainSearchRequest{
				Query:            candidateQuery,
				Mode:             "auto",
				MaxResults:       retrievalMaxResults(cfg),
				IncludeGraphHops: o.brainCfg.IncludeGraphHops,
				GraphHopDepth:    o.brainCfg.GraphHopDepth,
			})
			o.traceBrainQuery("proactive brain search result", "candidate", candidateQuery, "hit_count", len(hits), "err", err)
			if err != nil {
				return nil, err
			}

			keptAny := false
			for _, hit := range hits {
				path := strings.TrimSpace(hit.DocumentPath)
				if path == "" || brain.IsOperationalDocument(path) || hit.FinalScore < brainRelevanceThreshold(o.brainCfg) {
					continue
				}
				candidate := BrainHit{
					DocumentPath:    path,
					Title:           firstNonEmpty(hit.Title, filepath.Base(path)),
					SectionHeading:  strings.TrimSpace(hit.SectionHeading),
					Snippet:         strings.TrimSpace(hit.Snippet),
					LexicalScore:    hit.LexicalScore,
					SemanticScore:   hit.SemanticScore,
					MatchScore:      hit.FinalScore,
					MatchMode:       hit.MatchMode,
					MatchSources:    append([]string(nil), hit.MatchSources...),
					GraphSourcePath: strings.TrimSpace(hit.GraphSourcePath),
					GraphHopDepth:   hit.GraphHopDepth,
					Tags:            append([]string(nil), hit.Tags...),
				}
				existing, ok := seen[path]
				if !ok || candidate.MatchScore > existing.MatchScore {
					seen[path] = candidate
				}
				keptAny = true
			}
			if keptAny {
				break
			}
		}
	}

	results := make([]BrainHit, 0, len(seen))
	for _, hit := range seen {
		results = append(results, hit)
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].MatchScore != results[j].MatchScore {
			return results[i].MatchScore > results[j].MatchScore
		}
		return results[i].DocumentPath < results[j].DocumentPath
	})
	return results, nil
}

// brainKeywordCandidates converts a raw semantic query into an ordered list
// of keyword-search candidates that are tried against the brain backend in
// sequence. The orchestrator stops at the first candidate that yields at least
// one retained brain hit after local filtering, so earlier candidates should
// be more specific than later ones.
//
// Order:
//  1. raw query — highest specificity; catches cases where a note body
//     contains the question verbatim (e.g. canary notes).
//  2. stopword-stripped join — drops common function words so prose-shaped
//     queries collapse to their content skeleton.
//  3. distinctive-content-word fallback — when the stopword-stripped form is
//     still long enough (>= brainKeywordMinFallbackWords), also emit a short
//     phrase built from the top-N longest content words. This handles cases
//     where the full stopword-stripped query cannot substring-match a note
//     because of punctuation, hyphens, or list formatting in the note body.
const defaultBrainRelevanceThreshold = 0.30

func brainRelevanceThreshold(cfg config.BrainConfig) float64 {
	if cfg.BrainRelevanceThreshold <= 0 {
		return defaultBrainRelevanceThreshold
	}
	return cfg.BrainRelevanceThreshold
}

func (o *RetrievalOrchestrator) traceBrainQuery(msg string, args ...any) {
	if o == nil || !o.logBrainQueries || o.brainQueryTrace == nil {
		return
	}
	o.brainQueryTrace(msg, args...)
}

func brainKeywordCandidates(query string) []string {
	query = collapseWhitespace(strings.TrimSpace(strings.ToLower(query)))
	if query == "" {
		return nil
	}
	candidates := []string{query}
	words := strings.Fields(query)
	filtered := make([]string, 0, len(words))
	for _, word := range words {
		if _, stop := brainKeywordStopwords[word]; stop {
			continue
		}
		filtered = append(filtered, word)
	}
	if len(filtered) == 0 {
		return candidates
	}
	normalized := strings.Join(filtered, " ")
	if normalized != query {
		candidates = append(candidates, normalized)
	}
	if len(filtered) >= brainKeywordMinFallbackWords {
		if fallback := brainKeywordDistinctiveFallback(filtered); fallback != "" && fallback != normalized {
			candidates = append(candidates, fallback)
		}
	}
	return candidates
}

// brainKeywordDistinctiveFallback returns the longest content word in the
// filtered token list as a last-resort single-keyword candidate, or the empty
// string when no token is long enough to meet brainKeywordMinFallbackWordLen.
//
// Ties on length are broken by original order so the fallback stays
// deterministic. The fallback is intentionally a single word: a longest-word
// substring is almost certain to appear in any note that discusses the same
// topic, whereas multi-word fallbacks still lose to prose punctuation in the
// underlying vault keyword backend.
func brainKeywordDistinctiveFallback(filtered []string) string {
	best := ""
	for _, word := range filtered {
		if len(word) < brainKeywordMinFallbackWordLen {
			continue
		}
		if len(word) > len(best) {
			best = word
		}
	}
	return best
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
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
			TokenCount: len(content) / 4, // chars/4 approximation consistent with approximateTokenCount
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
