package eval

import (
	"context"
	"strings"

	"github.com/ponchione/sodoryard/internal/receipt"
)

type receiptContractSuite struct{}

type receiptFixture struct {
	Name                 string
	Path                 string
	ExpectedRole         string
	ExpectSchemaValid    bool
	ExpectedWarnings     []string
	ExpectedFindingIDs   []string
	ExpectedChangedFiles []string
}

var receiptFixtures = []receiptFixture{
	{
		Name:                 "coder-valid",
		Path:                 "fixtures/receipt-contract/coder-valid.md",
		ExpectedRole:         "coder",
		ExpectSchemaValid:    true,
		ExpectedChangedFiles: []string{"internal/example.go"},
	},
	{
		Name:               "auditor-open-finding",
		Path:               "fixtures/receipt-contract/auditor-open-finding.md",
		ExpectedRole:       "correctness-auditor",
		ExpectSchemaValid:  true,
		ExpectedFindingIDs: []string{"FIND-correctness-001"},
	},
	{
		Name:              "legacy-missing-schema-warning",
		Path:              "fixtures/receipt-contract/legacy-missing-schema.md",
		ExpectedRole:      "planner",
		ExpectSchemaValid: false,
		ExpectedWarnings:  []string{"missing schema_version"},
	},
}

func (receiptContractSuite) Info() SuiteInfo {
	return SuiteInfo{
		Name:        "receipt-contract",
		Description: "Parse deterministic receipt fixtures and assert schema metadata, sections, warnings, and findings.",
	}
}

func (s receiptContractSuite) Run(ctx context.Context) (Report, error) {
	select {
	case <-ctx.Done():
		return Report{}, ctx.Err()
	default:
	}
	report := newReport(s.Info())
	for _, fixture := range receiptFixtures {
		report.addCase(evaluateReceiptFixture(fixture))
	}
	report.finalize()
	return report, nil
}

func evaluateReceiptFixture(fixture receiptFixture) CaseResult {
	result := newCase(fixture.Name)
	data, err := fixtureFS.ReadFile(fixture.Path)
	if err != nil {
		result.addAssertion("read fixture", false, err.Error(), fixture.Path, nil)
		return result
	}
	result.addAssertion("read fixture", true, "", fixture.Path, fixture.Path)

	parsed, err := receipt.Parse(data)
	if err != nil {
		result.addAssertion("parse receipt", false, err.Error(), "valid receipt frontmatter", err.Error())
		return result
	}
	result.addAssertion("parse receipt", true, "", "valid receipt frontmatter", "valid receipt frontmatter")

	role := strings.TrimSpace(parsed.Role)
	if role == "" {
		role = strings.TrimSpace(parsed.Agent)
	}
	result.Details["role"] = role
	result.Details["verdict"] = string(parsed.Verdict)
	result.Details["schema_version"] = parsed.SchemaVersion
	assertEqual(&result, "role", role, fixture.ExpectedRole)

	warnings := parsed.StructuredMetadataWarnings()
	result.Warnings = append(result.Warnings, warnings...)
	schemaValid := len(warnings) == 0
	result.Details["schema_valid"] = schemaValid
	assertEqual(&result, "schema validity", schemaValid, fixture.ExpectSchemaValid)
	assertStringSet(&result, "schema warnings", warnings, fixture.ExpectedWarnings)

	requiredSections := receipt.RequiredSectionsForRole(role)
	sectionErr := receipt.ValidateRequiredSections(parsed.RawBody, requiredSections)
	requiredSectionsValid := sectionErr == nil
	result.Details["required_sections_valid"] = requiredSectionsValid
	if sectionErr != nil {
		result.addAssertion("required sections", false, sectionErr.Error(), requiredSections, sectionErr.Error())
	} else {
		result.addAssertion("required sections", true, "", requiredSections, requiredSections)
	}

	findingIDs := make([]string, 0, len(parsed.Findings))
	for _, finding := range parsed.Findings {
		findingIDs = append(findingIDs, finding.ID)
		result.Findings = append(result.Findings, FindingResult{
			ID:         finding.ID,
			SourceRole: role,
			Status:     finding.Status,
			Severity:   finding.Severity,
			Category:   finding.Category,
			Summary:    finding.Summary,
		})
	}
	result.Details["finding_count"] = len(result.Findings)
	assertStringSet(&result, "finding ids", findingIDs, fixture.ExpectedFindingIDs)
	assertStringSet(&result, "changed files", parsed.ChangedFiles, fixture.ExpectedChangedFiles)
	return result
}
