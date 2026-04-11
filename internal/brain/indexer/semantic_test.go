//go:build sqlite_fts5
// +build sqlite_fts5

package indexer

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/codeintel"
)

type semanticFakeStore struct {
	deleted []string
	upserts [][]codeintel.Chunk
}

func (f *semanticFakeStore) Upsert(_ context.Context, chunks []codeintel.Chunk) error {
	copied := append([]codeintel.Chunk(nil), chunks...)
	f.upserts = append(f.upserts, copied)
	return nil
}

func (f *semanticFakeStore) VectorSearch(context.Context, []float32, int, codeintel.Filter) ([]codeintel.SearchResult, error) {
	return nil, nil
}

func (f *semanticFakeStore) GetByFilePath(context.Context, string) ([]codeintel.Chunk, error) {
	return nil, nil
}
func (f *semanticFakeStore) GetByName(context.Context, string) ([]codeintel.Chunk, error) {
	return nil, nil
}
func (f *semanticFakeStore) DeleteByFilePath(_ context.Context, filePath string) error {
	f.deleted = append(f.deleted, filePath)
	return nil
}
func (f *semanticFakeStore) Close() error { return nil }

type semanticFakeEmbedder struct {
	texts [][]string
	err   error
}

func (f *semanticFakeEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.texts = append(f.texts, append([]string(nil), texts...))
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{0.1, 0.2}
	}
	return out, nil
}

func (f *semanticFakeEmbedder) EmbedQuery(context.Context, string) ([]float32, error) {
	return []float32{0.3, 0.4}, nil
}

func TestSemanticIndexerRebuildProjectIndexesBrainChunksAndDeletesStaleDocuments(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{docs: map[string]string{
		"notes/architecture.md": "---\ntags: [brain, architecture]\n---\n# Architecture\n\nOverview paragraph that keeps the document long enough to avoid short-document fallback.\n\n## Problem\n\n" +
			"Detail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\n" +
			"### Deep Dive\n\nNested details remain with the parent section.\n\n## Fix\n\n" +
			"Detail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\nDetail line with useful retrieval context.\n",
	}}
	store := &semanticFakeStore{}
	embedder := &semanticFakeEmbedder{}
	idx := NewSemantic(backend, store, embedder)
	idx.now = func() time.Time { return time.Date(2026, 4, 9, 14, 0, 0, 0, time.UTC) }

	result, err := idx.RebuildProject(ctx, "eyebox", []string{"notes/stale.md", "notes/architecture.md"})
	if err != nil {
		t.Fatalf("RebuildProject returned error: %v", err)
	}
	if result.SemanticChunksIndexed != 2 {
		t.Fatalf("SemanticChunksIndexed = %d, want 2", result.SemanticChunksIndexed)
	}
	if result.SemanticDocumentsDeleted != 1 {
		t.Fatalf("SemanticDocumentsDeleted = %d, want 1", result.SemanticDocumentsDeleted)
	}
	if !reflect.DeepEqual(store.deleted, []string{"notes/architecture.md", "notes/stale.md"}) {
		t.Fatalf("DeleteByFilePath calls = %#v, want architecture then stale delete", store.deleted)
	}
	if len(store.upserts) != 1 || len(store.upserts[0]) != 2 {
		t.Fatalf("upserts = %#v, want one 2-chunk upsert", store.upserts)
	}
	first := store.upserts[0][0]
	if first.ProjectName != "eyebox" {
		t.Fatalf("ProjectName = %q, want eyebox", first.ProjectName)
	}
	if first.FilePath != "notes/architecture.md" {
		t.Fatalf("FilePath = %q, want notes/architecture.md", first.FilePath)
	}
	if first.Language != "markdown" {
		t.Fatalf("Language = %q, want markdown", first.Language)
	}
	if first.ChunkType != codeintel.ChunkTypeSection {
		t.Fatalf("ChunkType = %q, want section", first.ChunkType)
	}
	if first.Name != "Problem" {
		t.Fatalf("Name = %q, want Problem", first.Name)
	}
	if first.ContentHash == "" {
		t.Fatal("expected non-empty ContentHash")
	}
	if len(first.Embedding) == 0 {
		t.Fatal("expected embedding to be populated")
	}
	if len(embedder.texts) != 1 || len(embedder.texts[0]) != 2 {
		t.Fatalf("embedder texts = %#v, want one 2-item batch", embedder.texts)
	}
	if embedder.texts[0][0] == "" || embedder.texts[0][1] == "" {
		t.Fatalf("embed texts should be non-empty: %#v", embedder.texts[0])
	}
}

func TestSemanticIndexerRebuildProjectReturnsEmbedderErrorWithoutUpsert(t *testing.T) {
	ctx := context.Background()
	backend := &fakeBackend{docs: map[string]string{
		"notes/short.md": "# Short\n\nA short note",
	}}
	store := &semanticFakeStore{}
	embedder := &semanticFakeEmbedder{err: errors.New("embedder offline")}
	idx := NewSemantic(backend, store, embedder)

	_, err := idx.RebuildProject(ctx, "eyebox", nil)
	if err == nil || err.Error() != "embed semantic brain chunks for notes/short.md: embedder offline" {
		t.Fatalf("err = %v, want embedder error", err)
	}
	if len(store.upserts) != 0 {
		t.Fatalf("unexpected Upsert calls after embedder failure: %#v", store.upserts)
	}
}
