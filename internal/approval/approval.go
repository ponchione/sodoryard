package approval

import (
	"encoding/json"
	"strings"
)

const (
	KindRequired   = "approval_required"
	ProgressPrefix = "approval_required: "
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

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}
