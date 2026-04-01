package agent

import (
	"strings"
	"testing"
)

func TestBuildPersistedToolResultMessageIncludesStructuredReferenceAndPreview(t *testing.T) {
	ref := "/tmp/persisted/search_text-tc-1.txt"
	content := strings.Repeat("SEARCH-RESULT-LINE\n", 20)

	got := buildPersistedToolResultMessage(ref, "tc-1", "search_text", content, 220)

	for _, want := range []string{
		"[persisted_tool_result]",
		"path=/tmp/persisted/search_text-tc-1.txt",
		"tool=search_text",
		"tool_use_id=tc-1",
		"preview=",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("message missing %q: %q", want, got)
		}
	}
	if !strings.Contains(got, "SEARCH-RESULT-LINE") {
		t.Fatalf("message missing preview body: %q", got)
	}
}

func TestBuildPersistedToolResultMessageFallsBackToBarePathForTinyBudget(t *testing.T) {
	ref := "/tmp/persisted/search_text-tc-1.txt"

	got := buildPersistedToolResultMessage(ref, "tc-1", "search_text", "preview", len(ref))

	if got != ref {
		t.Fatalf("message = %q, want bare path %q", got, ref)
	}
}
