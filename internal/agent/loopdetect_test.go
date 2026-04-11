package agent

import (
	"encoding/json"
	"testing"

	"github.com/ponchione/sodoryard/internal/provider"
)

func tc(name string, inputJSON string) provider.ToolCall {
	return provider.ToolCall{
		ID:    "id",
		Name:  name,
		Input: json.RawMessage(inputJSON),
	}
}

func TestLoopDetectorNoLoopBelowThreshold(t *testing.T) {
	d := newLoopDetector(3)

	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})
	if d.isLooping() {
		t.Fatal("isLooping = true after 1 iteration, want false")
	}

	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})
	if d.isLooping() {
		t.Fatal("isLooping = true after 2 iterations, want false (threshold=3)")
	}
}

func TestLoopDetectorDetectsLoopAtThreshold(t *testing.T) {
	d := newLoopDetector(3)

	calls := []provider.ToolCall{tc("read_file", `{"path":"a.go"}`)}
	d.record(calls)
	d.record(calls)
	d.record(calls)

	if !d.isLooping() {
		t.Fatal("isLooping = false after 3 identical iterations, want true")
	}
}

func TestLoopDetectorDifferentCallsBreakLoop(t *testing.T) {
	d := newLoopDetector(3)

	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})
	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})
	d.record([]provider.ToolCall{tc("read_file", `{"path":"b.go"}`)})

	if d.isLooping() {
		t.Fatal("isLooping = true with different args, want false")
	}
}

func TestLoopDetectorDifferentToolNames(t *testing.T) {
	d := newLoopDetector(3)

	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})
	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})
	d.record([]provider.ToolCall{tc("write_file", `{"path":"a.go"}`)})

	if d.isLooping() {
		t.Fatal("isLooping = true with different tool names, want false")
	}
}

func TestLoopDetectorJSONCanonicalizes(t *testing.T) {
	d := newLoopDetector(3)

	// Same content, different key order.
	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go","mode":"text"}`)})
	d.record([]provider.ToolCall{tc("read_file", `{"mode":"text","path":"a.go"}`)})
	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go","mode":"text"}`)})

	if !d.isLooping() {
		t.Fatal("isLooping = false despite identical canonical JSON, want true")
	}
}

func TestLoopDetectorMultipleToolCalls(t *testing.T) {
	d := newLoopDetector(3)

	calls := []provider.ToolCall{
		tc("read_file", `{"path":"a.go"}`),
		tc("read_file", `{"path":"b.go"}`),
	}
	d.record(calls)
	d.record(calls)
	d.record(calls)

	if !d.isLooping() {
		t.Fatal("isLooping = false with identical multi-tool iterations, want true")
	}
}

func TestLoopDetectorMultipleToolCallsDifferentOrder(t *testing.T) {
	d := newLoopDetector(3)

	// Same tool calls but different order — should still detect (signatures are sorted).
	d.record([]provider.ToolCall{
		tc("read_file", `{"path":"a.go"}`),
		tc("read_file", `{"path":"b.go"}`),
	})
	d.record([]provider.ToolCall{
		tc("read_file", `{"path":"b.go"}`),
		tc("read_file", `{"path":"a.go"}`),
	})
	d.record([]provider.ToolCall{
		tc("read_file", `{"path":"a.go"}`),
		tc("read_file", `{"path":"b.go"}`),
	})

	if !d.isLooping() {
		t.Fatal("isLooping = false despite identical sorted signatures, want true")
	}
}

func TestLoopDetectorDifferentToolCounts(t *testing.T) {
	d := newLoopDetector(3)

	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})
	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`), tc("read_file", `{"path":"b.go"}`)})
	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})

	if d.isLooping() {
		t.Fatal("isLooping = true with different tool call counts, want false")
	}
}

func TestLoopDetectorThresholdOne(t *testing.T) {
	d := newLoopDetector(1)

	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})
	// Threshold 1 means "never loop" — can't have 1 consecutive identical.
	if d.isLooping() {
		t.Fatal("isLooping = true with threshold=1, want false")
	}
}

func TestLoopDetectorThresholdZeroDisabled(t *testing.T) {
	d := newLoopDetector(0)

	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})
	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})
	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})

	if d.isLooping() {
		t.Fatal("isLooping = true with threshold=0 (disabled), want false")
	}
}

func TestLoopDetectorNilSafe(t *testing.T) {
	var d *loopDetector
	d.record([]provider.ToolCall{tc("read_file", `{}`)})
	if d.isLooping() {
		t.Fatal("nil detector isLooping should be false")
	}
}

func TestCanonicalizeJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"sorted keys", `{"b":2,"a":1}`, `{"a":1,"b":2}`},
		{"nested sort", `{"z":{"c":3,"a":1},"a":0}`, `{"a":0,"z":{"a":1,"c":3}}`},
		{"array preserved", `{"a":[3,2,1]}`, `{"a":[3,2,1]}`},
		{"empty object", `{}`, `{}`},
		{"empty input", ``, `{}`},
		{"invalid json", `{broken`, `{broken`},
		{"string value", `"hello"`, `"hello"`},
		{"number value", `42`, `42`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalizeJSON(json.RawMessage(tt.input))
			if got != tt.want {
				t.Fatalf("canonicalizeJSON(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestToolCallSignature(t *testing.T) {
	sig := toolCallSignature("read_file", json.RawMessage(`{"path":"a.go"}`))
	want := `read_file:{"path":"a.go"}`
	if sig != want {
		t.Fatalf("toolCallSignature = %q, want %q", sig, want)
	}
}

func TestLoopDetectorResetsAfterDifferentCall(t *testing.T) {
	d := newLoopDetector(3)

	// 3 identical, then different, then 2 identical — should not loop.
	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})
	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})
	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})

	if !d.isLooping() {
		t.Fatal("should be looping after 3 identical")
	}

	d.record([]provider.ToolCall{tc("write_file", `{"path":"b.go"}`)})
	if d.isLooping() {
		t.Fatal("should not be looping after different call")
	}

	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})
	d.record([]provider.ToolCall{tc("read_file", `{"path":"a.go"}`)})
	if d.isLooping() {
		t.Fatal("should not be looping — only 2 identical after break")
	}
}
