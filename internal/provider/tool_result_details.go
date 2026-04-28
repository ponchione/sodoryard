package provider

import "encoding/json"

const (
	ToolResultDetailsVersion = 1
	ToolResultDetailsMaxSize = 32 * 1024
)

var commonToolResultDetailKeys = []string{
	"version",
	"kind",
	"summary",
	"truncated",
	"original_size",
	"normalized_size",
	"returned_size",
	"persisted_path",
}

// NewToolResultDetails builds a small JSON details envelope for first-party UI
// and analytics. The returned payload is not part of provider-visible history.
func NewToolResultDetails(kind string, fields map[string]any) json.RawMessage {
	if kind == "" {
		return nil
	}
	obj := map[string]any{
		"version": ToolResultDetailsVersion,
		"kind":    kind,
	}
	for key, value := range fields {
		if key == "" || value == nil {
			continue
		}
		obj[key] = value
	}
	return marshalCappedToolResultDetails(obj)
}

// MergeToolResultDetails overlays small common/detail fields onto an existing
// details object. Invalid or non-object details are left unchanged.
func MergeToolResultDetails(details json.RawMessage, fields map[string]any) json.RawMessage {
	if len(details) == 0 || len(fields) == 0 {
		return details
	}
	var obj map[string]any
	if err := json.Unmarshal(details, &obj); err != nil || obj == nil {
		return details
	}
	for key, value := range fields {
		if key == "" || value == nil {
			continue
		}
		obj[key] = value
	}
	return marshalCappedToolResultDetails(obj)
}

func marshalCappedToolResultDetails(obj map[string]any) json.RawMessage {
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil
	}
	if len(raw) <= ToolResultDetailsMaxSize {
		return raw
	}

	truncated := make(map[string]any, len(commonToolResultDetailKeys)+1)
	for _, key := range commonToolResultDetailKeys {
		if value, ok := obj[key]; ok {
			truncated[key] = value
		}
	}
	if _, ok := truncated["version"]; !ok {
		truncated["version"] = ToolResultDetailsVersion
	}
	truncated["details_truncated"] = true

	raw, err = json.Marshal(truncated)
	if err != nil {
		return nil
	}
	if len(raw) <= ToolResultDetailsMaxSize {
		return raw
	}

	minimal := map[string]any{
		"version":           ToolResultDetailsVersion,
		"details_truncated": true,
	}
	if kind, ok := obj["kind"]; ok {
		minimal["kind"] = kind
	}
	raw, err = json.Marshal(minimal)
	if err != nil {
		return nil
	}
	return raw
}
