package chain

import (
	"strings"
	"testing"
)

func TestBuildStepBriefingIncludesReceiptsManifestAndWarnings(t *testing.T) {
	briefing := BuildStepBriefing(StepBriefingInput{
		Chain: Chain{
			ID:                "chain-1",
			SourceTask:        "ship guardrails",
			Status:            "running",
			TotalSteps:        1,
			TotalTokens:       42,
			TotalDurationSecs: 7,
			ResolverLoops:     0,
			MaxSteps:          5,
			MaxResolverLoops:  2,
			MaxDurationSecs:   120,
			TokenBudget:       1000,
		},
		Steps: []Step{{
			SequenceNum: 1,
			Role:        "coder",
			Status:      "completed",
			Verdict:     "completed",
			ReceiptPath: "receipts/coder/chain-1-step-001.md",
		}},
		Events: []Event{
			{EventType: EventStepChangedFiles, EventData: `{"paths":["internal/b.go","internal/a.go"],"count":2}`},
			{EventType: EventReceiptValidation, EventData: `{"role":"coder","receipt_path":"receipts/coder/chain-1-step-001.md","warning":"missing section"}`},
			{EventType: EventReceiptFindings, EventData: `{"role":"correctness-auditor","open_finding_ids":["FIND-correctness-001"]}`},
		},
		CurrentStepSequence: 2,
		CurrentRole:         "correctness-auditor",
		ReceiptPath:         "receipts/correctness-auditor/chain-1-step-002.md",
	})

	for _, want := range []string{
		"Chain ID: chain-1",
		"Launch task: ship guardrails",
		"Current step: 2 correctness-auditor",
		"step 1 coder status=completed verdict=completed receipt=receipts/coder/chain-1-step-001.md",
		"internal/a.go",
		"internal/b.go",
		"coder receipts/coder/chain-1-step-001.md: missing section",
		"FIND-correctness-001 from correctness-auditor",
	} {
		if !strings.Contains(briefing, want) {
			t.Fatalf("briefing missing %q:\n%s", want, briefing)
		}
	}
}

func TestBuildStepBriefingIncludesLifecycleAndGuardrailFacts(t *testing.T) {
	briefing := BuildStepBriefing(StepBriefingInput{
		Chain: Chain{
			ID:                "chain-facts",
			SourceTask:        "fix lifecycle",
			Status:            "running",
			TotalSteps:        4,
			TotalTokens:       200,
			TotalDurationSecs: 20,
			ResolverLoops:     2,
			MaxSteps:          8,
			MaxResolverLoops:  3,
			MaxDurationSecs:   300,
			TokenBudget:       1000,
		},
		Steps: []Step{
			{ID: "audit-1", SequenceNum: 1, Role: "correctness-auditor", Status: "completed", Verdict: "fix_required", ReceiptPath: "receipts/correctness-auditor/chain-facts-step-001.md"},
			{ID: "resolve-1", SequenceNum: 2, Role: "resolver", Status: "completed", Verdict: "completed", ReceiptPath: "receipts/resolver/chain-facts-step-002.md"},
			{ID: "audit-2", SequenceNum: 3, Role: "correctness-auditor", Status: "completed", Verdict: "fix_required", ReceiptPath: "receipts/correctness-auditor/chain-facts-step-003.md"},
			{ID: "resolve-2", SequenceNum: 4, Role: "resolver", Status: "completed", Verdict: "completed", ReceiptPath: "receipts/resolver/chain-facts-step-004.md"},
		},
		Events: []Event{
			{ID: 1, StepID: "audit-1", EventType: EventFindingLifecycleFacts, EventData: `{"role":"correctness-auditor","facts":[{"id":"FIND-correctness-001","source_role":"correctness-auditor","action":"opened","status":"open","severity":"high","evidence":"internal/example.go:42","summary":"nil panic","required_fix":"guard nil"}]}`},
			{ID: 2, StepID: "resolve-1", EventType: EventFindingLifecycleFacts, EventData: `{"role":"resolver","facts":[{"id":"FIND-correctness-001","action":"addressed","status":"addressed","resolution":"fixed","files_changed":["internal/example.go"],"validation":["rtk make test"]}]}`},
			{ID: 3, StepID: "audit-2", EventType: EventFindingLifecycleFacts, EventData: `{"role":"correctness-auditor","facts":[{"id":"FIND-correctness-001","source_role":"correctness-auditor","action":"reopened","status":"open","summary":"nil panic still possible"}]}`},
			{ID: 4, StepID: "resolve-2", EventType: EventFindingLifecycleFacts, EventData: `{"role":"resolver","facts":[{"id":"FIND-correctness-001","action":"addressed","status":"addressed","resolution":"fixed"}]}`},
			{ID: 5, StepID: "resolve-2", EventType: EventStepGuardrailFacts, EventData: `{"role":"resolver","sequence":4,"receipt_valid":true,"changed_file_manifest_present":true,"changed_file_count":1,"changed_file_claim_present":true,"claimed_changed_files":["internal/example.go"],"changed_file_claim_matches_manifest":true,"code_index_state_supported":true,"code_index_dirty":true,"code_index_dirty_reason":"source_write","brain_index_state_supported":true,"brain_index_dirty":true,"brain_index_dirty_reason":"complete_step_with_receipt","source_writer_lock_release_attempted":true,"source_writer_lock_released":true,"addressed_ids":["FIND-correctness-001"]}`},
		},
		CurrentStepSequence: 5,
		CurrentRole:         "correctness-auditor",
		ReceiptPath:         "receipts/correctness-auditor/chain-facts-step-005.md",
	})

	for _, want := range []string{
		"Latest post-step guardrail facts:",
		"step 4 role=resolver receipt_valid=true manifest=true changed=1 claim_present=true claim_matches=true claimed=internal/example.go lock_release_attempted=true lock_released=true code_index_dirty=true code_index_reason=source_write brain_index_dirty=true brain_index_reason=complete_step_with_receipt addressed=FIND-correctness-001",
		"Open audit findings:",
		"FIND-correctness-001 from correctness-auditor status=addressed addressed=2 reopened=1 severity=high evidence=internal/example.go:42 summary=nil panic still possible required_fix=guard nil resolution=fixed files=internal/example.go validation=rtk make test",
		"Addressed audit findings:",
		"Reopened audit findings:",
		"Repeated resolver loops:",
		"flow: repeated resolver loop for FIND-correctness-001 (2 resolver receipts)",
	} {
		if !strings.Contains(briefing, want) {
			t.Fatalf("briefing missing %q:\n%s", want, briefing)
		}
	}
}
