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
