package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusDenied   = "denied"
	ApprovalStatusExpired  = "expired"
)

type Approval struct {
	ID             string
	ChainID        string
	StepID         string
	ConversationID string
	TurnNumber     int
	Iteration      int
	ToolName       string
	ToolInput      json.RawMessage
	Reason         string
	RiskLevel      string
	Status         string
	CreatedAt      time.Time
	DecidedAt      *time.Time
	DecisionReason string
	DecidedBy      string
}

type ApprovalDecisionInput struct {
	ApprovalID string
	Status     string
	Reason     string
	DecidedBy  string
}

func (s *Store) ListApprovals(ctx context.Context, chainID string) ([]Approval, error) {
	events, err := s.ListEvents(ctx, chainID)
	if err != nil {
		return nil, err
	}
	return ApprovalsFromEvents(events), nil
}

func (s *Store) PendingApprovals(ctx context.Context, chainID string) ([]Approval, error) {
	approvals, err := s.ListApprovals(ctx, chainID)
	if err != nil {
		return nil, err
	}
	pending := make([]Approval, 0, len(approvals))
	for _, approval := range approvals {
		if approval.Status == ApprovalStatusPending {
			pending = append(pending, approval)
		}
	}
	return pending, nil
}

func (s *Store) GetApproval(ctx context.Context, chainID string, approvalID string) (Approval, bool, error) {
	approvals, err := s.ListApprovals(ctx, chainID)
	if err != nil {
		return Approval{}, false, err
	}
	approvalID = strings.TrimSpace(approvalID)
	for _, approval := range approvals {
		if approval.ID == approvalID {
			return approval, true, nil
		}
	}
	return Approval{}, false, nil
}

func (s *Store) RecordApprovalDecision(ctx context.Context, chainID string, in ApprovalDecisionInput) (Approval, error) {
	status := strings.TrimSpace(in.Status)
	switch status {
	case ApprovalStatusApproved, ApprovalStatusDenied:
	default:
		return Approval{}, fmt.Errorf("approval status must be %s or %s", ApprovalStatusApproved, ApprovalStatusDenied)
	}
	approvalID := strings.TrimSpace(in.ApprovalID)
	if approvalID == "" {
		return Approval{}, fmt.Errorf("approval id is required")
	}
	current, found, err := s.GetApproval(ctx, chainID, approvalID)
	if err != nil {
		return Approval{}, err
	}
	if !found {
		return Approval{}, fmt.Errorf("approval %s not found on chain %s", approvalID, chainID)
	}
	if current.Status != "" && current.Status != ApprovalStatusPending {
		return Approval{}, fmt.Errorf("approval %s is already %s", approvalID, current.Status)
	}
	decidedAt := s.clock().UTC()
	payload := map[string]any{
		"approval_id": approvalID,
		"status":      status,
		"decided_at":  decidedAt.Format(time.RFC3339Nano),
	}
	if reason := strings.TrimSpace(in.Reason); reason != "" {
		payload["reason"] = reason
	}
	if decidedBy := strings.TrimSpace(in.DecidedBy); decidedBy != "" {
		payload["decided_by"] = decidedBy
	}
	if err := s.LogEvent(ctx, chainID, current.StepID, EventApprovalDecision, payload); err != nil {
		return Approval{}, err
	}
	current.Status = status
	current.DecidedAt = &decidedAt
	current.DecisionReason = strings.TrimSpace(in.Reason)
	current.DecidedBy = strings.TrimSpace(in.DecidedBy)
	return current, nil
}

func ApprovalsFromEvents(events []Event) []Approval {
	byID := make(map[string]*Approval)
	order := make([]string, 0)
	for _, event := range events {
		switch event.EventType {
		case EventApprovalRequired:
			payload := decodeApprovalPayload(event.EventData)
			approvalID := strings.TrimSpace(payload.stringValue("approval_id"))
			toolName := strings.TrimSpace(payload.stringValue("tool_name"))
			if approvalID == "" || toolName == "" {
				continue
			}
			approval, ok := byID[approvalID]
			if !ok {
				approval = &Approval{ID: approvalID, Status: ApprovalStatusPending}
				byID[approvalID] = approval
				order = append(order, approvalID)
			}
			approval.ChainID = firstNonEmpty(payload.stringValue("chain_id"), event.ChainID, approval.ChainID)
			approval.StepID = firstNonEmpty(payload.stringValue("step_id"), event.StepID, approval.StepID)
			approval.ConversationID = firstNonEmpty(payload.stringValue("conversation_id"), approval.ConversationID)
			approval.TurnNumber = firstNonZero(payload.intValue("turn_number"), approval.TurnNumber)
			approval.Iteration = firstNonZero(payload.intValue("iteration"), approval.Iteration)
			approval.ToolName = toolName
			approval.ToolInput = payload.rawValue("tool_input", approval.ToolInput)
			approval.Reason = firstNonEmpty(payload.stringValue("reason"), approval.Reason)
			approval.RiskLevel = firstNonEmpty(payload.stringValue("risk_level"), approval.RiskLevel)
			if approval.Status == "" || approval.Status == ApprovalStatusPending {
				approval.Status = firstNonEmpty(payload.stringValue("status"), approval.Status, ApprovalStatusPending)
			}
			approval.CreatedAt = firstNonZeroTime(parsePayloadTime(payload.stringValue("created_at")), event.CreatedAt, approval.CreatedAt)
		case EventApprovalDecision:
			payload := decodeApprovalPayload(event.EventData)
			approvalID := strings.TrimSpace(payload.stringValue("approval_id"))
			if approvalID == "" {
				continue
			}
			approval, ok := byID[approvalID]
			if !ok {
				approval = &Approval{ID: approvalID, ChainID: event.ChainID}
				byID[approvalID] = approval
				order = append(order, approvalID)
			}
			approval.Status = firstNonEmpty(payload.stringValue("status"), approval.Status)
			approval.DecisionReason = firstNonEmpty(payload.stringValue("reason"), approval.DecisionReason)
			approval.DecidedBy = firstNonEmpty(payload.stringValue("decided_by"), approval.DecidedBy)
			decidedAt := firstNonZeroTime(parsePayloadTime(payload.stringValue("decided_at")), event.CreatedAt)
			if !decidedAt.IsZero() {
				approval.DecidedAt = &decidedAt
			}
		}
	}
	out := make([]Approval, 0, len(order))
	for _, approvalID := range order {
		approval := *byID[approvalID]
		if approval.Status == "" {
			approval.Status = ApprovalStatusPending
		}
		out = append(out, approval)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i].CreatedAt
		right := out[j].CreatedAt
		if left.Equal(right) {
			return out[i].ID < out[j].ID
		}
		return left.Before(right)
	})
	return out
}

type approvalPayload map[string]json.RawMessage

func decodeApprovalPayload(data string) approvalPayload {
	payload := approvalPayload{}
	_ = json.Unmarshal([]byte(strings.TrimSpace(data)), &payload)
	return payload
}

func (p approvalPayload) stringValue(key string) string {
	var value string
	if raw, ok := p[key]; ok {
		_ = json.Unmarshal(raw, &value)
	}
	return strings.TrimSpace(value)
}

func (p approvalPayload) intValue(key string) int {
	var value int
	if raw, ok := p[key]; ok {
		_ = json.Unmarshal(raw, &value)
	}
	return value
}

func (p approvalPayload) rawValue(key string, fallback json.RawMessage) json.RawMessage {
	raw, ok := p[key]
	if !ok || len(raw) == 0 {
		return append(json.RawMessage(nil), fallback...)
	}
	return append(json.RawMessage(nil), raw...)
}

func parsePayloadTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts.UTC()
		}
	}
	return time.Time{}
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Time{}
}
