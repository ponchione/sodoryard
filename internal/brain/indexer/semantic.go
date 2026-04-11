package indexer

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	brainchunks "github.com/ponchione/sodoryard/internal/brain/chunks"
	"github.com/ponchione/sodoryard/internal/brain/parser"
	"github.com/ponchione/sodoryard/internal/codeintel"
)

type SemanticIndexer struct {
	backend  documentBackend
	store    codeintel.Store
	embedder codeintel.Embedder
	now      func() time.Time
}

type documentBackend interface {
	ListDocuments(ctx context.Context, directory string) ([]string, error)
	ReadDocument(ctx context.Context, path string) (string, error)
}

func NewSemantic(backend documentBackend, store codeintel.Store, embedder codeintel.Embedder) *SemanticIndexer {
	return &SemanticIndexer{
		backend:  backend,
		store:    store,
		embedder: embedder,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (i *SemanticIndexer) RebuildProject(ctx context.Context, projectName string, previousDocumentPaths []string) (Result, error) {
	if i == nil || i.backend == nil {
		return Result{}, fmt.Errorf("semantic indexer backend is required")
	}
	if i.store == nil {
		return Result{}, fmt.Errorf("semantic indexer store is required")
	}
	if i.embedder == nil {
		return Result{}, fmt.Errorf("semantic indexer embedder is required")
	}
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return Result{}, fmt.Errorf("semantic indexer project name is required")
	}

	paths, err := i.backend.ListDocuments(ctx, "")
	if err != nil {
		return Result{}, fmt.Errorf("list brain documents for semantic index: %w", err)
	}
	sort.Strings(paths)
	previous := make(map[string]struct{}, len(previousDocumentPaths))
	for _, path := range previousDocumentPaths {
		path = strings.TrimSpace(path)
		if path != "" {
			previous[path] = struct{}{}
		}
	}

	result := Result{}
	seen := make(map[string]struct{}, len(paths))
	for _, docPath := range paths {
		if isOperationalBrainDocument(docPath) {
			continue
		}
		content, err := i.backend.ReadDocument(ctx, docPath)
		if err != nil {
			return Result{}, fmt.Errorf("read brain document %s for semantic index: %w", docPath, err)
		}
		doc, err := parser.ParseDocument(docPath, content)
		if err != nil {
			return Result{}, fmt.Errorf("parse brain document %s for semantic index: %w", docPath, err)
		}
		brainDocChunks := brainchunks.BuildDocument(doc)
		if err := i.store.DeleteByFilePath(ctx, doc.Path); err != nil {
			return Result{}, fmt.Errorf("delete existing semantic brain chunks for %s: %w", doc.Path, err)
		}
		if len(brainDocChunks) == 0 {
			seen[doc.Path] = struct{}{}
			continue
		}
		storeChunks, embedTexts := toSemanticChunks(projectName, brainDocChunks, i.now().UTC())
		embeddings, err := i.embedder.EmbedTexts(ctx, embedTexts)
		if err != nil {
			return Result{}, fmt.Errorf("embed semantic brain chunks for %s: %w", doc.Path, err)
		}
		for idx := range storeChunks {
			if idx < len(embeddings) {
				storeChunks[idx].Embedding = embeddings[idx]
			}
		}
		if err := i.store.Upsert(ctx, storeChunks); err != nil {
			return Result{}, fmt.Errorf("upsert semantic brain chunks for %s: %w", doc.Path, err)
		}
		result.SemanticChunksIndexed += len(storeChunks)
		seen[doc.Path] = struct{}{}
	}

	for stalePath := range previous {
		if _, ok := seen[stalePath]; ok || isOperationalBrainDocument(stalePath) {
			continue
		}
		if err := i.store.DeleteByFilePath(ctx, stalePath); err != nil {
			return Result{}, fmt.Errorf("delete stale semantic brain chunks for %s: %w", stalePath, err)
		}
		result.SemanticDocumentsDeleted++
	}

	return result, nil
}

func toSemanticChunks(projectName string, brainDocChunks []brainchunks.Chunk, indexedAt time.Time) ([]codeintel.Chunk, []string) {
	chunks := make([]codeintel.Chunk, 0, len(brainDocChunks))
	texts := make([]string, 0, len(brainDocChunks))
	for _, chunk := range brainDocChunks {
		name := chunk.DocumentTitle
		chunkType := codeintel.ChunkTypeFallback
		if chunk.SectionHeading != "" {
			name = chunk.SectionHeading
			chunkType = codeintel.ChunkTypeSection
		}
		signature := chunk.DocumentPath
		if chunk.SectionHeading != "" {
			signature = fmt.Sprintf("%s\n## %s", chunk.DocumentPath, chunk.SectionHeading)
		}
		text := strings.TrimSpace(chunk.Text)
		chunks = append(chunks, codeintel.Chunk{
			ID:          chunk.ID,
			ProjectName: projectName,
			FilePath:    chunk.DocumentPath,
			Language:    "markdown",
			ChunkType:   chunkType,
			Name:        name,
			Signature:   signature,
			Body:        codeintel.TruncateUTF8(text, codeintel.MaxBodyLength),
			ContentHash: chunk.DocumentContentHash,
			IndexedAt:   indexedAt,
		})
		texts = append(texts, buildBrainEmbedText(chunk))
	}
	return chunks, texts
}

func buildBrainEmbedText(chunk brainchunks.Chunk) string {
	parts := []string{chunk.DocumentTitle}
	if chunk.SectionHeading != "" {
		parts = append(parts, chunk.SectionHeading)
	}
	if len(chunk.Tags) > 0 {
		parts = append(parts, "tags: "+strings.Join(chunk.Tags, ", "))
	}
	parts = append(parts, chunk.Text)
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
