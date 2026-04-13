package receipt

import "testing"

func TestDefaultPath(t *testing.T) {
	if got := DefaultPath("coder", "chain-1"); got != "receipts/coder/chain-1.md" {
		t.Fatalf("DefaultPath() = %q, want receipts/coder/chain-1.md", got)
	}
}

func TestStepPath(t *testing.T) {
	if got := StepPath("coder", "chain-1", 3); got != "receipts/coder/chain-1-step-003.md" {
		t.Fatalf("StepPath() = %q, want receipts/coder/chain-1-step-003.md", got)
	}
}

func TestOrchestratorPath(t *testing.T) {
	if got := OrchestratorPath("chain-1"); got != "receipts/orchestrator/chain-1.md" {
		t.Fatalf("OrchestratorPath() = %q, want receipts/orchestrator/chain-1.md", got)
	}
}

func TestStepFromPath(t *testing.T) {
	if got := StepFromPath("receipts/coder/chain-1-step-003.md"); got != 3 {
		t.Fatalf("StepFromPath(step path) = %d, want 3", got)
	}
	if got := StepFromPath("receipts/coder/chain-1.md"); got != 1 {
		t.Fatalf("StepFromPath(default path) = %d, want 1", got)
	}
}
