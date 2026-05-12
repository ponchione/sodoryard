package context

import (
	stdctx "context"
	"strings"

	"github.com/ponchione/sodoryard/internal/codeintel"
	"github.com/ponchione/sodoryard/internal/config"
)

// RetrievalRequest is the source-level request shape for common retriever
// wrappers. It is intentionally narrower than full context assembly.
type RetrievalRequest struct {
	Needs         *ContextNeeds
	Queries       []string
	ContextConfig config.ContextConfig
	BrainConfig   config.BrainConfig
}

// SourceRetriever wraps one retrieval source and returns normalized result
// metadata without changing orchestration, ranking, or budget policy.
type SourceRetriever interface {
	Name() string
	Retrieve(ctx stdctx.Context, req RetrievalRequest) ([]RetrievalResult, error)
}

type CodeSearchSourceRetriever struct {
	searcher codeintel.Searcher
}

func NewCodeSearchSourceRetriever(searcher codeintel.Searcher) *CodeSearchSourceRetriever {
	return &CodeSearchSourceRetriever{searcher: searcher}
}

func (r *CodeSearchSourceRetriever) Name() string {
	return "code"
}

func (r *CodeSearchSourceRetriever) Retrieve(ctx stdctx.Context, req RetrievalRequest) ([]RetrievalResult, error) {
	if r == nil || r.searcher == nil || !shouldRunSemanticSearch(req.Needs, req.Queries) {
		return nil, nil
	}
	if ctx == nil {
		ctx = stdctx.Background()
	}
	orchestrator := &RetrievalOrchestrator{searcher: r.searcher}
	hits, err := orchestrator.retrieveSemanticSearch(ctx, req.Queries, req.ContextConfig)
	if err != nil {
		return nil, err
	}
	return BuildRetrievalResults(hits, nil, nil, nil), nil
}

type BrainSearchSourceRetriever struct {
	brain BrainSearcher
}

func NewBrainSearchSourceRetriever(brain BrainSearcher) *BrainSearchSourceRetriever {
	return &BrainSearchSourceRetriever{brain: brain}
}

func (r *BrainSearchSourceRetriever) Name() string {
	return "brain"
}

func (r *BrainSearchSourceRetriever) Retrieve(ctx stdctx.Context, req RetrievalRequest) ([]RetrievalResult, error) {
	if r == nil || r.brain == nil || len(req.Queries) == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = stdctx.Background()
	}
	orchestrator := &RetrievalOrchestrator{
		brain:           r.brain,
		brainCfg:        req.BrainConfig,
		brainQueryTrace: func(string, ...any) {},
	}
	hits, err := orchestrator.retrieveBrainSearch(ctx, req.Queries, req.ContextConfig)
	if err != nil {
		return nil, err
	}
	return BuildRetrievalResults(nil, hits, nil, nil), nil
}

type GraphSourceRetriever struct {
	graph codeintel.GraphStore
}

func NewGraphSourceRetriever(graph codeintel.GraphStore) *GraphSourceRetriever {
	return &GraphSourceRetriever{graph: graph}
}

func (r *GraphSourceRetriever) Name() string {
	return "graph"
}

func (r *GraphSourceRetriever) Retrieve(ctx stdctx.Context, req RetrievalRequest) ([]RetrievalResult, error) {
	if r == nil || r.graph == nil || req.Needs == nil || len(req.Needs.ExplicitSymbols) == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = stdctx.Background()
	}
	orchestrator := &RetrievalOrchestrator{graph: r.graph}
	hits, err := orchestrator.retrieveStructuralGraph(ctx, req.Needs.ExplicitSymbols, req.ContextConfig)
	if err != nil {
		return nil, err
	}
	return BuildRetrievalResults(nil, nil, hits, nil), nil
}

type ExplicitFileSourceRetriever struct {
	projectRoot          string
	fileReader           fileReaderFunc
	maxExplicitFileBytes int
}

func NewExplicitFileSourceRetriever(projectRoot string) *ExplicitFileSourceRetriever {
	return &ExplicitFileSourceRetriever{projectRoot: projectRoot, fileReader: readFileBounded}
}

func (r *ExplicitFileSourceRetriever) Name() string {
	return "explicit_file"
}

func (r *ExplicitFileSourceRetriever) Retrieve(ctx stdctx.Context, req RetrievalRequest) ([]RetrievalResult, error) {
	if r == nil || strings.TrimSpace(r.projectRoot) == "" || req.Needs == nil || len(req.Needs.ExplicitFiles) == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = stdctx.Background()
	}
	fileReader := r.fileReader
	if fileReader == nil {
		fileReader = readFileBounded
	}
	orchestrator := &RetrievalOrchestrator{
		projectRoot:          r.projectRoot,
		fileReader:           fileReader,
		maxExplicitFileBytes: r.maxExplicitFileBytes,
	}
	results, err := orchestrator.retrieveExplicitFiles(ctx, req.Needs.ExplicitFiles, req.ContextConfig)
	if err != nil {
		return nil, err
	}
	return BuildRetrievalResults(nil, nil, nil, results), nil
}

type ConventionSourceRetriever struct {
	conventions ConventionSource
}

func NewConventionSourceRetriever(conventions ConventionSource) *ConventionSourceRetriever {
	return &ConventionSourceRetriever{conventions: conventions}
}

func (r *ConventionSourceRetriever) Name() string {
	return "conventions"
}

func (r *ConventionSourceRetriever) Retrieve(ctx stdctx.Context, req RetrievalRequest) ([]RetrievalResult, error) {
	if r == nil || r.conventions == nil || req.Needs == nil || !req.Needs.IncludeConventions {
		return nil, nil
	}
	if ctx == nil {
		ctx = stdctx.Background()
	}
	orchestrator := &RetrievalOrchestrator{conventions: r.conventions}
	text, err := orchestrator.retrieveConventions(ctx)
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	return []RetrievalResult{{
		Source:        "conventions",
		Kind:          "convention",
		Content:       text,
		TokenEstimate: approximateTokenCount(text),
	}}, nil
}

type GitContextSourceRetriever struct {
	projectRoot string
	gitRunner   gitRunnerFunc
}

func NewGitContextSourceRetriever(projectRoot string) *GitContextSourceRetriever {
	return &GitContextSourceRetriever{projectRoot: projectRoot, gitRunner: defaultGitRunner}
}

func (r *GitContextSourceRetriever) Name() string {
	return "git"
}

func (r *GitContextSourceRetriever) Retrieve(ctx stdctx.Context, req RetrievalRequest) ([]RetrievalResult, error) {
	if r == nil || strings.TrimSpace(r.projectRoot) == "" || req.Needs == nil || !req.Needs.IncludeGitContext {
		return nil, nil
	}
	if ctx == nil {
		ctx = stdctx.Background()
	}
	gitRunner := r.gitRunner
	if gitRunner == nil {
		gitRunner = defaultGitRunner
	}
	depth := req.Needs.GitContextDepth
	if depth <= 0 {
		depth = defaultGitContextDepth
	}
	orchestrator := &RetrievalOrchestrator{projectRoot: r.projectRoot, gitRunner: gitRunner}
	text, err := orchestrator.retrieveGitContext(ctx, depth)
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	return []RetrievalResult{{
		Source:        "git",
		Kind:          "git",
		Content:       text,
		TokenEstimate: approximateTokenCount(text),
		Metadata: map[string]any{
			"depth": depth,
		},
	}}, nil
}
