package tool

import (
	"encoding/json"
	"testing"
)

func decodeToolResultDetails(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	if len(raw) == 0 {
		t.Fatal("details are empty")
	}
	var details map[string]any
	if err := json.Unmarshal(raw, &details); err != nil {
		t.Fatalf("unmarshal details: %v", err)
	}
	return details
}

func detailInt(t *testing.T, details map[string]any, key string) int {
	t.Helper()
	value, ok := details[key].(float64)
	if !ok {
		t.Fatalf("details[%q] = %#v, want number", key, details[key])
	}
	return int(value)
}
