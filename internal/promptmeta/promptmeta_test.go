package promptmeta

import (
	"strings"
	"testing"
)

func TestParsePromptWithoutFrontmatter(t *testing.T) {
	parsed := Parse("# Prompt\n\nBody")
	if parsed.HasFrontmatter || parsed.Body != "# Prompt\n\nBody" || len(parsed.Warnings) != 0 {
		t.Fatalf("parsed = %+v, want body unchanged and no frontmatter", parsed)
	}
}

func TestParsePromptFrontmatter(t *testing.T) {
	parsed := Parse(`---
role_key: coder
persona: Thomas
expected_tools: [brain, file, file]
receipt_schema: yard.receipt.v1
recommended_max_turns: 12
requires_structured_findings: true
---
# Prompt
`)
	if !parsed.HasFrontmatter || parsed.Body != "# Prompt\n" {
		t.Fatalf("parsed frontmatter/body = %+v, want stripped body", parsed)
	}
	if parsed.Metadata.RoleKey != "coder" || parsed.Metadata.Persona != "Thomas" || parsed.Metadata.RecommendedMaxTurns != 12 || !parsed.Metadata.RequiresStructuredFindings {
		t.Fatalf("metadata = %+v, want parsed metadata", parsed.Metadata)
	}
	if len(parsed.Metadata.ExpectedTools) != 2 || parsed.Metadata.ExpectedTools[0] != "brain" || parsed.Metadata.ExpectedTools[1] != "file" {
		t.Fatalf("expected tools = %v, want sorted unique brain/file", parsed.Metadata.ExpectedTools)
	}
}

func TestParseMalformedPromptFrontmatterWarnsAndKeepsBody(t *testing.T) {
	content := "---\nrole_key: coder\n# Prompt\n"
	parsed := Parse(content)
	if parsed.HasFrontmatter || parsed.Body != content {
		t.Fatalf("parsed = %+v, want malformed prompt body unchanged", parsed)
	}
	if len(parsed.Warnings) != 1 || parsed.Warnings[0] != "malformed prompt frontmatter" {
		t.Fatalf("warnings = %v, want malformed frontmatter warning", parsed.Warnings)
	}
}

func TestValidateRoleWarnsForMismatches(t *testing.T) {
	warnings := ValidateRole("coder", []string{"file"}, Metadata{
		RoleKey:       "reviewer",
		ExpectedTools: []string{"brain", "file"},
		ReceiptSchema: "other.schema",
	})
	for _, want := range []string{
		`role_key "reviewer" differs from configured role "coder"`,
		`expected_tools [brain,file] differ from configured tools [file]`,
		`receipt_schema "other.schema" is not yard.receipt.v1`,
	} {
		if !warningsContain(warnings, want) {
			t.Fatalf("warnings = %v, want %q", warnings, want)
		}
	}
}

func warningsContain(warnings []string, want string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, want) {
			return true
		}
	}
	return false
}
