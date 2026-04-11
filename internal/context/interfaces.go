package context

import (
	stdctx "context"

	"github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/db"
)

// TurnAnalyzer determines what context should be retrieved for a turn.
//
// The rule-based analyzer lands first in v0.1, but callers depend on this
// interface so a future LLM-assisted analyzer can be swapped in later without
// changing downstream orchestration.
type TurnAnalyzer interface {
	AnalyzeTurn(message string, recentHistory []db.Message) *ContextNeeds
}

// QueryExtractor derives semantic-search queries from a user message and the
// analyzer output for that turn.
type QueryExtractor interface {
	ExtractQueries(message string, needs *ContextNeeds) []string
}

// MomentumTracker enriches ContextNeeds using recent conversation history when
// a turn appears to be a continuation or otherwise underspecified.
type MomentumTracker interface {
	Apply(recentHistory []db.Message, needs *ContextNeeds, cfg config.ContextConfig)
}

// ConventionSource loads cached project conventions for writing-oriented turns.
//
// Implementations may return an empty string when conventions are unavailable.
type ConventionSource interface {
	Load(ctx stdctx.Context) (string, error)
}

// BrainSearchRequest captures one proactive/runtime brain search query.
type BrainSearchRequest struct {
	Query            string
	Mode             string
	MaxResults       int
	IncludeGraphHops bool
	GraphHopDepth    int
}

// BrainSearchResult is the richer retrieval payload returned by runtime brain
// searchers for proactive context assembly.
type BrainSearchResult struct {
	DocumentPath    string
	Title           string
	SectionHeading  string
	Snippet         string
	Tags            []string
	LexicalScore    float64
	SemanticScore   float64
	FinalScore      float64
	MatchMode       string
	MatchSources    []string
	GraphSourcePath string
	GraphHopDepth   int
}

// BrainSearcher provides the narrow brain search surface context assembly needs
// for proactive retrieval.
type BrainSearcher interface {
	Search(ctx stdctx.Context, request BrainSearchRequest) ([]BrainSearchResult, error)
}

// Retriever executes the retrieval phase and returns the collected pre-budget
// results.
type Retriever interface {
	Retrieve(ctx stdctx.Context, needs *ContextNeeds, queries []string, cfg config.ContextConfig) (*RetrievalResults, error)
}

// BudgetManager selects which retrieved content fits into the assembled-context
// budget.
type BudgetManager interface {
	Fit(results *RetrievalResults, modelContextLimit int, historyTokenCount int, cfg config.ContextConfig) (*BudgetResult, error)
}

// Serializer renders the budget-selected content into the stable markdown block
// that becomes system prompt cache block 2.
type Serializer interface {
	Serialize(result *BudgetResult, seenFiles SeenFileLookup) (string, error)
}
