package analysis

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sirtopham/internal/brain"
)

type fakeBackend struct {
	docs map[string]string
}

func newFakeBackend(docs map[string]string) *fakeBackend {
	return &fakeBackend{docs: docs}
}

func (f *fakeBackend) ReadDocument(ctx context.Context, path string) (string, error) {
	content, ok := f.docs[path]
	if !ok {
		return "", fmt.Errorf("Document not found: %s", path)
	}
	return content, nil
}

func (f *fakeBackend) WriteDocument(ctx context.Context, path string, content string) error {
	if f.docs == nil {
		f.docs = map[string]string{}
	}
	f.docs[path] = content
	return nil
}

func (f *fakeBackend) PatchDocument(ctx context.Context, path string, operation string, content string) error {
	return nil
}

func (f *fakeBackend) SearchKeyword(ctx context.Context, query string) ([]brain.SearchHit, error) {
	return nil, nil
}

func (f *fakeBackend) ListDocuments(ctx context.Context, directory string) ([]string, error) {
	var out []string
	for docPath := range f.docs {
		if directory == "" || strings.HasPrefix(docPath, directory) {
			out = append(out, docPath)
		}
	}
	return out, nil
}

func TestParseDocumentExtractsWikilinksTagsAndUpdatedAt(t *testing.T) {
	content := `---
updated_at: 2026-04-01T10:00:00Z
tags: [architecture, brain]
---
# Brain Plan
Links to [[notes/design]] and [[debugging/auth-race]].
Inline #implementation tag.`

	doc, err := ParseDocument("notes/plan.md", content)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if !doc.HasUpdatedAt {
		t.Fatal("expected updated_at to be parsed")
	}
	if len(doc.Wikilinks) != 2 {
		t.Fatalf("wikilinks = %v, want 2 entries", doc.Wikilinks)
	}
	if len(doc.Tags) < 3 {
		t.Fatalf("tags = %v, want frontmatter + inline tag", doc.Tags)
	}
}

func TestParseDocumentDedupesFlattenedWikilinksByTarget(t *testing.T) {
	content := `# Brain Plan
See [[notes/design]], [[notes/design|Design Alias]], and [[notes/design#section|Section Alias]].`

	doc, err := ParseDocument("notes/plan.md", content)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if got := doc.Wikilinks; len(got) != 1 || got[0] != "notes/design" {
		t.Fatalf("Wikilinks = %v, want deduped [notes/design]", got)
	}
}

func TestLoadDocumentsFullScope(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md":        "# A",
		"architecture/b.md": "# B",
	})

	docs, err := LoadDocuments(context.Background(), backend, "full")
	if err != nil {
		t.Fatalf("LoadDocuments: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("len(docs) = %d, want 2", len(docs))
	}
}

func TestLoadDocumentsTagScopeFiltersResults(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md": `---
tags: [architecture]
---
# A`,
		"notes/b.md": `---
tags: [debugging]
---
# B`,
	})

	docs, err := LoadDocuments(context.Background(), backend, "#architecture")
	if err != nil {
		t.Fatalf("LoadDocuments: %v", err)
	}
	if len(docs) != 1 || docs[0].Path != "notes/a.md" {
		t.Fatalf("unexpected tag-scoped docs: %+v", docs)
	}
}

func TestLoadDocumentsPathScopeFiltersResults(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md":        "# A",
		"architecture/b.md": "# B",
	})

	docs, err := LoadDocuments(context.Background(), backend, "notes/")
	if err != nil {
		t.Fatalf("LoadDocuments: %v", err)
	}
	if len(docs) != 1 || docs[0].Path != "notes/a.md" {
		t.Fatalf("unexpected scoped docs: %+v", docs)
	}
}

func TestLoadDocumentsCombinedScopeFiltersResults(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md": `---
tags: [architecture]
---
# A`,
		"notes/b.md": `---
tags: [debugging]
---
# B`,
		"architecture/c.md": `---
tags: [architecture]
---
# C`,
	})

	docs, err := LoadDocuments(context.Background(), backend, "notes/+#architecture")
	if err != nil {
		t.Fatalf("LoadDocuments: %v", err)
	}
	if len(docs) != 1 || docs[0].Path != "notes/a.md" {
		t.Fatalf("unexpected combined-scope docs: %+v", docs)
	}
}

func TestLoadDocumentsRejectsInvalidCombinedScope(t *testing.T) {
	backend := newFakeBackend(map[string]string{
		"notes/a.md": "# A",
	})

	_, err := LoadDocuments(context.Background(), backend, "notes/+#architecture+#debugging")
	if err == nil {
		t.Fatal("expected invalid combined scope error")
	}
	if !strings.Contains(err.Error(), "invalid brain lint scope") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunLintScopedGraphChecksUseFullUniverseForCombinedScope(t *testing.T) {
	newer := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
	older := time.Date(2025, 12, 1, 10, 0, 0, 0, time.UTC)

	universe := []Document{
		{Path: "notes/overview.md", Tags: []string{"architecture"}, Wikilinks: []string{"shared/reference"}, UpdatedAt: older, HasUpdatedAt: true},
		{Path: "notes/inbound.md", Wikilinks: []string{"notes/overview"}},
		{Path: "shared/reference.md", UpdatedAt: newer, HasUpdatedAt: true},
	}
	scoped := []Document{
		universe[0],
	}

	report := RunLint(scoped, LintOptions{
		Scope:      "notes/+#architecture",
		Checks:     []string{"orphans", "dead_links", "stale_references"},
		StaleAfter: 90 * 24 * time.Hour,
		Universe:   universe,
	})
	if report.Summary.Orphans != 0 {
		t.Fatalf("orphans = %d, want 0 with full universe", report.Summary.Orphans)
	}
	if report.Summary.DeadLinks != 0 {
		t.Fatalf("dead links = %d, want 0 with full universe", report.Summary.DeadLinks)
	}
	if report.Summary.StaleReferences != 1 {
		t.Fatalf("stale references = %d, want 1 with full universe", report.Summary.StaleReferences)
	}
}

func TestRunLintFindsDeadLinks(t *testing.T) {
	docs := []Document{
		{Path: "notes/a.md", Wikilinks: []string{"notes/missing"}},
		{Path: "notes/b.md"},
	}

	report := RunLint(docs, LintOptions{Checks: []string{"dead_links"}})
	if len(report.Findings.DeadLinks) != 1 {
		t.Fatalf("dead links = %d, want 1", len(report.Findings.DeadLinks))
	}
}

func TestRunLintFindsOrphansExcludingReservedDocs(t *testing.T) {
	docs := []Document{
		{Path: "_log.md"},
		{Path: "_index.md"},
		{Path: "notes/a.md"},
		{Path: "notes/b.md", Wikilinks: []string{"notes/a"}},
	}

	report := RunLint(docs, LintOptions{Checks: []string{"orphans"}})
	if len(report.Findings.Orphans) != 1 || report.Findings.Orphans[0].Path != "notes/b.md" {
		t.Fatalf("unexpected orphan findings: %+v", report.Findings.Orphans)
	}
}

func TestRunLintFindsStaleReferences(t *testing.T) {
	newer := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
	older := time.Date(2025, 12, 1, 10, 0, 0, 0, time.UTC)

	docs := []Document{
		{Path: "notes/summary.md", Wikilinks: []string{"notes/source"}, UpdatedAt: older, HasUpdatedAt: true},
		{Path: "notes/source.md", UpdatedAt: newer, HasUpdatedAt: true},
	}

	report := RunLint(docs, LintOptions{Checks: []string{"stale_references"}, StaleAfter: 90 * 24 * time.Hour})
	if len(report.Findings.StaleReferences) != 1 {
		t.Fatalf("stale refs = %d, want 1", len(report.Findings.StaleReferences))
	}
}

func TestRunLintFindsTagHygieneIssues(t *testing.T) {
	docs := []Document{
		{Path: "notes/a.md", Tags: []string{"architecture"}},
		{Path: "notes/b.md", Tags: []string{"arch"}},
		{Path: "notes/c.md", Tags: nil},
	}

	report := RunLint(docs, LintOptions{Checks: []string{"tag_hygiene"}})
	if len(report.Findings.TagHygiene.UntaggedDocuments) != 1 {
		t.Fatalf("untagged = %d, want 1", len(report.Findings.TagHygiene.UntaggedDocuments))
	}
	if len(report.Findings.TagHygiene.SimilarTagPairs) != 1 {
		t.Fatalf("similar tag pairs = %d, want 1", len(report.Findings.TagHygiene.SimilarTagPairs))
	}
}

func TestRunLintFindsMissingPagesFromRepeatedUnresolvedLinks(t *testing.T) {
	universe := []Document{
		{Path: "notes/a.md", Wikilinks: []string{"concepts/shared-gap", "concepts/single-gap"}},
		{Path: "notes/b.md", Wikilinks: []string{"concepts/shared-gap"}},
		{Path: "notes/c.md"},
	}

	report := RunLint([]Document{universe[0], universe[1]}, LintOptions{Checks: []string{"missing_pages"}, Universe: universe})
	if len(report.Findings.MissingPages) != 1 {
		t.Fatalf("missing pages = %d, want 1", len(report.Findings.MissingPages))
	}
	if got := report.Findings.MissingPages[0]; got.Target != "concepts/shared-gap" || got.Count != 2 {
		t.Fatalf("unexpected missing page finding: %+v", got)
	}
	if report.Summary.MissingPages != 1 {
		t.Fatalf("summary missing pages = %d, want 1", report.Summary.MissingPages)
	}
}

func TestRunLintMissingPagesSuppressesExistingPageAndSingleUseTargets(t *testing.T) {
	universe := []Document{
		{Path: "notes/a.md", Wikilinks: []string{"concepts/shared-gap", "concepts/existing", "concepts/single-gap"}},
		{Path: "notes/b.md", Wikilinks: []string{"concepts/shared-gap", "concepts/existing"}},
		{Path: "concepts/existing.md"},
	}

	report := RunLint([]Document{universe[0], universe[1]}, LintOptions{Checks: []string{"missing_pages"}, Universe: universe})
	if len(report.Findings.MissingPages) != 1 {
		t.Fatalf("missing pages = %d, want 1", len(report.Findings.MissingPages))
	}
	if report.Findings.MissingPages[0].Target != "concepts/shared-gap" {
		t.Fatalf("unexpected missing page findings: %+v", report.Findings.MissingPages)
	}
}

func TestRunLintMissingPagesUsesFullUniverseForScopedSubset(t *testing.T) {
	universe := []Document{
		{Path: "notes/arch-a.md", Tags: []string{"architecture"}, Wikilinks: []string{"concepts/shared-gap"}},
		{Path: "notes/arch-b.md", Tags: []string{"architecture"}, Wikilinks: []string{"concepts/shared-gap"}},
		{Path: "notes/debug.md", Tags: []string{"debugging"}, Wikilinks: []string{"concepts/shared-gap"}},
	}

	report := RunLint([]Document{universe[0]}, LintOptions{Scope: "notes/+#architecture", Checks: []string{"missing_pages"}, Universe: universe})
	if len(report.Findings.MissingPages) != 1 {
		t.Fatalf("missing pages = %d, want 1", len(report.Findings.MissingPages))
	}
	if got := report.Findings.MissingPages[0]; got.Target != "concepts/shared-gap" || got.Count != 3 {
		t.Fatalf("unexpected missing page finding: %+v", got)
	}
}

func TestFindContradictionCandidatesUsesDeterministicRelations(t *testing.T) {
	docs := []Document{
		{Path: "notes/a.md", Tags: []string{"architecture"}},
		{Path: "notes/b.md", Tags: []string{"architecture"}},
		{Path: "notes/c.md", Wikilinks: []string{"notes/d"}},
		{Path: "notes/d.md"},
		{Path: "notes/shared-one.md", Wikilinks: []string{"concepts/auth"}},
		{Path: "notes/shared-two.md", Wikilinks: []string{"concepts/auth"}},
		{Path: "notes/auth-plan.md", Title: "Auth Plan"},
		{Path: "notes/auth-plan-rollout.md", Title: "Auth Plan Rollout"},
		{Path: "notes/e.md", Tags: []string{"debugging"}},
	}

	candidates := FindContradictionCandidates(docs)
	if len(candidates) != 4 {
		t.Fatalf("candidates = %d, want 4", len(candidates))
	}
	got := [][2]string{}
	for _, candidate := range candidates {
		got = append(got, [2]string{candidate.Left.Path, candidate.Right.Path})
	}
	want := [][2]string{
		{"notes/a.md", "notes/b.md"},
		{"notes/auth-plan.md", "notes/auth-plan-rollout.md"},
		{"notes/c.md", "notes/d.md"},
		{"notes/shared-one.md", "notes/shared-two.md"},
	}
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		t.Fatalf("candidate pairs = %v, want %v", got, want)
	}
}
