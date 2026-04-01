package tokenbudget

// Estimate is a preflight estimate of request cost.
type Estimate struct {
	PromptTokensEstimate int
	ReservedOutputTokens int
	AvailableInputTokens int
}

// UsageSnapshot captures actual usage returned by the provider, when available.
type UsageSnapshot struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// OverflowDecision describes what to do when the request does not fit budget.
type OverflowDecision string

const (
	OverflowDecisionCompressHistory     OverflowDecision = "compress_history"
	OverflowDecisionPersistMoreToolData OverflowDecision = "persist_more_tool_data"
	OverflowDecisionTrimOptionalContext OverflowDecision = "trim_optional_context"
	OverflowDecisionFail                OverflowDecision = "fail"
)

// ReservePolicy controls output-space reservation.
type ReservePolicy struct {
	MinOutputTokens int
	TargetOutputTokens int
}

// BudgetTracker combines rough estimation, reserve policy, and actual-usage reconciliation.
type BudgetTracker interface {
	Estimate(promptText string, reserve ReservePolicy) (Estimate, error)
	Reconcile(estimate Estimate, usage UsageSnapshot) error
	DecideOnOverflow(estimate Estimate) OverflowDecision
}
