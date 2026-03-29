package graph

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ponchione/sirtopham/internal/codeintel"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "graph.db")
	s, err := NewStore(dsn)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewStore(t *testing.T) {
	s := newTestStore(t)
	if s == nil {
		t.Fatal("store is nil")
	}
}

func TestInsertAndGetSymbols(t *testing.T) {
	s := newTestStore(t)

	syms := []Symbol{
		{
			ID: "go:main:function:Hello", Name: "Hello", Kind: "function",
			Language: "go", Package: "main", FilePath: "main.go",
			LineStart: 5, LineEnd: 10, Signature: "func Hello()", Exported: true,
		},
		{
			ID: "go:main:method:Validate", Name: "Validate", Kind: "method",
			Language: "go", Package: "main", FilePath: "main.go",
			LineStart: 12, LineEnd: 20, Receiver: "Config",
		},
	}

	if err := s.InsertSymbols(syms); err != nil {
		t.Fatalf("InsertSymbols: %v", err)
	}

	// GetSymbol
	sym, err := s.GetSymbol("go:main:function:Hello")
	if err != nil {
		t.Fatalf("GetSymbol: %v", err)
	}
	if sym.Name != "Hello" || !sym.Exported {
		t.Errorf("got %+v", sym)
	}

	// GetSymbolsByFile
	byFile, err := s.GetSymbolsByFile("main.go")
	if err != nil {
		t.Fatalf("GetSymbolsByFile: %v", err)
	}
	if len(byFile) != 2 {
		t.Fatalf("got %d symbols, want 2", len(byFile))
	}

	// GetSymbolsByName
	byName, err := s.GetSymbolsByName("Validate")
	if err != nil {
		t.Fatalf("GetSymbolsByName: %v", err)
	}
	if len(byName) != 1 || byName[0].Receiver != "Config" {
		t.Errorf("GetSymbolsByName = %+v", byName)
	}
}

func TestInsertEdgesAndQuery(t *testing.T) {
	s := newTestStore(t)

	syms := []Symbol{
		{ID: "a", Name: "A", Kind: "function", Language: "go", FilePath: "a.go", LineStart: 1, LineEnd: 5},
		{ID: "b", Name: "B", Kind: "function", Language: "go", FilePath: "b.go", LineStart: 1, LineEnd: 5},
	}
	if err := s.InsertSymbols(syms); err != nil {
		t.Fatalf("InsertSymbols: %v", err)
	}

	edges := []Edge{
		{SourceID: "a", TargetID: "b", EdgeType: "CALLS", Confidence: 1.0, SourceLine: 3},
	}
	if err := s.InsertEdges(edges); err != nil {
		t.Fatalf("InsertEdges: %v", err)
	}

	from, err := s.GetEdgesFrom("a")
	if err != nil {
		t.Fatalf("GetEdgesFrom: %v", err)
	}
	if len(from) != 1 || from[0].TargetID != "b" {
		t.Errorf("GetEdgesFrom = %+v", from)
	}

	to, err := s.GetEdgesTo("b")
	if err != nil {
		t.Fatalf("GetEdgesTo: %v", err)
	}
	if len(to) != 1 || to[0].SourceID != "a" {
		t.Errorf("GetEdgesTo = %+v", to)
	}
}

func TestBlastRadius_Upstream(t *testing.T) {
	s := newTestStore(t)

	// A -> B -> C (A calls B, B calls C)
	syms := []Symbol{
		{ID: "a", Name: "A", Kind: "function", Language: "go", FilePath: "a.go", LineStart: 1, LineEnd: 5},
		{ID: "b", Name: "B", Kind: "function", Language: "go", FilePath: "b.go", LineStart: 1, LineEnd: 5},
		{ID: "c", Name: "C", Kind: "function", Language: "go", FilePath: "c.go", LineStart: 1, LineEnd: 5},
	}
	s.InsertSymbols(syms)
	s.InsertEdges([]Edge{
		{SourceID: "a", TargetID: "b", EdgeType: "CALLS", Confidence: 1.0},
		{SourceID: "b", TargetID: "c", EdgeType: "CALLS", Confidence: 1.0},
	})

	// Blast upstream from C — should find B (depth 1) and A (depth 2)
	result, err := s.BlastRadius(context.Background(), codeintel.GraphQuery{
		Symbol:   "c",
		MaxDepth: 3,
		MaxNodes: 10,
	})
	if err != nil {
		t.Fatalf("BlastRadius: %v", err)
	}
	if len(result.Upstream) < 2 {
		t.Fatalf("got %d upstream, want >= 2", len(result.Upstream))
	}
}

func TestBlastRadius_SymbolNotFound(t *testing.T) {
	s := newTestStore(t)

	_, err := s.BlastRadius(context.Background(), codeintel.GraphQuery{
		Symbol: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent symbol")
	}
}

func TestStoreAnalysisResult(t *testing.T) {
	s := newTestStore(t)

	result := &AnalysisResult{
		Symbols: []Symbol{
			{ID: "x", Name: "X", Kind: "function", Language: "go", FilePath: "x.go", LineStart: 1, LineEnd: 5},
		},
		Edges: []Edge{},
		BoundarySymbols: []BoundarySymbol{
			{ID: "ext:fmt:Println", Name: "Println", Kind: "function", Language: "go", Package: "fmt"},
		},
	}

	if err := s.StoreAnalysisResult(result); err != nil {
		t.Fatalf("StoreAnalysisResult: %v", err)
	}

	sym, err := s.GetSymbol("x")
	if err != nil {
		t.Fatalf("GetSymbol after store: %v", err)
	}
	if sym.Name != "X" {
		t.Errorf("Name = %q, want X", sym.Name)
	}
}

func TestChunkMappings(t *testing.T) {
	s := newTestStore(t)

	s.InsertSymbols([]Symbol{
		{ID: "s1", Name: "S1", Kind: "function", Language: "go", FilePath: "s.go", LineStart: 1, LineEnd: 5},
	})

	if err := s.InsertChunkMappings("s1", []string{"chunk-a", "chunk-b"}); err != nil {
		t.Fatalf("InsertChunkMappings: %v", err)
	}

	ids, err := s.GetChunkMappingsForSymbol("s1")
	if err != nil {
		t.Fatalf("GetChunkMappingsForSymbol: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("got %d mappings, want 2", len(ids))
	}
}

func TestStoreImplementsInterface(t *testing.T) {
	var _ codeintel.GraphStore = (*Store)(nil)
}

func TestBlastRadius_CycleDetection_SubstringIDs(t *testing.T) {
	s := newTestStore(t)

	// Symbol IDs where one is a substring of another: "ab" is inside "abc".
	syms := []Symbol{
		{ID: "abc", Name: "ABC", Kind: "function", Language: "go", FilePath: "a.go", LineStart: 1, LineEnd: 5},
		{ID: "ab", Name: "AB", Kind: "function", Language: "go", FilePath: "b.go", LineStart: 1, LineEnd: 5},
		{ID: "target", Name: "Target", Kind: "function", Language: "go", FilePath: "c.go", LineStart: 1, LineEnd: 5},
	}
	s.InsertSymbols(syms)
	// abc -> ab -> target
	s.InsertEdges([]Edge{
		{SourceID: "abc", TargetID: "ab", EdgeType: "CALLS", Confidence: 1.0},
		{SourceID: "ab", TargetID: "target", EdgeType: "CALLS", Confidence: 1.0},
	})

	result, err := s.BlastRadius(context.Background(), codeintel.GraphQuery{
		Symbol:   "target",
		MaxDepth: 5,
		MaxNodes: 10,
	})
	if err != nil {
		t.Fatalf("BlastRadius: %v", err)
	}
	if len(result.Upstream) != 2 {
		t.Fatalf("got %d upstream, want 2 (ab and abc)", len(result.Upstream))
	}
}

func TestBlastRadius_Downstream(t *testing.T) {
	s := newTestStore(t)

	syms := []Symbol{
		{ID: "a", Name: "A", Kind: "function", Language: "go", FilePath: "a.go", LineStart: 1, LineEnd: 5},
		{ID: "b", Name: "B", Kind: "function", Language: "go", FilePath: "b.go", LineStart: 1, LineEnd: 5},
		{ID: "c", Name: "C", Kind: "function", Language: "go", FilePath: "c.go", LineStart: 1, LineEnd: 5},
	}
	s.InsertSymbols(syms)
	s.InsertEdges([]Edge{
		{SourceID: "a", TargetID: "b", EdgeType: "CALLS", Confidence: 1.0},
		{SourceID: "b", TargetID: "c", EdgeType: "CALLS", Confidence: 1.0},
	})

	result, err := s.BlastRadius(context.Background(), codeintel.GraphQuery{
		Symbol:   "a",
		MaxDepth: 3,
		MaxNodes: 10,
	})
	if err != nil {
		t.Fatalf("BlastRadius: %v", err)
	}
	if len(result.Downstream) != 2 {
		t.Fatalf("got %d downstream, want 2", len(result.Downstream))
	}
	if result.Downstream[0].Symbol != "B" {
		t.Errorf("downstream[0] = %q, want B", result.Downstream[0].Symbol)
	}
	if result.Downstream[1].Symbol != "C" {
		t.Errorf("downstream[1] = %q, want C", result.Downstream[1].Symbol)
	}
}

func TestBlastRadius_Cycle(t *testing.T) {
	s := newTestStore(t)

	syms := []Symbol{
		{ID: "a", Name: "A", Kind: "function", Language: "go", FilePath: "a.go", LineStart: 1, LineEnd: 5},
		{ID: "b", Name: "B", Kind: "function", Language: "go", FilePath: "b.go", LineStart: 1, LineEnd: 5},
		{ID: "c", Name: "C", Kind: "function", Language: "go", FilePath: "c.go", LineStart: 1, LineEnd: 5},
	}
	s.InsertSymbols(syms)
	s.InsertEdges([]Edge{
		{SourceID: "a", TargetID: "b", EdgeType: "CALLS", Confidence: 1.0},
		{SourceID: "b", TargetID: "c", EdgeType: "CALLS", Confidence: 1.0},
		{SourceID: "c", TargetID: "a", EdgeType: "CALLS", Confidence: 1.0},
	})

	result, err := s.BlastRadius(context.Background(), codeintel.GraphQuery{
		Symbol:   "a",
		MaxDepth: 10,
		MaxNodes: 30,
	})
	if err != nil {
		t.Fatalf("BlastRadius: %v", err)
	}
	if len(result.Downstream) != 2 {
		t.Fatalf("got %d downstream, want 2", len(result.Downstream))
	}
}

func TestBlastRadius_MaxDepth(t *testing.T) {
	s := newTestStore(t)

	syms := []Symbol{
		{ID: "a", Name: "A", Kind: "function", Language: "go", FilePath: "a.go", LineStart: 1, LineEnd: 5},
		{ID: "b", Name: "B", Kind: "function", Language: "go", FilePath: "b.go", LineStart: 1, LineEnd: 5},
		{ID: "c", Name: "C", Kind: "function", Language: "go", FilePath: "c.go", LineStart: 1, LineEnd: 5},
		{ID: "d", Name: "D", Kind: "function", Language: "go", FilePath: "d.go", LineStart: 1, LineEnd: 5},
		{ID: "e", Name: "E", Kind: "function", Language: "go", FilePath: "e.go", LineStart: 1, LineEnd: 5},
	}
	s.InsertSymbols(syms)
	s.InsertEdges([]Edge{
		{SourceID: "a", TargetID: "b", EdgeType: "CALLS", Confidence: 1.0},
		{SourceID: "b", TargetID: "c", EdgeType: "CALLS", Confidence: 1.0},
		{SourceID: "c", TargetID: "d", EdgeType: "CALLS", Confidence: 1.0},
		{SourceID: "d", TargetID: "e", EdgeType: "CALLS", Confidence: 1.0},
	})

	result, err := s.BlastRadius(context.Background(), codeintel.GraphQuery{
		Symbol:   "a",
		MaxDepth: 2,
		MaxNodes: 10,
	})
	if err != nil {
		t.Fatalf("BlastRadius: %v", err)
	}
	if len(result.Downstream) != 2 {
		t.Fatalf("got %d downstream, want 2 (depth-limited)", len(result.Downstream))
	}
}
