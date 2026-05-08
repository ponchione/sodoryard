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
