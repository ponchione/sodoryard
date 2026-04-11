package context

import (
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/provider"
)

func TestHistoryMomentumTrackerExtractsFilesAndModuleFromRecentTurns(t *testing.T) {
	tracker := HistoryMomentumTracker{}
	history := []db.Message{
		assistantToolUseMessage(1, provider.NewToolUseBlock("call-old", "file_read", mustRawJSON(t, `{"path":"internal/old/ignore.go"}`))),
		assistantToolUseMessage(2,
			provider.NewToolUseBlock("call-1", "file_read", mustRawJSON(t, `{"path":"internal/auth/middleware.go"}`)),
			provider.NewToolUseBlock("call-2", "file_edit", mustRawJSON(t, `{"path":"internal/auth/service.go"}`)),
		),
	}
	needs := &ContextNeeds{}

	tracker.Apply(history, needs, config.ContextConfig{MomentumLookbackTurns: 1})

	if !slices.Contains(needs.MomentumFiles, "internal/auth/middleware.go") {
		t.Fatalf("MomentumFiles = %v, want internal/auth/middleware.go", needs.MomentumFiles)
	}
	if !slices.Contains(needs.MomentumFiles, "internal/auth/service.go") {
		t.Fatalf("MomentumFiles = %v, want internal/auth/service.go", needs.MomentumFiles)
	}
	if slices.Contains(needs.MomentumFiles, "internal/old/ignore.go") {
		t.Fatalf("MomentumFiles = %v, did not expect lookback-excluded file", needs.MomentumFiles)
	}
	if needs.MomentumModule != "internal/auth" {
		t.Fatalf("MomentumModule = %q, want internal/auth", needs.MomentumModule)
	}
}

func TestHistoryMomentumTrackerExtractsPathsFromSearchToolResults(t *testing.T) {
	tracker := HistoryMomentumTracker{}
	history := []db.Message{
		toolResultMessage(3, "search_text", "internal/auth/middleware.go:12\ninternal/auth/service.go:44\ninternal/auth/service.go:88"),
	}
	needs := &ContextNeeds{}

	tracker.Apply(history, needs, config.ContextConfig{MomentumLookbackTurns: 2})

	if len(needs.MomentumFiles) != 2 {
		t.Fatalf("MomentumFiles = %v, want 2 deduped files", needs.MomentumFiles)
	}
	if needs.MomentumModule != "internal/auth" {
		t.Fatalf("MomentumModule = %q, want internal/auth", needs.MomentumModule)
	}
}

func TestHistoryMomentumTrackerReturnsEmptyWithoutRelevantHistory(t *testing.T) {
	tracker := HistoryMomentumTracker{}
	history := []db.Message{{Role: "user", TurnNumber: 1, Content: sql.NullString{String: "hello", Valid: true}}}
	needs := &ContextNeeds{}

	tracker.Apply(history, needs, config.ContextConfig{MomentumLookbackTurns: 2})

	if len(needs.MomentumFiles) != 0 {
		t.Fatalf("MomentumFiles = %v, want empty", needs.MomentumFiles)
	}
	if needs.MomentumModule != "" {
		t.Fatalf("MomentumModule = %q, want empty", needs.MomentumModule)
	}
}

func TestHistoryMomentumTrackerIgnoresStrongSignalTurns(t *testing.T) {
	tracker := HistoryMomentumTracker{}
	history := []db.Message{
		assistantToolUseMessage(2, provider.NewToolUseBlock("call-1", "file_read", mustRawJSON(t, `{"path":"internal/auth/middleware.go"}`))),
	}
	needs := &ContextNeeds{
		ExplicitFiles:  []string{"internal/config/loader.go"},
		MomentumFiles:  []string{"internal/auth/middleware.go"},
		MomentumModule: "internal/auth",
	}

	tracker.Apply(history, needs, config.ContextConfig{MomentumLookbackTurns: 2})

	if len(needs.MomentumFiles) != 0 {
		t.Fatalf("MomentumFiles = %v, want empty on strong-signal turn", needs.MomentumFiles)
	}
	if needs.MomentumModule != "" {
		t.Fatalf("MomentumModule = %q, want empty on strong-signal turn", needs.MomentumModule)
	}
}

func TestMomentumIntegrationWeakSignalTurnGeneratesMomentumEnhancedQuery(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}
	tracker := HistoryMomentumTracker{}
	extractor := HeuristicQueryExtractor{}
	history := []db.Message{
		assistantToolUseMessage(4,
			provider.NewToolUseBlock("call-1", "file_read", mustRawJSON(t, `{"path":"internal/auth/middleware.go"}`)),
			provider.NewToolUseBlock("call-2", "file_write", mustRawJSON(t, `{"path":"internal/auth/service.go"}`)),
		),
	}

	needs := analyzer.AnalyzeTurn("keep going", nil)
	tracker.Apply(history, needs, config.ContextConfig{MomentumLookbackTurns: 2})
	queries := extractor.ExtractQueries("keep going", needs)

	if needs.MomentumModule != "internal/auth" {
		t.Fatalf("MomentumModule = %q, want internal/auth", needs.MomentumModule)
	}
	if !slices.Contains(needs.MomentumFiles, "internal/auth/middleware.go") {
		t.Fatalf("MomentumFiles = %v, want middleware.go", needs.MomentumFiles)
	}
	if !queryContains(queries, "internal/auth keep going") {
		t.Fatalf("queries = %v, want momentum-enhanced query", queries)
	}
}

func TestMomentumIntegrationStrongSignalTurnDoesNotUseStaleMomentum(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}
	tracker := HistoryMomentumTracker{}
	extractor := HeuristicQueryExtractor{}
	history := []db.Message{
		assistantToolUseMessage(5, provider.NewToolUseBlock("call-1", "file_read", mustRawJSON(t, `{"path":"internal/auth/middleware.go"}`))),
	}

	needs := analyzer.AnalyzeTurn("fix internal/config/loader.go", nil)
	needs.MomentumFiles = []string{"internal/auth/middleware.go"}
	needs.MomentumModule = "internal/auth"
	tracker.Apply(history, needs, config.ContextConfig{MomentumLookbackTurns: 2})
	queries := extractor.ExtractQueries("fix internal/config/loader.go", needs)

	if needs.MomentumModule != "" {
		t.Fatalf("MomentumModule = %q, want empty", needs.MomentumModule)
	}
	for _, query := range queries {
		if strings.Contains(query, "internal/auth") {
			t.Fatalf("query %q unexpectedly contains stale momentum", query)
		}
	}
}

func assistantToolUseMessage(turn int64, blocks ...provider.ContentBlock) db.Message {
	raw, _ := json.Marshal(blocks)
	return db.Message{
		Role:       "assistant",
		TurnNumber: turn,
		Content:    sql.NullString{String: string(raw), Valid: true},
	}
}

func toolResultMessage(turn int64, toolName string, content string) db.Message {
	return db.Message{
		Role:       "tool",
		TurnNumber: turn,
		ToolName:   sql.NullString{String: toolName, Valid: true},
		Content:    sql.NullString{String: content, Valid: true},
	}
}

func mustRawJSON(t *testing.T, value string) json.RawMessage {
	t.Helper()
	return json.RawMessage(value)
}
