package context

import (
	stdctx "context"
	"testing"

	"github.com/ponchione/sirtopham/internal/config"
	"github.com/ponchione/sirtopham/internal/db"
)

type seenFilesStub struct {
	path string
	turn int
}

func (s seenFilesStub) Contains(path string) (bool, int) {
	if path == s.path {
		return true, s.turn
	}
	return false, 0
}

type turnAnalyzerStub struct{}

func (turnAnalyzerStub) AnalyzeTurn(message string, recentHistory []db.Message) *ContextNeeds {
	return &ContextNeeds{SemanticQueries: []string{message}}
}

type queryExtractorStub struct{}

func (queryExtractorStub) ExtractQueries(message string, needs *ContextNeeds) []string {
	return append([]string{}, needs.SemanticQueries...)
}

type momentumTrackerStub struct{}

func (momentumTrackerStub) Apply(recentHistory []db.Message, needs *ContextNeeds, cfg config.ContextConfig) {
	needs.MomentumModule = "internal/context"
}

type conventionSourceStub struct{}

func (conventionSourceStub) Load(stdctx.Context) (string, error) {
	return "table-driven tests", nil
}

type retrieverStub struct{}

func (retrieverStub) Retrieve(stdctx.Context, *ContextNeeds, []string, config.ContextConfig) (*RetrievalResults, error) {
	return &RetrievalResults{}, nil
}

type budgetManagerStub struct{}

func (budgetManagerStub) Fit(*RetrievalResults, int, int, config.ContextConfig) (*BudgetResult, error) {
	return &BudgetResult{}, nil
}

type serializerStub struct{}

func (serializerStub) Serialize(*BudgetResult, SeenFileLookup) (string, error) {
	return "serialized", nil
}

func TestContextNeedsZeroValueIsUsable(t *testing.T) {
	var needs ContextNeeds
	needs.SemanticQueries = append(needs.SemanticQueries, "fix auth middleware")
	needs.ExplicitFiles = append(needs.ExplicitFiles, "internal/auth/middleware.go")
	needs.Signals = append(needs.Signals, Signal{Type: "file_ref", Source: "middleware.go", Value: "internal/auth/middleware.go"})

	if got := len(needs.SemanticQueries); got != 1 {
		t.Fatalf("len(SemanticQueries) = %d, want 1", got)
	}
	if got := len(needs.ExplicitFiles); got != 1 {
		t.Fatalf("len(ExplicitFiles) = %d, want 1", got)
	}
	if got := len(needs.Signals); got != 1 {
		t.Fatalf("len(Signals) = %d, want 1", got)
	}
}

func TestRetrievalResultsZeroValueIsUsable(t *testing.T) {
	var results RetrievalResults
	results.RAGHits = append(results.RAGHits, RAGHit{ChunkID: "chunk-1", FilePath: "internal/auth/service.go"})
	results.FileResults = append(results.FileResults, FileResult{FilePath: "internal/auth/service.go", Content: "package auth"})

	if got := len(results.RAGHits); got != 1 {
		t.Fatalf("len(RAGHits) = %d, want 1", got)
	}
	if got := len(results.FileResults); got != 1 {
		t.Fatalf("len(FileResults) = %d, want 1", got)
	}
}

func TestAssemblyScopeUsesSeenFileLookup(t *testing.T) {
	scope := AssemblyScope{
		ConversationID: "conv-123",
		TurnNumber:     7,
		SeenFiles:      seenFilesStub{path: "internal/context/types.go", turn: 3},
	}

	seen, turn := scope.SeenFiles.Contains("internal/context/types.go")
	if !seen {
		t.Fatal("expected path to be marked seen")
	}
	if turn != 3 {
		t.Fatalf("turn = %d, want 3", turn)
	}
}

func TestFullContextPackagePreservesFrozenAndReportPointer(t *testing.T) {
	report := &ContextAssemblyReport{
		BudgetTotal: 1000,
		BudgetUsed:  250,
	}
	pkg := FullContextPackage{
		Content:    "## Relevant Code",
		TokenCount: 250,
		Report:     report,
		Frozen:     true,
	}

	if !pkg.Frozen {
		t.Fatal("expected package to be frozen")
	}
	if pkg.Report != report {
		t.Fatal("expected report pointer to be preserved")
	}
}

func TestLayer3InterfacesCompileWithStubs(t *testing.T) {
	var analyzer TurnAnalyzer = turnAnalyzerStub{}
	var tracker MomentumTracker = momentumTrackerStub{}
	var extractor QueryExtractor = queryExtractorStub{}
	var conventions ConventionSource = conventionSourceStub{}
	var retriever Retriever = retrieverStub{}
	var budgeter BudgetManager = budgetManagerStub{}
	var serializer Serializer = serializerStub{}

	needs := analyzer.AnalyzeTurn("fix auth middleware", nil)
	if needs == nil {
		t.Fatal("AnalyzeTurn returned nil needs")
	}

	tracker.Apply(nil, needs, config.ContextConfig{})
	if needs.MomentumModule != "internal/context" {
		t.Fatalf("MomentumModule = %q, want internal/context", needs.MomentumModule)
	}

	queries := extractor.ExtractQueries("fix auth middleware", needs)
	if len(queries) != 1 {
		t.Fatalf("len(queries) = %d, want 1", len(queries))
	}

	text, err := conventions.Load(stdctx.Background())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if text == "" {
		t.Fatal("expected convention text")
	}

	results, err := retriever.Retrieve(stdctx.Background(), needs, queries, config.ContextConfig{})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	budget, err := budgeter.Fit(results, 200000, 0, config.ContextConfig{})
	if err != nil {
		t.Fatalf("Fit returned error: %v", err)
	}

	content, err := serializer.Serialize(budget, seenFilesStub{})
	if err != nil {
		t.Fatalf("Serialize returned error: %v", err)
	}
	if content != "serialized" {
		t.Fatalf("content = %q, want serialized", content)
	}
}
