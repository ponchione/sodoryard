package eval

import (
	"context"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/chain"
)

func TestListSuitesIncludesDeterministicSuites(t *testing.T) {
	suites := ListSuites()
	names := make([]string, 0, len(suites))
	for _, suite := range suites {
		names = append(names, suite.Name)
	}
	assertStringSetForTest(t, names, []string{"chain-flow", "receipt-contract"})
}

func TestReceiptContractSuitePassesAndReportsLegacyWarning(t *testing.T) {
	report, err := Run(context.Background(), "receipt-contract")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if report.Status != StatusPass {
		t.Fatalf("status = %q, want pass: %+v", report.Status, report)
	}
	if report.Totals.Cases != 3 || report.Totals.CasesPassed != 3 {
		t.Fatalf("case totals = %+v, want 3/3", report.Totals)
	}
	if report.Totals.Warnings != 1 || report.Totals.Findings != 1 {
		t.Fatalf("warning/finding totals = %+v, want warnings=1 findings=1", report.Totals)
	}
	legacy := findEvalCase(t, report, "legacy-missing-schema-warning")
	assertStringSetForTest(t, legacy.Warnings, []string{"missing schema_version"})
	auditor := findEvalCase(t, report, "auditor-open-finding")
	if len(auditor.Findings) != 1 || auditor.Findings[0].ID != "FIND-correctness-001" || auditor.Findings[0].Status != "open" {
		t.Fatalf("auditor findings = %+v, want open structured finding", auditor.Findings)
	}
}

func TestChainFlowSuitePassesAndReportsExpectedWarnings(t *testing.T) {
	report, err := Run(context.Background(), "chain-flow")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if report.Status != StatusPass {
		t.Fatalf("status = %q, want pass: %+v", report.Status, report)
	}
	warningsCase := findEvalCase(t, report, "guardrail-warnings")
	if len(warningsCase.Warnings) != 3 {
		t.Fatalf("guardrail warnings = %+v, want 3 warnings", warningsCase.Warnings)
	}
	for _, want := range []string{"coder step 1 started before planner", "resolver step 2 ran without open findings", "completed after coder step 1 without later auditor"} {
		if !caseWarningsContain(warningsCase, want) {
			t.Fatalf("warnings = %+v, want substring %q", warningsCase.Warnings, want)
		}
	}
	findingsCase := findEvalCase(t, report, "finding-lifecycle")
	if len(findingsCase.Findings) != 1 || findingsCase.Findings[0].Status != "closed" {
		t.Fatalf("finding lifecycle findings = %+v, want one closed finding", findingsCase.Findings)
	}
	conflictCase := findEvalCase(t, report, "source-writer-conflict")
	if !caseWarningsContain(conflictCase, "multiple source-writing steps running") {
		t.Fatalf("source writer warnings = %+v, want conflict warning", conflictCase.Warnings)
	}
}

func TestEvaluateChainFlowReportsStoredChainPass(t *testing.T) {
	report := EvaluateChainFlow(
		chain.Chain{ID: "chain-ok", Status: "completed"},
		[]chain.Step{
			{ID: "step-planner", ChainID: "chain-ok", SequenceNum: 1, Role: "planner", Status: "completed"},
			{ID: "step-coder", ChainID: "chain-ok", SequenceNum: 2, Role: "coder", Status: "completed"},
			{ID: "step-auditor", ChainID: "chain-ok", SequenceNum: 3, Role: "correctness-auditor", Status: "completed"},
		},
		nil,
	)
	if report.Status != StatusPass {
		t.Fatalf("status = %q, want pass: %+v", report.Status, report)
	}
	c := findEvalCase(t, report, "chain-ok")
	if c.Details["step_count"] != 3 {
		t.Fatalf("details = %+v, want step_count=3", c.Details)
	}
}

func TestEvaluateChainFlowFailsOnStoredChainWarnings(t *testing.T) {
	report := EvaluateChainFlow(
		chain.Chain{ID: "chain-warn", Status: "completed"},
		[]chain.Step{
			{ID: "step-coder", ChainID: "chain-warn", SequenceNum: 1, Role: "coder", Status: "completed"},
		},
		nil,
	)
	if report.Status != StatusFail {
		t.Fatalf("status = %q, want fail: %+v", report.Status, report)
	}
	c := findEvalCase(t, report, "chain-warn")
	if !caseWarningsContain(c, "coder step 1 started before planner") {
		t.Fatalf("warnings = %+v, want coder-before-planner warning", c.Warnings)
	}
}

func TestRunRejectsUnknownSuite(t *testing.T) {
	if _, err := Run(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), `unknown eval suite "missing"`) {
		t.Fatalf("Run unknown error = %v, want unknown suite", err)
	}
}

func findEvalCase(t *testing.T, report Report, name string) CaseResult {
	t.Helper()
	for _, c := range report.Cases {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("case %q not found in report %+v", name, report)
	return CaseResult{}
}

func caseWarningsContain(c CaseResult, want string) bool {
	for _, warning := range c.Warnings {
		if strings.Contains(warning, want) {
			return true
		}
	}
	return false
}

func assertStringSetForTest(t *testing.T, got []string, want []string) {
	t.Helper()
	got = uniqueSortedStrings(got)
	want = uniqueSortedStrings(want)
	if !equalStrings(got, want) {
		t.Fatalf("strings = %v, want %v", got, want)
	}
}
