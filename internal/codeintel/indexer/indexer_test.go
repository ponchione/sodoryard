package indexer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/codeintel"
)

type mockParser struct{}

func (m *mockParser) Parse(filePath string, content []byte) ([]codeintel.RawChunk, error) {
	if !strings.HasSuffix(filePath, ".go") && !strings.HasSuffix(filePath, ".py") {
		return []codeintel.RawChunk{}, nil
	}
	return []codeintel.RawChunk{
		{
			Name:      "TestFunc",
			Signature: "func TestFunc()",
			Body:      string(content),
			ChunkType: codeintel.ChunkTypeFunction,
			LineStart: 1,
			LineEnd:   5,
		},
	}, nil
}

type mockEmbedder struct{ called int }

func (m *mockEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	m.called += len(texts)
	result := make([][]float32, len(texts))
	for i := range result {
		result[i] = make([]float32, 4)
	}
	return result, nil
}

func (m *mockEmbedder) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, 4), nil
}

type mockDescriber struct{ called int }

func (m *mockDescriber) DescribeFile(_ context.Context, _ string, _ string) ([]codeintel.Description, error) {
	m.called++
	return []codeintel.Description{
		{Name: "TestFunc", Description: "A test function"},
	}, nil
}

type mockStore struct{ chunks []codeintel.Chunk }

func (m *mockStore) Upsert(_ context.Context, chunks []codeintel.Chunk) error {
	m.chunks = append(m.chunks, chunks...)
	return nil
}
func (m *mockStore) VectorSearch(_ context.Context, _ []float32, _ int, _ codeintel.Filter) ([]codeintel.SearchResult, error) {
	return nil, nil
}
func (m *mockStore) GetByFilePath(_ context.Context, _ string) ([]codeintel.Chunk, error) {
	return nil, nil
}
func (m *mockStore) GetByName(_ context.Context, _ string) ([]codeintel.Chunk, error) {
	return nil, nil
}
func (m *mockStore) DeleteByFilePath(_ context.Context, _ string) error { return nil }
func (m *mockStore) Close() error                                       { return nil }

func TestIndexRepo_BasicPipeline(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()

	writeTestFile(filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	writeTestFile(filepath.Join(dir, "lib.go"), "package main\n\nfunc helper() {}\n")

	store := &mockStore{}
	embedder := &mockEmbedder{}
	describer := &mockDescriber{}
	parser := &mockParser{}

	cfg := IndexConfig{
		ProjectName: "testproject",
		ProjectRoot: dir,
		DataDir:     dataDir,
		Include:     []string{"**/*.go"},
	}

	err := IndexRepo(context.Background(), cfg, parser, store, embedder, describer, IndexOpts{})
	if err != nil {
		t.Fatalf("IndexRepo: %v", err)
	}

	if len(store.chunks) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(store.chunks))
	}
	if embedder.called != 2 {
		t.Errorf("expected 2 embed calls, got %d", embedder.called)
	}
	if describer.called != 2 {
		t.Errorf("expected 2 describe calls, got %d", describer.called)
	}

	for _, c := range store.chunks {
		if c.ProjectName != "testproject" {
			t.Errorf("ProjectName = %q", c.ProjectName)
		}
		if c.Language != "go" {
			t.Errorf("Language = %q", c.Language)
		}
		if len(c.Embedding) != 4 {
			t.Errorf("Embedding length = %d", len(c.Embedding))
		}
		if c.Description != "A test function" {
			t.Errorf("Description = %q", c.Description)
		}
	}
}

func TestIndexRepo_IncrementalSkipsUnchanged(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	writeTestFile(filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")

	store := &mockStore{}
	embedder := &mockEmbedder{}
	describer := &mockDescriber{}
	parser := &mockParser{}

	cfg := IndexConfig{
		ProjectName: "testproject",
		ProjectRoot: dir,
		DataDir:     dataDir,
		Include:     []string{"**/*.go"},
	}

	if err := IndexRepo(context.Background(), cfg, parser, store, embedder, describer, IndexOpts{}); err != nil {
		t.Fatal(err)
	}
	if len(store.chunks) != 1 {
		t.Fatalf("first run: expected 1, got %d", len(store.chunks))
	}

	store.chunks = nil
	embedder.called = 0
	describer.called = 0
	if err := IndexRepo(context.Background(), cfg, parser, store, embedder, describer, IndexOpts{}); err != nil {
		t.Fatal(err)
	}
	if len(store.chunks) != 0 {
		t.Errorf("second run: expected 0 (unchanged), got %d", len(store.chunks))
	}
}

func TestIndexRepo_ForceReindexes(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	writeTestFile(filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")

	store := &mockStore{}
	cfg := IndexConfig{ProjectName: "test", ProjectRoot: dir, DataDir: dataDir, Include: []string{"**/*.go"}}

	IndexRepo(context.Background(), cfg, &mockParser{}, store, &mockEmbedder{}, &mockDescriber{}, IndexOpts{})

	store.chunks = nil
	IndexRepo(context.Background(), cfg, &mockParser{}, store, &mockEmbedder{}, &mockDescriber{}, IndexOpts{Force: true})
	if len(store.chunks) != 1 {
		t.Errorf("force: expected 1, got %d", len(store.chunks))
	}
}

func TestIndexRepo_ExcludeGlobs(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	writeTestFile(filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	os.MkdirAll(filepath.Join(dir, "vendor"), 0755)
	writeTestFile(filepath.Join(dir, "vendor", "dep.go"), "package dep\n")

	store := &mockStore{}
	cfg := IndexConfig{
		ProjectName: "test",
		ProjectRoot: dir,
		DataDir:     dataDir,
		Include:     []string{"**/*.go"},
		Exclude:     []string{"vendor/**"},
	}

	IndexRepo(context.Background(), cfg, &mockParser{}, store, &mockEmbedder{}, &mockDescriber{}, IndexOpts{})
	if len(store.chunks) != 1 {
		t.Errorf("expected 1 (vendor excluded), got %d", len(store.chunks))
	}
}
