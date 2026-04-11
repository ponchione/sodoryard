package context

import (
	stdctx "context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/codeintel"
	appdb "github.com/ponchione/sodoryard/internal/db"
)

type hybridBrainBackendStub struct {
	hits  []brain.SearchHit
	calls int
}

func (s *hybridBrainBackendStub) ReadDocument(stdctx.Context, string) (string, error) { return "", nil }
func (s *hybridBrainBackendStub) WriteDocument(stdctx.Context, string, string) error  { return nil }
func (s *hybridBrainBackendStub) PatchDocument(stdctx.Context, string, string, string) error {
	return nil
}
func (s *hybridBrainBackendStub) SearchKeyword(stdctx.Context, string) ([]brain.SearchHit, error) {
	s.calls++
	return append([]brain.SearchHit(nil), s.hits...), nil
}
func (s *hybridBrainBackendStub) ListDocuments(stdctx.Context, string) ([]string, error) {
	return nil, nil
}

type hybridBrainStoreStub struct {
	results []codeintel.SearchResult
	calls   int
}

func (s *hybridBrainStoreStub) Upsert(stdctx.Context, []codeintel.Chunk) error { return nil }
func (s *hybridBrainStoreStub) VectorSearch(stdctx.Context, []float32, int, codeintel.Filter) ([]codeintel.SearchResult, error) {
	s.calls++
	return append([]codeintel.SearchResult(nil), s.results...), nil
}
func (s *hybridBrainStoreStub) GetByFilePath(stdctx.Context, string) ([]codeintel.Chunk, error) {
	return nil, nil
}
func (s *hybridBrainStoreStub) GetByName(stdctx.Context, string) ([]codeintel.Chunk, error) {
	return nil, nil
}
func (s *hybridBrainStoreStub) DeleteByFilePath(stdctx.Context, string) error { return nil }
func (s *hybridBrainStoreStub) Close() error                                  { return nil }

type hybridBrainEmbedderStub struct{ calls int }

func (s *hybridBrainEmbedderStub) EmbedTexts(stdctx.Context, []string) ([][]float32, error) {
	return nil, nil
}
func (s *hybridBrainEmbedderStub) EmbedQuery(stdctx.Context, string) ([]float32, error) {
	s.calls++
	return []float32{0.1, 0.2}, nil
}

func TestHybridBrainSearcherMergesKeywordSemanticAndMetadata(t *testing.T) {
	database, err := appdb.OpenDB(stdctx.Background(), t.TempDir()+"/brain.db")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()
	if err := appdb.Init(stdctx.Background(), database); err != nil {
		t.Fatalf("Init: %v", err)
	}
	projectID := "/tmp/project"
	if _, err := database.ExecContext(stdctx.Background(), `
INSERT INTO projects(id, name, root_path, language, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
`, projectID, "project", projectID, "markdown", "2026-04-09T00:00:00Z", "2026-04-09T00:00:00Z"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	tagsJSON, _ := json.Marshal([]string{"brain", "runtime"})
	queries := appdb.New(database)
	if err := queries.UpsertBrainDocument(stdctx.Background(), appdb.UpsertBrainDocumentParams{
		ProjectID:   projectID,
		Path:        "notes/runtime-cache.md",
		Title:       sql.NullString{String: "Runtime Cache Notes", Valid: true},
		ContentHash: "hash-1",
		Tags:        sql.NullString{String: string(tagsJSON), Valid: true},
		CreatedAt:   time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		UpdatedAt:   time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("UpsertBrainDocument: %v", err)
	}

	backend := &hybridBrainBackendStub{hits: []brain.SearchHit{{
		Path:    "notes/runtime-cache.md",
		Snippet: "keyword cache reminder",
		Score:   0.62,
	}}}
	store := &hybridBrainStoreStub{results: []codeintel.SearchResult{{
		Chunk: codeintel.Chunk{FilePath: "notes/runtime-cache.md", Name: "Cache invalidation", Body: "Invalidate the runtime cache after semantic brain reindex.", ChunkType: codeintel.ChunkTypeSection},
		Score: 0.91,
	}}}
	embedder := &hybridBrainEmbedderStub{}
	searcher := NewHybridBrainSearcher(backend, store, embedder, queries, projectID)

	results, err := searcher.Search(stdctx.Background(), BrainSearchRequest{Query: "runtime cache invalidation", Mode: "auto"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if backend.calls != 1 {
		t.Fatalf("keyword backend calls = %d, want 1", backend.calls)
	}
	if store.calls != 1 {
		t.Fatalf("semantic store calls = %d, want 1", store.calls)
	}
	if embedder.calls != 1 {
		t.Fatalf("embedder calls = %d, want 1", embedder.calls)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	hit := results[0]
	if hit.DocumentPath != "notes/runtime-cache.md" {
		t.Fatalf("DocumentPath = %q, want notes/runtime-cache.md", hit.DocumentPath)
	}
	if hit.MatchMode != "hybrid" {
		t.Fatalf("MatchMode = %q, want hybrid", hit.MatchMode)
	}
	if hit.LexicalScore != 0.62 || hit.SemanticScore != 0.91 || hit.FinalScore != 0.91 {
		t.Fatalf("scores = lexical %.2f semantic %.2f final %.2f, want 0.62/0.91/0.91", hit.LexicalScore, hit.SemanticScore, hit.FinalScore)
	}
	if len(hit.MatchSources) != 2 || hit.MatchSources[0] != "keyword" || hit.MatchSources[1] != "semantic" {
		t.Fatalf("MatchSources = %v, want [keyword semantic]", hit.MatchSources)
	}
	if hit.Title != "Runtime Cache Notes" {
		t.Fatalf("Title = %q, want Runtime Cache Notes", hit.Title)
	}
	if hit.SectionHeading != "Cache invalidation" {
		t.Fatalf("SectionHeading = %q, want Cache invalidation", hit.SectionHeading)
	}
	if len(hit.Tags) != 2 || hit.Tags[0] != "brain" || hit.Tags[1] != "runtime" {
		t.Fatalf("Tags = %v, want [brain runtime]", hit.Tags)
	}
	if hit.Snippet != "Invalidate the runtime cache after semantic brain reindex." {
		t.Fatalf("Snippet = %q, want semantic snippet", hit.Snippet)
	}
}

func TestHybridBrainSearcherExpandsBacklinksAndGraphHopsFromBrainLinks(t *testing.T) {
	database, err := appdb.OpenDB(stdctx.Background(), t.TempDir()+"/brain.db")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()
	if err := appdb.Init(stdctx.Background(), database); err != nil {
		t.Fatalf("Init: %v", err)
	}
	projectID := "/tmp/project"
	if _, err := database.ExecContext(stdctx.Background(), `
INSERT INTO projects(id, name, root_path, language, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
`, projectID, "project", projectID, "markdown", "2026-04-09T00:00:00Z", "2026-04-09T00:00:00Z"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	queries := appdb.New(database)
	for _, doc := range []struct {
		path  string
		title string
	}{
		{path: "notes/runtime-cache.md", title: "Runtime Cache Notes"},
		{path: "notes/runtime-rationale.md", title: "Runtime Cache Rationale"},
		{path: "notes/ops-checklist.md", title: "Ops Checklist"},
	} {
		if err := queries.UpsertBrainDocument(stdctx.Background(), appdb.UpsertBrainDocumentParams{
			ProjectID:   projectID,
			Path:        doc.path,
			Title:       sql.NullString{String: doc.title, Valid: true},
			ContentHash: "hash-" + doc.path,
			CreatedAt:   time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			UpdatedAt:   time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		}); err != nil {
			t.Fatalf("UpsertBrainDocument(%s): %v", doc.path, err)
		}
	}
	for _, link := range []appdb.InsertBrainLinkParams{
		{ProjectID: projectID, SourcePath: "notes/runtime-rationale.md", TargetPath: "notes/runtime-cache.md", LinkText: sql.NullString{String: "runtime cache", Valid: true}},
		{ProjectID: projectID, SourcePath: "notes/ops-checklist.md", TargetPath: "notes/runtime-rationale.md", LinkText: sql.NullString{String: "rationale", Valid: true}},
	} {
		if err := queries.InsertBrainLink(stdctx.Background(), link); err != nil {
			t.Fatalf("InsertBrainLink(%s -> %s): %v", link.SourcePath, link.TargetPath, err)
		}
	}

	backend := &hybridBrainBackendStub{hits: []brain.SearchHit{{
		Path:    "notes/runtime-cache.md",
		Snippet: "keyword cache reminder",
		Score:   0.8,
	}}}
	searcher := NewHybridBrainSearcher(backend, nil, nil, queries, projectID)

	results, err := searcher.Search(stdctx.Background(), BrainSearchRequest{
		Query:            "runtime cache",
		Mode:             "auto",
		IncludeGraphHops: true,
		GraphHopDepth:    2,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if results[0].DocumentPath != "notes/runtime-cache.md" || results[0].MatchMode != "keyword" {
		t.Fatalf("first result = %+v, want direct keyword seed first", results[0])
	}
	if results[1].DocumentPath != "notes/runtime-rationale.md" || results[1].MatchMode != "backlink" {
		t.Fatalf("second result = %+v, want backlink expansion", results[1])
	}
	if results[1].GraphSourcePath != "notes/runtime-cache.md" || results[1].GraphHopDepth != 1 {
		t.Fatalf("second result graph metadata = %+v, want source runtime-cache depth 1", results[1])
	}
	if results[2].DocumentPath != "notes/ops-checklist.md" || results[2].MatchMode != "graph" {
		t.Fatalf("third result = %+v, want second-hop graph expansion", results[2])
	}
	if results[2].GraphSourcePath != "notes/runtime-rationale.md" || results[2].GraphHopDepth != 2 {
		t.Fatalf("third result graph metadata = %+v, want source runtime-rationale depth 2", results[2])
	}
	if results[1].FinalScore >= results[0].FinalScore {
		t.Fatalf("depth-1 graph score %.2f should stay below direct score %.2f", results[1].FinalScore, results[0].FinalScore)
	}
	if results[2].FinalScore >= results[1].FinalScore {
		t.Fatalf("depth-2 graph score %.2f should stay below depth-1 score %.2f", results[2].FinalScore, results[1].FinalScore)
	}
	if results[1].Title != "Runtime Cache Rationale" || results[2].Title != "Ops Checklist" {
		t.Fatalf("graph titles = %q / %q, want metadata-enriched titles", results[1].Title, results[2].Title)
	}
	if results[1].Snippet == "" || results[2].Snippet == "" {
		t.Fatalf("graph snippets should explain expansion: %#v", results)
	}
}

func TestHybridBrainSearcherAnnotatesDirectSemanticHitsWithGraphEvidence(t *testing.T) {
	database, err := appdb.OpenDB(stdctx.Background(), t.TempDir()+"/brain.db")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()
	if err := appdb.Init(stdctx.Background(), database); err != nil {
		t.Fatalf("Init: %v", err)
	}
	projectID := "/tmp/project"
	if _, err := database.ExecContext(stdctx.Background(), `
INSERT INTO projects(id, name, root_path, language, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
`, projectID, "project", projectID, "markdown", "2026-04-09T00:00:00Z", "2026-04-09T00:00:00Z"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	queries := appdb.New(database)
	for _, doc := range []struct {
		path  string
		title string
	}{
		{path: "notes/past-debugging-lunar-hinge-bridge.md", title: "Past debugging bridge: LUNAR HINGE 91"},
		{path: "notes/past-debugging-deep-panel-fix.md", title: "Past debugging fix note"},
	} {
		if err := queries.UpsertBrainDocument(stdctx.Background(), appdb.UpsertBrainDocumentParams{
			ProjectID:   projectID,
			Path:        doc.path,
			Title:       sql.NullString{String: doc.title, Valid: true},
			ContentHash: "hash-" + doc.path,
			CreatedAt:   time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			UpdatedAt:   time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		}); err != nil {
			t.Fatalf("UpsertBrainDocument(%s): %v", doc.path, err)
		}
	}
	if err := queries.InsertBrainLink(stdctx.Background(), appdb.InsertBrainLinkParams{
		ProjectID:  projectID,
		SourcePath: "notes/past-debugging-lunar-hinge-bridge.md",
		TargetPath: "notes/past-debugging-deep-panel-fix.md",
		LinkText:   sql.NullString{String: "the linked fix note", Valid: true},
	}); err != nil {
		t.Fatalf("InsertBrainLink: %v", err)
	}
	store := &hybridBrainStoreStub{results: []codeintel.SearchResult{
		{
			Chunk: codeintel.Chunk{FilePath: "notes/past-debugging-lunar-hinge-bridge.md", Body: "LUNAR HINGE 91 points to the linked fix note."},
			Score: 0.53,
		},
		{
			Chunk: codeintel.Chunk{FilePath: "notes/past-debugging-deep-panel-fix.md", Body: "The canary phrase is DEEP PANEL 23."},
			Score: 0.44,
		},
	}}
	searcher := NewHybridBrainSearcher(nil, store, &hybridBrainEmbedderStub{}, queries, projectID)

	results, err := searcher.Search(stdctx.Background(), BrainSearchRequest{
		Query:            "what phrase sits behind lunar hinge 91",
		Mode:             "auto",
		IncludeGraphHops: true,
		GraphHopDepth:    1,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	bridge := results[0]
	if bridge.DocumentPath != "notes/past-debugging-lunar-hinge-bridge.md" {
		t.Fatalf("first result path = %q, want lunar-hinge bridge", bridge.DocumentPath)
	}
	if bridge.MatchMode != "semantic" {
		t.Fatalf("bridge MatchMode = %q, want semantic without reverse-edge backlink promotion", bridge.MatchMode)
	}
	if bridge.GraphSourcePath != "" || bridge.GraphHopDepth != 0 {
		t.Fatalf("bridge graph metadata = %+v, want none", bridge)
	}
	if stringSliceContains(bridge.MatchSources, "backlink") {
		t.Fatalf("bridge MatchSources = %v, want no backlink annotation on direct bridge seed", bridge.MatchSources)
	}

	deepPanel := results[1]
	if deepPanel.DocumentPath != "notes/past-debugging-deep-panel-fix.md" {
		t.Fatalf("second result path = %q, want deep-panel fix", deepPanel.DocumentPath)
	}
	if deepPanel.MatchMode != "hybrid-graph" {
		t.Fatalf("MatchMode = %q, want hybrid-graph", deepPanel.MatchMode)
	}
	if deepPanel.GraphSourcePath != "notes/past-debugging-lunar-hinge-bridge.md" || deepPanel.GraphHopDepth != 1 {
		t.Fatalf("graph metadata = %+v, want source lunar-hinge bridge depth 1", deepPanel)
	}
	if !stringSliceContains(deepPanel.MatchSources, "semantic") || !stringSliceContains(deepPanel.MatchSources, "graph") {
		t.Fatalf("MatchSources = %v, want semantic+graph", deepPanel.MatchSources)
	}
	if deepPanel.FinalScore <= deepPanel.SemanticScore {
		t.Fatalf("FinalScore = %.2f, want graph expansion to lift direct semantic score %.2f", deepPanel.FinalScore, deepPanel.SemanticScore)
	}
}

func TestHybridBrainSearcherSkipsGraphExpansionWhenDisabled(t *testing.T) {
	database, err := appdb.OpenDB(stdctx.Background(), t.TempDir()+"/brain.db")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()
	if err := appdb.Init(stdctx.Background(), database); err != nil {
		t.Fatalf("Init: %v", err)
	}
	projectID := "/tmp/project"
	if _, err := database.ExecContext(stdctx.Background(), `
INSERT INTO projects(id, name, root_path, language, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
`, projectID, "project", projectID, "markdown", "2026-04-09T00:00:00Z", "2026-04-09T00:00:00Z"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	queries := appdb.New(database)
	if err := queries.InsertBrainLink(stdctx.Background(), appdb.InsertBrainLinkParams{
		ProjectID:  projectID,
		SourcePath: "notes/runtime-rationale.md",
		TargetPath: "notes/runtime-cache.md",
	}); err != nil {
		t.Fatalf("InsertBrainLink: %v", err)
	}
	backend := &hybridBrainBackendStub{hits: []brain.SearchHit{{
		Path:    "notes/runtime-cache.md",
		Snippet: "keyword cache reminder",
		Score:   0.8,
	}}}
	searcher := NewHybridBrainSearcher(backend, nil, nil, queries, projectID)

	results, err := searcher.Search(stdctx.Background(), BrainSearchRequest{Query: "runtime cache", Mode: "auto"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1 without graph expansion", len(results))
	}
}

func TestBrainMatchModeDistinguishesBacklinkGraphAndHybridStructuralResults(t *testing.T) {
	tests := []struct {
		name   string
		result BrainSearchResult
		want   string
	}{
		{
			name:   "direct backlink",
			result: BrainSearchResult{MatchSources: []string{"backlink"}, GraphHopDepth: 1},
			want:   "backlink",
		},
		{
			name:   "multi-hop graph",
			result: BrainSearchResult{MatchSources: []string{"backlink"}, GraphHopDepth: 2},
			want:   "graph",
		},
		{
			name:   "hybrid backlink",
			result: BrainSearchResult{MatchSources: []string{"keyword", "backlink"}, GraphHopDepth: 1},
			want:   "hybrid-backlink",
		},
		{
			name:   "hybrid graph",
			result: BrainSearchResult{MatchSources: []string{"semantic", "graph"}, GraphHopDepth: 2},
			want:   "hybrid-graph",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := brainMatchMode(tt.result); got != tt.want {
				t.Fatalf("brainMatchMode(%+v) = %q, want %q", tt.result, got, tt.want)
			}
		})
	}
}
