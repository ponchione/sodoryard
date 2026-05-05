package indexer

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/brain/parser"
)

type MetadataIndexer struct {
	backend brain.Backend
}

type MetadataResult struct {
	Result
	DocumentPaths []string
}

func NewMetadata(backend brain.Backend) *MetadataIndexer {
	return &MetadataIndexer{backend: backend}
}

func (i *MetadataIndexer) RebuildProject(ctx context.Context, projectID string, previousDocumentPaths []string) (MetadataResult, error) {
	if i == nil || i.backend == nil {
		return MetadataResult{}, fmt.Errorf("metadata indexer backend is required")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return MetadataResult{}, fmt.Errorf("project id is required")
	}

	paths, err := i.backend.ListDocuments(ctx, "")
	if err != nil {
		return MetadataResult{}, fmt.Errorf("list brain documents: %w", err)
	}
	sort.Strings(paths)

	previous := make(map[string]struct{}, len(previousDocumentPaths))
	for _, path := range previousDocumentPaths {
		path = strings.TrimSpace(path)
		if path == "" || isOperationalBrainDocument(path) {
			continue
		}
		previous[path] = struct{}{}
	}

	result := Result{}
	seen := make(map[string]struct{}, len(paths))
	currentPaths := make([]string, 0, len(paths))
	for _, docPath := range paths {
		docPath = strings.TrimSpace(docPath)
		if docPath == "" || isOperationalBrainDocument(docPath) {
			continue
		}
		content, err := i.backend.ReadDocument(ctx, docPath)
		if err != nil {
			return MetadataResult{}, fmt.Errorf("read brain document %s: %w", docPath, err)
		}
		doc, err := parser.ParseDocument(docPath, content)
		if err != nil {
			return MetadataResult{}, fmt.Errorf("parse brain document %s: %w", docPath, err)
		}
		seen[doc.Path] = struct{}{}
		currentPaths = append(currentPaths, doc.Path)
		result.DocumentsIndexed++
		result.LinksIndexed += len(doc.Wikilinks)
	}

	for path := range previous {
		if _, ok := seen[path]; ok {
			continue
		}
		result.DocumentsDeleted++
	}

	sort.Strings(currentPaths)
	return MetadataResult{Result: result, DocumentPaths: currentPaths}, nil
}
