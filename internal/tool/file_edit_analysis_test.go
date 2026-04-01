package tool

import (
	"strings"
	"testing"
)

func TestAnalyzeFileEditMatchesSingleLine(t *testing.T) {
	analysis := analyzeFileEditMatches("foo bar foo baz foo\n", "foo")
	if analysis.count != 3 {
		t.Fatalf("expected count 3, got %d", analysis.count)
	}
	if !strings.Contains(analysis.candidateLines, "line 1") {
		t.Fatalf("expected line summary, got %q", analysis.candidateLines)
	}
	if !strings.Contains(analysis.candidateSnippets, "line 1: foo bar foo baz foo") {
		t.Fatalf("expected snippet summary, got %q", analysis.candidateSnippets)
	}
}

func TestAnalyzeFileEditMatchesMultiline(t *testing.T) {
	content := "start\nalpha\nbeta\nmid\nalpha\nbeta\nend\n"
	analysis := analyzeFileEditMatches(content, "alpha\nbeta")
	if analysis.count != 2 {
		t.Fatalf("expected count 2, got %d", analysis.count)
	}
	if analysis.candidateLines != "line 2, line 5" {
		t.Fatalf("unexpected candidate lines: %q", analysis.candidateLines)
	}
	if !strings.Contains(analysis.candidateSnippets, "line 2: alpha\\nbeta\\nmid") {
		t.Fatalf("expected first multiline snippet, got %q", analysis.candidateSnippets)
	}
	if !strings.Contains(analysis.candidateSnippets, "line 5: alpha\\nbeta\\nend") {
		t.Fatalf("expected second multiline snippet, got %q", analysis.candidateSnippets)
	}
}

func TestAnalyzeFileEditMatchesUnknownWhenAbsent(t *testing.T) {
	analysis := analyzeFileEditMatches("hello world\n", "missing")
	if analysis.count != 0 {
		t.Fatalf("expected count 0, got %d", analysis.count)
	}
	if analysis.candidateLines != "unknown" || analysis.candidateSnippets != "unknown" {
		t.Fatalf("expected unknown diagnostics, got lines=%q snippets=%q", analysis.candidateLines, analysis.candidateSnippets)
	}
}
