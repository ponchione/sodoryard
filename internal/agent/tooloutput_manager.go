package agent

import (
	"context"

	"github.com/ponchione/sirtopham/internal/provider"
)

// ToolOutputManager is the explicit domain seam for model-visible tool-result
// shaping. It centralizes aggregate budgeting and persisted-output replacement
// policy while keeping the loop wiring narrow.
type ToolOutputManager struct {
	store ToolResultStore
}

type ManagedToolResults struct {
	Results []provider.ToolResult
	Report  AggregateToolResultBudgetReport
}

func NewToolOutputManager(store ToolResultStore) *ToolOutputManager {
	return &ToolOutputManager{store: store}
}

func (m *ToolOutputManager) ApplyAggregateBudget(ctx context.Context, results []provider.ToolResult, toolCalls []provider.ToolCall, maxChars int) ManagedToolResults {
	if m == nil {
		budgeted, report := applyAggregateToolResultBudget(ctx, nil, results, toolCalls, maxChars)
		return ManagedToolResults{Results: budgeted, Report: report}
	}
	budgeted, report := applyAggregateToolResultBudget(ctx, m.store, results, toolCalls, maxChars)
	return ManagedToolResults{Results: budgeted, Report: report}
}
