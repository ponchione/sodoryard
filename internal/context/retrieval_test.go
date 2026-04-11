package context

import (
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/codeintel"
	"github.com/ponchione/sodoryard/internal/config"
)

type retrievalSearcherStub struct {
	results    []codeintel.SearchResult
	err        error
	delay      time.Duration
	calls      int
	gotQueries []string
	gotOpts    codeintel.SearchOptions
}

func (s *retrievalSearcherStub) Search(ctx stdctx.Context, queries []string, opts codeintel.SearchOptions) ([]codeintel.SearchResult, error) {
	s.calls++
	s.gotQueries = append([]string{}, queries...)
	s.gotOpts = opts
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	return append([]codeintel.SearchResult(nil), s.results...), nil
}

type retrievalGraphStoreStub struct {
	result  *codeintel.BlastRadiusResult
	err     error
	delay   time.Duration
	calls   int
	queries []codeintel.GraphQuery
}

func (s *retrievalGraphStoreStub) BlastRadius(ctx stdctx.Context, query codeintel.GraphQuery) (*codeintel.BlastRadiusResult, error) {
	s.calls++
	s.queries = append(s.queries, query)
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.result == nil {
		return &codeintel.BlastRadiusResult{}, nil
	}
	return s.result, nil
}

func (s *retrievalGraphStoreStub) Close() error { return nil }

type retrievalConventionSourceStub struct {
	text  string
	err   error
	delay time.Duration
	calls int
}

func (s *retrievalConventionSourceStub) Load(ctx stdctx.Context) (string, error) {
	s.calls++
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if s.err != nil {
		return "", s.err
	}
	return s.text, nil
}

type retrievalBrainSearcherStub struct {
	hits        []brain.SearchHit
	hitsByQuery map[string][]brain.SearchHit
	err         error
	delay       time.Duration
	calls       int
	gotQueries  []string
	gotRequests []BrainSearchRequest
}

func (s *retrievalBrainSearcherStub) Search(ctx stdctx.Context, request BrainSearchRequest) ([]BrainSearchResult, error) {
	s.calls++
	s.gotQueries = append(s.gotQueries, request.Query)
	s.gotRequests = append(s.gotRequests, request)
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	var hits []brain.SearchHit
	if s.hitsByQuery != nil {
		hits = append([]brain.SearchHit(nil), s.hitsByQuery[request.Query]...)
	} else {
		hits = append([]brain.SearchHit(nil), s.hits...)
	}
	results := make([]BrainSearchResult, 0, len(hits))
	for _, hit := range hits {
		results = append(results, BrainSearchResult{
			DocumentPath: hit.Path,
			Snippet:      hit.Snippet,
			LexicalScore: hit.Score,
			FinalScore:   hit.Score,
			MatchMode:    "keyword",
			MatchSources: []string{"keyword"},
		})
	}
	return results, nil
}

func TestRetrievalOrchestratorNormalizesBrainKeywordQueries(t *testing.T) {
	brainSearcher := &retrievalBrainSearcherStub{hitsByQuery: map[string][]brain.SearchHit{
		"runtime brain proof canary": {{Path: "notes/runtime-brain-proof-apr-07.md", Snippet: "canary phrase", Score: 0.9}},
	}}
	orchestrator := NewRetrievalOrchestrator(nil, nil, NoopConventionSource{}, brainSearcher, t.TempDir())

	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{}, []string{"what is the runtime brain proof canary"}, config.ContextConfig{})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if !slices.Equal(brainSearcher.gotQueries, []string{"what is the runtime brain proof canary", "runtime brain proof canary"}) {
		t.Fatalf("brain queries = %v, want normalized fallback", brainSearcher.gotQueries)
	}
	if len(results.BrainHits) != 1 || results.BrainHits[0].DocumentPath != "notes/runtime-brain-proof-apr-07.md" {
		t.Fatalf("BrainHits = %v, want normalized query hit", results.BrainHits)
	}
}

func TestBrainKeywordCandidatesEmitsLongestWordFallbackForProseQueries(t *testing.T) {
	query := "walk me through the rationale behind our minimal content first layout decision"
	got := brainKeywordCandidates(query)

	want := []string{
		"walk me through the rationale behind our minimal content first layout decision",
		"rationale behind minimal content first layout decision",
		"rationale",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("brainKeywordCandidates:\n got  = %#v\n want = %#v", got, want)
	}
}

func TestBrainKeywordCandidatesDoesNotEmitFallbackForShortSpecificQueries(t *testing.T) {
	query := "auth middleware"
	got := brainKeywordCandidates(query)

	want := []string{"auth middleware"}
	if !slices.Equal(got, want) {
		t.Fatalf("brainKeywordCandidates:\n got  = %#v\n want = %#v", got, want)
	}
}

func TestRetrievalOrchestratorLongestWordFallbackHitsBrainNote(t *testing.T) {
	brainSearcher := &retrievalBrainSearcherStub{hitsByQuery: map[string][]brain.SearchHit{
		// The first two candidates return nothing so the fallback must fire.
		"rationale": {{Path: "notes/minimal-content-first-layout-rationale.md", Snippet: "Rationale: ...", Score: 0.75}},
	}}
	orchestrator := NewRetrievalOrchestrator(nil, nil, NoopConventionSource{}, brainSearcher, t.TempDir())

	results, err := orchestrator.Retrieve(
		stdctx.Background(),
		&ContextNeeds{PreferBrainContext: true},
		[]string{"walk me through the rationale behind our minimal content first layout decision"},
		config.ContextConfig{},
	)
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	wantCandidates := []string{
		"walk me through the rationale behind our minimal content first layout decision",
		"rationale behind minimal content first layout decision",
		"rationale",
	}
	if !slices.Equal(brainSearcher.gotQueries, wantCandidates) {
		t.Fatalf("brain queries = %v, want fallback cascade %v", brainSearcher.gotQueries, wantCandidates)
	}
	if len(results.BrainHits) != 1 || results.BrainHits[0].DocumentPath != "notes/minimal-content-first-layout-rationale.md" {
		t.Fatalf("BrainHits = %v, want rationale note", results.BrainHits)
	}
}

func TestRetrievalOrchestratorExcludesOperationalBrainLogHits(t *testing.T) {
	brainSearcher := &retrievalBrainSearcherStub{hitsByQuery: map[string][]brain.SearchHit{
		"runtime brain proof canary": {
			{Path: ".brain/_log.md", Snippet: "recent session log", Score: 0.99},
			{Path: "notes/runtime-brain-proof-apr-07.md", Snippet: "ORBIT LANTERN 642", Score: 0.91},
		},
	}}
	orchestrator := NewRetrievalOrchestrator(nil, nil, NoopConventionSource{}, brainSearcher, t.TempDir())

	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{}, []string{"runtime brain proof canary"}, config.ContextConfig{})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if !slices.Equal(brainSearcher.gotQueries, []string{"runtime brain proof canary"}) {
		t.Fatalf("brain queries = %v, want exact query only", brainSearcher.gotQueries)
	}
	if len(results.BrainHits) != 1 {
		t.Fatalf("len(BrainHits) = %d, want 1 after excluding operational log", len(results.BrainHits))
	}
	if results.BrainHits[0].DocumentPath != "notes/runtime-brain-proof-apr-07.md" {
		t.Fatalf("DocumentPath = %q, want notes/runtime-brain-proof-apr-07.md", results.BrainHits[0].DocumentPath)
	}
}

func TestRetrievalOrchestratorHonorsBrainRelevanceThreshold(t *testing.T) {
	brainSearcher := &retrievalBrainSearcherStub{hitsByQuery: map[string][]brain.SearchHit{
		"auth middleware": {
			{Path: "notes/high.md", Snippet: "high relevance", Score: 0.82},
			{Path: "notes/low.md", Snippet: "low relevance", Score: 0.41},
		},
	}}
	orchestrator := NewRetrievalOrchestrator(nil, nil, NoopConventionSource{}, brainSearcher, t.TempDir())
	orchestrator.SetBrainConfig(config.BrainConfig{BrainRelevanceThreshold: 0.5})

	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{PreferBrainContext: true}, []string{"auth middleware"}, config.ContextConfig{})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if len(results.BrainHits) != 1 {
		t.Fatalf("len(BrainHits) = %d, want 1 above threshold", len(results.BrainHits))
	}
	if results.BrainHits[0].DocumentPath != "notes/high.md" {
		t.Fatalf("DocumentPath = %q, want notes/high.md", results.BrainHits[0].DocumentPath)
	}
}

func TestRetrievalOrchestratorFallsBackWhenEarlyBrainHitsAreFilteredOut(t *testing.T) {
	brainSearcher := &retrievalBrainSearcherStub{hitsByQuery: map[string][]brain.SearchHit{
		"what is the runtime brain proof canary": {
			{Path: "notes/too-low.md", Snippet: "below threshold", Score: 0.41},
		},
		"runtime brain proof canary": {
			{Path: "notes/runtime-brain-proof-apr-07.md", Snippet: "ORBIT LANTERN 642", Score: 0.91},
		},
	}}
	orchestrator := NewRetrievalOrchestrator(nil, nil, NoopConventionSource{}, brainSearcher, t.TempDir())
	orchestrator.SetBrainConfig(config.BrainConfig{BrainRelevanceThreshold: 0.5})

	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{PreferBrainContext: true}, []string{"what is the runtime brain proof canary"}, config.ContextConfig{})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	wantQueries := []string{"what is the runtime brain proof canary", "runtime brain proof canary"}
	if !slices.Equal(brainSearcher.gotQueries, wantQueries) {
		t.Fatalf("brain queries = %v, want fallback after filtered-out early hits %v", brainSearcher.gotQueries, wantQueries)
	}
	if len(results.BrainHits) != 1 || results.BrainHits[0].DocumentPath != "notes/runtime-brain-proof-apr-07.md" {
		t.Fatalf("BrainHits = %v, want fallback candidate hit", results.BrainHits)
	}
}

func TestRetrievalOrchestratorSuppressesProactiveBrainTraceWhenDisabled(t *testing.T) {
	brainSearcher := &retrievalBrainSearcherStub{hitsByQuery: map[string][]brain.SearchHit{
		"runtime brain proof canary": {{Path: "notes/runtime-brain-proof-apr-07.md", Snippet: "ORBIT LANTERN 642", Score: 0.91}},
	}}
	orchestrator := NewRetrievalOrchestrator(nil, nil, NoopConventionSource{}, brainSearcher, t.TempDir())
	var traceCalls []string
	orchestrator.brainQueryTrace = func(msg string, args ...any) {
		traceCalls = append(traceCalls, msg)
	}

	_, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{}, []string{"runtime brain proof canary"}, config.ContextConfig{})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if len(traceCalls) != 0 {
		t.Fatalf("trace calls = %v, want none when logBrainQueries disabled", traceCalls)
	}
}

func TestRetrievalOrchestratorEmitsProactiveBrainTraceWhenEnabled(t *testing.T) {
	brainSearcher := &retrievalBrainSearcherStub{hitsByQuery: map[string][]brain.SearchHit{
		"runtime brain proof canary": {{Path: "notes/runtime-brain-proof-apr-07.md", Snippet: "ORBIT LANTERN 642", Score: 0.91}},
	}}
	orchestrator := NewRetrievalOrchestrator(nil, nil, NoopConventionSource{}, brainSearcher, t.TempDir())
	orchestrator.logBrainQueries = true
	var traceCalls []string
	orchestrator.brainQueryTrace = func(msg string, args ...any) {
		traceCalls = append(traceCalls, msg)
	}

	_, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{}, []string{"runtime brain proof canary"}, config.ContextConfig{})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if !slices.Equal(traceCalls, []string{"proactive brain search", "proactive brain search result"}) {
		t.Fatalf("trace calls = %v, want proactive brain search traces", traceCalls)
	}
}

func TestRetrievalOrchestratorSkipsSemanticSearchForBrainPreferredTurns(t *testing.T) {
	searcher := &retrievalSearcherStub{results: []codeintel.SearchResult{{
		Chunk: codeintel.Chunk{ID: "chunk-1", FilePath: "internal/auth/service.go", Name: "ValidateToken"},
		Score: 0.91,
	}}}
	brainSearcher := &retrievalBrainSearcherStub{hitsByQuery: map[string][]brain.SearchHit{
		"runtime brain proof canary": {{Path: "notes/runtime-brain-proof-apr-07.md", Snippet: "ORBIT LANTERN 642", Score: 0.91}},
	}}
	orchestrator := NewRetrievalOrchestrator(searcher, nil, NoopConventionSource{}, brainSearcher, t.TempDir())

	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{PreferBrainContext: true}, []string{"runtime brain proof canary"}, config.ContextConfig{})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if searcher.calls != 0 {
		t.Fatalf("searcher calls = %d, want 0 when brain context is preferred", searcher.calls)
	}
	if len(results.RAGHits) != 0 {
		t.Fatalf("RAGHits = %v, want no semantic search results", results.RAGHits)
	}
	if len(results.BrainHits) != 1 || results.BrainHits[0].DocumentPath != "notes/runtime-brain-proof-apr-07.md" {
		t.Fatalf("BrainHits = %v, want retained brain hit", results.BrainHits)
	}
}

func TestRetrievalOrchestratorSkipsSemanticSearchForLayoutGraphBrainPrompt(t *testing.T) {
	searcher := &retrievalSearcherStub{results: []codeintel.SearchResult{{
		Chunk: codeintel.Chunk{ID: "chunk-1", FilePath: "src/components/SideNav.tsx", Name: "SideNav"},
		Score: 0.91,
	}}}
	brainSearcher := &retrievalBrainSearcherStub{hitsByQuery: map[string][]brain.SearchHit{
		"from our layout graph notes what linked layout canary phrase sits behind saturn rail": {
			{Path: "notes/layout-graph-bridge.md", Snippet: "SATURN RAIL is the bridge term.", Score: 0.52},
			{Path: "notes/layout-graph-proof.md", Snippet: "The linked layout canary phrase is PROSE FIRST 17.", Score: 0.50},
		},
	}}
	orchestrator := NewRetrievalOrchestrator(searcher, nil, NoopConventionSource{}, brainSearcher, t.TempDir())
	analyzer := RuleBasedAnalyzer{}
	prompt := "From our layout graph notes, what linked layout canary phrase sits behind SATURN RAIL?"
	needs := analyzer.AnalyzeTurn(prompt, nil)

	results, err := orchestrator.Retrieve(stdctx.Background(), needs, []string{"from our layout graph notes what linked layout canary phrase sits behind saturn rail"}, config.ContextConfig{})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if !needs.PreferBrainContext {
		t.Fatal("PreferBrainContext = false, want true for layout graph brain prompt")
	}
	if searcher.calls != 0 {
		t.Fatalf("searcher calls = %d, want 0 when layout graph prompt prefers brain context", searcher.calls)
	}
	if len(results.RAGHits) != 0 {
		t.Fatalf("RAGHits = %v, want no semantic search results for layout graph brain prompt", results.RAGHits)
	}
	if len(results.BrainHits) != 2 {
		t.Fatalf("len(BrainHits) = %d, want 2 retained layout brain hits", len(results.BrainHits))
	}
	if results.BrainHits[1].DocumentPath != "notes/layout-graph-proof.md" {
		t.Fatalf("second BrainHit = %+v, want layout graph proof note", results.BrainHits[1])
	}
}

func TestNoopConventionSourceReturnsEmptyString(t *testing.T) {
	source := NoopConventionSource{}

	text, err := source.Load(stdctx.Background())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if text != "" {
		t.Fatalf("text = %q, want empty", text)
	}
}

func TestRetrievalOrchestratorRunsAllEnabledPathsAndMapsResults(t *testing.T) {
	projectRoot := t.TempDir()
	mustWriteFile(t, projectRoot, "internal/auth/middleware.go", "package auth\n\nfunc ValidateToken() {}\n")

	searcher := &retrievalSearcherStub{results: []codeintel.SearchResult{{
		Chunk: codeintel.Chunk{
			ID:          "chunk-1",
			FilePath:    "internal/auth/service.go",
			Name:        "ValidateToken",
			Signature:   "func ValidateToken() error",
			Description: "Validates tokens.",
			Body:        "func ValidateToken() error { return nil }",
			Language:    "go",
			ChunkType:   codeintel.ChunkTypeFunction,
			LineStart:   10,
			LineEnd:     20,
		},
		Score:     0.91,
		MatchedBy: "auth middleware",
		HitCount:  2,
	}}}
	graph := &retrievalGraphStoreStub{result: &codeintel.BlastRadiusResult{Upstream: []codeintel.GraphNode{{
		Symbol:    "AuthHandler",
		FilePath:  "internal/auth/handler.go",
		Kind:      "function",
		Depth:     1,
		LineStart: 5,
		LineEnd:   18,
	}}}}
	conventions := &retrievalConventionSourceStub{text: "- use table-driven tests"}
	brainSearcher := &retrievalBrainSearcherStub{hits: []brain.SearchHit{{
		Path:    "notes/auth.md",
		Snippet: "Auth middleware decisions live here.",
		Score:   0.87,
	}}}

	orchestrator := NewRetrievalOrchestrator(searcher, graph, conventions, brainSearcher, projectRoot)
	var gotGitDepth int
	var gotGitDir string
	orchestrator.gitRunner = func(ctx stdctx.Context, workdir string, depth int) (string, error) {
		gotGitDepth = depth
		gotGitDir = workdir
		return "abc123 fix auth", nil
	}

	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{
		ExplicitFiles:      []string{"internal/auth/middleware.go"},
		ExplicitSymbols:    []string{"ValidateToken"},
		IncludeConventions: true,
		IncludeGitContext:  true,
		GitContextDepth:    3,
	}, []string{"auth middleware"}, config.ContextConfig{
		MaxChunks:           25,
		MaxExplicitFiles:    5,
		RelevanceThreshold:  0.35,
		StructuralHopDepth:  1,
		StructuralHopBudget: 10,
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}

	if searcher.calls != 1 {
		t.Fatalf("searcher calls = %d, want 1", searcher.calls)
	}
	if !slices.Equal(searcher.gotQueries, []string{"auth middleware"}) {
		t.Fatalf("queries = %v, want [auth middleware]", searcher.gotQueries)
	}
	if searcher.gotOpts.TopK != 10 {
		t.Fatalf("TopK = %d, want 10", searcher.gotOpts.TopK)
	}
	if searcher.gotOpts.HopBudgetFraction != 0.5 {
		t.Fatalf("HopBudgetFraction = %v, want 0.5", searcher.gotOpts.HopBudgetFraction)
	}
	if searcher.gotOpts.MaxResults != 25 {
		t.Fatalf("MaxResults = %d, want 25", searcher.gotOpts.MaxResults)
	}

	if len(results.RAGHits) != 1 {
		t.Fatalf("len(RAGHits) = %d, want 1", len(results.RAGHits))
	}
	if results.RAGHits[0].ChunkID != "chunk-1" {
		t.Fatalf("ChunkID = %q, want chunk-1", results.RAGHits[0].ChunkID)
	}
	if !slices.Equal(results.RAGHits[0].Sources, []string{"rag"}) {
		t.Fatalf("Sources = %v, want [rag]", results.RAGHits[0].Sources)
	}

	if len(results.FileResults) != 1 {
		t.Fatalf("len(FileResults) = %d, want 1", len(results.FileResults))
	}
	if results.FileResults[0].FilePath != "internal/auth/middleware.go" {
		t.Fatalf("FilePath = %q, want internal/auth/middleware.go", results.FileResults[0].FilePath)
	}
	if results.FileResults[0].Content == "" {
		t.Fatal("expected file content")
	}

	if len(results.GraphHits) != 1 {
		t.Fatalf("len(GraphHits) = %d, want 1", len(results.GraphHits))
	}
	if results.GraphHits[0].RelationshipType != "upstream" {
		t.Fatalf("RelationshipType = %q, want upstream", results.GraphHits[0].RelationshipType)
	}
	if brainSearcher.calls != 1 {
		t.Fatalf("brain search calls = %d, want 1", brainSearcher.calls)
	}
	if !slices.Equal(brainSearcher.gotQueries, []string{"auth middleware"}) {
		t.Fatalf("brain queries = %v, want [auth middleware]", brainSearcher.gotQueries)
	}
	if len(results.BrainHits) != 1 {
		t.Fatalf("len(BrainHits) = %d, want 1", len(results.BrainHits))
	}
	if results.BrainHits[0].DocumentPath != "notes/auth.md" {
		t.Fatalf("DocumentPath = %q, want notes/auth.md", results.BrainHits[0].DocumentPath)
	}
	if results.BrainHits[0].MatchMode != "keyword" {
		t.Fatalf("MatchMode = %q, want keyword", results.BrainHits[0].MatchMode)
	}

	if results.ConventionText != "- use table-driven tests" {
		t.Fatalf("ConventionText = %q, want convention text", results.ConventionText)
	}
	if results.GitContext != "abc123 fix auth" {
		t.Fatalf("GitContext = %q, want git output", results.GitContext)
	}
	if gotGitDepth != 3 {
		t.Fatalf("git depth = %d, want 3", gotGitDepth)
	}
	if gotGitDir != projectRoot {
		t.Fatalf("git dir = %q, want %q", gotGitDir, projectRoot)
	}
}

func TestRetrievalOrchestratorFiltersAndMergesOverlappingGraphHits(t *testing.T) {
	searcher := &retrievalSearcherStub{results: []codeintel.SearchResult{
		{
			Chunk: codeintel.Chunk{ID: "chunk-good", FilePath: "internal/auth/service.go", Name: "ValidateToken", ChunkType: codeintel.ChunkTypeFunction},
			Score: 0.81,
		},
		{
			Chunk: codeintel.Chunk{ID: "chunk-low", FilePath: "internal/auth/noise.go", Name: "Noise", ChunkType: codeintel.ChunkTypeFunction},
			Score: 0.20,
		},
	}}
	graph := &retrievalGraphStoreStub{result: &codeintel.BlastRadiusResult{Downstream: []codeintel.GraphNode{{
		Symbol:    "ValidateToken",
		FilePath:  "internal/auth/service.go",
		Depth:     1,
		LineStart: 10,
		LineEnd:   20,
	}}}}
	orchestrator := NewRetrievalOrchestrator(searcher, graph, NoopConventionSource{}, nil, t.TempDir())

	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{ExplicitSymbols: []string{"ValidateToken"}}, []string{"auth token"}, config.ContextConfig{RelevanceThreshold: 0.35})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}

	if len(results.RAGHits) != 1 {
		t.Fatalf("len(RAGHits) = %d, want 1", len(results.RAGHits))
	}
	if results.RAGHits[0].ChunkID != "chunk-good" {
		t.Fatalf("ChunkID = %q, want chunk-good", results.RAGHits[0].ChunkID)
	}
	if !slices.Contains(results.RAGHits[0].Sources, "rag") || !slices.Contains(results.RAGHits[0].Sources, "graph") {
		t.Fatalf("Sources = %v, want rag and graph", results.RAGHits[0].Sources)
	}
	if len(results.GraphHits) != 0 {
		t.Fatalf("GraphHits = %v, want overlap to be merged into RAG hit", results.GraphHits)
	}
}

func TestRetrievalOrchestratorSkipsTraversalAndMissingFilesGracefully(t *testing.T) {
	projectRoot := t.TempDir()
	mustWriteFile(t, projectRoot, "internal/auth/middleware.go", "0123456789abcdef")

	orchestrator := NewRetrievalOrchestrator(nil, nil, NoopConventionSource{}, nil, projectRoot)
	orchestrator.maxExplicitFileBytes = 8

	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{ExplicitFiles: []string{
		"../secret.txt",
		"missing.go",
		"internal/auth/middleware.go",
	}}, nil, config.ContextConfig{MaxExplicitFiles: 5})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}

	if len(results.FileResults) != 1 {
		t.Fatalf("FileResults = %v, want only the valid in-root file", results.FileResults)
	}
	if !results.FileResults[0].Truncated {
		t.Fatal("expected file result to be truncated")
	}
	if results.FileResults[0].Content != "01234567" {
		t.Fatalf("Content = %q, want truncated prefix", results.FileResults[0].Content)
	}
}

func TestRetrievalOrchestratorTimeoutDoesNotBlockOtherPaths(t *testing.T) {
	projectRoot := t.TempDir()
	mustWriteFile(t, projectRoot, "internal/auth/middleware.go", "package auth")

	searcher := &retrievalSearcherStub{
		results: []codeintel.SearchResult{{Chunk: codeintel.Chunk{ID: "chunk-slow", FilePath: "internal/auth/middleware.go", Name: "ValidateToken"}, Score: 0.9}},
		delay:   200 * time.Millisecond,
	}
	orchestrator := NewRetrievalOrchestrator(searcher, nil, NoopConventionSource{}, nil, projectRoot)
	orchestrator.timeout = 20 * time.Millisecond

	start := time.Now()
	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{ExplicitFiles: []string{"internal/auth/middleware.go"}}, []string{"auth middleware"}, config.ContextConfig{MaxExplicitFiles: 5})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("Retrieve took %v, expected timeout to return sooner", elapsed)
	}
	if len(results.RAGHits) != 0 {
		t.Fatalf("RAGHits = %v, want empty after timeout", results.RAGHits)
	}
	if len(results.FileResults) != 1 {
		t.Fatalf("len(FileResults) = %d, want 1", len(results.FileResults))
	}
}

func TestRetrievalOrchestratorContinuesAfterPathErrors(t *testing.T) {
	projectRoot := t.TempDir()
	mustWriteFile(t, projectRoot, "internal/auth/middleware.go", "package auth")

	searcher := &retrievalSearcherStub{err: errors.New("search failed")}
	conventions := &retrievalConventionSourceStub{err: errors.New("cache unavailable")}
	orchestrator := NewRetrievalOrchestrator(searcher, nil, conventions, nil, projectRoot)
	orchestrator.gitRunner = func(ctx stdctx.Context, workdir string, depth int) (string, error) {
		return "", errors.New("git failed")
	}

	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{
		ExplicitFiles:      []string{"internal/auth/middleware.go"},
		IncludeConventions: true,
		IncludeGitContext:  true,
		GitContextDepth:    2,
	}, []string{"auth middleware"}, config.ContextConfig{MaxExplicitFiles: 5})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if len(results.FileResults) != 1 {
		t.Fatalf("len(FileResults) = %d, want 1", len(results.FileResults))
	}
	if len(results.RAGHits) != 0 {
		t.Fatalf("RAGHits = %v, want empty after search error", results.RAGHits)
	}
	if results.ConventionText != "" {
		t.Fatalf("ConventionText = %q, want empty", results.ConventionText)
	}
	if results.GitContext != "" {
		t.Fatalf("GitContext = %q, want empty", results.GitContext)
	}
}

func TestRetrievalOrchestratorPassesGraphExpansionConfigToBrainSearch(t *testing.T) {
	brainSearcher := &retrievalBrainSearcherStub{hitsByQuery: map[string][]brain.SearchHit{
		"runtime cache": {{Path: "notes/runtime-cache.md", Snippet: "cache reminder", Score: 0.9}},
	}}
	orchestrator := NewRetrievalOrchestrator(nil, nil, NoopConventionSource{}, brainSearcher, t.TempDir())
	orchestrator.SetBrainConfig(config.BrainConfig{IncludeGraphHops: true, GraphHopDepth: 2})

	results, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{PreferBrainContext: true}, []string{"runtime cache"}, config.ContextConfig{})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if len(results.BrainHits) != 1 {
		t.Fatalf("len(BrainHits) = %d, want 1", len(results.BrainHits))
	}
	if results.BrainHits[0].GraphHopDepth != 0 || results.BrainHits[0].GraphSourcePath != "" {
		t.Fatalf("BrainHits[0] graph metadata = %+v, want zero values for direct hit", results.BrainHits[0])
	}
	if len(brainSearcher.gotRequests) != 1 {
		t.Fatalf("gotRequests = %d, want 1", len(brainSearcher.gotRequests))
	}
	if !brainSearcher.gotRequests[0].IncludeGraphHops || brainSearcher.gotRequests[0].GraphHopDepth != 2 {
		t.Fatalf("brain request = %+v, want graph expansion enabled with depth 2", brainSearcher.gotRequests[0])
	}
	if brainSearcher.gotRequests[0].MaxResults != defaultRetrievalMaxResults {
		t.Fatalf("brain request MaxResults = %d, want %d", brainSearcher.gotRequests[0].MaxResults, defaultRetrievalMaxResults)
	}
}

func TestRetrievalOrchestratorPassesContextMaxChunksToBrainSearch(t *testing.T) {
	brainSearcher := &retrievalBrainSearcherStub{hitsByQuery: map[string][]brain.SearchHit{
		"runtime cache": {{Path: "notes/runtime-cache.md", Snippet: "cache reminder", Score: 0.9}},
	}}
	orchestrator := NewRetrievalOrchestrator(nil, nil, NoopConventionSource{}, brainSearcher, t.TempDir())
	orchestrator.SetBrainConfig(config.BrainConfig{IncludeGraphHops: true, GraphHopDepth: 2})

	_, err := orchestrator.Retrieve(stdctx.Background(), &ContextNeeds{PreferBrainContext: true}, []string{"runtime cache"}, config.ContextConfig{MaxChunks: 17})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if len(brainSearcher.gotRequests) != 1 {
		t.Fatalf("gotRequests = %d, want 1", len(brainSearcher.gotRequests))
	}
	if brainSearcher.gotRequests[0].MaxResults != 17 {
		t.Fatalf("brain request MaxResults = %d, want 17", brainSearcher.gotRequests[0].MaxResults)
	}
}

func mustWriteFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}
