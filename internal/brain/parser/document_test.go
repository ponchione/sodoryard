package parser

import (
	"reflect"
	"testing"
	"time"
)

func TestParseDocumentExtractsFrontmatterTitleTagsLinksHeadingsAndHash(t *testing.T) {
	content := `---
updated_at: 2026-04-09T12:34:56Z
tags: [architecture, brain]
status: active
---
# Brain Plan

Intro paragraph with inline #implementation tag.

## Problem
Links to [[notes/design|Design Note]] and [[debugging/auth-race]].

### Workaround
Use [[debugging/auth-race]] again and [[notes/design#section|Section Link]].
`
	doc, err := ParseDocument("notes/plan.md", content)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if doc.Path != "notes/plan.md" {
		t.Fatalf("Path = %q, want notes/plan.md", doc.Path)
	}
	if doc.Title != "Brain Plan" {
		t.Fatalf("Title = %q, want Brain Plan", doc.Title)
	}
	if doc.Content != content {
		t.Fatalf("Content mismatch")
	}
	if doc.Body == content || doc.Body == "" {
		t.Fatalf("Body = %q, want frontmatter-stripped body", doc.Body)
	}
	if doc.Frontmatter["status"] != "active" {
		t.Fatalf("Frontmatter status = %v, want active", doc.Frontmatter["status"])
	}
	if !doc.HasUpdatedAt {
		t.Fatal("expected updated_at to be parsed")
	}
	wantUpdated := time.Date(2026, 4, 9, 12, 34, 56, 0, time.UTC)
	if !doc.UpdatedAt.Equal(wantUpdated) {
		t.Fatalf("UpdatedAt = %v, want %v", doc.UpdatedAt, wantUpdated)
	}
	wantTags := []string{"architecture", "brain", "implementation"}
	if !reflect.DeepEqual(doc.Tags, wantTags) {
		t.Fatalf("Tags = %#v, want %#v", doc.Tags, wantTags)
	}
	wantLinks := []ParsedLink{
		{Target: "notes/design", Display: "Design Note", Raw: "notes/design|Design Note"},
		{Target: "debugging/auth-race", Display: "", Raw: "debugging/auth-race"},
		{Target: "notes/design", Display: "Section Link", Raw: "notes/design#section|Section Link"},
	}
	if !reflect.DeepEqual(doc.Wikilinks, wantLinks) {
		t.Fatalf("Wikilinks = %#v, want %#v", doc.Wikilinks, wantLinks)
	}
	wantHeadings := []Heading{
		{Level: 1, Text: "Brain Plan", Line: 1},
		{Level: 2, Text: "Problem", Line: 5},
		{Level: 3, Text: "Workaround", Line: 8},
	}
	if !reflect.DeepEqual(doc.Headings, wantHeadings) {
		t.Fatalf("Headings = %#v, want %#v", doc.Headings, wantHeadings)
	}
	if doc.ContentHash == "" {
		t.Fatal("ContentHash = empty, want sha256 hash")
	}
	if doc.TokenCount <= 0 {
		t.Fatalf("TokenCount = %d, want positive", doc.TokenCount)
	}
}

func TestParseDocumentContentHashIsStableAndChangesWithContent(t *testing.T) {
	base := "# Plan\nSame content"
	doc1, err := ParseDocument("notes/plan.md", base)
	if err != nil {
		t.Fatalf("ParseDocument first: %v", err)
	}
	doc2, err := ParseDocument("notes/plan.md", base)
	if err != nil {
		t.Fatalf("ParseDocument second: %v", err)
	}
	if doc1.ContentHash != doc2.ContentHash {
		t.Fatalf("ContentHash not stable: %q != %q", doc1.ContentHash, doc2.ContentHash)
	}
	doc3, err := ParseDocument("notes/plan.md", base+" changed")
	if err != nil {
		t.Fatalf("ParseDocument third: %v", err)
	}
	if doc1.ContentHash == doc3.ContentHash {
		t.Fatalf("ContentHash did not change for modified content: %q", doc1.ContentHash)
	}
}

func TestParseDocumentFallsBackToFilenameWhenNoHeadingTitleExists(t *testing.T) {
	doc, err := ParseDocument("debugging/auth-race.md", "No heading here\njust body text")
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if doc.Title != "auth-race" {
		t.Fatalf("Title = %q, want auth-race", doc.Title)
	}
}

func TestParseDocumentUsesFileModTimeWhenFrontmatterUpdatedAtMissing(t *testing.T) {
	modTime := time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)
	doc, err := ParseDocumentWithFileModTime("notes/plan.md", "# Plan\nBody", modTime)
	if err != nil {
		t.Fatalf("ParseDocumentWithFileModTime: %v", err)
	}
	if !doc.HasUpdatedAt {
		t.Fatal("HasUpdatedAt = false, want true from file mod time")
	}
	if !doc.UpdatedAt.Equal(modTime) {
		t.Fatalf("UpdatedAt = %v, want %v", doc.UpdatedAt, modTime)
	}
}

func TestParseDocumentIgnoresHeadingsInsideFencedCodeBlocks(t *testing.T) {
	content := "```md\n# not-a-title\n## not-a-heading\n```\n\n# Real Title\n\nBody\n\n## Real Section\n"
	doc, err := ParseDocument("notes/example.md", content)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if doc.Title != "Real Title" {
		t.Fatalf("Title = %q, want Real Title", doc.Title)
	}
	wantHeadings := []Heading{
		{Level: 1, Text: "Real Title", Line: 6},
		{Level: 2, Text: "Real Section", Line: 10},
	}
	if !reflect.DeepEqual(doc.Headings, wantHeadings) {
		t.Fatalf("Headings = %#v, want %#v", doc.Headings, wantHeadings)
	}
}

func TestParseDocumentIgnoresHeadingsAndTagsInsideTildeFences(t *testing.T) {
	content := "~~~markdown\n# fake-title\ninside #fake-tag\n~~~\n\n# Real Title\nVisible #real-tag\n"
	doc, err := ParseDocument("notes/example.md", content)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if doc.Title != "Real Title" {
		t.Fatalf("Title = %q, want Real Title", doc.Title)
	}
	wantHeadings := []Heading{{Level: 1, Text: "Real Title", Line: 6}}
	if !reflect.DeepEqual(doc.Headings, wantHeadings) {
		t.Fatalf("Headings = %#v, want %#v", doc.Headings, wantHeadings)
	}
	wantTags := []string{"real-tag"}
	if !reflect.DeepEqual(doc.Tags, wantTags) {
		t.Fatalf("Tags = %#v, want %#v", doc.Tags, wantTags)
	}
}

func TestParseDocumentSupportsLongerFencesAndRejectsInvalidClosers(t *testing.T) {
	content := "````markdown\n# still-code\ninside #still-code\n```not a closer\n## also-still-code\n````\n\n# Real Title\nVisible #real-tag\n"
	doc, err := ParseDocument("notes/example.md", content)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if doc.Title != "Real Title" {
		t.Fatalf("Title = %q, want Real Title", doc.Title)
	}
	wantHeadings := []Heading{{Level: 1, Text: "Real Title", Line: 8}}
	if !reflect.DeepEqual(doc.Headings, wantHeadings) {
		t.Fatalf("Headings = %#v, want %#v", doc.Headings, wantHeadings)
	}
	wantTags := []string{"real-tag"}
	if !reflect.DeepEqual(doc.Tags, wantTags) {
		t.Fatalf("Tags = %#v, want %#v", doc.Tags, wantTags)
	}
}

func TestParseDocumentRejectsInvalidFrontmatter(t *testing.T) {
	_, err := ParseDocument("notes/bad.md", "---\ntags: [broken\n---\n# Bad")
	if err == nil {
		t.Fatal("expected invalid frontmatter error")
	}
}
