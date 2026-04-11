//go:build sqlite_fts5
// +build sqlite_fts5

package context

import (
	stdctx "context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ponchione/sodoryard/internal/config"
	dbpkg "github.com/ponchione/sodoryard/internal/db"
)

type assemblerAnalyzerStub struct {
	result     *ContextNeeds
	gotMessage string
	gotHistory []dbpkg.Message
}

func (s *assemblerAnalyzerStub) AnalyzeTurn(message string, recentHistory []dbpkg.Message) *ContextNeeds {
	s.gotMessage = message
	s.gotHistory = append([]dbpkg.Message(nil), recentHistory...)
	if s.result == nil {
		return &ContextNeeds{}
	}
	copy := *s.result
	copy.SemanticQueries = append([]string(nil), s.result.SemanticQueries...)
	copy.ExplicitFiles = append([]string(nil), s.result.ExplicitFiles...)
	copy.ExplicitSymbols = append([]string(nil), s.result.ExplicitSymbols...)
	copy.MomentumFiles = append([]string(nil), s.result.MomentumFiles...)
	copy.Signals = append([]Signal(nil), s.result.Signals...)
	return &copy
}

type assemblerMomentumStub struct {
	applied     bool
	gotHistory  []dbpkg.Message
	gotBefore   *ContextNeeds
	moduleToSet string
	filesToSet  []string
}

func (s *assemblerMomentumStub) Apply(recentHistory []dbpkg.Message, needs *ContextNeeds, _ config.ContextConfig) {
	s.applied = true
	s.gotHistory = append([]dbpkg.Message(nil), recentHistory...)
	clone := *needs
	clone.SemanticQueries = append([]string(nil), needs.SemanticQueries...)
	clone.ExplicitFiles = append([]string(nil), needs.ExplicitFiles...)
	clone.ExplicitSymbols = append([]string(nil), needs.ExplicitSymbols...)
	clone.MomentumFiles = append([]string(nil), needs.MomentumFiles...)
	clone.Signals = append([]Signal(nil), needs.Signals...)
	s.gotBefore = &clone
	needs.MomentumModule = s.moduleToSet
	needs.MomentumFiles = append([]string(nil), s.filesToSet...)
}

type assemblerQueryExtractorStub struct {
	queries    []string
	gotNeeds   *ContextNeeds
	gotMessage string
}

func (s *assemblerQueryExtractorStub) ExtractQueries(message string, needs *ContextNeeds) []string {
	s.gotMessage = message
	clone := *needs
	clone.SemanticQueries = append([]string(nil), needs.SemanticQueries...)
	clone.ExplicitFiles = append([]string(nil), needs.ExplicitFiles...)
	clone.ExplicitSymbols = append([]string(nil), needs.ExplicitSymbols...)
	clone.MomentumFiles = append([]string(nil), needs.MomentumFiles...)
	clone.Signals = append([]Signal(nil), needs.Signals...)
	s.gotNeeds = &clone
	return append([]string(nil), s.queries...)
}

type assemblerRetrieverStub struct {
	result     *RetrievalResults
	gotNeeds   *ContextNeeds
	gotQueries []string
}

func (s *assemblerRetrieverStub) Retrieve(_ stdctx.Context, needs *ContextNeeds, queries []string, _ config.ContextConfig) (*RetrievalResults, error) {
	clone := *needs
	clone.SemanticQueries = append([]string(nil), needs.SemanticQueries...)
	clone.ExplicitFiles = append([]string(nil), needs.ExplicitFiles...)
	clone.ExplicitSymbols = append([]string(nil), needs.ExplicitSymbols...)
	clone.MomentumFiles = append([]string(nil), needs.MomentumFiles...)
	clone.Signals = append([]Signal(nil), needs.Signals...)
	s.gotNeeds = &clone
	s.gotQueries = append([]string(nil), queries...)
	return s.result, nil
}

type assemblerBudgetManagerStub struct {
	result               *BudgetResult
	gotModelContextLimit int
	gotHistoryTokenCount int
}

func (s *assemblerBudgetManagerStub) Fit(_ *RetrievalResults, modelContextLimit int, historyTokenCount int, _ config.ContextConfig) (*BudgetResult, error) {
	s.gotModelContextLimit = modelContextLimit
	s.gotHistoryTokenCount = historyTokenCount
	return s.result, nil
}

type assemblerSerializerStub struct {
	content      string
	seenPath     string
	seenTurn     int
	gotSeenFiles bool
}

func (s *assemblerSerializerStub) Serialize(_ *BudgetResult, seenFiles SeenFileLookup) (string, error) {
	if seenFiles != nil {
		s.gotSeenFiles = true
		seen, turn := seenFiles.Contains("internal/auth/service.go")
		if seen {
			s.seenPath = "internal/auth/service.go"
			s.seenTurn = turn
		}
	}
	return s.content, nil
}

func TestContextAssemblerAssemblePersistsReportAndReturnsFrozenPackage(t *testing.T) {
	db := newCompressionTestDB(t)
	conversationID := seedCompressionConversation(t, db)
	history := []dbpkg.Message{{
		Role:       "user",
		Content:    sql.NullString{String: "previous message", Valid: true},
		TurnNumber: 1,
		Iteration:  1,
	}}

	analyzer := &assemblerAnalyzerStub{result: &ContextNeeds{
		ExplicitFiles:      []string{"internal/auth/middleware.go"},
		ExplicitSymbols:    []string{"ValidateToken"},
		PreferBrainContext: true,
		Signals: []Signal{
			{Type: "brain_intent", Source: "project brain", Value: "prefer_brain_context"},
			{Type: "file_ref", Source: "middleware.go", Value: "internal/auth/middleware.go"},
		},
	}}
	momentum := &assemblerMomentumStub{moduleToSet: "internal/auth", filesToSet: []string{"internal/auth/service.go"}}
	extractor := &assemblerQueryExtractorStub{queries: []string{"auth middleware"}}
	retriever := &assemblerRetrieverStub{result: &RetrievalResults{
		RAGHits: []RAGHit{{ChunkID: "chunk-1", FilePath: "internal/auth/service.go", Name: "ValidateToken", Description: "Validates tokens.", Body: "func ValidateToken() error { return nil }"}},
		BrainHits: []BrainHit{{
			DocumentPath:    "notes/auth-decisions.md",
			Title:           "Auth decisions",
			Snippet:         "Auth rationale belongs in the project brain.",
			MatchMode:       "backlink",
			MatchSources:    []string{"backlink"},
			GraphSourcePath: "notes/runtime-cache.md",
			GraphHopDepth:   1,
			MatchScore:      0.92,
		}},
		GraphHits:   []GraphHit{{ChunkID: "graph-1", FilePath: "internal/auth/handler.go", SymbolName: "AuthHandler", RelationshipType: "upstream"}},
		FileResults: []FileResult{{FilePath: "internal/auth/middleware.go", Content: "package auth"}},
	}}
	budgeter := &assemblerBudgetManagerStub{result: &BudgetResult{
		SelectedRAGHits: []RAGHit{{ChunkID: "chunk-1", FilePath: "internal/auth/service.go", Name: "ValidateToken", Description: "Validates tokens.", Body: "func ValidateToken() error { return nil }"}},
		SelectedBrainHits: []BrainHit{{
			DocumentPath:    "notes/auth-decisions.md",
			Title:           "Auth decisions",
			Snippet:         "Auth rationale belongs in the project brain.",
			MatchMode:       "backlink",
			MatchSources:    []string{"backlink"},
			GraphSourcePath: "notes/runtime-cache.md",
			GraphHopDepth:   1,
			MatchScore:      0.92,
			Included:        true,
		}},
		SelectedFileResults: []FileResult{{FilePath: "internal/auth/middleware.go", Content: "package auth"}},
		BudgetTotal:         1200,
		BudgetUsed:          352,
		BudgetBreakdown:     map[string]int{"explicit_files": 80, "brain": 72, "rag": 200},
		IncludedChunks:      []string{"internal/auth/middleware.go", "notes/auth-decisions.md", "chunk-1"},
		ExcludedChunks:      []string{"graph-1"},
		ExclusionReasons:    map[string]string{"graph-1": "budget_exceeded"},
		CompressionNeeded:   true,
	}}
	serializer := &assemblerSerializerStub{content: "## Relevant Code\n\nassembled context"}
	assembler := NewContextAssembler(analyzer, extractor, momentum, retriever, budgeter, serializer, config.ContextConfig{StoreAssemblyReports: true}, db)

	pkg, compressionNeeded, err := assembler.Assemble(stdctx.Background(), "fix auth middleware", history, AssemblyScope{
		ConversationID: conversationID,
		TurnNumber:     2,
		SeenFiles:      seenFilesStub{path: "internal/auth/service.go", turn: 1},
	}, 200000, 1234)
	if err != nil {
		t.Fatalf("Assemble returned error: %v", err)
	}
	if !compressionNeeded {
		t.Fatal("compressionNeeded = false, want true")
	}
	if pkg == nil {
		t.Fatal("pkg = nil, want package")
	}
	if !pkg.Frozen {
		t.Fatal("pkg.Frozen = false, want true")
	}
	if pkg.TokenCount != approximateTokenCount(serializer.content) {
		t.Fatalf("TokenCount = %d, want %d", pkg.TokenCount, approximateTokenCount(serializer.content))
	}
	if pkg.Report == nil {
		t.Fatal("pkg.Report = nil, want report")
	}
	if analyzer.gotMessage != "fix auth middleware" {
		t.Fatalf("analyzer message = %q, want fix auth middleware", analyzer.gotMessage)
	}
	if !momentum.applied {
		t.Fatal("momentum tracker was not applied")
	}
	if extractor.gotNeeds == nil || extractor.gotNeeds.MomentumModule != "internal/auth" {
		t.Fatalf("extractor got needs = %+v, want momentum module internal/auth", extractor.gotNeeds)
	}
	if retriever.gotNeeds == nil || retriever.gotNeeds.MomentumModule != "internal/auth" {
		t.Fatalf("retriever got needs = %+v, want momentum module internal/auth", retriever.gotNeeds)
	}
	if budgeter.gotModelContextLimit != 200000 || budgeter.gotHistoryTokenCount != 1234 {
		t.Fatalf("budget inputs = (%d, %d), want (200000, 1234)", budgeter.gotModelContextLimit, budgeter.gotHistoryTokenCount)
	}
	if !serializer.gotSeenFiles || serializer.seenTurn != 1 {
		t.Fatalf("serializer seen-files state = (%v, %d), want true/1", serializer.gotSeenFiles, serializer.seenTurn)
	}
	if len(pkg.Report.RAGResults) != 1 || !pkg.Report.RAGResults[0].Included {
		t.Fatalf("RAGResults = %+v, want included chunk", pkg.Report.RAGResults)
	}
	if len(pkg.Report.BrainResults) != 1 || !pkg.Report.BrainResults[0].Included {
		t.Fatalf("BrainResults = %+v, want included brain hit", pkg.Report.BrainResults)
	}
	if len(pkg.Report.GraphResults) != 1 || pkg.Report.GraphResults[0].ExclusionReason != "budget_exceeded" {
		t.Fatalf("GraphResults = %+v, want budget_exceeded exclusion", pkg.Report.GraphResults)
	}
	if !pkg.Report.Needs.PreferBrainContext {
		t.Fatalf("report PreferBrainContext = false, want true")
	}
	if got := pkg.Report.Needs.SemanticQueries; len(got) != 1 || got[0] != "auth middleware" {
		t.Fatalf("report semantic queries = %v, want [auth middleware]", got)
	}

	row, err := dbpkg.New(db).GetContextReportByTurn(stdctx.Background(), dbpkg.GetContextReportByTurnParams{ConversationID: conversationID, TurnNumber: 2})
	if err != nil {
		t.Fatalf("GetContextReportByTurn returned error: %v", err)
	}
	if !row.NeedsJson.Valid || !row.RagResultsJson.Valid || !row.GraphResultsJson.Valid {
		t.Fatalf("expected populated JSON fields, got row %+v", row)
	}
	if !row.AgentReadFilesJson.Valid || row.AgentReadFilesJson.String != "[]" {
		t.Fatalf("AgentReadFilesJson = %+v, want []", row.AgentReadFilesJson)
	}
	var persistedNeeds ContextNeeds
	if err := json.Unmarshal([]byte(row.NeedsJson.String), &persistedNeeds); err != nil {
		t.Fatalf("unmarshal needs_json: %v", err)
	}
	if !persistedNeeds.PreferBrainContext {
		t.Fatalf("persisted PreferBrainContext = false, want true")
	}
	if got := persistedNeeds.SemanticQueries; len(got) != 1 || got[0] != "auth middleware" {
		t.Fatalf("persisted semantic queries = %v, want [auth middleware]", got)
	}
	var ragResults []RAGHit
	if err := json.Unmarshal([]byte(row.RagResultsJson.String), &ragResults); err != nil {
		t.Fatalf("unmarshal rag_results_json: %v", err)
	}
	if len(ragResults) != 1 || !ragResults[0].Included {
		t.Fatalf("persisted rag results = %+v, want included chunk", ragResults)
	}
	var brainResults []BrainHit
	if err := json.Unmarshal([]byte(row.BrainResultsJson.String), &brainResults); err != nil {
		t.Fatalf("unmarshal brain_results_json: %v", err)
	}
	if len(brainResults) != 1 || !brainResults[0].Included || brainResults[0].DocumentPath != "notes/auth-decisions.md" {
		t.Fatalf("persisted brain results = %+v, want included auth brain hit", brainResults)
	}
	if brainResults[0].GraphSourcePath != "notes/runtime-cache.md" || brainResults[0].GraphHopDepth != 1 || brainResults[0].MatchMode != "backlink" {
		t.Fatalf("persisted brain graph metadata = %+v, want backlink source/runtime-cache depth=1", brainResults[0])
	}
}

func TestContextAssemblerUpdateQualityPersistsHitRate(t *testing.T) {
	db := newCompressionTestDB(t)
	conversationID := seedCompressionConversation(t, db)
	assembler := seedAssemblerReportForQuality(t, db, conversationID)

	if err := assembler.UpdateQuality(stdctx.Background(), conversationID, 2, true, []string{"internal/auth/service.go", "internal/auth/other.go", "internal/auth/service.go"}); err != nil {
		t.Fatalf("UpdateQuality returned error: %v", err)
	}

	row, err := dbpkg.New(db).GetContextReportByTurn(stdctx.Background(), dbpkg.GetContextReportByTurnParams{ConversationID: conversationID, TurnNumber: 2})
	if err != nil {
		t.Fatalf("GetContextReportByTurn returned error: %v", err)
	}
	if !row.AgentUsedSearchTool.Valid || row.AgentUsedSearchTool.Int64 != 1 {
		t.Fatalf("AgentUsedSearchTool = %+v, want 1", row.AgentUsedSearchTool)
	}
	if !row.ContextHitRate.Valid || row.ContextHitRate.Float64 != 0.5 {
		t.Fatalf("ContextHitRate = %+v, want 0.5", row.ContextHitRate)
	}
	if !row.AgentReadFilesJson.Valid || row.AgentReadFilesJson.String != `["internal/auth/other.go","internal/auth/service.go"]` {
		t.Fatalf("AgentReadFilesJson = %+v, want sorted unique read files", row.AgentReadFilesJson)
	}
}

func TestContextAssemblerUpdateQualityCountsBrainReadsAndHits(t *testing.T) {
	db := newCompressionTestDB(t)
	conversationID := seedCompressionConversation(t, db)
	assembler := seedAssemblerReportForQuality(t, db, conversationID)

	queries := dbpkg.New(db)
	row, err := queries.GetContextReportByTurn(stdctx.Background(), dbpkg.GetContextReportByTurnParams{ConversationID: conversationID, TurnNumber: 2})
	if err != nil {
		t.Fatalf("GetContextReportByTurn before patch returned error: %v", err)
	}
	row.BrainResultsJson = sql.NullString{String: `[{"document_path":"notes/runtime.md","included":true}]`, Valid: true}
	if _, err := db.Exec(`UPDATE context_reports SET brain_results_json = ? WHERE conversation_id = ? AND turn_number = ?`, row.BrainResultsJson.String, conversationID, 2); err != nil {
		t.Fatalf("update brain_results_json: %v", err)
	}

	if err := assembler.UpdateQuality(stdctx.Background(), conversationID, 2, true, []string{"notes/runtime.md"}); err != nil {
		t.Fatalf("UpdateQuality returned error: %v", err)
	}

	updated, err := queries.GetContextReportByTurn(stdctx.Background(), dbpkg.GetContextReportByTurnParams{ConversationID: conversationID, TurnNumber: 2})
	if err != nil {
		t.Fatalf("GetContextReportByTurn after patch returned error: %v", err)
	}
	if !updated.ContextHitRate.Valid || updated.ContextHitRate.Float64 != 1.0 {
		t.Fatalf("ContextHitRate = %+v, want 1.0", updated.ContextHitRate)
	}
	if !updated.AgentReadFilesJson.Valid || updated.AgentReadFilesJson.String != `["notes/runtime.md"]` {
		t.Fatalf("AgentReadFilesJson = %+v, want brain read path", updated.AgentReadFilesJson)
	}
}

type assemblerRetrieverErrorStub struct {
	err error
}

func (s *assemblerRetrieverErrorStub) Retrieve(_ stdctx.Context, _ *ContextNeeds, _ []string, _ config.ContextConfig) (*RetrievalResults, error) {
	return nil, s.err
}

func TestContextAssemblerPropagatesRetrieverError(t *testing.T) {
	db := newCompressionTestDB(t)
	conversationID := seedCompressionConversation(t, db)

	retrieverErr := errors.New("vector store unavailable")
	analyzer := &assemblerAnalyzerStub{result: &ContextNeeds{}}
	retriever := &assemblerRetrieverErrorStub{err: retrieverErr}
	budgeter := &assemblerBudgetManagerStub{result: &BudgetResult{}}
	serializer := &assemblerSerializerStub{content: ""}
	assembler := NewContextAssembler(analyzer, nil, nil, retriever, budgeter, serializer, config.ContextConfig{}, db)

	_, _, err := assembler.Assemble(stdctx.Background(), "test message", nil, AssemblyScope{
		ConversationID: conversationID,
		TurnNumber:     1,
	}, 200000, 0)
	if err == nil {
		t.Fatal("expected error from Assemble, got nil")
	}
	if !errors.Is(err, retrieverErr) {
		t.Fatalf("expected wrapped retriever error, got: %v", err)
	}
}

func TestContextAssemblerHandlesNilOptionalComponents(t *testing.T) {
	db := newCompressionTestDB(t)
	conversationID := seedCompressionConversation(t, db)

	analyzer := &assemblerAnalyzerStub{result: &ContextNeeds{
		ExplicitFiles: []string{"internal/auth/middleware.go"},
	}}
	retriever := &assemblerRetrieverStub{result: &RetrievalResults{
		FileResults: []FileResult{{FilePath: "internal/auth/middleware.go", Content: "package auth"}},
	}}
	budgeter := &assemblerBudgetManagerStub{result: &BudgetResult{
		SelectedFileResults: []FileResult{{FilePath: "internal/auth/middleware.go", Content: "package auth"}},
		BudgetTotal:         1200,
		BudgetUsed:          80,
		BudgetBreakdown:     map[string]int{"explicit_files": 80},
		IncludedChunks:      []string{"internal/auth/middleware.go"},
		ExcludedChunks:      []string{},
		ExclusionReasons:    map[string]string{},
	}}
	serializer := &assemblerSerializerStub{content: "## Code\ncontext block"}

	// momentum=nil, extractor=nil — only required components provided
	assembler := NewContextAssembler(analyzer, nil, nil, retriever, budgeter, serializer, config.ContextConfig{StoreAssemblyReports: true}, db)

	pkg, compressionNeeded, err := assembler.Assemble(stdctx.Background(), "show me the middleware", nil, AssemblyScope{
		ConversationID: conversationID,
		TurnNumber:     2,
	}, 200000, 500)
	if err != nil {
		t.Fatalf("Assemble returned error: %v", err)
	}
	if compressionNeeded {
		t.Fatal("compressionNeeded = true, want false")
	}
	if pkg == nil {
		t.Fatal("pkg = nil, want package")
	}
	if !pkg.Frozen {
		t.Fatal("pkg.Frozen = false, want true")
	}
	if pkg.TokenCount != approximateTokenCount(serializer.content) {
		t.Fatalf("TokenCount = %d, want %d", pkg.TokenCount, approximateTokenCount(serializer.content))
	}
	if pkg.Report == nil {
		t.Fatal("pkg.Report = nil, want report")
	}
}

func seedAssemblerReportForQuality(t *testing.T, db *sql.DB, conversationID string) *ContextAssembler {
	t.Helper()
	analyzer := &assemblerAnalyzerStub{result: &ContextNeeds{}}
	momentum := &assemblerMomentumStub{}
	extractor := &assemblerQueryExtractorStub{queries: []string{"auth middleware"}}
	retriever := &assemblerRetrieverStub{result: &RetrievalResults{
		RAGHits:     []RAGHit{{ChunkID: "chunk-1", FilePath: "internal/auth/service.go", Name: "ValidateToken", Description: "Validates tokens.", Body: "func ValidateToken() error { return nil }"}},
		FileResults: []FileResult{{FilePath: "internal/auth/middleware.go", Content: "package auth"}},
	}}
	budgeter := &assemblerBudgetManagerStub{result: &BudgetResult{
		SelectedRAGHits:     []RAGHit{{ChunkID: "chunk-1", FilePath: "internal/auth/service.go", Name: "ValidateToken", Description: "Validates tokens.", Body: "func ValidateToken() error { return nil }"}},
		SelectedFileResults: []FileResult{{FilePath: "internal/auth/middleware.go", Content: "package auth"}},
		BudgetTotal:         1200,
		BudgetUsed:          280,
		BudgetBreakdown:     map[string]int{"explicit_files": 80, "rag": 200},
		IncludedChunks:      []string{"internal/auth/middleware.go", "chunk-1"},
		ExcludedChunks:      []string{},
		ExclusionReasons:    map[string]string{},
	}}
	serializer := &assemblerSerializerStub{content: "assembled context"}
	assembler := NewContextAssembler(analyzer, extractor, momentum, retriever, budgeter, serializer, config.ContextConfig{StoreAssemblyReports: true}, db)

	_, _, err := assembler.Assemble(stdctx.Background(), "fix auth middleware", nil, AssemblyScope{ConversationID: conversationID, TurnNumber: 2}, 200000, 0)
	if err != nil {
		t.Fatalf("seed assemble returned error: %v", err)
	}
	return assembler
}
