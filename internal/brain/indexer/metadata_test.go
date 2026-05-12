package indexer

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ponchione/sodoryard/internal/brain"
)

type metadataFakeBackend struct {
	docs map[string]string
}

func (f *metadataFakeBackend) ReadDocument(_ context.Context, path string) (string, error) {
	content, ok := f.docs[path]
	if !ok {
		return "", errors.New("not found")
	}
	return content, nil
}

func (f *metadataFakeBackend) WriteDocument(context.Context, string, string) error {
	panic("unexpected WriteDocument call")
}

func (f *metadataFakeBackend) PatchDocument(context.Context, string, string, string) error {
	panic("unexpected PatchDocument call")
}

func (f *metadataFakeBackend) SearchKeyword(context.Context, string) ([]brain.SearchHit, error) {
	panic("unexpected SearchKeyword call")
}

func (f *metadataFakeBackend) ListDocuments(context.Context, string) ([]string, error) {
	paths := make([]string, 0, len(f.docs))
	for path := range f.docs {
		paths = append(paths, path)
	}
	return paths, nil
}

func TestMetadataIndexerRebuildProjectCountsDocumentsLinksAndDeletesWithoutDatabase(t *testing.T) {
	ctx := context.Background()
	backend := &metadataFakeBackend{docs: map[string]string{
		"notes/alpha.md": "# Alpha\n\nSee [[notes/beta|Beta]].\n",
		"notes/beta.md":  "# Beta\n",
		"_log.md":        "# Operational log\n",
	}}

	result, err := NewMetadata(backend).RebuildProject(ctx, "project", []string{
		"notes/beta.md",
		"notes/stale.md",
		"_log.md",
	})
	if err != nil {
		t.Fatalf("RebuildProject returned error: %v", err)
	}
	if result.DocumentsIndexed != 2 || result.LinksIndexed != 1 || result.DocumentsDeleted != 1 {
		t.Fatalf("result = %+v, want 2 docs, 1 link, 1 delete", result)
	}
	if !reflect.DeepEqual(result.DocumentPaths, []string{"notes/alpha.md", "notes/beta.md"}) {
		t.Fatalf("DocumentPaths = %#v, want active non-operational docs", result.DocumentPaths)
	}
}
