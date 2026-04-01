package tooloutput

import "context"

// ResultKind classifies raw tool output so formatting and budget policy can vary.
type ResultKind string

const (
	ResultKindText  ResultKind = "text"
	ResultKindJSON  ResultKind = "json"
	ResultKindShell ResultKind = "shell"
	ResultKindDiff  ResultKind = "diff"
	ResultKindError ResultKind = "error"
)

// ReplacementReason explains why model-visible content differs from raw output.
type ReplacementReason string

const (
	ReplacementReasonNone            ReplacementReason = "none"
	ReplacementReasonPerResultBudget ReplacementReason = "per_result_budget"
	ReplacementReasonAggregateBudget ReplacementReason = "aggregate_budget"
	ReplacementReasonToolPolicy      ReplacementReason = "tool_policy"
)

// ResultEnvelope is the raw tool execution result before model-visible shaping.
type ResultEnvelope struct {
	ToolCallID string
	ToolName   string
	Kind       ResultKind
	RawText    string
	ExitCode   *int
	IsError    bool
}

// FormatPolicy controls tool-specific output shaping before budgeting.
type FormatPolicy struct {
	PreserveTail             bool
	PreserveHead             bool
	NormalizeEmptyToMessage  bool
	AllowPersistence         bool
	PreviewChars             int
	MaxVisibleCharsPerResult int
}

// BudgetPolicy controls the final visible budget across all tool results.
type BudgetPolicy struct {
	MaxVisibleCharsPerMessage int
	PreferPersistLargestFirst bool
}

// PersistedRef points to the full raw output stored outside the prompt.
type PersistedRef struct {
	RefID        string
	StoragePath  string
	ContentBytes int
	PreviewText  string
}

// VisibleResult is what the model will actually see in the next request.
type VisibleResult struct {
	ToolCallID       string
	ToolName         string
	VisibleText      string
	Persisted        *PersistedRef
	Replacement      ReplacementReason
	VisibleChars     int
	OriginalChars    int
	FormattingPolicy FormatPolicy
}

// ArtifactStore stores oversized raw outputs for later inspection or reread.
type ArtifactStore interface {
	SaveToolOutput(ctx context.Context, env ResultEnvelope) (PersistedRef, error)
}

// Formatter shapes a raw tool result before budget enforcement.
type Formatter interface {
	Format(env ResultEnvelope, policy FormatPolicy) (visibleText string, originalChars int)
}

// PolicyResolver returns tool-specific formatting policy.
type PolicyResolver interface {
	PolicyFor(toolName string, kind ResultKind) FormatPolicy
}

// Manager normalizes tool results for model visibility.
type Manager interface {
	Normalize(ctx context.Context, results []ResultEnvelope, budget BudgetPolicy) ([]VisibleResult, error)
}

// Suggested algorithm sketch:
// 1. Resolve per-tool policy and format each raw result.
// 2. Apply per-result limit; persist oversized outputs when allowed.
// 3. Compute total visible chars across all results that will be shown together.
// 4. If over budget, replace the largest remaining visible results first.
// 5. Memoize replacements by ToolCallID within the request-preparation path so replay is stable.
