package chunks

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sirtopham/internal/brain/parser"
)

func TestBuildDocumentUsesSingleChunkForShortDocument(t *testing.T) {
	doc, err := parser.ParseDocument("notes/short.md", "---\ntags: [alpha, brain]\n---\n# Short Note\n\nA short brain note with one paragraph.")
	if err != nil {
		t.Fatalf("ParseDocument returned error: %v", err)
	}
	updatedAt := time.Date(2026, 4, 9, 13, 0, 0, 0, time.UTC)
	doc.UpdatedAt = updatedAt
	doc.HasUpdatedAt = true

	got := BuildDocument(doc)
	if len(got) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(got))
	}
	chunk := got[0]
	if chunk.DocumentPath != "notes/short.md" {
		t.Fatalf("DocumentPath = %q, want notes/short.md", chunk.DocumentPath)
	}
	if chunk.DocumentTitle != "Short Note" {
		t.Fatalf("DocumentTitle = %q, want Short Note", chunk.DocumentTitle)
	}
	if chunk.ChunkIndex != 0 {
		t.Fatalf("ChunkIndex = %d, want 0", chunk.ChunkIndex)
	}
	if chunk.SectionHeading != "" {
		t.Fatalf("SectionHeading = %q, want empty", chunk.SectionHeading)
	}
	if chunk.LineStart != 1 {
		t.Fatalf("LineStart = %d, want 1", chunk.LineStart)
	}
	if chunk.LineEnd < chunk.LineStart {
		t.Fatalf("LineEnd = %d, want >= %d", chunk.LineEnd, chunk.LineStart)
	}
	if !reflect.DeepEqual(chunk.Tags, []string{"alpha", "brain"}) {
		t.Fatalf("Tags = %#v, want [alpha brain]", chunk.Tags)
	}
	if chunk.DocumentUpdatedAt != updatedAt {
		t.Fatalf("DocumentUpdatedAt = %v, want %v", chunk.DocumentUpdatedAt, updatedAt)
	}
	if !chunk.HasDocumentUpdatedAt {
		t.Fatal("expected HasDocumentUpdatedAt=true")
	}
	if !strings.Contains(chunk.Text, "A short brain note with one paragraph.") {
		t.Fatalf("chunk text missing body content: %q", chunk.Text)
	}
	if chunk.ID == "" {
		t.Fatal("expected non-empty chunk ID")
	}
}

func TestBuildDocumentSplitsLongDocumentAtLevelTwoHeadings(t *testing.T) {
	sectionBody := strings.Repeat("Detail line with useful retrieval context.\n", 20)
	content := "# Architecture\n\nOverview paragraph that keeps the document long enough to avoid short-document fallback.\n\n" +
		"## Problem\n\n" + sectionBody +
		"### Deep Dive\n\nNested details remain with the parent section.\n\n" +
		"## Fix\n\n" + sectionBody
	doc, err := parser.ParseDocument("notes/architecture.md", content)
	if err != nil {
		t.Fatalf("ParseDocument returned error: %v", err)
	}

	got := BuildDocument(doc)
	if len(got) != 2 {
		t.Fatalf("len(chunks) = %d, want 2", len(got))
	}
	if got[0].SectionHeading != "Problem" || got[1].SectionHeading != "Fix" {
		t.Fatalf("section headings = [%q, %q], want [Problem Fix]", got[0].SectionHeading, got[1].SectionHeading)
	}
	if got[0].ChunkIndex != 0 || got[1].ChunkIndex != 1 {
		t.Fatalf("chunk indexes = [%d, %d], want [0 1]", got[0].ChunkIndex, got[1].ChunkIndex)
	}
	if !strings.HasPrefix(got[0].Text, "## Problem") {
		t.Fatalf("first chunk missing section heading prefix: %q", got[0].Text)
	}
	if strings.Contains(got[0].Text, "## Fix") {
		t.Fatalf("first chunk leaked next section: %q", got[0].Text)
	}
	if !strings.Contains(got[0].Text, "### Deep Dive") {
		t.Fatalf("first chunk missing nested subsection content: %q", got[0].Text)
	}
	if !strings.HasPrefix(got[1].Text, "## Fix") {
		t.Fatalf("second chunk missing section heading prefix: %q", got[1].Text)
	}
	if got[0].DocumentPath != "notes/architecture.md" || got[1].DocumentPath != "notes/architecture.md" {
		t.Fatalf("unexpected document paths: %#v", []string{got[0].DocumentPath, got[1].DocumentPath})
	}
	if got[0].DocumentTitle != "Architecture" || got[1].DocumentTitle != "Architecture" {
		t.Fatalf("unexpected document titles: %#v", []string{got[0].DocumentTitle, got[1].DocumentTitle})
	}
	if got[0].LineStart != 5 {
		t.Fatalf("first chunk LineStart = %d, want 5", got[0].LineStart)
	}
	if got[1].LineStart <= got[0].LineStart {
		t.Fatalf("second chunk LineStart = %d, want > %d", got[1].LineStart, got[0].LineStart)
	}
}

func TestBuildDocumentUsesSingleChunkWhenLongDocumentHasNoLevelTwoHeadings(t *testing.T) {
	content := "# Long Note\n\n" + strings.Repeat("Paragraph text without subsection headings.\n", 40)
	doc, err := parser.ParseDocument("notes/long.md", content)
	if err != nil {
		t.Fatalf("ParseDocument returned error: %v", err)
	}

	got := BuildDocument(doc)
	if len(got) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(got))
	}
	if got[0].SectionHeading != "" {
		t.Fatalf("SectionHeading = %q, want empty", got[0].SectionHeading)
	}
	if !strings.Contains(got[0].Text, "Paragraph text without subsection headings.") {
		t.Fatalf("single chunk missing body text: %q", got[0].Text)
	}
}
