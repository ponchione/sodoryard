package receipt

import "testing"

func TestParseAuditFindings(t *testing.T) {
	body := `## Summary
Audit found one issue.

## Findings

### FIND-correctness-001
Severity: high
Status: open
Evidence: internal/example.go:42
Summary: The nil case can panic.
Required fix: Guard before dereferencing.

### not-a-finding
Severity: low
Status: open
`
	findings := ParseAuditFindings(body, "correctness-auditor")
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one parsed finding", findings)
	}
	got := findings[0]
	if got.ID != "FIND-correctness-001" || got.SourceRole != "correctness-auditor" || got.Severity != "high" || got.Status != "open" || got.Evidence != "internal/example.go:42" || got.Summary == "" || got.RequiredFix == "" {
		t.Fatalf("finding = %+v, want populated fields", got)
	}
}

func TestParseAuditFindingsPreservesReopenedStatus(t *testing.T) {
	body := `## Findings

### FIND-correctness-001
Severity: high
Status: reopened
Evidence: internal/example.go:42
Summary: The nil case still panics.
Required fix: Guard before dereferencing.
`
	findings := ParseAuditFindings(body, "correctness-auditor")
	if len(findings) != 1 || findings[0].Status != "reopened" {
		t.Fatalf("findings = %+v, want reopened status preserved", findings)
	}
}

func TestParseFindingResolutions(t *testing.T) {
	body := `## Findings Addressed

### FIND-correctness-001
Resolution: fixed
Files changed:
- internal/example.go
Validation:
- make test passed
`
	resolutions := ParseFindingResolutions(body)
	if len(resolutions) != 1 {
		t.Fatalf("resolutions = %+v, want one parsed resolution", resolutions)
	}
	got := resolutions[0]
	if got.ID != "FIND-correctness-001" || got.Resolution != "fixed" {
		t.Fatalf("resolution = %+v, want ID and resolution", got)
	}
	if len(got.FilesChanged) != 1 || got.FilesChanged[0] != "internal/example.go" {
		t.Fatalf("FilesChanged = %+v, want internal/example.go", got.FilesChanged)
	}
	if len(got.Validation) != 1 || got.Validation[0] != "make test passed" {
		t.Fatalf("Validation = %+v, want make test passed", got.Validation)
	}
}

func TestStructuredFindingMetadata(t *testing.T) {
	r := Receipt{Findings: []Finding{{
		ID:              "FIND-correctness-001",
		Status:          "open",
		Severity:        "high",
		File:            "internal/example.go",
		Line:            42,
		Summary:         "Nil case can panic.",
		Recommendation:  "Guard before dereferencing.",
		RequiredFix:     "Add nil guard.",
		FilesChanged:    []string{"internal/example.go"},
		Validation:      []string{"rtk make test"},
		AddressedByStep: "step-002",
	}, {
		ID:           "FIND-correctness-002",
		Status:       "addressed",
		Summary:      "Fixed validation issue.",
		FilesChanged: []string{"internal/validation.go"},
		Validation:   []string{"rtk make test"},
	}}}

	findings := StructuredAuditFindings(r, "correctness-auditor")
	if len(findings) != 1 {
		t.Fatalf("structured findings = %+v, want one active audit finding", findings)
	}
	got := findings[0]
	if got.ID != "FIND-correctness-001" || got.Evidence != "internal/example.go:42" || got.RequiredFix != "Add nil guard." {
		t.Fatalf("structured finding = %+v, want file evidence and required fix", got)
	}

	resolutions := StructuredFindingResolutions(r)
	if len(resolutions) != 1 || resolutions[0].ID != "FIND-correctness-002" || resolutions[0].Resolution != "Fixed validation issue." || len(resolutions[0].FilesChanged) != 1 {
		t.Fatalf("structured resolutions = %+v, want addressed finding resolution", resolutions)
	}
}

func TestParseValidationCommands(t *testing.T) {
	body := `## Validation

- rtk make test
- rtk make build
- rtk make test
Reviewed the changed files.
go test -tags sqlite_fts5 ./internal/chain

` + "```" + `
npm exec vitest -- run src/pages/chain-detail.test.tsx
` + "```" + `
`
	got := ParseValidationCommands(body)
	want := []string{
		"rtk make test",
		"rtk make build",
		"go test -tags sqlite_fts5 ./internal/chain",
		"npm exec vitest -- run src/pages/chain-detail.test.tsx",
	}
	if len(got) != len(want) {
		t.Fatalf("commands = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("commands[%d] = %q, want %q (all=%+v)", i, got[i], want[i], got)
		}
	}
}

func TestParseChangedFiles(t *testing.T) {
	body := `## Changed Files

- internal/example.go
- internal/example_test.go
- internal/example.go
- "docs/new file.md"
- old/name.go -> internal/name.go
- None

` + "```" + `
docs/specs/22-sequential-agent-guardrails.md
None.
` + "```" + `

## Validation
- rtk make test
`
	got := ParseChangedFiles(body)
	want := []string{
		"internal/example.go",
		"internal/example_test.go",
		"docs/new file.md",
		"old/name.go",
		"internal/name.go",
		"docs/specs/22-sequential-agent-guardrails.md",
	}
	if len(got) != len(want) {
		t.Fatalf("changed files = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("changed files[%d] = %q, want %q (all=%+v)", i, got[i], want[i], got)
		}
	}
	if !HasSection(body, "Changed Files") || HasSection(body, "Missing") {
		t.Fatalf("HasSection returned unexpected values")
	}
}
