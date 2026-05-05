package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	brainindexstate "github.com/ponchione/sodoryard/internal/brain/indexstate"
	"github.com/ponchione/sodoryard/internal/config"
	appcontext "github.com/ponchione/sodoryard/internal/context"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/provider"
)

// ── helpers ──────────────────────────────────────────────────────────

func brainConfig(enabled bool) config.BrainConfig {
	return config.BrainConfig{
		Enabled:            enabled,
		LogBrainOperations: true,
		LintStaleDays:      90,
	}
}

type fakeBackend struct {
	docs      map[string]string
	searches  map[string][]brain.SearchHit
	listings  map[string][]string
	readErr   error
	searchErr error
	writeErr  error
	patchErr  error
	patchOps  []string
}

func newFakeBackend(docs map[string]string) *fakeBackend {
	backend := &fakeBackend{
		docs:     docs,
		searches: map[string][]brain.SearchHit{},
		listings: map[string][]string{},
	}
	for docPath, content := range docs {
		snippet := content[:min(100, len(content))]
		for _, token := range searchTokens(docPath + "\n" + content) {
			backend.searches[token] = append(backend.searches[token], brain.SearchHit{
				Path:    docPath,
				Snippet: snippet,
				Score:   0.8,
			})
		}
		parts := strings.Split(docPath, "/")
		for i := 0; i < len(parts); i++ {
			dir := strings.Join(parts[:i], "/")
			backend.listings[dir] = appendIfMissing(backend.listings[dir], docPath)
		}
		backend.listings[""] = appendIfMissing(backend.listings[""], docPath)
	}
	return backend
}

func (f *fakeBackend) ReadDocument(ctx context.Context, path string) (string, error) {
	if f.readErr != nil {
		return "", f.readErr
	}
	content, ok := f.docs[path]
	if !ok {
		return "", fmt.Errorf("Document not found: %s", path)
	}
	return content, nil
}

func (f *fakeBackend) WriteDocument(ctx context.Context, path string, content string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if f.docs == nil {
		f.docs = map[string]string{}
	}
	f.docs[path] = content
	return nil
}

func (f *fakeBackend) PatchDocument(ctx context.Context, path string, operation string, content string) error {
	if f.patchErr != nil {
		return f.patchErr
	}
	current, ok := f.docs[path]
	if !ok {
		return fmt.Errorf("Document not found: %s", path)
	}
	f.patchOps = append(f.patchOps, operation)
	switch operation {
	case "append":
		f.docs[path] = appendContent(current, content)
	case "prepend":
		f.docs[path] = prependContent(current, content)
	case "replace_section":
		heading := firstHeadingLine(content)
		if heading == "" {
			return fmt.Errorf("replace_section content must start with a heading")
		}
		updated, err := replaceSectionContent(current, heading, content)
		if err != nil {
			return err
		}
		f.docs[path] = updated
	default:
		return fmt.Errorf("unsupported patch operation: %s", operation)
	}
	return nil
}

func (f *fakeBackend) SearchKeyword(ctx context.Context, query string) ([]brain.SearchHit, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	lowerQuery := strings.ToLower(query)
	var hits []brain.SearchHit
	for path, content := range f.docs {
		if strings.Contains(strings.ToLower(path), lowerQuery) || strings.Contains(strings.ToLower(content), lowerQuery) {
			hits = append(hits, brain.SearchHit{
				Path:    path,
				Snippet: content[:min(100, len(content))],
				Score:   0.8,
			})
		}
	}
	if len(hits) > 0 {
		return sortSearchHits(hits), nil
	}
	return sortSearchHits(append([]brain.SearchHit(nil), f.searches[lowerQuery]...)), nil
}

func sortSearchHits(hits []brain.SearchHit) []brain.SearchHit {
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].Path != hits[j].Path {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Snippet < hits[j].Snippet
	})
	return hits
}

func (f *fakeBackend) ListDocuments(ctx context.Context, directory string) ([]string, error) {
	if files, ok := f.listings[directory]; ok && len(files) > 0 {
		return append([]string(nil), files...), nil
	}
	var files []string
	for path := range f.docs {
		if directory == "" || strings.HasPrefix(path, directory+"/") {
			files = append(files, path)
		}
	}
	return files, nil
}

func searchTokens(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z':
			return false
		case r >= '0' && r <= '9':
			return false
		default:
			return true
		}
	})
	seen := map[string]struct{}{}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

func appendIfMissing(items []string, item string) []string {
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}

func firstHeadingLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if level, text := parseHeading(line); level > 0 {
			return strings.Repeat("#", level) + " " + text
		}
	}
	return ""
}

type fakeRuntimeBrainSearcher struct {
	resultsByMode map[string][]appcontext.BrainSearchResult
	gotRequests   []appcontext.BrainSearchRequest
	err           error
}

func (f *fakeRuntimeBrainSearcher) Search(ctx context.Context, request appcontext.BrainSearchRequest) ([]appcontext.BrainSearchResult, error) {
	f.gotRequests = append(f.gotRequests, request)
	if f.err != nil {
		return nil, f.err
	}
	if f.resultsByMode != nil {
		return append([]appcontext.BrainSearchResult(nil), f.resultsByMode[request.Mode]...), nil
	}
	return nil, nil
}

// ── brain_search tests ──────────────────────────────────────────────

func TestBrainSearchDisabled(t *testing.T) {
	tool := NewBrainSearch(nil, brainConfig(false))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":"test"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for disabled brain")
	}
	if !strings.Contains(result.Content, "not configured") {
		t.Fatalf("content = %q, want 'not configured' message", result.Content)
	}
}

func TestBrainSearchEmptyQuery(t *testing.T) {
	tool := NewBrainSearch(nil, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":""}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for empty query")
	}
}

func TestBrainSearchKeywordSuccess(t *testing.T) {
	docs := map[string]string{
		"arch/design.md": "---\ntags: [architecture]\n---\n# Design\nThe pipeline architecture uses channels.",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainSearch(backend, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":"pipeline"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	want := "Found 1 brain document for \"pipeline\":\n- arch/design.md — Design\n  --- tags: [architecture] --- # Design The pipeline architecture uses channels."
	if got := result.Content; got != want {
		t.Fatalf("content = %q\nwant    %q", got, want)
	}
}

func TestBrainSearchNoResults(t *testing.T) {
	docs := map[string]string{}
	backend := newFakeBackend(docs)
	tool := NewBrainSearch(backend, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":"nonexistent"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false for zero results")
	}
	if !strings.Contains(result.Content, "No brain documents found") {
		t.Fatalf("content = %q, want 'No brain documents found'", result.Content)
	}
}

func TestBrainToolDefinitionsSteerVaultNotePathsToBrainTools(t *testing.T) {
	reg := NewRegistry()
	RegisterBrainTools(reg, newFakeBackend(map[string]string{}), brainConfig(true))

	defs := reg.ToolDefinitions()
	byName := make(map[string]provider.ToolDefinition, len(defs))
	for _, def := range defs {
		byName[def.Name] = def
	}

	brainRead, ok := byName["brain_read"]
	if !ok {
		t.Fatal("brain_read definition missing")
	}
	for _, want := range []string{"notes/...md", "file_read", "vault-relative", ".brain paths"} {
		if !strings.Contains(brainRead.Description, want) {
			t.Fatalf("brain_read description = %q, want substring %q", brainRead.Description, want)
		}
	}

	brainSearch, ok := byName["brain_search"]
	if !ok {
		t.Fatal("brain_search definition missing")
	}
	for _, want := range []string{"notes/...md", "search_text", "brain_read", ".brain paths", "do not double-check a successful brain hit"} {
		if !strings.Contains(brainSearch.Description, want) {
			t.Fatalf("brain_search description = %q, want substring %q", brainSearch.Description, want)
		}
	}
}

func TestRegistryToolDefinitionsPreferBrainToolsBeforeSearchTools(t *testing.T) {
	reg := NewRegistry()
	RegisterBrainTools(reg, newFakeBackend(map[string]string{}), brainConfig(true))
	RegisterSearchTools(reg, nil)

	defs := reg.ToolDefinitions()
	index := map[string]int{}
	for i, def := range defs {
		index[def.Name] = i
	}
	if index["brain_search"] >= index["search_text"] {
		t.Fatalf("tool order brain_search/search_text = %d/%d, want brain_search before search_text", index["brain_search"], index["search_text"])
	}
	if index["brain_read"] >= index["search_text"] {
		t.Fatalf("tool order brain_read/search_text = %d/%d, want brain_read before search_text", index["brain_read"], index["search_text"])
	}
}

func TestBrainSearchSemanticFallback(t *testing.T) {
	docs := map[string]string{
		"notes/a.md": "architecture decisions about auth",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainSearch(backend, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":"auth","mode":"semantic"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "Semantic/index-backed brain search is not a landed runtime path yet") {
		t.Fatalf("content missing semantic notice: %q", result.Content)
	}
	// Should still return results via keyword fallback.
	if !strings.Contains(result.Content, "notes/a.md") {
		t.Fatalf("content missing keyword results: %q", result.Content)
	}
}

func TestBrainSearchAutoPassesGraphExpansionConfigToRuntime(t *testing.T) {
	backend := newFakeBackend(map[string]string{})
	runtime := &fakeRuntimeBrainSearcher{resultsByMode: map[string][]appcontext.BrainSearchResult{
		"auto": {{
			DocumentPath:    "notes/runtime-rationale.md",
			Title:           "Runtime Cache Rationale",
			Snippet:         "Backlinks to runtime cache notes.",
			MatchMode:       "backlink",
			MatchSources:    []string{"backlink"},
			GraphHopDepth:   1,
			GraphSourcePath: "notes/runtime-cache.md",
		}},
	}}
	cfg := brainConfig(true)
	cfg.IncludeGraphHops = true
	cfg.GraphHopDepth = 2
	tool := NewBrainSearchWithRuntime(backend, runtime, cfg)
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":"runtime cache","mode":"auto"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if len(runtime.gotRequests) != 1 {
		t.Fatalf("runtime requests = %+v, want one request", runtime.gotRequests)
	}
	if !runtime.gotRequests[0].IncludeGraphHops || runtime.gotRequests[0].GraphHopDepth != 2 {
		t.Fatalf("runtime request = %+v, want graph expansion enabled with depth 2", runtime.gotRequests[0])
	}
	if !strings.Contains(result.Content, "Runtime Cache Rationale [backlink]") {
		t.Fatalf("content = %q, want backlink marker in title", result.Content)
	}
}

func TestBrainSearchFormatsMultiHopGraphAndHybridGraphLabels(t *testing.T) {
	backend := newFakeBackend(map[string]string{})
	runtime := &fakeRuntimeBrainSearcher{resultsByMode: map[string][]appcontext.BrainSearchResult{
		"auto": {
			{
				DocumentPath:    "notes/ops-checklist.md",
				Title:           "Ops Checklist",
				Snippet:         "Graph-related to runtime-rationale.md via 2 graph hops.",
				MatchMode:       "graph",
				MatchSources:    []string{"backlink"},
				GraphHopDepth:   2,
				GraphSourcePath: "notes/runtime-rationale.md",
			},
			{
				DocumentPath:    "notes/cache-playbook.md",
				Title:           "Cache Playbook",
				Snippet:         "Hybrid graph expansion result.",
				MatchMode:       "hybrid-graph",
				MatchSources:    []string{"semantic", "graph"},
				GraphHopDepth:   2,
				GraphSourcePath: "notes/runtime-cache.md",
			},
		},
	}}
	cfg := brainConfig(true)
	cfg.IncludeGraphHops = true
	cfg.GraphHopDepth = 2
	tool := NewBrainSearchWithRuntime(backend, runtime, cfg)
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":"runtime cache","mode":"auto"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "Ops Checklist [graph]") {
		t.Fatalf("content = %q, want graph marker in title", result.Content)
	}
	if !strings.Contains(result.Content, "Cache Playbook [hybrid-graph]") {
		t.Fatalf("content = %q, want hybrid-graph marker in title", result.Content)
	}
}

func TestBrainSearchRuntimeTagFilterMatchesNormalizedTagsWithoutReadingDocument(t *testing.T) {
	backend := newFakeBackend(map[string]string{})
	backend.readErr = fmt.Errorf("should not read document")
	runtime := &fakeRuntimeBrainSearcher{resultsByMode: map[string][]appcontext.BrainSearchResult{
		"auto": {{
			DocumentPath: "notes/debug-loop.md",
			Title:        "Debug Loop",
			Snippet:      "Generated barrel loop notes.",
			MatchMode:    "semantic",
			MatchSources: []string{"semantic"},
			Tags:         []string{"debug-history", "runtime_cache"},
		}},
	}}
	tool := NewBrainSearchWithRuntime(backend, runtime, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":"vite rebuild loop","mode":"auto","tags":["debug history","runtime cache"]}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "notes/debug-loop.md") {
		t.Fatalf("content = %q, want runtime-tag-filtered hit", result.Content)
	}
}

func TestBrainSearchWithTagsFiltersHitsByTag(t *testing.T) {
	docs := map[string]string{
		"_log.md":                "## [2026-04-07T16:41:44Z] query | vite rebuild loop fix (tags: debug history) Returned 0 results via keyword search.",
		"notes/debug-loop.md":    "---\ntags: [debug-history]\n---\n# Debug Loop\nThe vite rebuild loop fix moved generated code out of src.",
		"notes/untagged-loop.md": "# Untagged Loop\nThe vite rebuild loop fix moved generated code out of src.",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainSearch(backend, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":"vite rebuild loop fix","tags":["debug-history"]}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "notes/debug-loop.md") {
		t.Fatalf("content = %q, want tagged hit", result.Content)
	}
	if strings.Contains(result.Content, "notes/untagged-loop.md") {
		t.Fatalf("content = %q, should exclude untagged hit", result.Content)
	}
	if strings.Contains(result.Content, "_log.md") {
		t.Fatalf("content = %q, should exclude operational log note", result.Content)
	}
}

func TestBrainSearchWithTagsFallsBackToLooseMatchWithinTaggedDocs(t *testing.T) {
	docs := map[string]string{
		"notes/debug-loop.md": "# Past debugging: vite rebuild loop\n\nDate: 2026-04-07\nFamily: debug-history\n\n## Fix\nMoved the generated barrel out of src and into .generated/barrel.ts.",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainSearch(backend, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":"vite rebuild loop fix","tags":["debug-history"]}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "notes/debug-loop.md") {
		t.Fatalf("content = %q, want loose tagged match", result.Content)
	}
}

func TestBrainSearchWithTagsSkipsLooseFallbackForPunctuationOnlyQuery(t *testing.T) {
	docs := map[string]string{
		"notes/debug-loop.md":  "---\ntags: [debug-history]\n---\n# Debug Loop\nMoved the generated barrel out of src.",
		"notes/debug-cache.md": "---\ntags: [debug-history]\n---\n# Debug Cache\nReset the runtime cache after retries.",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainSearch(backend, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":"!!! ---,","tags":["debug-history"]}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "No brain documents found") {
		t.Fatalf("content = %q, want no-results message when normalized query has no tokens", result.Content)
	}
	if strings.Contains(result.Content, "notes/debug-loop.md") || strings.Contains(result.Content, "notes/debug-cache.md") {
		t.Fatalf("content = %q, should not broad-match all tagged docs for punctuation-only query", result.Content)
	}
}

func TestBrainSearchWithTagsExcludesMissingTags(t *testing.T) {
	docs := map[string]string{
		"notes/untagged-loop.md": "# Untagged Loop\nThe vite rebuild loop fix moved generated code out of src.",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainSearch(backend, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":"vite rebuild loop fix","tags":["debug-history"]}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false for missing tags, content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "No brain documents found") {
		t.Fatalf("content = %q, want no-results message", result.Content)
	}
}

func TestBrainSearchWithTagOnlyQueryReturnsTaggedNotes(t *testing.T) {
	docs := map[string]string{
		"notes/debug-loop.md": "---\ntags: [debug-history]\n---\n# Debug Loop\nGenerated barrel loop notes.",
		"notes/debug-lint.md": "# Debug Lint\nKeep the #debug-history journal up to date.",
		"notes/rationale.md":  "---\ntags: [rationale]\n---\n# Layout\nMinimal content first rationale.",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainSearch(backend, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":"","tags":["debug-history"]}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "notes/debug-loop.md") || !strings.Contains(result.Content, "notes/debug-lint.md") {
		t.Fatalf("content = %q, want both tagged notes", result.Content)
	}
	if strings.Contains(result.Content, "notes/rationale.md") {
		t.Fatalf("content = %q, should exclude differently tagged note", result.Content)
	}
}

func TestBrainSearchMaxResults(t *testing.T) {
	docs := map[string]string{
		"a.md": "test content A",
		"b.md": "test content B",
		"c.md": "test content C",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainSearch(backend, brainConfig(true))
	maxResults := 1
	input, _ := json.Marshal(brainSearchInput{Query: "test", MaxResults: &maxResults})
	result, err := tool.Execute(context.Background(), "/tmp", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if got, want := result.Content, "Found 1 brain document for \"test\":\n- a.md — A\n  test content A"; got != want {
		t.Fatalf("content = %q\nwant    %q", got, want)
	}
}

func TestBrainSearchAppendsQueryLogWhenEnabled(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/auth.md": "# Auth\nAuthentication architecture notes.",
	})
	cfg := brainConfig(true)
	cfg.LogBrainQueries = true
	tool := NewBrainSearch(backend, cfg)

	ctx := ContextWithExecutionMeta(context.Background(), ExecutionMeta{
		ConversationID: "conv-search",
		TurnNumber:     1,
		Iteration:      1,
	})
	result, err := tool.Execute(ctx, "/tmp", json.RawMessage(`{"query":"auth"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}

	logDoc := backend.docs["_log.md"]
	if !strings.Contains(logDoc, "query | auth") {
		t.Fatalf("expected query log entry, got:\n%s", logDoc)
	}
	if !strings.Contains(logDoc, "Returned 1 result via keyword search.") {
		t.Fatalf("expected deterministic query summary, got:\n%s", logDoc)
	}
	if !strings.Contains(logDoc, "Session: conv-search") {
		t.Fatalf("expected session log entry, got:\n%s", logDoc)
	}
}

func TestBrainSearchDoesNotAppendQueryLogWhenDisabled(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/auth.md": "# Auth\nAuthentication architecture notes.",
	})
	tool := NewBrainSearch(backend, brainConfig(true))

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":"auth"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if _, ok := backend.docs["_log.md"]; ok {
		t.Fatalf("expected no query log document when disabled, got:\n%s", backend.docs["_log.md"])
	}
}

func TestBrainSearchDoesNotAppendQueryLogOnFailure(t *testing.T) {
	backend := newFakeBackend(map[string]string{})
	backend.searchErr = fmt.Errorf("search backend unavailable")
	cfg := brainConfig(true)
	cfg.LogBrainQueries = true
	tool := NewBrainSearch(backend, cfg)

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":"auth"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected Success=false, content = %q", result.Content)
	}
	if _, ok := backend.docs["_log.md"]; ok {
		t.Fatalf("expected no query log document on failure, got:\n%s", backend.docs["_log.md"])
	}
}

func TestBrainSearchPurityDependsOnQueryLogging(t *testing.T) {
	if got := NewBrainSearch(nil, brainConfig(true)).ToolPurity(); got != Pure {
		t.Fatalf("ToolPurity() = %v, want %v when query logging disabled", got, Pure)
	}

	cfg := brainConfig(true)
	cfg.LogBrainQueries = true
	if got := NewBrainSearch(nil, cfg).ToolPurity(); got != Mutating {
		t.Fatalf("ToolPurity() = %v, want %v when query logging enabled", got, Mutating)
	}
}

// ── brain_read tests ────────────────────────────────────────────────

func TestBrainReadDisabled(t *testing.T) {
	tool := NewBrainRead(nil, brainConfig(false))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":"test.md"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for disabled brain")
	}
}

func TestBrainReadSuccess(t *testing.T) {
	docs := map[string]string{
		"arch/design.md": "---\ntags: [architecture]\nstatus: active\n---\n# Design\n\nThe auth system uses [[provider-design]] and [[error-handling]].\n",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainRead(backend, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":"arch/design.md"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	want := "Brain document: arch/design.md\n\nFrontmatter:\n- tags: [architecture]\n- status: active\n\nOutgoing links:\n- [[provider-design]]\n- [[error-handling]]\n\nContent:\n```md\n# Design\n\nThe auth system uses [[provider-design]] and [[error-handling]].\n```"
	if got := result.Content; got != want {
		t.Fatalf("content = %q\nwant    %q", got, want)
	}
}

func TestBrainReadNotFound(t *testing.T) {
	docs := map[string]string{
		"arch/other.md": "content",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainRead(backend, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":"arch/missing.md"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for missing document")
	}
	if !strings.Contains(result.Content, "Document not found") {
		t.Fatalf("content = %q, want not found message", result.Content)
	}
	if !strings.Contains(result.Content, "arch/other.md") {
		t.Fatalf("content = %q, want directory listing with arch/other.md", result.Content)
	}
}

func TestBrainReadNoFrontmatter(t *testing.T) {
	docs := map[string]string{
		"notes/plain.md": "# Just a heading\nPlain content without frontmatter.",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainRead(backend, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":"notes/plain.md"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if strings.Contains(result.Content, "Frontmatter:") {
		t.Fatalf("should not contain Frontmatter section: %q", result.Content)
	}
	if !strings.Contains(result.Content, "# Just a heading") {
		t.Fatalf("missing content: %q", result.Content)
	}
}

func TestBrainReadWithBacklinks(t *testing.T) {
	docs := map[string]string{
		"arch/design.md":   "# Design\nCore design document.",
		"notes/session.md": "Working on [[design]] today.",
		"notes/review.md":  "Reviewed the design changes.",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainRead(backend, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp",
		json.RawMessage(`{"path":"arch/design.md","include_backlinks":true}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "Referenced by:") {
		t.Fatalf("missing backlinks section: %q", result.Content)
	}
}

func TestBrainReadWithIndexedBacklinksPrefersBrainLinks(t *testing.T) {
	database, err := appdb.OpenDB(context.Background(), t.TempDir()+"/brain.db")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()
	if err := appdb.Init(context.Background(), database); err != nil {
		t.Fatalf("Init: %v", err)
	}
	projectID := "/tmp/project"
	if _, err := database.ExecContext(context.Background(), `
INSERT INTO projects(id, name, root_path, language, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
`, projectID, "project", projectID, "markdown", "2026-04-09T00:00:00Z", "2026-04-09T00:00:00Z"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	queries := appdb.New(database)
	for _, path := range []string{"arch/design.md", "notes/session.md", "notes/review.md"} {
		if err := queries.UpsertBrainDocument(context.Background(), appdb.UpsertBrainDocumentParams{
			ProjectID:   projectID,
			Path:        path,
			Title:       sql.NullString{String: path, Valid: true},
			ContentHash: "hash-" + path,
			CreatedAt:   time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			UpdatedAt:   time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		}); err != nil {
			t.Fatalf("UpsertBrainDocument(%s): %v", path, err)
		}
	}
	for _, link := range []appdb.InsertBrainLinkParams{
		{ProjectID: projectID, SourcePath: "notes/session.md", TargetPath: "arch/design.md"},
		{ProjectID: projectID, SourcePath: "notes/review.md", TargetPath: "arch/design.md"},
	} {
		if err := queries.InsertBrainLink(context.Background(), link); err != nil {
			t.Fatalf("InsertBrainLink(%s -> %s): %v", link.SourcePath, link.TargetPath, err)
		}
	}

	backend := newFakeBackend(map[string]string{
		"arch/design.md":   "# Design\nCore design document.",
		"notes/session.md": "This text does not mention the basename.",
		"notes/review.md":  "This text also avoids basename fallback.",
	})
	tool := NewBrainReadWithIndex(backend, brainConfig(true), queries, projectID)
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":"arch/design.md","include_backlinks":true}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "- notes/review.md") || !strings.Contains(result.Content, "- notes/session.md") {
		t.Fatalf("content = %q, want indexed backlinks", result.Content)
	}
}

func TestBrainReadBacklinksFallbacksToHeuristicWhenNoBrainLinksExist(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"arch/design.md":   "# Design\nCore design document.",
		"notes/session.md": "Working on design today.",
	})
	tool := NewBrainReadWithIndex(backend, brainConfig(true), nil, "/tmp/project")
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":"arch/design.md","include_backlinks":true}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "- notes/session.md") {
		t.Fatalf("content = %q, want heuristic fallback backlink", result.Content)
	}
}

// ── brain_write tests ───────────────────────────────────────────────

func TestBrainWriteDisabled(t *testing.T) {
	tool := NewBrainWrite(nil, brainConfig(false))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":"test.md","content":"hello"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for disabled brain")
	}
}

func TestBrainWriteSuccess(t *testing.T) {
	docs := map[string]string{}
	backend := newFakeBackend(docs)
	tool := NewBrainWrite(backend, brainConfig(true))
	content := "---\ntags: [test]\n---\n# New Document\nContent here."
	input, _ := json.Marshal(brainWriteInput{Path: "notes/new.md", Content: content})
	result, err := tool.Execute(context.Background(), "/tmp", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "Wrote brain document: notes/new.md") {
		t.Fatalf("content = %q, want write success message", result.Content)
	}
	if !strings.Contains(result.Content, "Derived brain metadata and semantic chunks are now stale") {
		t.Fatalf("content = %q, want explicit stale-index reminder", result.Content)
	}
	if !strings.Contains(result.Content, "yard brain index") {
		t.Fatalf("content = %q, want explicit reindex command", result.Content)
	}
	// Verify the doc was actually stored.
	if docs["notes/new.md"] != content {
		t.Fatalf("stored content = %q, want original", docs["notes/new.md"])
	}
}

func TestBrainWriteShunterSkipsFileBackedStaleState(t *testing.T) {
	projectRoot := t.TempDir()
	backend := newFakeBackend(map[string]string{})
	cfg := brainConfig(true)
	cfg.Backend = "shunter"
	cfg.LogBrainOperations = false
	tool := NewBrainWrite(backend, cfg)

	result, err := tool.Execute(context.Background(), projectRoot, json.RawMessage(`{"path":"notes/shunter.md","content":"---\ntags: [memory]\n---\n# Shunter"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if _, err := os.Stat(brainindexstate.Path(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("brain index state file stat err = %v, want not-exist for Shunter backend", err)
	}
}

func TestBrainWriteAppendsOperationLogWithSession(t *testing.T) {
	backend := newFakeBackend(map[string]string{})
	tool := NewBrainWrite(backend, brainConfig(true))

	ctx := ContextWithExecutionMeta(context.Background(), ExecutionMeta{
		ConversationID: "conv-write",
		TurnNumber:     1,
		Iteration:      1,
	})
	input := json.RawMessage(`{"path":"notes/design.md","content":"---\ntags: [architecture]\n---\n# Design\nPipeline notes"}`)
	result, err := tool.Execute(ctx, "/tmp", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}

	logDoc := backend.docs["_log.md"]
	if !strings.Contains(logDoc, "Session: conv-write") {
		t.Fatalf("expected session log entry, got:\n%s", logDoc)
	}
}

func TestBrainWriteAppendsOperationLog(t *testing.T) {
	backend := newFakeBackend(map[string]string{})
	tool := NewBrainWrite(backend, brainConfig(true))

	input := json.RawMessage(`{"path":"notes/design.md","content":"---\ntags: [architecture]\n---\n# Design\nPipeline notes"}`)
	result, err := tool.Execute(context.Background(), "/tmp", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}

	logDoc := backend.docs["_log.md"]
	if !strings.Contains(logDoc, "write | notes/design.md") {
		t.Fatalf("expected write log entry, got:\n%s", logDoc)
	}
}

func TestBrainWriteNoFrontmatterWarning(t *testing.T) {
	docs := map[string]string{}
	backend := newFakeBackend(docs)
	tool := NewBrainWrite(backend, brainConfig(true))
	// No frontmatter — should still succeed but log warning.
	input, _ := json.Marshal(brainWriteInput{Path: "notes/bare.md", Content: "# No Frontmatter\nJust content."})
	result, err := tool.Execute(context.Background(), "/tmp", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
}

func TestBrainWriteEmptyPath(t *testing.T) {
	tool := NewBrainWrite(nil, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":"","content":"hello"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for empty path")
	}
}

func TestBrainWriteEmptyContent(t *testing.T) {
	tool := NewBrainWrite(nil, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":"test.md","content":""}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for empty content")
	}
}

func TestBrainWriteNormalizesPathAndAllowsScopedWrite(t *testing.T) {
	backend := newFakeBackend(map[string]string{})
	cfg := brainConfig(true)
	cfg.BrainWritePaths = []string{"receipts/**"}
	tool := NewBrainWrite(backend, cfg)

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":".brain/receipts/reviewer/run.md","content":"---\nagent: reviewer\n---\n# Receipt"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if _, ok := backend.docs["receipts/reviewer/run.md"]; !ok {
		t.Fatalf("docs = %#v, want normalized receipts/reviewer/run.md", backend.docs)
	}
}

func TestBrainWriteRejectsDeniedPathEvenWhenAllowed(t *testing.T) {
	backend := newFakeBackend(map[string]string{})
	cfg := brainConfig(true)
	cfg.BrainWritePaths = []string{"receipts/**"}
	cfg.BrainDenyPaths = []string{"receipts/private/**"}
	tool := NewBrainWrite(backend, cfg)

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":"receipts/private/run.md","content":"---\nagent: reviewer\n---\n# Receipt"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for denied path")
	}
	if !strings.Contains(result.Content, "denied by policy") {
		t.Fatalf("content = %q, want denied-by-policy message", result.Content)
	}
}

func TestBrainWriteRejectsPathOutsideAllowlist(t *testing.T) {
	backend := newFakeBackend(map[string]string{})
	cfg := brainConfig(true)
	cfg.BrainWritePaths = []string{"receipts/**"}
	tool := NewBrainWrite(backend, cfg)

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":"notes/run.md","content":"---\nagent: reviewer\n---\n# Receipt"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for path outside allowlist")
	}
	if !strings.Contains(result.Content, "not allowed by policy") {
		t.Fatalf("content = %q, want allowlist message", result.Content)
	}
}

// ── brain_update tests ──────────────────────────────────────────────

func TestBrainUpdateUsesBackendPatchDocument(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/design.md": "# Design\n\nOriginal details.",
	})
	tool := NewBrainUpdate(backend, brainConfig(true))
	input := json.RawMessage(`{"path":"notes/design.md","operation":"append","content":"## Appendix\n\nExtra notes."}`)

	result, err := tool.Execute(context.Background(), "/tmp", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if len(backend.patchOps) != 1 || backend.patchOps[0] != "append" {
		t.Fatalf("patchOps = %#v, want [append]", backend.patchOps)
	}
}

func TestBrainUpdateShunterSkipsFileBackedStaleState(t *testing.T) {
	projectRoot := t.TempDir()
	backend := newFakeBackend(map[string]string{
		"notes/design.md": "# Design\n\nOriginal details.",
	})
	cfg := brainConfig(true)
	cfg.Backend = "shunter"
	cfg.LogBrainOperations = false
	tool := NewBrainUpdate(backend, cfg)
	input := json.RawMessage(`{"path":"notes/design.md","operation":"append","content":"## Appendix\n\nExtra notes."}`)

	result, err := tool.Execute(context.Background(), projectRoot, input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if _, err := os.Stat(brainindexstate.Path(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("brain index state file stat err = %v, want not-exist for Shunter backend", err)
	}
}

func TestBrainUpdateAppendsOperationLogWithSession(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/design.md": "---\ntags: [architecture]\n---\n# Design\nInitial content",
	})
	tool := NewBrainUpdate(backend, brainConfig(true))

	ctx := ContextWithExecutionMeta(context.Background(), ExecutionMeta{
		ConversationID: "conv-update",
		TurnNumber:     1,
		Iteration:      1,
	})
	input := json.RawMessage(`{"path":"notes/design.md","operation":"append","content":"More notes"}`)
	result, err := tool.Execute(ctx, "/tmp", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}

	logDoc := backend.docs["_log.md"]
	if !strings.Contains(logDoc, "Session: conv-update") {
		t.Fatalf("expected session log entry, got:\n%s", logDoc)
	}
}

func TestBrainUpdateAppendsOperationLog(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/design.md": "---\ntags: [architecture]\n---\n# Design\nInitial content",
	})
	tool := NewBrainUpdate(backend, brainConfig(true))

	input := json.RawMessage(`{"path":"notes/design.md","operation":"append","content":"More notes"}`)
	result, err := tool.Execute(context.Background(), "/tmp", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}

	logDoc := backend.docs["_log.md"]
	if !strings.Contains(logDoc, "update | notes/design.md") {
		t.Fatalf("expected update log entry, got:\n%s", logDoc)
	}
}

func TestBrainUpdateDisabled(t *testing.T) {
	tool := NewBrainUpdate(nil, brainConfig(false))
	result, err := tool.Execute(context.Background(), "/tmp",
		json.RawMessage(`{"path":"test.md","operation":"append","content":"hello"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for disabled brain")
	}
}

func TestBrainUpdateInvalidOperation(t *testing.T) {
	tool := NewBrainUpdate(nil, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp",
		json.RawMessage(`{"path":"test.md","operation":"delete","content":"hello"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for invalid operation")
	}
	if !strings.Contains(result.Content, "Invalid operation") {
		t.Fatalf("content = %q, want invalid operation message", result.Content)
	}
}

func TestBrainUpdateAppend(t *testing.T) {
	docs := map[string]string{
		"notes/journal.md": "---\ntags: [debug]\n---\n# Journal\n\nFirst entry.",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainUpdate(backend, brainConfig(true))
	input, _ := json.Marshal(brainUpdateInput{
		Path:      "notes/journal.md",
		Operation: "append",
		Content:   "## Second Entry\nMore notes here.",
	})
	result, err := tool.Execute(context.Background(), "/tmp", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "Updated brain document: notes/journal.md (append)") {
		t.Fatalf("content = %q, want update success message", result.Content)
	}
	if !strings.Contains(result.Content, "Derived brain metadata and semantic chunks are now stale") {
		t.Fatalf("content = %q, want explicit stale-index reminder", result.Content)
	}
	if !strings.Contains(result.Content, "yard brain index") {
		t.Fatalf("content = %q, want explicit reindex command", result.Content)
	}
	if !strings.Contains(result.Content, "Content preview:\n```md\n---\ntags: [debug]\n---\n# Journal\n\nFirst entry.\n\n## Second Entry\nMore notes here.\n```") {
		t.Fatalf("content preview missing from result: %q", result.Content)
	}
	if !strings.Contains(docs["notes/journal.md"], "First entry.") {
		t.Fatal("original content missing after append")
	}
	if !strings.Contains(docs["notes/journal.md"], "Second Entry") {
		t.Fatal("appended content missing")
	}
}

func TestBrainUpdatePrependWithFrontmatter(t *testing.T) {
	docs := map[string]string{
		"notes/journal.md": "---\ntags: [debug]\n---\n# Journal\n\nExisting content.",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainUpdate(backend, brainConfig(true))
	input, _ := json.Marshal(brainUpdateInput{
		Path:      "notes/journal.md",
		Operation: "prepend",
		Content:   "## Prepended Section\nThis goes after frontmatter.",
	})
	result, err := tool.Execute(context.Background(), "/tmp", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	updated := docs["notes/journal.md"]
	// Frontmatter should come first.
	if !strings.HasPrefix(updated, "---") {
		t.Fatalf("frontmatter should be first: %q", updated[:50])
	}
	// Prepended content should come before original content.
	prependIdx := strings.Index(updated, "Prepended Section")
	originalIdx := strings.Index(updated, "Existing content")
	if prependIdx < 0 || originalIdx < 0 || prependIdx >= originalIdx {
		t.Fatalf("prepend should come before original content. prepend=%d, original=%d\ncontent:\n%s",
			prependIdx, originalIdx, updated)
	}
}

func TestBrainUpdatePrependNoFrontmatter(t *testing.T) {
	docs := map[string]string{
		"notes/plain.md": "# Plain\n\nJust content.",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainUpdate(backend, brainConfig(true))
	input, _ := json.Marshal(brainUpdateInput{
		Path:      "notes/plain.md",
		Operation: "prepend",
		Content:   "## Notice\nThis is prepended.",
	})
	result, err := tool.Execute(context.Background(), "/tmp", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	updated := docs["notes/plain.md"]
	if !strings.HasPrefix(updated, "## Notice") {
		t.Fatalf("prepended content should be at start: %q", updated[:50])
	}
}

func TestBrainUpdateReplaceSection(t *testing.T) {
	docs := map[string]string{
		"arch/design.md": "---\ntags: [arch]\n---\n# Design\n\nOverview text.\n\n## Problem\n\nOld problem description.\n\n### Sub Problem\n\nSub problem text.\n\n## Solution\n\nSolution text.\n",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainUpdate(backend, brainConfig(true))
	input, _ := json.Marshal(brainUpdateInput{
		Path:      "arch/design.md",
		Operation: "replace_section",
		Section:   "## Problem",
		Content:   "\nNew problem description.\n",
	})
	result, err := tool.Execute(context.Background(), "/tmp", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	updated := docs["arch/design.md"]
	if !strings.Contains(updated, "New problem description") {
		t.Fatalf("replacement content missing: %s", updated)
	}
	if strings.Contains(updated, "Old problem description") {
		t.Fatalf("old content should be replaced: %s", updated)
	}
	if strings.Contains(updated, "Sub Problem") {
		t.Fatalf("sub-heading should be replaced: %s", updated)
	}
	if !strings.Contains(updated, "## Solution") {
		t.Fatalf("next section should be preserved: %s", updated)
	}
}

func TestBrainUpdateReplaceSectionNotFound(t *testing.T) {
	docs := map[string]string{
		"arch/design.md": "# Design\n\n## Overview\n\nText.\n\n## Conclusion\n\nEnd.\n",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainUpdate(backend, brainConfig(true))
	input, _ := json.Marshal(brainUpdateInput{
		Path:      "arch/design.md",
		Operation: "replace_section",
		Section:   "## Workaround",
		Content:   "New content.",
	})
	result, err := tool.Execute(context.Background(), "/tmp", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for missing section")
	}
	want := "Section not found in arch/design.md: ## Workaround\n\nAvailable headings:\n- # Design\n- ## Overview\n- ## Conclusion"
	if got := result.Content; got != want {
		t.Fatalf("content = %q\nwant    %q", got, want)
	}
}

func TestBrainUpdateReplaceSectionMissingSectionParam(t *testing.T) {
	docs := map[string]string{
		"notes/a.md": "# Title\n\nContent.\n",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainUpdate(backend, brainConfig(true))
	input, _ := json.Marshal(brainUpdateInput{
		Path:      "notes/a.md",
		Operation: "replace_section",
		Content:   "New content.",
	})
	result, err := tool.Execute(context.Background(), "/tmp", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for missing section param")
	}
	if !strings.Contains(result.Content, "'section' parameter is required") {
		t.Fatalf("content = %q, want section parameter message", result.Content)
	}
}

func TestBrainUpdateDocumentNotFound(t *testing.T) {
	docs := map[string]string{
		"notes/existing.md": "content",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainUpdate(backend, brainConfig(true))
	input, _ := json.Marshal(brainUpdateInput{
		Path:      "notes/missing.md",
		Operation: "append",
		Content:   "New content.",
	})
	result, err := tool.Execute(context.Background(), "/tmp", input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for missing document")
	}
	if !strings.Contains(result.Content, "Document not found") {
		t.Fatalf("content = %q, want not found message", result.Content)
	}
}

func TestBrainUpdateRejectsDeniedPathPolicy(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"receipts/private/run.md": "# Receipt\n\nOriginal.",
	})
	cfg := brainConfig(true)
	cfg.BrainWritePaths = []string{"receipts/**"}
	cfg.BrainDenyPaths = []string{"receipts/private/**"}
	tool := NewBrainUpdate(backend, cfg)

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":"receipts/private/run.md","operation":"append","content":"More"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Fatal("expected Success=false for denied path")
	}
	if !strings.Contains(result.Content, "denied by policy") {
		t.Fatalf("content = %q, want denied-by-policy message", result.Content)
	}
}

func TestBrainUpdateUnrestrictedWhenAllowlistEmpty(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/design.md": "# Design\n\nOriginal.",
	})
	tool := NewBrainUpdate(backend, brainConfig(true))

	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":"notes/design.md","operation":"append","content":"More"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
	}
	if !strings.Contains(backend.docs["notes/design.md"], "More") {
		t.Fatalf("updated doc = %q, want appended content", backend.docs["notes/design.md"])
	}
}

// ── Schema validation ───────────────────────────────────────────────

func TestBrainToolSchemas(t *testing.T) {
	cfg := brainConfig(true)
	tools := []Tool{
		NewBrainSearch(nil, cfg),
		NewBrainRead(nil, cfg),
		NewBrainWrite(nil, cfg),
		NewBrainUpdate(nil, cfg),
		NewBrainLint(nil, cfg),
	}

	for _, tool := range tools {
		t.Run(tool.Name(), func(t *testing.T) {
			schema := tool.Schema()
			var parsed map[string]interface{}
			if err := json.Unmarshal(schema, &parsed); err != nil {
				t.Fatalf("Schema() is not valid JSON: %v", err)
			}
			if parsed["name"] != tool.Name() {
				t.Fatalf("schema name = %q, want %q", parsed["name"], tool.Name())
			}
			if _, ok := parsed["input_schema"]; !ok {
				t.Fatal("schema missing input_schema")
			}
		})
	}
}

// ── Registration ────────────────────────────────────────────────────

func TestRegisterBrainToolsOmitsToolsWhenBrainDisabled(t *testing.T) {
	reg := NewRegistry()
	RegisterBrainTools(reg, nil, brainConfig(false))
	if len(reg.All()) != 0 {
		t.Fatalf("tool count = %d, want 0 when brain disabled", len(reg.All()))
	}
}

func TestRegisterBrainToolsNilClient(t *testing.T) {
	// Should not panic even with nil client.
	reg := NewRegistry()
	RegisterBrainTools(reg, nil, brainConfig(true))
	if len(reg.All()) != 5 {
		t.Fatalf("tool count = %d, want 5", len(reg.All()))
	}
}

// ── Integration test ────────────────────────────────────────────────

func TestBrainToolsIntegrationLifecycle(t *testing.T) {
	docs := map[string]string{}
	backend := newFakeBackend(docs)

	cfg := brainConfig(true)

	writeT := NewBrainWrite(backend, cfg)
	readT := NewBrainRead(backend, cfg)
	searchT := NewBrainSearch(backend, cfg)
	updateT := NewBrainUpdate(backend, cfg)

	ctx := context.Background()

	// 1. Write a document.
	writeInput, _ := json.Marshal(brainWriteInput{
		Path:    "decisions/error-handling.md",
		Content: "---\ntags: [architecture, error-handling]\nstatus: active\n---\n# Error Handling Strategy\n\n## Overview\n\nTool errors are not Go errors, they are ToolResult values.\n\n## Workaround\n\nOld workaround text.\n\n## Impact\n\nMinimal.\n",
	})
	result, err := writeT.Execute(ctx, "/tmp", writeInput)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !result.Success {
		t.Fatalf("write failed: %s", result.Content)
	}

	// 2. Read it back.
	readInput := json.RawMessage(`{"path":"decisions/error-handling.md"}`)
	result, err = readT.Execute(ctx, "/tmp", readInput)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !result.Success {
		t.Fatalf("read failed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Error Handling Strategy") {
		t.Fatalf("read content missing title: %s", result.Content)
	}
	if !strings.Contains(result.Content, "tags: [architecture, error-handling]") {
		t.Fatalf("read content missing frontmatter: %s", result.Content)
	}

	// 3. Search for it.
	searchInput := json.RawMessage(`{"query":"error handling"}`)
	result, err = searchT.Execute(ctx, "/tmp", searchInput)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !result.Success {
		t.Fatalf("search failed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "decisions/error-handling.md") {
		t.Fatalf("search didn't find document: %s", result.Content)
	}

	// 4. Update — replace the Workaround section.
	updateInput, _ := json.Marshal(brainUpdateInput{
		Path:      "decisions/error-handling.md",
		Operation: "replace_section",
		Section:   "## Workaround",
		Content:   "\nNew workaround: use ToolResult.Error field instead of returning Go errors.\n",
	})
	result, err = updateT.Execute(ctx, "/tmp", updateInput)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !result.Success {
		t.Fatalf("update failed: %s", result.Content)
	}

	// 5. Read again to verify the update.
	result, err = readT.Execute(ctx, "/tmp", readInput)
	if err != nil {
		t.Fatalf("read after update: %v", err)
	}
	if !result.Success {
		t.Fatalf("read after update failed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "New workaround") {
		t.Fatalf("updated content missing: %s", result.Content)
	}
	if strings.Contains(result.Content, "Old workaround") {
		t.Fatalf("old content should be replaced: %s", result.Content)
	}
	if !strings.Contains(result.Content, "## Impact") {
		t.Fatalf("next section should be preserved: %s", result.Content)
	}
}

// ── wikilink extraction unit tests ──────────────────────────────────

func TestBrainWriteUpdateAndLintProduceLogHistory(t *testing.T) {
	backend := newFakeBackend(map[string]string{})

	writeTool := NewBrainWrite(backend, brainConfig(true))
	_, err := writeTool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":"notes/design.md","content":"---\nupdated_at: 2026-04-01T10:00:00Z\ntags: [architecture]\n---\n# Design\n[[notes/missing]]"}`))
	if err != nil {
		t.Fatalf("write Execute returned error: %v", err)
	}

	updateTool := NewBrainUpdate(backend, brainConfig(true))
	_, err = updateTool.Execute(context.Background(), "/tmp", json.RawMessage(`{"path":"notes/design.md","operation":"append","content":"More details"}`))
	if err != nil {
		t.Fatalf("update Execute returned error: %v", err)
	}

	lintTool := NewBrainLint(backend, brainConfig(true))
	_, err = lintTool.Execute(context.Background(), "/tmp", json.RawMessage(`{"scope":"full"}`))
	if err != nil {
		t.Fatalf("lint Execute returned error: %v", err)
	}

	logDoc := backend.docs["_log.md"]
	for _, needle := range []string{"write | notes/design.md", "update | notes/design.md", "lint | full"} {
		if !strings.Contains(logDoc, needle) {
			t.Fatalf("expected %q in log doc:\n%s", needle, logDoc)
		}
	}
}

func TestExtractWikilinks(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{"basic", "See [[design]] and [[auth]].", []string{"design", "auth"}},
		{"with display text", "See [[design|Design Doc]].", []string{"design"}},
		{"duplicates", "[[a]] and [[a]] again.", []string{"a"}},
		{"none", "No links here.", nil},
		{"nested in context", "text\n- [[link1]]\n- [[link2]]\n", []string{"link1", "link2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractWikilinks(tt.content)
			if len(got) != len(tt.want) {
				t.Fatalf("extractWikilinks() = %v, want %v", got, tt.want)
			}
			for i, link := range got {
				if link != tt.want[i] {
					t.Fatalf("link[%d] = %q, want %q", i, link, tt.want[i])
				}
			}
		})
	}
}

// ── frontmatter extraction unit tests ───────────────────────────────

func TestExtractFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantFM   string
		wantBody string
	}{
		{
			"with frontmatter",
			"---\ntags: [test]\nstatus: active\n---\n# Title\nBody.",
			"tags: [test]\nstatus: active",
			"# Title\nBody.",
		},
		{
			"no frontmatter",
			"# Title\nBody.",
			"",
			"# Title\nBody.",
		},
		{
			"unclosed frontmatter",
			"---\ntags: [test]\n# Title",
			"",
			"---\ntags: [test]\n# Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body := extractFrontmatter(tt.content)
			if fm != tt.wantFM {
				t.Fatalf("frontmatter = %q, want %q", fm, tt.wantFM)
			}
			if body != tt.wantBody {
				t.Fatalf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

// ── test helpers for fake PatchDocument ────────────────────────────

func appendContent(current, addition string) string {
	if !strings.HasSuffix(current, "\n") {
		current += "\n"
	}
	return current + "\n" + addition
}

func prependContent(current, addition string) string {
	if !strings.HasPrefix(current, "---") {
		return addition + "\n\n" + current
	}
	rest := current[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return addition + "\n\n" + current
	}
	fmEnd := 3 + idx + 4
	fm := current[:fmEnd]
	body := strings.TrimLeft(current[fmEnd:], "\n")
	return fm + "\n\n" + addition + "\n\n" + body
}

func replaceSectionContent(current, section, newContent string) (string, error) {
	lines := strings.Split(current, "\n")
	targetLevel, targetText := parseHeading(section)
	if targetLevel == 0 {
		targetText = section
	}
	targetIdx := -1
	for i, line := range lines {
		level, text := parseHeading(line)
		if level == 0 {
			continue
		}
		if targetLevel > 0 && level == targetLevel && strings.TrimSpace(text) == strings.TrimSpace(targetText) {
			targetIdx = i
			break
		}
		if targetLevel == 0 && strings.TrimSpace(text) == strings.TrimSpace(targetText) {
			targetIdx = i
			targetLevel = level
			break
		}
	}
	if targetIdx < 0 {
		headings := listHeadings(lines)
		if len(headings) > 0 {
			return "", fmt.Errorf("Section '%s' not found. Available headings: %s",
				section, strings.Join(headings, ", "))
		}
		return "", fmt.Errorf("Section '%s' not found. The document has no headings.", section)
	}
	endIdx := len(lines)
	for i := targetIdx + 1; i < len(lines); i++ {
		level, _ := parseHeading(lines[i])
		if level > 0 && level <= targetLevel {
			endIdx = i
			break
		}
	}
	var parts []string
	parts = append(parts, lines[:targetIdx+1]...)
	parts = append(parts, newContent)
	if endIdx < len(lines) {
		parts = append(parts, lines[endIdx:]...)
	}
	return strings.Join(parts, "\n"), nil
}

func parseHeading(line string) (int, string) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return 0, ""
	}
	level := 0
	for _, ch := range trimmed {
		if ch == '#' {
			level++
		} else {
			break
		}
	}
	if level > 6 || level == 0 {
		return 0, ""
	}
	return level, strings.TrimSpace(trimmed[level:])
}

func listHeadings(lines []string) []string {
	var headings []string
	for _, line := range lines {
		level, text := parseHeading(line)
		if level > 0 {
			headings = append(headings, strings.Repeat("#", level)+" "+text)
		}
	}
	return headings
}

// ── section replacement unit tests ──────────────────────────────────

func TestReplaceSectionContent(t *testing.T) {
	doc := "# Title\n\n## First\n\nFirst content.\n\n### Sub\n\nSub content.\n\n## Second\n\nSecond content.\n"

	// Replace First section (should include Sub heading).
	updated, err := replaceSectionContent(doc, "## First", "\nReplaced content.\n")
	if err != nil {
		t.Fatalf("replaceSectionContent returned error: %v", err)
	}
	if !strings.Contains(updated, "Replaced content") {
		t.Fatalf("replacement missing: %s", updated)
	}
	if strings.Contains(updated, "First content") {
		t.Fatalf("old content should be gone: %s", updated)
	}
	if strings.Contains(updated, "Sub content") {
		t.Fatalf("sub section should be replaced: %s", updated)
	}
	if !strings.Contains(updated, "## Second") {
		t.Fatalf("next section should be preserved: %s", updated)
	}
}
