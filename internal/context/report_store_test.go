package context

import (
	stdctx "context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/projectmemory"
)

func TestCollectIncludedChunkKeysIncludesBrainResults(t *testing.T) {
	report := &ContextAssemblyReport{
		BrainResults: []BrainHit{
			{DocumentPath: "notes/runtime.md", Included: true},
			{DocumentPath: "notes/ignored.md", Included: false},
		},
	}

	included := collectIncludedChunkKeys(report)
	if len(included) != 1 || included[0] != "notes/runtime.md" {
		t.Fatalf("included = %#v, want [notes/runtime.md]", included)
	}
}

func TestCollectExcludedChunkKeysIncludesBrainResults(t *testing.T) {
	report := &ContextAssemblyReport{
		BrainResults: []BrainHit{
			{DocumentPath: "notes/runtime.md", ExclusionReason: "budget"},
			{DocumentPath: "notes/empty.md", ExclusionReason: ""},
		},
	}

	excluded, reasons := collectExcludedChunkKeys(report)
	if len(excluded) != 1 || excluded[0] != "notes/runtime.md" {
		t.Fatalf("excluded = %#v, want [notes/runtime.md]", excluded)
	}
	if got := reasons["notes/runtime.md"]; got != "budget" {
		t.Fatalf("reasons[notes/runtime.md] = %q, want budget", got)
	}
}

func TestProjectMemoryReportStoreRoundTripsQuality(t *testing.T) {
	ctx := stdctx.Background()
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{
		DataDir:    filepath.Join(t.TempDir(), "memory"),
		DurableAck: true,
	})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	defer backend.Close()
	createdAt := time.Date(2026, 5, 5, 23, 0, 0, 0, time.UTC)
	if err := backend.CreateConversation(ctx, projectmemory.CreateConversationArgs{
		ID:          "conv-context-store",
		ProjectID:   "project-1",
		Title:       "Context Store",
		CreatedAtUS: uint64(createdAt.UnixMicro()),
	}); err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	store := NewProjectMemoryReportStore(backend)
	store.now = func() time.Time { return createdAt.Add(time.Second) }
	report := &ContextAssemblyReport{
		TurnNumber:         2,
		AnalysisLatencyMs:  11,
		RetrievalLatencyMs: 22,
		TotalLatencyMs:     33,
		Needs: ContextNeeds{
			SemanticQueries:    []string{"auth middleware"},
			PreferBrainContext: true,
		},
		RAGResults: []RAGHit{{
			ChunkID:  "chunk-1",
			FilePath: "internal/auth/service.go",
			Included: true,
		}},
		BrainResults: []BrainHit{{
			DocumentPath: "notes/runtime.md",
			Included:     true,
		}},
		BudgetTotal:     1000,
		BudgetUsed:      200,
		BudgetBreakdown: map[string]int{"rag": 100, "brain": 100},
	}
	if err := store.Insert(ctx, "conv-context-store", report); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got, err := store.Get(ctx, "conv-context-store", 2)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.TurnNumber != 2 || got.Needs.SemanticQueries[0] != "auth middleware" || len(got.IncludedChunks) != 2 {
		t.Fatalf("got report = %+v, want persisted Shunter report", got)
	}

	store.now = func() time.Time { return createdAt.Add(2 * time.Second) }
	if err := store.UpdateQuality(ctx, "conv-context-store", 2, true, []string{"notes/runtime.md", "notes/runtime.md"}, 1.0); err != nil {
		t.Fatalf("UpdateQuality: %v", err)
	}
	got, err = store.Get(ctx, "conv-context-store", 2)
	if err != nil {
		t.Fatalf("Get after quality: %v", err)
	}
	if !got.AgentUsedSearchTool || got.ContextHitRate != 1.0 || len(got.AgentReadFiles) != 1 || got.AgentReadFiles[0] != "notes/runtime.md" {
		t.Fatalf("quality report = %+v, want sorted unique Shunter quality", got)
	}
}
