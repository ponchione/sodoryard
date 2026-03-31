package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestExecutorPureCallsConcurrent(t *testing.T) {
	reg := NewRegistry()

	// Two pure tools that signal via channels to prove concurrent execution.
	// Each tool blocks until both have started, which can only happen if
	// they're running in separate goroutines.
	var started atomic.Int32
	gate := make(chan struct{})

	makePureTool := func(name string) *mockTool {
		m := newMockTool(name, Pure)
		m.executeFn = func(ctx context.Context, _ string, _ json.RawMessage) (*ToolResult, error) {
			if started.Add(1) == 2 {
				close(gate) // both started — unblock
			}
			select {
			case <-gate:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return &ToolResult{Success: true, Content: name + " done"}, nil
		}
		return m
	}

	reg.Register(makePureTool("read_a"))
	reg.Register(makePureTool("read_b"))

	exec := NewExecutor(reg, ExecutorConfig{}, nil)

	results := exec.Execute(context.Background(), []ToolCall{
		{ID: "tc-1", Name: "read_a", Arguments: json.RawMessage(`{}`)},
		{ID: "tc-2", Name: "read_b", Arguments: json.RawMessage(`{}`)},
	})

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if !results[0].Success || results[0].Content != "read_a done" {
		t.Fatalf("results[0] = %+v, want success with 'read_a done'", results[0])
	}
	if !results[1].Success || results[1].Content != "read_b done" {
		t.Fatalf("results[1] = %+v, want success with 'read_b done'", results[1])
	}
}

func TestExecutorMutatingCallsSequential(t *testing.T) {
	reg := NewRegistry()

	var mu sync.Mutex
	var order []string

	makeMutatingTool := func(name string) *mockTool {
		m := newMockTool(name, Mutating)
		m.executeFn = func(ctx context.Context, _ string, _ json.RawMessage) (*ToolResult, error) {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return &ToolResult{Success: true, Content: name + " done"}, nil
		}
		return m
	}

	reg.Register(makeMutatingTool("file_write"))
	reg.Register(makeMutatingTool("shell"))

	exec := NewExecutor(reg, ExecutorConfig{}, nil)

	results := exec.Execute(context.Background(), []ToolCall{
		{ID: "tc-1", Name: "file_write", Arguments: json.RawMessage(`{}`)},
		{ID: "tc-2", Name: "shell", Arguments: json.RawMessage(`{}`)},
	})

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if len(order) != 2 || order[0] != "file_write" || order[1] != "shell" {
		t.Fatalf("execution order = %v, want [file_write, shell]", order)
	}
}

func TestExecutorMixedBatchOrdering(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("file_read", Pure))
	reg.Register(newMockTool("search_text", Pure))
	reg.Register(newMockTool("file_write", Mutating))
	reg.Register(newMockTool("shell", Mutating))

	exec := NewExecutor(reg, ExecutorConfig{}, nil)

	calls := []ToolCall{
		{ID: "tc-1", Name: "file_read", Arguments: json.RawMessage(`{}`)},
		{ID: "tc-2", Name: "file_write", Arguments: json.RawMessage(`{}`)},
		{ID: "tc-3", Name: "search_text", Arguments: json.RawMessage(`{}`)},
		{ID: "tc-4", Name: "shell", Arguments: json.RawMessage(`{}`)},
	}

	results := exec.Execute(context.Background(), calls)
	if len(results) != 4 {
		t.Fatalf("got %d results, want 4", len(results))
	}

	// Results must be in the original call order (tc-1, tc-2, tc-3, tc-4)
	// regardless of purity partitioning.
	for i, result := range results {
		wantID := calls[i].ID
		if result.CallID != wantID {
			t.Fatalf("results[%d].CallID = %q, want %q", i, result.CallID, wantID)
		}
		if !result.Success {
			t.Fatalf("results[%d] failed unexpectedly: %s", i, result.Error)
		}
	}
}

func TestExecutorUnknownTool(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("file_read", Pure))

	exec := NewExecutor(reg, ExecutorConfig{}, nil)

	results := exec.Execute(context.Background(), []ToolCall{
		{ID: "tc-1", Name: "nonexistent", Arguments: json.RawMessage(`{}`)},
	})

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Success {
		t.Fatal("expected failure for unknown tool")
	}
	if !strings.Contains(results[0].Content, "Unknown tool") {
		t.Fatalf("expected 'Unknown tool' in content, got: %s", results[0].Content)
	}
	if !strings.Contains(results[0].Content, "file_read") {
		t.Fatalf("expected available tools in content, got: %s", results[0].Content)
	}
}

func TestExecutorPanicRecovery(t *testing.T) {
	reg := NewRegistry()

	panicTool := newMockTool("bad_tool", Pure)
	panicTool.executeFn = func(ctx context.Context, _ string, _ json.RawMessage) (*ToolResult, error) {
		panic("something went horribly wrong")
	}
	reg.Register(panicTool)

	exec := NewExecutor(reg, ExecutorConfig{}, nil)

	results := exec.Execute(context.Background(), []ToolCall{
		{ID: "tc-1", Name: "bad_tool", Arguments: json.RawMessage(`{}`)},
	})

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Success {
		t.Fatal("expected failure for panicking tool")
	}
	if !strings.Contains(results[0].Content, "panicked") {
		t.Fatalf("expected 'panicked' in content, got: %s", results[0].Content)
	}
	if !strings.Contains(results[0].Error, "panic") {
		t.Fatalf("expected 'panic' in error, got: %s", results[0].Error)
	}
}

func TestExecutorContextCancellation(t *testing.T) {
	reg := NewRegistry()

	// A slow mutating tool that blocks until context is cancelled.
	slowTool := newMockTool("slow_write", Mutating)
	slowTool.executeFn = func(ctx context.Context, _ string, _ json.RawMessage) (*ToolResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	reg.Register(slowTool)

	// A second mutating tool that should never execute.
	reg.Register(newMockTool("second_write", Mutating))

	exec := NewExecutor(reg, ExecutorConfig{}, nil)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	results := exec.Execute(ctx, []ToolCall{
		{ID: "tc-1", Name: "slow_write", Arguments: json.RawMessage(`{}`)},
		{ID: "tc-2", Name: "second_write", Arguments: json.RawMessage(`{}`)},
	})

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	// First tool should have context cancelled error.
	if results[0].Success {
		t.Fatal("expected failure for cancelled tool")
	}
	// Second tool should not have executed — context already cancelled.
	if results[1].Success {
		t.Fatal("expected failure for second tool (context cancelled)")
	}
	if !strings.Contains(results[1].Content, "cancelled") {
		t.Fatalf("expected 'cancelled' in second result content, got: %s", results[1].Content)
	}
}

func TestExecutorEmptyBatch(t *testing.T) {
	reg := NewRegistry()
	exec := NewExecutor(reg, ExecutorConfig{}, nil)

	results := exec.Execute(context.Background(), nil)
	if results != nil {
		t.Fatalf("expected nil for empty batch, got %v", results)
	}

	results = exec.Execute(context.Background(), []ToolCall{})
	if results != nil {
		t.Fatalf("expected nil for empty slice, got %v", results)
	}
}

func TestExecutorToolError(t *testing.T) {
	reg := NewRegistry()

	errTool := newMockTool("err_tool", Pure)
	errTool.executeFn = func(ctx context.Context, _ string, _ json.RawMessage) (*ToolResult, error) {
		return nil, errors.New("disk full")
	}
	reg.Register(errTool)

	exec := NewExecutor(reg, ExecutorConfig{}, nil)

	results := exec.Execute(context.Background(), []ToolCall{
		{ID: "tc-1", Name: "err_tool", Arguments: json.RawMessage(`{}`)},
	})

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Success {
		t.Fatal("expected failure for erroring tool")
	}
	if !strings.Contains(results[0].Content, "disk full") {
		t.Fatalf("expected 'disk full' in content, got: %s", results[0].Content)
	}
}

func TestExecutorDurationTracking(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockTool("file_read", Pure))

	// Use a deterministic time function.
	callCount := 0
	exec := NewExecutor(reg, ExecutorConfig{}, nil)
	exec.nowFn = func() time.Time {
		callCount++
		// First call is the start time, second is the end time.
		// Return times 100ms apart.
		base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		if callCount%2 == 1 {
			return base
		}
		return base.Add(100 * time.Millisecond)
	}

	results := exec.Execute(context.Background(), []ToolCall{
		{ID: "tc-1", Name: "file_read", Arguments: json.RawMessage(`{}`)},
	})

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].DurationMs != 100 {
		t.Fatalf("DurationMs = %d, want 100", results[0].DurationMs)
	}
}

// TestExecutorIntegrationMixedBatch is an integration test that registers
// a pure and a mutating tool, dispatches a mixed batch, and verifies
// execution order, result order, and field population.
func TestExecutorIntegrationMixedBatch(t *testing.T) {
	reg := NewRegistry()

	var mu sync.Mutex
	var execOrder []string

	pureTool := newMockTool("reader", Pure)
	pureTool.executeFn = func(ctx context.Context, _ string, input json.RawMessage) (*ToolResult, error) {
		mu.Lock()
		execOrder = append(execOrder, "reader")
		mu.Unlock()
		return &ToolResult{Success: true, Content: "read result"}, nil
	}

	mutTool := newMockTool("writer", Mutating)
	mutTool.executeFn = func(ctx context.Context, _ string, input json.RawMessage) (*ToolResult, error) {
		mu.Lock()
		execOrder = append(execOrder, "writer")
		mu.Unlock()
		return &ToolResult{Success: true, Content: "write result"}, nil
	}

	reg.Register(pureTool)
	reg.Register(mutTool)

	exec := NewExecutor(reg, ExecutorConfig{ProjectRoot: "/tmp/test"}, nil)

	calls := []ToolCall{
		{ID: "tc-1", Name: "reader", Arguments: json.RawMessage(`{"path":"a.go"}`)},
		{ID: "tc-2", Name: "writer", Arguments: json.RawMessage(`{"content":"hello"}`)},
		{ID: "tc-3", Name: "reader", Arguments: json.RawMessage(`{"path":"b.go"}`)},
	}

	results := exec.Execute(context.Background(), calls)

	// Verify result count and ordering.
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	for i, call := range calls {
		if results[i].CallID != call.ID {
			t.Fatalf("results[%d].CallID = %q, want %q", i, results[i].CallID, call.ID)
		}
		if !results[i].Success {
			t.Fatalf("results[%d] failed: %s", i, results[i].Error)
		}
	}

	// Verify expected content.
	if results[0].Content != "read result" {
		t.Fatalf("results[0].Content = %q, want 'read result'", results[0].Content)
	}
	if results[1].Content != "write result" {
		t.Fatalf("results[1].Content = %q, want 'write result'", results[1].Content)
	}

	// Verify all three executed.
	mu.Lock()
	if len(execOrder) != 3 {
		t.Fatalf("expected 3 executions, got %d: %v", len(execOrder), execOrder)
	}
	mu.Unlock()

	// Verify DurationMs is populated (>= 0).
	for i, r := range results {
		if r.DurationMs < 0 {
			t.Fatalf("results[%d].DurationMs = %d, want >= 0", i, r.DurationMs)
		}
	}
}
