package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ponchione/sirtopham/internal/brain"
	"github.com/ponchione/sirtopham/internal/brain/parser"
	"github.com/ponchione/sirtopham/internal/db"
)

type Indexer struct {
	db      *sql.DB
	backend brain.Backend
	now     func() time.Time
}

type Result struct {
	DocumentsIndexed         int
	LinksIndexed             int
	DocumentsDeleted         int
	SemanticChunksIndexed    int
	SemanticDocumentsDeleted int
}

func New(database *sql.DB, backend brain.Backend) *Indexer {
	return &Indexer{
		db:      database,
		backend: backend,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (i *Indexer) RebuildProject(ctx context.Context, projectID string) (Result, error) {
	if i == nil || i.db == nil {
		return Result{}, fmt.Errorf("indexer database is required")
	}
	if i.backend == nil {
		return Result{}, fmt.Errorf("indexer backend is required")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Result{}, fmt.Errorf("project id is required")
	}

	paths, err := i.backend.ListDocuments(ctx, "")
	if err != nil {
		return Result{}, fmt.Errorf("list brain documents: %w", err)
	}
	sort.Strings(paths)

	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, fmt.Errorf("begin brain index transaction: %w", err)
	}
	defer tx.Rollback()

	queries := db.New(tx)
	existingDocs, err := queries.ListBrainDocumentsByProject(ctx, projectID)
	if err != nil {
		return Result{}, fmt.Errorf("list existing brain documents: %w", err)
	}
	existingByPath := make(map[string]db.BrainDocument, len(existingDocs))
	for _, doc := range existingDocs {
		existingByPath[doc.Path] = doc
	}

	seen := make(map[string]struct{}, len(paths))
	result := Result{}
	now := i.now().Format(time.RFC3339)
	for _, docPath := range paths {
		if isOperationalBrainDocument(docPath) {
			continue
		}
		content, err := i.backend.ReadDocument(ctx, docPath)
		if err != nil {
			return Result{}, fmt.Errorf("read brain document %s: %w", docPath, err)
		}
		doc, err := parser.ParseDocument(docPath, content)
		if err != nil {
			return Result{}, fmt.Errorf("parse brain document %s: %w", docPath, err)
		}
		if err := upsertDocument(ctx, queries, projectID, doc, now, existingByPath[doc.Path]); err != nil {
			return Result{}, err
		}
		if err := queries.DeleteBrainLinksForSource(ctx, db.DeleteBrainLinksForSourceParams{
			ProjectID:  projectID,
			SourcePath: doc.Path,
		}); err != nil {
			return Result{}, fmt.Errorf("delete brain links for %s: %w", doc.Path, err)
		}
		for _, link := range doc.Wikilinks {
			if err := queries.InsertBrainLink(ctx, db.InsertBrainLinkParams{
				ProjectID:  projectID,
				SourcePath: doc.Path,
				TargetPath: link.Target,
				LinkText:   nullableString(link.Display),
			}); err != nil {
				return Result{}, fmt.Errorf("insert brain link %s -> %s: %w", doc.Path, link.Target, err)
			}
			result.LinksIndexed++
		}
		seen[doc.Path] = struct{}{}
		result.DocumentsIndexed++
	}

	for _, existing := range existingDocs {
		if _, ok := seen[existing.Path]; ok || isOperationalBrainDocument(existing.Path) {
			continue
		}
		if err := queries.DeleteBrainLinksForSource(ctx, db.DeleteBrainLinksForSourceParams{
			ProjectID:  projectID,
			SourcePath: existing.Path,
		}); err != nil {
			return Result{}, fmt.Errorf("delete stale brain links for %s: %w", existing.Path, err)
		}
		if err := queries.DeleteBrainDocumentByPath(ctx, db.DeleteBrainDocumentByPathParams{
			ProjectID: projectID,
			Path:      existing.Path,
		}); err != nil {
			return Result{}, fmt.Errorf("delete stale brain document %s: %w", existing.Path, err)
		}
		result.DocumentsDeleted++
	}

	if err := tx.Commit(); err != nil {
		return Result{}, fmt.Errorf("commit brain index transaction: %w", err)
	}
	return result, nil
}

func upsertDocument(ctx context.Context, queries *db.Queries, projectID string, doc parser.Document, now string, existing db.BrainDocument) error {
	frontmatterJSON, err := json.Marshal(doc.Frontmatter)
	if err != nil {
		return fmt.Errorf("marshal frontmatter for %s: %w", doc.Path, err)
	}
	tagsJSON, err := json.Marshal(doc.Tags)
	if err != nil {
		return fmt.Errorf("marshal tags for %s: %w", doc.Path, err)
	}
	createdAt := now
	if existing.ID != 0 {
		createdAt = existing.CreatedAt
	}
	if err := queries.UpsertBrainDocument(ctx, db.UpsertBrainDocumentParams{
		ProjectID:   projectID,
		Path:        doc.Path,
		Title:       nullableString(doc.Title),
		ContentHash: doc.ContentHash,
		Tags:        nullableJSON(tagsJSON),
		Frontmatter: nullableJSON(frontmatterJSON),
		TokenCount:  sql.NullInt64{Int64: int64(doc.TokenCount), Valid: true},
		CreatedAt:   createdAt,
		UpdatedAt:   now,
	}); err != nil {
		return fmt.Errorf("upsert brain document %s: %w", doc.Path, err)
	}
	return nil
}

func nullableString(s string) sql.NullString {
	s = strings.TrimSpace(s)
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullableJSON(data []byte) sql.NullString {
	if len(data) == 0 || string(data) == "null" {
		return sql.NullString{}
	}
	return sql.NullString{String: string(data), Valid: true}
}

func isOperationalBrainDocument(docPath string) bool {
	cleaned := strings.Trim(filepath.ToSlash(strings.TrimSpace(docPath)), "/")
	if cleaned == "" {
		return false
	}
	return filepath.Base(cleaned) == "_log.md"
}
