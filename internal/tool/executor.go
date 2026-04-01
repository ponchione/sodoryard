package tool

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// ExecutorConfig carries executor-level configuration.
type ExecutorConfig struct {
	// MaxOutputTokens is the global truncation limit. Token estimation uses
	// chars/4 as a rough heuristic. Default: 50000.
	MaxOutputTokens int

	// ProjectRoot restricts file tool operations to this directory.
	ProjectRoot string
}

// Executor dispatches tool call batches with purity-based execution strategy.
// It is the single entry point for all tool dispatch — the agent loop never
// calls tools directly.
type Executor struct {
	registry *Registry
	recorder *ToolExecutionRecorder
	config   ExecutorConfig
	logger   *slog.Logger
	nowFn    func() time.Time // injectable for testing
}

// NewExecutor creates an executor backed by the given registry.
// The recorder is optional — pass nil to skip tool_executions persistence.
func NewExecutor(registry *Registry, config ExecutorConfig, logger *slog.Logger) *Executor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{
		registry: registry,
		config:   config,
		logger:   logger,
		nowFn:    time.Now,
	}
}

// SetRecorder attaches a tool execution recorder for analytics persistence.
// Safe to call before any Execute calls. Passing nil disables persistence.
func (e *Executor) SetRecorder(recorder *ToolExecutionRecorder) {
	e.recorder = recorder
}

// Execute dispatches a batch of tool calls with purity-based strategy:
//  1. Partition calls into pure and mutating based on registry lookup.
//  2. Execute all pure calls concurrently (goroutines + WaitGroup).
//  3. Execute mutating calls sequentially in input order.
//  4. Return results in the original call order.
//
// Tool execution errors are caught and returned as failed ToolResult values —
// they never propagate as Go errors. Only infrastructure failures (panics)
// produce error results via recovery.
func (e *Executor) Execute(ctx context.Context, calls []ToolCall) []ToolResult {
	if len(calls) == 0 {
		return nil
	}

	results := make([]ToolResult, len(calls))

	// Index calls by position and partition by purity.
	type indexedCall struct {
		index int
		call  ToolCall
		tool  Tool
	}
	var pureCalls, mutatingCalls []indexedCall

	availableNames := strings.Join(e.registry.Names(), ", ")

	for i, call := range calls {
		t, ok := e.registry.Get(call.Name)
		if !ok {
			results[i] = ToolResult{
				CallID:  call.ID,
				Content: fmt.Sprintf("Unknown tool: %q. Available tools: %s", call.Name, availableNames),
				Success: false,
				Error:   "unknown tool",
			}
			continue
		}
		ic := indexedCall{index: i, call: call, tool: t}
		if t.ToolPurity() == Pure {
			pureCalls = append(pureCalls, ic)
		} else {
			mutatingCalls = append(mutatingCalls, ic)
		}
	}

	// Execute pure calls concurrently.
	var wg sync.WaitGroup
	for _, ic := range pureCalls {
		wg.Add(1)
		go func(ic indexedCall) {
			defer wg.Done()
			results[ic.index] = e.executeSingle(ctx, ic.call, ic.tool)
		}(ic)
	}
	wg.Wait()

	// Execute mutating calls sequentially in input order.
	for _, ic := range mutatingCalls {
		if ctx.Err() != nil {
			results[ic.index] = ToolResult{
				CallID:  ic.call.ID,
				Content: "Tool execution cancelled",
				Success: false,
				Error:   ctx.Err().Error(),
			}
			continue
		}
		results[ic.index] = e.executeSingle(ctx, ic.call, ic.tool)
	}

	// Apply Phase 1 normalization then output truncation to successful results.
	for i := range results {
		if results[i].Success {
			results[i].Content = NormalizeToolResult(calls[i].Name, results[i].Content)
			limit := e.config.MaxOutputTokens
			if t, ok := e.registry.Get(calls[i].Name); ok {
				if ol, ok := t.(OutputLimiter); ok {
					limit = ol.OutputLimit()
				}
			}
			truncateResult(&results[i], limit, calls[i].Name)
		}
	}

	return results
}

// executeSingle runs a single tool call with panic recovery and timing.
func (e *Executor) executeSingle(ctx context.Context, call ToolCall, t Tool) (result ToolResult) {
	start := e.nowFn()

	// Panic recovery — tool panics become failed results, not crashes.
	defer func() {
		if r := recover(); r != nil {
			result = ToolResult{
				CallID:     call.ID,
				Content:    fmt.Sprintf("Tool %q panicked: %v", call.Name, r),
				Success:    false,
				Error:      fmt.Sprintf("panic: %v", r),
				DurationMs: e.nowFn().Sub(start).Milliseconds(),
			}
			e.logger.Error("tool panic recovered",
				"tool", call.Name,
				"call_id", call.ID,
				"panic", r,
			)
		}
	}()

	tr, err := t.Execute(ctx, e.config.ProjectRoot, call.Arguments)
	duration := e.nowFn().Sub(start)

	if err != nil {
		return ToolResult{
			CallID:     call.ID,
			Content:    fmt.Sprintf("Tool %q failed: %v", call.Name, err),
			Success:    false,
			Error:      err.Error(),
			DurationMs: duration.Milliseconds(),
		}
	}

	tr.CallID = call.ID
	tr.DurationMs = duration.Milliseconds()
	return *tr
}

// ExecuteWithMeta dispatches tool calls and records analytics for each
// execution. It delegates to Execute for the actual dispatch, then
// persists a tool_executions row per call. Database write failures are
// logged but do not affect the returned results.
func (e *Executor) ExecuteWithMeta(ctx context.Context, calls []ToolCall, meta ExecutionMeta) []ToolResult {
	results := e.Execute(ctx, calls)

	if e.recorder == nil {
		return results
	}

	now := e.nowFn()
	for i, call := range calls {
		if err := e.recorder.Record(ctx, call, results[i], meta, now); err != nil {
			e.logger.Warn("failed to record tool execution",
				"tool", call.Name,
				"call_id", call.ID,
				"error", err,
			)
		}
	}

	return results
}
