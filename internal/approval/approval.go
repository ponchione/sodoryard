package approval

import (
	"encoding/json"
	"strings"
)

const (
	KindRequired   = "approval_required"
	KindDenied     = "approval_denied"
	ProgressPrefix = "approval_required: "
	EnvDecisions   = "SODORYARD_APPROVAL_DECISIONS"
)

const (
	StatusApproved = "approved"
	StatusDenied   = "denied"
)

var payloadKeys = []string{
	"approval_id",
	"status",
	"tool_name",
	"reason",
	"risk_level",
	"tool_input",
	"created_at",
	"chain_id",
	"step_id",
	"conversation_id",
	"turn_number",
	"iteration",
}

type Decision struct {
	ID        string          `json:"approval_id"`
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input,omitempty"`
	Status    string          `json:"status"`
	Reason    string          `json:"reason,omitempty"`
}

func PayloadFromToolResultDetails(details json.RawMessage) (map[string]any, bool) {
	if len(details) == 0 {
		return nil, false
	}
	var decoded map[string]any
	if err := json.Unmarshal(details, &decoded); err != nil {
		return nil, false
	}
	if stringValue(decoded["kind"]) != KindRequired {
		return nil, false
	}
	payload := make(map[string]any, len(payloadKeys))
	for _, key := range payloadKeys {
		if value, ok := decoded[key]; ok {
			payload[key] = value
		}
	}
	if !validPayload(payload) {
		return nil, false
	}
	return payload, true
}

func EncodeProgressLine(payload map[string]any) string {
	if !validPayload(payload) {
		return ""
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return ProgressPrefix + string(data)
}

func PayloadFromProgressLine(line string) (map[string]any, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, ProgressPrefix) {
		return nil, false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(trimmed, ProgressPrefix))), &payload); err != nil {
		return nil, false
	}
	if !validPayload(payload) {
		return nil, false
	}
	return payload, true
}

func EncodeDecisionEnv(decisions []Decision) string {
	filtered := make([]Decision, 0, len(decisions))
	for _, decision := range decisions {
		if !validDecision(decision) {
			continue
		}
		filtered = append(filtered, decision)
	}
	if len(filtered) == 0 {
		return ""
	}
	data, err := json.Marshal(filtered)
	if err != nil {
		return ""
	}
	return string(data)
}

func DecodeDecisionEnv(raw string) []Decision {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var decisions []Decision
	if err := json.Unmarshal([]byte(raw), &decisions); err != nil {
		return nil
	}
	out := make([]Decision, 0, len(decisions))
	for _, decision := range decisions {
		if validDecision(decision) {
			out = append(out, decision)
		}
	}
	return out
}

func WithDefaultChainStep(payload map[string]any, chainID string, stepID string) map[string]any {
	if payload == nil {
		return nil
	}
	copied := make(map[string]any, len(payload)+2)
	for key, value := range payload {
		copied[key] = value
	}
	if strings.TrimSpace(chainID) != "" && strings.TrimSpace(stringValue(copied["chain_id"])) == "" {
		copied["chain_id"] = strings.TrimSpace(chainID)
	}
	if strings.TrimSpace(stepID) != "" && strings.TrimSpace(stringValue(copied["step_id"])) == "" {
		copied["step_id"] = strings.TrimSpace(stepID)
	}
	return copied
}

func StepID(payload map[string]any) string {
	return strings.TrimSpace(stringValue(payload["step_id"]))
}

func validPayload(payload map[string]any) bool {
	return strings.TrimSpace(stringValue(payload["approval_id"])) != "" && strings.TrimSpace(stringValue(payload["tool_name"])) != ""
}

func validDecision(decision Decision) bool {
	status := strings.TrimSpace(decision.Status)
	if status != StatusApproved && status != StatusDenied {
		return false
	}
	return strings.TrimSpace(decision.ID) != "" && strings.TrimSpace(decision.ToolName) != ""
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}
