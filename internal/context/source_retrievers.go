package context

import (
	stdctx "context"

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
