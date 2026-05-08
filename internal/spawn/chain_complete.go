package spawn

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/receipt"
	"github.com/ponchione/sodoryard/internal/tool"
)

type ChainCompleteTool struct {
	Store   *chain.Store
	Backend brain.Backend
	ChainID string
	Now     func() time.Time
}

func NewChainCompleteTool(store *chain.Store, backend brain.Backend, chainID string) *ChainCompleteTool {
	return &ChainCompleteTool{Store: store, Backend: backend, ChainID: chainID, Now: time.Now}
}

func (t *ChainCompleteTool) Name() string { return "chain_complete" }
func (t *ChainCompleteTool) Description() string {
	return "Signal that the chain is complete. Provide a summary of what was accomplished."
}
func (t *ChainCompleteTool) ToolPurity() tool.Purity { return tool.Mutating }
func (t *ChainCompleteTool) Schema() json.RawMessage {
	return json.RawMessage(`{"name":"chain_complete","description":"Signal that the chain is complete. Provide a summary of what was accomplished.","input_schema":{"type":"object","properties":{"summary":{"type":"string","description":"Summary of the chain execution — what was built, what passed, any remaining concerns."},"status":{"type":"string","enum":["success","partial","failed"],"description":"Overall chain outcome."}},"required":["summary","status"]}}`)
}

type chainCompleteInput struct {
	Summary string `json:"summary"`
	Status  string `json:"status"`
}

func (t *ChainCompleteTool) Execute(ctx context.Context, projectRoot string, raw json.RawMessage) (*tool.ToolResult, error) {
	var in chainCompleteInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("chain_complete: parse input: %w", err)
	}
	status := strings.TrimSpace(in.Status)
	chainStatus := "completed"
	receiptVerdict := "completed"
	switch status {
	case "success":
		chainStatus = "completed"
		receiptVerdict = "completed"
	case "partial":
		chainStatus = "partial"
		receiptVerdict = "completed"
	case "failed":
		chainStatus = "failed"
		receiptVerdict = "blocked"
	default:
		return nil, fmt.Errorf("chain_complete: invalid status %q", in.Status)
	}
	now := t.Now
	if now == nil {
		now = time.Now
	}

	// Read real chain metrics from the store so the receipt reflects
	// actual resource usage instead of hardcoded zeros (TECH-DEBT R7).
	var turnsUsed, tokensUsed, durationSecs int
	if ch, err := t.Store.GetChain(ctx, t.ChainID); err == nil && ch != nil {
		turnsUsed = ch.TotalSteps
		tokensUsed = ch.TotalTokens
		durationSecs = ch.TotalDurationSecs
	}

	receiptPath := receipt.OrchestratorPath(t.ChainID)
	receiptBody := fmt.Sprintf(`---
agent: orchestrator
chain_id: %s
step: 1
verdict: %s
timestamp: %s
turns_used: %d
tokens_used: %d
duration_seconds: %d
---

## Summary
Status: %s

%s

## Changes
See step receipts for files and brain docs created during the chain.

## Validation
See auditor and test-writer receipts for validation performed during the chain.

## Concerns
See step receipts for concerns raised during the chain.

## Next Steps
Review the summary and any unresolved concerns before starting follow-up work.
`, t.ChainID, receiptVerdict, now().UTC().Format(time.RFC3339), turnsUsed, tokensUsed, durationSecs, status, strings.TrimSpace(in.Summary))
	if err := t.Backend.WriteDocument(ctx, receiptPath, receiptBody); err != nil {
		return nil, fmt.Errorf("chain_complete: write receipt: %w", err)
	}
	closureSummary := in.Summary
	if err := chain.ApplyTerminalChainClosure(ctx, t.Store, t.ChainID, chain.TerminalChainClosure{
		Status:    chainStatus,
		EventType: chain.EventChainCompleted,
		Summary:   &closureSummary,
		Extra:     map[string]any{"summary": in.Summary},
	}); err != nil {
		return nil, fmt.Errorf("chain_complete: update chain state: %w", err)
	}
	return &tool.ToolResult{Success: true, Content: fmt.Sprintf("Chain %s marked as %s. Receipt at %s.", t.ChainID, chainStatus, receiptPath)}, tool.ErrChainComplete
}
