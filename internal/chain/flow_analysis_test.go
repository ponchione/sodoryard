package chain

import (
	"strings"
	"testing"
)

func TestFlowAnalyzerWarnsForSuspiciousRoleOrder(t *testing.T) {
	analysis := AnalyzeFlow(FlowAnalysisInput{
		Chain: Chain{ID: "flow-chain", Status: "completed"},
		Steps: []Step{
			{ID: "step-1", SequenceNum: 1, Role: "coder", Status: "completed"},
			{ID: "step-2", SequenceNum: 2, Role: "resolver", Status: "completed"},
		},
		Events: []Event{
			{ID: 1, StepID: "step-1", EventType: EventStepChangedFiles, EventData: `{"paths":["docs/specs/22-sequential-agent-guardrails.md"],"count":1}`},
		},
	})

	for _, want := range []string{
		"flow: coder step 1 started before planner",
		"flow: resolver step 2 ran without open findings",
		"flow: chain completed after coder step 1 without later auditor",
		"flow: docs-arbiter missing after docs-impacting changes in step 1",
	} {
		if !hasFlowWarning(analysis.Warnings, want) {
			t.Fatalf("warnings = %+v, want %q", analysis.Warnings, want)
		}
	}
}

func TestFlowAnalyzerTracksFindingLifecycle(t *testing.T) {
	analysis := AnalyzeFlow(FlowAnalysisInput{
		Chain: Chain{ID: "finding-chain", Status: "running"},
		Steps: []Step{
			{ID: "step-1", SequenceNum: 1, Role: "planner", Status: "completed"},
			{ID: "step-2", SequenceNum: 2, Role: "coder", Status: "completed"},
			{ID: "step-3", SequenceNum: 3, Role: "correctness-auditor", Status: "completed"},
			{ID: "step-4", SequenceNum: 4, Role: "resolver", Status: "completed"},
			{ID: "step-5", SequenceNum: 5, Role: "correctness-auditor", Status: "completed"},
			{ID: "step-6", SequenceNum: 6, Role: "resolver", Status: "completed"},
			{ID: "step-7", SequenceNum: 7, Role: "correctness-auditor", Status: "completed"},
			{ID: "step-8", SequenceNum: 8, Role: "resolver", Status: "completed"},
			{ID: "step-9", SequenceNum: 9, Role: "correctness-auditor", Status: "completed"},
		},
		Events: []Event{
			{ID: 1, StepID: "step-3", EventType: EventReceiptFindings, EventData: `{"role":"correctness-auditor","verdict":"fix_required","open_finding_ids":["FIND-correctness-001"]}`},
			{ID: 2, StepID: "step-4", EventType: EventReceiptFindings, EventData: `{"role":"resolver","verdict":"completed","addressed_ids":["FIND-correctness-001"],"addressed_count":1}`},
			{ID: 3, StepID: "step-5", EventType: EventReceiptFindings, EventData: `{"role":"correctness-auditor","verdict":"fix_required","open_finding_ids":["FIND-correctness-001"]}`},
			{ID: 4, StepID: "step-6", EventType: EventReceiptFindings, EventData: `{"role":"resolver","verdict":"completed","addressed_ids":["FIND-correctness-001"],"addressed_count":1}`},
			{ID: 5, StepID: "step-7", EventType: EventReceiptFindings, EventData: `{"role":"correctness-auditor","verdict":"completed","closed_finding_ids":["FIND-correctness-001"]}`},
			{ID: 6, StepID: "step-8", EventType: EventReceiptFindings, EventData: `{"role":"resolver","verdict":"completed","addressed_ids":["FIND-correctness-001"],"addressed_count":1}`},
			{ID: 7, StepID: "step-9", EventType: EventReceiptFindings, EventData: `{"role":"correctness-auditor","verdict":"fix_required","open_finding_ids":["FIND-correctness-001"]}`},
		},
	})

	if len(analysis.Findings.Findings) != 1 {
		t.Fatalf("findings = %+v, want one lifecycle entry", analysis.Findings.Findings)
	}
	got := analysis.Findings.Findings[0]
	if got.ID != "FIND-correctness-001" || got.SourceRole != "correctness-auditor" || got.Status != "open" || !got.Open {
		t.Fatalf("finding = %+v, want reopened correctness finding", got)
	}
	if got.AddressedCount != 3 || got.ClosedCount != 1 || got.ReopenedCount != 1 {
		t.Fatalf("finding counts = %+v, want addressed=3 closed=1 reopened=1", got)
	}
	if !equalStrings(analysis.Findings.OpenIDs, []string{"FIND-correctness-001"}) ||
		!equalStrings(analysis.Findings.AddressedIDs, []string{"FIND-correctness-001"}) ||
		!equalStrings(analysis.Findings.ReopenedIDs, []string{"FIND-correctness-001"}) ||
		!equalStrings(analysis.Findings.RepeatedResolverIDs, []string{"FIND-correctness-001"}) {
		t.Fatalf("finding summaries = %+v, want open/addressed/reopened/repeated ID", analysis.Findings)
	}
	if !hasFlowWarning(analysis.Warnings, "flow: repeated resolver loop for FIND-correctness-001 (2 resolver receipts)") {
		t.Fatalf("warnings = %+v, want repeated resolver loop", analysis.Warnings)
	}
}

func TestFlowAnalyzerAllowsAuditorAndDocsArbiterAfterCoder(t *testing.T) {
	analysis := AnalyzeFlow(FlowAnalysisInput{
		Chain: Chain{ID: "clean-chain", Status: "completed"},
		Steps: []Step{
			{ID: "step-1", SequenceNum: 1, Role: "planner", Status: "completed"},
			{ID: "step-2", SequenceNum: 2, Role: "coder", Status: "completed"},
			{ID: "step-3", SequenceNum: 3, Role: "correctness-auditor", Status: "completed"},
			{ID: "step-4", SequenceNum: 4, Role: "docs-arbiter", Status: "completed"},
		},
		Events: []Event{
			{ID: 1, StepID: "step-2", EventType: EventStepChangedFiles, EventData: `{"paths":["README.md"],"count":1}`},
		},
	})

	if len(analysis.Warnings) != 0 {
		t.Fatalf("warnings = %+v, want none", analysis.Warnings)
	}
}

func hasFlowWarning(warnings []FlowWarning, want string) bool {
	for _, warning := range warnings {
		if warning.Message == want || strings.Contains(warning.Message, want) {
			return true
		}
	}
	return false
}

func equalStrings(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
