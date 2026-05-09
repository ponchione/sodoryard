package tool

import (
	"bytes"
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
var ErrApprovalDenied = errors.New("approval denied")

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

// ApprovalDeniedError preserves the durable operator decision that rejected a
// matching approval-gated tool call.
type ApprovalDeniedError struct {
	Decision approval.Decision
}

func (e *ApprovalDeniedError) Error() string {
	reason := strings.TrimSpace(e.Decision.Reason)
	if reason == "" {
		return ErrApprovalDenied.Error()
	}
	return ErrApprovalDenied.Error() + ": " + reason
}

func (e *ApprovalDeniedError) Unwrap() error {
	return ErrApprovalDenied
}

type ShellApprovalHook struct {
	patterns  []string
	decisions []approval.Decision
	nowFn     func() time.Time
}

func NewShellApprovalHook(patterns []string, decisions []approval.Decision) *ShellApprovalHook {
	normalized := normalizeApprovalPatterns(patterns)
	if len(normalized) == 0 {
		return nil
	}
	return &ShellApprovalHook{
		patterns:  normalized,
		decisions: cloneApprovalDecisions(decisions),
		nowFn:     time.Now,
	}
}

func cloneApprovalDecisions(decisions []approval.Decision) []approval.Decision {
	out := make([]approval.Decision, 0, len(decisions))
	for _, decision := range decisions {
		decision.ToolInput = append(json.RawMessage(nil), decision.ToolInput...)
		out = append(out, decision)
	}
	return out
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
		if decision, ok := h.matchingDecision(call); ok {
			switch strings.TrimSpace(decision.Status) {
			case ApprovalStatusApproved:
				return ctx, nil
			case ApprovalStatusDenied:
				return ctx, &ApprovalDeniedError{Decision: decision}
			}
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

func (h *ShellApprovalHook) matchingDecision(call ToolCall) (approval.Decision, bool) {
	if h == nil || len(h.decisions) == 0 {
		return approval.Decision{}, false
	}
	callApprovalID := approvalIDForCall(call)
	for i := len(h.decisions) - 1; i >= 0; i-- {
		decision := h.decisions[i]
		if strings.TrimSpace(decision.ToolName) != call.Name {
			continue
		}
		if len(decision.ToolInput) > 0 {
			if approvalToolInputMatches(decision.ToolInput, call.Arguments) {
				return decision, true
			}
			continue
		}
		if strings.TrimSpace(decision.ID) == callApprovalID {
			return decision, true
		}
	}
	return approval.Decision{}, false
}

func approvalToolInputMatches(a json.RawMessage, b json.RawMessage) bool {
	left, ok := canonicalApprovalJSON(a)
	if !ok {
		return false
	}
	right, ok := canonicalApprovalJSON(b)
	if !ok {
		return false
	}
	return bytes.Equal(left, right)
}

func canonicalApprovalJSON(raw json.RawMessage) ([]byte, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	return data, true
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

func approvalDeniedDetails(decision approval.Decision) json.RawMessage {
	fields := map[string]any{
		"approval_id": decision.ID,
		"status":      ApprovalStatusDenied,
		"tool_name":   decision.ToolName,
	}
	if strings.TrimSpace(decision.Reason) != "" {
		fields["reason"] = strings.TrimSpace(decision.Reason)
	}
	if len(decision.ToolInput) > 0 {
		fields["tool_input"] = append(json.RawMessage(nil), decision.ToolInput...)
	}
	return provider.NewToolResultDetails(approval.KindDenied, fields)
}
