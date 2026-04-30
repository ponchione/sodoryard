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

func writeTestFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
