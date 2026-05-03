package tui

import (
	"testing"

	"github.com/ponchione/sodoryard/internal/operator"
)

func TestFilterChainsMatchesVisibleFields(t *testing.T) {
	chains := []operator.ChainSummary{
		{
			ID:          "chain-alpha",
			Status:      "running",
			SourceTask:  "repair broken tests",
			SourceSpecs: []string{"docs/specs/testing.md"},
			TotalSteps:  2,
			TotalTokens: 144,
			CurrentStep: &operator.StepSummary{Role: "coder", Status: "running", Verdict: "pending", ReceiptPath: "receipts/coder/alpha.md"},
		},
		{
			ID:          "chain-bravo",
			Status:      "completed",
			SourceTask:  "write operator docs",
			SourceSpecs: []string{"docs/specs/operator.md"},
			TotalSteps:  1,
			TotalTokens: 55,
			CurrentStep: &operator.StepSummary{Role: "planner", Status: "completed", Verdict: "accepted", ReceiptPath: "receipts/planner/bravo.md"},
		},
	}

	for _, tc := range []struct {
		query string
		want  string
	}{
		{query: "alpha", want: "chain-alpha"},
		{query: "completed", want: "chain-bravo"},
		{query: "operator.md", want: "chain-bravo"},
		{query: "accepted", want: "chain-bravo"},
		{query: "144", want: "chain-alpha"},
	} {
		got := filterChains(chains, tc.query)
		if len(got) != 1 || got[0].ID != tc.want {
			t.Fatalf("filterChains(%q) = %+v, want %s", tc.query, got, tc.want)
		}
	}
}

func TestFilterReceiptItemsMatchesLoadedContent(t *testing.T) {
	items := []receiptItem{
		{Label: "orchestrator", Path: "receipts/orchestrator/chain-1.md"},
		{Label: "step 1 coder", Step: "1", Path: "receipts/coder/chain-1-step-001.md"},
	}
	loaded := &operator.ReceiptView{Step: "1", Path: "receipts/coder/chain-1-step-001.md", Content: "Detailed acceptance notes"}

	got := filterReceiptItems(items, loaded, "acceptance")
	if len(got) != 1 || got[0].Step != "1" {
		t.Fatalf("content filter = %+v, want loaded step receipt", got)
	}

	got = filterReceiptItems(items, loaded, "orchestrator")
	if len(got) != 1 || got[0].Label != "orchestrator" {
		t.Fatalf("label filter = %+v, want orchestrator", got)
	}
}
