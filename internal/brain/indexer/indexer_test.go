//go:build sqlite_fts5
// +build sqlite_fts5

package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ponchione/sirtopham/internal/brain"
	"github.com/ponchione/sirtopham/internal/db"
	sid "github.com/ponchione/sirtopham/internal/id"
)

type fakeBackend struct {
	docs map[string]string
}

func (f *fakeBackend) ReadDocument(ctx context.Context, path string) (string, error) {
	_ = ctx
	content, ok := f.docs[path]
	if !ok {
		return "", errors.New("not found")
	}
	return content, nil
}

func (f *fakeBackend) WriteDocument(ctx context.Context, path string, content string) error {
	panic("unexpected WriteDocument call")
}

func (f *fakeBackend) PatchDocument(ctx context.Context, path string, operation string, content string) error {
	panic("unexpected PatchDocument call")
}

func (f *fakeBackend) SearchKeyword(ctx context.Context, query string) ([]brain.SearchHit, error) {
	panic("unexpected SearchKeyword call")
}

func (f *fakeBackend) ListDocuments(ctx context.Context, directory string) ([]string, error) {
	_ = ctx
	_ = directory
	paths := make([]string, 0, len(f.docs))
	for path := range f.docs {
		paths = append(paths, path)
	}
	return paths, nil
}

func TestIndexerRebuildProjectIndexesDocsLinksAndSkipsOperationalLog(t *testing.T) {
	ctx := context.Background()
	sqlDB, queries, projectID := newIndexerTestDB(t)
	backend := &fakeBackend{docs: map[string]string{
		"notes/alpha.md": "---\ntags: [brain, alpha]\nstatus: draft\n---\n# Alpha\n\nAlpha body with [[notes/beta|Beta Note]].\n",
		"notes/beta.md":  "# Beta\n\n#beta\n",
		"_log.md":        "# Log\n\nshould stay operational\n",
	}}
	idx := New(sqlDB, backend)
	idx.now = func() time.Time { return time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC) }

	result, err := idx.RebuildProject(ctx, projectID)
	if err != nil {
		t.Fatalf("RebuildProject returned error: %v", err)
	}
	if result.DocumentsIndexed != 2 || result.LinksIndexed != 1 || result.DocumentsDeleted != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	docs, err := queries.ListBrainDocumentsByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("ListBrainDocumentsByProject returned error: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("brain document count = %d, want 2", len(docs))
	}
	if docs[0].Path != "notes/alpha.md" || docs[1].Path != "notes/beta.md" {
		t.Fatalf("unexpected indexed document paths: %+v", docs)
	}
	if docs[0].Title.String != "Alpha" {
		t.Fatalf("alpha title = %q, want Alpha", docs[0].Title.String)
	}
	if !docs[0].Tags.Valid {
		t.Fatal("expected alpha tags to be persisted")
	}
	var tags []string
	if err := json.Unmarshal([]byte(docs[0].Tags.String), &tags); err != nil {
		t.Fatalf("unmarshal tags: %v", err)
	}
	if !reflect.DeepEqual(tags, []string{"alpha", "brain"}) {
		t.Fatalf("tags = %#v, want [alpha brain]", tags)
	}
	if !docs[0].Frontmatter.Valid {
		t.Fatal("expected alpha frontmatter to be persisted")
	}
	var frontmatter map[string]any
	if err := json.Unmarshal([]byte(docs[0].Frontmatter.String), &frontmatter); err != nil {
		t.Fatalf("unmarshal frontmatter: %v", err)
	}
	if got := frontmatter["status"]; got != "draft" {
		t.Fatalf("frontmatter status = %#v, want draft", got)
	}

	linkRows := queryLinks(t, sqlDB, projectID)
	wantLinks := []brainLinkRow{{SourcePath: "notes/alpha.md", TargetPath: "notes/beta", LinkText: sql.NullString{String: "Beta Note", Valid: true}}}
	if !reflect.DeepEqual(linkRows, wantLinks) {
		t.Fatalf("links = %#v, want %#v", linkRows, wantLinks)
	}
}

func TestIndexerRebuildProjectRewritesLinksAndDeletesMissingDocuments(t *testing.T) {
	ctx := context.Background()
	sqlDB, queries, projectID := newIndexerTestDB(t)
	backend := &fakeBackend{docs: map[string]string{
		"notes/alpha.md": "# Alpha\n\nSee [[notes/beta]].\n",
		"notes/beta.md":  "# Beta\n",
	}}
	idx := New(sqlDB, backend)
	idx.now = func() time.Time { return time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC) }

	if _, err := idx.RebuildProject(ctx, projectID); err != nil {
		t.Fatalf("initial RebuildProject returned error: %v", err)
	}
	firstDoc, err := queries.GetBrainDocumentByPath(ctx, db.GetBrainDocumentByPathParams{ProjectID: projectID, Path: "notes/alpha.md"})
	if err != nil {
		t.Fatalf("GetBrainDocumentByPath after first rebuild: %v", err)
	}

	backend.docs = map[string]string{
		"notes/alpha.md": "# Alpha\n\nNow see [[notes/gamma|Gamma]].\n",
		"notes/gamma.md": "# Gamma\n",
	}
	idx.now = func() time.Time { return time.Date(2026, 4, 9, 13, 0, 0, 0, time.UTC) }

	result, err := idx.RebuildProject(ctx, projectID)
	if err != nil {
		t.Fatalf("second RebuildProject returned error: %v", err)
	}
	if result.DocumentsIndexed != 2 || result.LinksIndexed != 1 || result.DocumentsDeleted != 1 {
		t.Fatalf("unexpected second result: %+v", result)
	}

	_, err = queries.GetBrainDocumentByPath(ctx, db.GetBrainDocumentByPathParams{ProjectID: projectID, Path: "notes/beta.md"})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetBrainDocumentByPath beta err = %v, want sql.ErrNoRows", err)
	}
	updatedAlpha, err := queries.GetBrainDocumentByPath(ctx, db.GetBrainDocumentByPathParams{ProjectID: projectID, Path: "notes/alpha.md"})
	if err != nil {
		t.Fatalf("GetBrainDocumentByPath alpha after second rebuild: %v", err)
	}
	if updatedAlpha.CreatedAt != firstDoc.CreatedAt {
		t.Fatalf("created_at changed across rebuilds: got %q want %q", updatedAlpha.CreatedAt, firstDoc.CreatedAt)
	}
	if updatedAlpha.UpdatedAt == firstDoc.UpdatedAt {
		t.Fatalf("expected updated_at to change across rebuilds: first=%q second=%q", firstDoc.UpdatedAt, updatedAlpha.UpdatedAt)
	}

	docs, err := queries.ListBrainDocumentsByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("ListBrainDocumentsByProject returned error: %v", err)
	}
	if len(docs) != 2 || docs[0].Path != "notes/alpha.md" || docs[1].Path != "notes/gamma.md" {
		t.Fatalf("unexpected final docs: %+v", docs)
	}

	linkRows := queryLinks(t, sqlDB, projectID)
	wantLinks := []brainLinkRow{{SourcePath: "notes/alpha.md", TargetPath: "notes/gamma", LinkText: sql.NullString{String: "Gamma", Valid: true}}}
	if !reflect.DeepEqual(linkRows, wantLinks) {
		t.Fatalf("links after rewrite = %#v, want %#v", linkRows, wantLinks)
	}
}

type brainLinkRow struct {
	SourcePath string
	TargetPath string
	LinkText   sql.NullString
}

func queryLinks(t *testing.T, sqlDB *sql.DB, projectID string) []brainLinkRow {
	t.Helper()
	rows, err := sqlDB.Query(`SELECT source_path, target_path, link_text FROM brain_links WHERE project_id = ? ORDER BY source_path, target_path`, projectID)
	if err != nil {
		t.Fatalf("query brain_links: %v", err)
	}
	defer rows.Close()
	var out []brainLinkRow
	for rows.Next() {
		var row brainLinkRow
		if err := rows.Scan(&row.SourcePath, &row.TargetPath, &row.LinkText); err != nil {
			t.Fatalf("scan brain_links row: %v", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate brain_links rows: %v", err)
	}
	return out
}

func newIndexerTestDB(t *testing.T) (*sql.DB, *db.Queries, string) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "brain-index.db")
	sqlDB, err := db.OpenDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenDB returned error: %v", err)
	}
	if err := db.Init(ctx, sqlDB); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	projectID := sid.New()
	createdAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := sqlDB.Exec(`INSERT INTO projects(id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, projectID, "proj", "/tmp/proj", createdAt, createdAt); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return sqlDB, db.New(sqlDB), projectID
}
