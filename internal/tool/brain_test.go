package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/ponchione/sirtopham/internal/brain"
	"github.com/ponchione/sirtopham/internal/config"
	"github.com/ponchione/sirtopham/internal/provider"
)

// ── helpers ──────────────────────────────────────────────────────────

func brainConfig(enabled bool) config.BrainConfig {
	return config.BrainConfig{Enabled: enabled}
}

type fakeBackend struct {
	docs     map[string]string
	searches map[string][]brain.SearchHit
	listings map[string][]string
	readErr  error
	writeErr error
	patchErr error
	patchOps []string
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
	for _, want := range []string{"notes/...md", "file_read", "vault-relative"} {
		if !strings.Contains(brainRead.Description, want) {
			t.Fatalf("brain_read description = %q, want substring %q", brainRead.Description, want)
		}
	}

	brainSearch, ok := byName["brain_search"]
	if !ok {
		t.Fatal("brain_search definition missing")
	}
	for _, want := range []string{"notes/...md", "search_text", "brain_read"} {
		if !strings.Contains(brainSearch.Description, want) {
			t.Fatalf("brain_search description = %q, want substring %q", brainSearch.Description, want)
		}
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
	if !strings.Contains(result.Content, "Semantic search is not yet available") {
		t.Fatalf("content missing semantic notice: %q", result.Content)
	}
	// Should still return results via keyword fallback.
	if !strings.Contains(result.Content, "notes/a.md") {
		t.Fatalf("content missing keyword results: %q", result.Content)
	}
}

func TestBrainSearchWithTags(t *testing.T) {
	docs := map[string]string{
		"notes/tagged.md": "# Tagged\nContent with #debugging tag",
	}
	backend := newFakeBackend(docs)
	tool := NewBrainSearch(backend, brainConfig(true))
	result, err := tool.Execute(context.Background(), "/tmp", json.RawMessage(`{"query":"tagged","tags":["debugging"]}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, content = %q", result.Content)
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
		"arch/design.md":    "# Design\nCore design document.",
		"notes/session.md":  "Working on [[design]] today.",
		"notes/review.md":   "Reviewed the design changes.",
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
	if got, want := result.Content, "Wrote brain document: notes/new.md"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	// Verify the doc was actually stored.
	if docs["notes/new.md"] != content {
		t.Fatalf("stored content = %q, want original", docs["notes/new.md"])
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
	want := "Updated brain document: notes/journal.md (append)\n\nContent preview:\n```md\n---\ntags: [debug]\n---\n# Journal\n\nFirst entry.\n\n## Second Entry\nMore notes here.\n```"
	if got := result.Content; got != want {
		t.Fatalf("content = %q\nwant    %q", got, want)
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

// ── Schema validation ───────────────────────────────────────────────

func TestBrainToolSchemas(t *testing.T) {
	cfg := brainConfig(true)
	tools := []Tool{
		NewBrainSearch(nil, cfg),
		NewBrainRead(nil, cfg),
		NewBrainWrite(nil, cfg),
		NewBrainUpdate(nil, cfg),
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

func TestRegisterBrainTools(t *testing.T) {
	reg := NewRegistry()
	RegisterBrainTools(reg, nil, brainConfig(false))
	names := make(map[string]bool)
	for _, tool := range reg.All() {
		names[tool.Name()] = true
	}
	for _, expected := range []string{"brain_search", "brain_read", "brain_write", "brain_update"} {
		if !names[expected] {
			t.Fatalf("missing tool %q in registry", expected)
		}
	}
}

func TestRegisterBrainToolsNilClient(t *testing.T) {
	// Should not panic even with nil client.
	reg := NewRegistry()
	RegisterBrainTools(reg, nil, brainConfig(true))
	if len(reg.All()) != 4 {
		t.Fatalf("tool count = %d, want 4", len(reg.All()))
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
