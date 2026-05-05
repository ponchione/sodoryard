package context

import (
	stdctx "context"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/brain"
)

func TestBrainBackendConventionSourceLoadReturnsEmptyWhenMissing(t *testing.T) {
	source := NewBrainBackendConventionSource(fakeConventionBackend{})
	text, err := source.Load(stdctx.Background())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if text != "" {
		t.Fatalf("Load = %q, want empty string", text)
	}
}

func TestBrainBackendConventionSourceLoadExtractsBulletsAndParagraphSummaries(t *testing.T) {
	source := NewBrainBackendConventionSource(fakeConventionBackend{
		"conventions/testing-patterns.md": "---\ntags: [convention]\n---\n\n# Testing patterns\n\n- Prefer table-driven tests\n- Keep fixtures local to each test file\n",
		"conventions/error-handling.md":   "# Error handling\n\nAlways wrap errors with operation context and preserve the original cause.\n\n```go\nfmt.Errorf(\"load config: %w\", err)\n```\n",
	})
	text, err := source.Load(stdctx.Background())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	want := "Error handling: Always wrap errors with operation context and preserve the original cause.\nPrefer table-driven tests\nKeep fixtures local to each test file"
	if text != want {
		t.Fatalf("Load mismatch\n got: %q\nwant: %q", text, want)
	}
}

func TestBrainBackendConventionSourceLoadDeduplicatesAndRespectsLimit(t *testing.T) {
	source := NewBrainBackendConventionSource(fakeConventionBackend{
		"conventions/a.md": "- Same rule\n- Same rule\n- Rule A\n",
		"conventions/b.md": "- Rule B\n",
	})
	source.bulletLimit = 2

	text, err := source.Load(stdctx.Background())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if text != "Same rule\nRule A" {
		t.Fatalf("Load = %q, want first two unique bullets", text)
	}
}

func TestExtractConventionBulletsFallsBackToFilenameWhenHeadingMissing(t *testing.T) {
	got := extractConventionBullets("anti-patterns.md", "Never patch production data manually.\n")
	if len(got) != 1 || got[0] != "anti patterns: Never patch production data manually." {
		t.Fatalf("extractConventionBullets = %#v, want filename-derived summary", got)
	}
}

type fakeConventionBackend map[string]string

func (f fakeConventionBackend) ReadDocument(_ stdctx.Context, path string) (string, error) {
	return f[path], nil
}

func (f fakeConventionBackend) WriteDocument(stdctx.Context, string, string) error { return nil }
func (f fakeConventionBackend) PatchDocument(stdctx.Context, string, string, string) error {
	return nil
}
func (f fakeConventionBackend) SearchKeyword(stdctx.Context, string) ([]brain.SearchHit, error) {
	return nil, nil
}
func (f fakeConventionBackend) ListDocuments(_ stdctx.Context, directory string) ([]string, error) {
	var paths []string
	prefix := strings.TrimSuffix(directory, "/") + "/"
	for path := range f {
		if strings.HasPrefix(path, prefix) {
			paths = append(paths, path)
		}
	}
	return paths, nil
}
