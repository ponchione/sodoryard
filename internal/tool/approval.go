package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/approval"
	"github.com/ponchione/sodoryard/internal/provider"
	tracepkg "github.com/ponchione/sodoryard/internal/trace"
)

const (
	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusDenied   = "denied"
	ApprovalStatusExpired  = "expired"

	ApprovalRiskHigh = "high"
)

var ErrApprovalRequired = errors.New("approval required")

// PendingApproval is the durable approval contract used when a tool call is
// valid but must not execute until an operator approves it.
type PendingApproval struct {
	ID             string          `json:"id"`
	ChainID        string          `json:"chain_id,omitempty"`
	StepID         string          `json:"step_id,omitempty"`
	ConversationID string          `json:"conversation_id,omitempty"`
	TurnNumber     int             `json:"turn_number,omitempty"`
	Iteration      int             `json:"iteration,omitempty"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input,omitempty"`
	Reason         string          `json:"reason"`
	RiskLevel      string          `json:"risk_level"`
	CreatedAt      time.Time       `json:"created_at"`
	Status         string          `json:"status"`
}

// ApprovalRequiredError lets tool hooks fail closed while preserving structured
// pending-approval metadata for the model, traces, and future resume paths.
type ApprovalRequiredError struct {
	Pending PendingApproval
}

func (e *ApprovalRequiredError) Error() string {
	reason := strings.TrimSpace(e.Pending.Reason)
	if reason == "" {
		return ErrApprovalRequired.Error()
	}
	return ErrApprovalRequired.Error() + ": " + reason
}

func (e *ApprovalRequiredError) Unwrap() error {
	return ErrApprovalRequired
}

type ShellApprovalHook struct {
	patterns []string
	nowFn    func() time.Time
}

func NewShellApprovalHook(patterns []string) *ShellApprovalHook {
	normalized := normalizeApprovalPatterns(patterns)
	if len(normalized) == 0 {
		return nil
	}
	return &ShellApprovalHook{
		patterns: normalized,
		nowFn:    time.Now,
	}
}

func normalizeApprovalPatterns(patterns []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		out = append(out, pattern)
	}
	return out
}

func (h *ShellApprovalHook) BeforeTool(ctx context.Context, call ToolCall, def Tool) (context.Context, error) {
	if h == nil || call.Name != "shell" {
		return ctx, nil
	}
	var input shellInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return ctx, nil
	}
	command := strings.TrimSpace(input.Command)
	if command == "" {
		return ctx, nil
	}
	for _, pattern := range h.patterns {
		if !shellCommandMatchesPattern(command, pattern) {
			continue
		}
		scope := tracepkg.ScopeFromContext(ctx)
		return ctx, &ApprovalRequiredError{Pending: PendingApproval{
			ID:             approvalIDForCall(call),
			ChainID:        scope.ChainID,
			StepID:         scope.StepID,
			ConversationID: scope.ConversationID,
			TurnNumber:     scope.TurnNumber,
			Iteration:      scope.Iteration,
			ToolName:       call.Name,
			ToolInput:      append(json.RawMessage(nil), call.Arguments...),
			Reason:         fmt.Sprintf("shell command matches approval pattern %q", pattern),
			RiskLevel:      ApprovalRiskHigh,
			CreatedAt:      h.now().UTC(),
			Status:         ApprovalStatusPending,
		}}
	}
	return ctx, nil
}

func (h *ShellApprovalHook) AfterTool(_ context.Context, _ ToolCall, result ToolResult) (ToolResult, error) {
	return result, nil
}

func (h *ShellApprovalHook) now() time.Time {
	if h != nil && h.nowFn != nil {
		return h.nowFn()
	}
	return time.Now()
}

func approvalIDForCall(call ToolCall) string {
	id := strings.TrimSpace(call.ID)
	if id == "" {
		id = strings.TrimSpace(call.Name)
	}
	if id == "" {
		id = "tool"
	}
	return "approval-" + id
}

func approvalRequiredDetails(pending PendingApproval) json.RawMessage {
	fields := map[string]any{
		"approval_id": pending.ID,
		"status":      pending.Status,
		"tool_name":   pending.ToolName,
		"reason":      pending.Reason,
		"risk_level":  pending.RiskLevel,
	}
	if len(pending.ToolInput) > 0 {
		fields["tool_input"] = pending.ToolInput
	}
	if !pending.CreatedAt.IsZero() {
		fields["created_at"] = pending.CreatedAt.Format(time.RFC3339Nano)
	}
	if pending.ChainID != "" {
		fields["chain_id"] = pending.ChainID
	}
	if pending.StepID != "" {
		fields["step_id"] = pending.StepID
	}
	if pending.ConversationID != "" {
		fields["conversation_id"] = pending.ConversationID
	}
	if pending.TurnNumber > 0 {
		fields["turn_number"] = pending.TurnNumber
	}
	if pending.Iteration > 0 {
		fields["iteration"] = pending.Iteration
	}
	return provider.NewToolResultDetails(approval.KindRequired, fields)
}
