package tool

import (
	"strings"
	"testing"
)

func TestUnifiedDiffBasic(t *testing.T) {
	old := "line one\nline two\nline three\n"
	new_ := "line one\nline TWO\nline three\n"

	diff := unifiedDiff("a/file.txt", "b/file.txt", old, new_, 3)
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(diff, "--- a/file.txt") {
		t.Fatalf("expected old file header, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+++ b/file.txt") {
		t.Fatalf("expected new file header, got:\n%s", diff)
	}
	if !strings.Contains(diff, "-line two") {
		t.Fatalf("expected removed line, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+line TWO") {
		t.Fatalf("expected added line, got:\n%s", diff)
	}
}

func TestUnifiedDiffIdentical(t *testing.T) {
	text := "same\ncontent\n"
	diff := unifiedDiff("a", "b", text, text, 3)
	if diff != "" {
		t.Fatalf("expected empty diff for identical content, got:\n%s", diff)
	}
}

func TestUnifiedDiffNewFile(t *testing.T) {
	diff := unifiedDiff("a/new.go", "b/new.go", "", "package main\n", 3)
	if diff == "" {
		t.Fatal("expected non-empty diff for new content")
	}
	if !strings.Contains(diff, "+package main") {
		t.Fatalf("expected added line, got:\n%s", diff)
	}
}

func TestUnifiedDiffDeleteFile(t *testing.T) {
	diff := unifiedDiff("a/old.go", "b/old.go", "package old\n", "", 3)
	if diff == "" {
		t.Fatal("expected non-empty diff for deleted content")
	}
	if !strings.Contains(diff, "-package old") {
		t.Fatalf("expected removed line, got:\n%s", diff)
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"one\n", 1},
		{"one\ntwo\n", 2},
		{"one\ntwo", 2},
	}
	for _, tt := range tests {
		got := splitLines(tt.input)
		if len(got) != tt.want {
			t.Errorf("splitLines(%q) = %d lines, want %d", tt.input, len(got), tt.want)
		}
	}
}
