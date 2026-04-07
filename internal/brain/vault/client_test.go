package vault

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestClientWriteReadAndListDocuments(t *testing.T) {
	root := t.TempDir()
	client, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	content := "---\ntags: [architecture]\n---\n# Design\nPipeline notes"
	if err := client.WriteDocument(context.Background(), "notes/design.md", content); err != nil {
		t.Fatalf("WriteDocument: %v", err)
	}

	got, err := client.ReadDocument(context.Background(), "notes/design.md")
	if err != nil {
		t.Fatalf("ReadDocument: %v", err)
	}
	if got != content {
		t.Fatalf("ReadDocument mismatch\n got: %q\nwant: %q", got, content)
	}

	files, err := client.ListDocuments(context.Background(), "notes")
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if !slices.Equal(files, []string{"notes/design.md"}) {
		t.Fatalf("ListDocuments = %#v, want [notes/design.md]", files)
	}
}

func TestClientRejectsPathTraversal(t *testing.T) {
	client, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.ReadDocument(context.Background(), "../outside.md")
	if !errors.Is(err, ErrPathTraversal) {
		t.Fatalf("ReadDocument error = %v, want ErrPathTraversal", err)
	}

	err = client.WriteDocument(context.Background(), "../outside.md", "nope")
	if !errors.Is(err, ErrPathTraversal) {
		t.Fatalf("WriteDocument error = %v, want ErrPathTraversal", err)
	}
}

func TestClientSearchKeywordFindsContentAndPath(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "notes", "auth.md"), "# Auth\nToken refresh workaround")
	mustWriteFile(t, filepath.Join(root, "architecture", "pipeline.md"), "# Design\nStreaming pipeline")

	client, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	hits, err := client.SearchKeyword(context.Background(), "pipeline", 10)
	if err != nil {
		t.Fatalf("SearchKeyword: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != "architecture/pipeline.md" {
		t.Fatalf("pipeline hits = %#v, want architecture/pipeline.md", hits)
	}

	hits, err = client.SearchKeyword(context.Background(), "refresh", 10)
	if err != nil {
		t.Fatalf("SearchKeyword: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != "notes/auth.md" {
		t.Fatalf("refresh hits = %#v, want notes/auth.md", hits)
	}
}

func TestNormalizeForKeyword(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "only punctuation", in: "---, \t\n", want: ""},
		{name: "ascii lowercase", in: "Hello", want: "hello"},
		{name: "hyphenated phrase", in: "content-first layout", want: "content first layout"},
		{name: "comma phrase", in: "minimal, content-first layout", want: "minimal content first layout"},
		{name: "multiline", in: "vite,\nrebuild loop", want: "vite rebuild loop"},
		{name: "hash tag", in: "#rationale", want: "rationale"},
		{name: "path-like", in: "notes/past-debugging-vite.md", want: "notes past debugging vite md"},
		{name: "trailing punct", in: "hello, world!!!", want: "hello world"},
		{name: "numbers preserved", in: "abc 123 def", want: "abc 123 def"},
		{name: "unicode letter lowercased", in: "Café-Bar", want: "café bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeForKeyword(tc.in)
			if got != tc.want {
				t.Fatalf("normalizeForKeyword(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestClientSearchKeywordHandlesPathologicalPunctuation(t *testing.T) {
	// Each note is crafted to contain a phrase separated by the kind of
	// punctuation (commas, hyphens, line breaks) that defeated the old
	// strict-substring keyword search. With normalizeForKeyword on both
	// sides of the match, the multi-word query should now hit.
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "notes", "rationale.md"),
		"---\ntags: [rationale]\n---\n# Layout\n\nWe picked a minimal, content-first layout on purpose.\n")
	mustWriteFile(t, filepath.Join(root, "notes", "past-debugging-vite-rebuild-loop.md"),
		"# Past debugging\n\nFamily: debug-history\nSymptom: vite,\nrebuild loop during hot reload.\n")
	mustWriteFile(t, filepath.Join(root, "notes", "naming.md"),
		"# Analyzer pattern list naming convention\n\nUse the `brainSeekingXPatterns` shape.\n")
	mustWriteFile(t, filepath.Join(root, "notes", "irrelevant.md"),
		"# Something else\n\nNo pathological content here.\n")

	client, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cases := []struct {
		name     string
		query    string
		wantPath string
	}{
		{
			name:     "comma and hyphen between phrase words",
			query:    "minimal content first layout",
			wantPath: "notes/rationale.md",
		},
		{
			name:     "comma and newline between phrase words",
			query:    "vite rebuild loop",
			wantPath: "notes/past-debugging-vite-rebuild-loop.md",
		},
		{
			name:     "hyphens inside path",
			query:    "past debugging vite rebuild loop",
			wantPath: "notes/past-debugging-vite-rebuild-loop.md",
		},
		{
			name:     "hash tag strips and matches",
			query:    "#rationale",
			wantPath: "notes/rationale.md",
		},
		{
			name:     "phrase spanning heading words",
			query:    "analyzer pattern list naming convention",
			wantPath: "notes/naming.md",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits, err := client.SearchKeyword(context.Background(), tc.query, 10)
			if err != nil {
				t.Fatalf("SearchKeyword: %v", err)
			}
			if len(hits) == 0 {
				t.Fatalf("SearchKeyword(%q) returned no hits, want at least one containing %s", tc.query, tc.wantPath)
			}
			found := false
			for _, h := range hits {
				if h.Path == tc.wantPath {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("SearchKeyword(%q) hits = %#v, want a hit for %s", tc.query, hits, tc.wantPath)
			}
		})
	}
}

func TestClientSearchKeywordDoesNotOverMatchAcrossUnrelatedWords(t *testing.T) {
	// Normalization should NOT make unrelated noise match. The note below
	// contains the phrase "foo baz bar" — a query of "foo bar" must not
	// hit because normalization does not reorder tokens.
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "notes", "noise.md"), "# Noise\n\nfoo baz bar\n")

	client, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	hits, err := client.SearchKeyword(context.Background(), "foo bar", 10)
	if err != nil {
		t.Fatalf("SearchKeyword: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("SearchKeyword(\"foo bar\") = %#v, want no hits", hits)
	}
}

func TestClientListDocumentsSkipsObsidianDir(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, ".obsidian", "workspace.md"), "ignore")
	mustWriteFile(t, filepath.Join(root, "notes", "keep.md"), "keep")

	client, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	files, err := client.ListDocuments(context.Background(), "")
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if !slices.Equal(files, []string{"notes/keep.md"}) {
		t.Fatalf("ListDocuments = %#v, want only notes/keep.md", files)
	}
}

func TestClientPatchDocumentAppendPrependAndReplaceSection(t *testing.T) {
	root := t.TempDir()
	path := "notes/design.md"
	initial := "---\ntitle: Design\n---\n# Design\n\n## Overview\n\nOriginal overview.\n\n## Details\n\nOriginal details.\n"
	mustWriteFile(t, filepath.Join(root, path), initial)

	client, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := client.PatchDocument(context.Background(), path, "append", "## Appendix\n\nExtra notes."); err != nil {
		t.Fatalf("append PatchDocument: %v", err)
	}
	if err := client.PatchDocument(context.Background(), path, "prepend", "Intro paragraph."); err != nil {
		t.Fatalf("prepend PatchDocument: %v", err)
	}
	if err := client.PatchDocument(context.Background(), path, "replace_section", "## Details\n\nUpdated details."); err != nil {
		t.Fatalf("replace_section PatchDocument: %v", err)
	}

	got, err := client.ReadDocument(context.Background(), path)
	if err != nil {
		t.Fatalf("ReadDocument: %v", err)
	}
	if !containsAll(got,
		"---\ntitle: Design\n---\n\nIntro paragraph.",
		"## Appendix\n\nExtra notes.",
		"## Details\n\nUpdated details.",
	) {
		t.Fatalf("patched document missing expected content:\n%s", got)
	}
	if containsAll(got, "Original details.") {
		t.Fatalf("patched document still contains replaced content:\n%s", got)
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
