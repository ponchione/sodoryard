package tool

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
	tracepkg "github.com/ponchione/sodoryard/internal/trace"
)

// ExecutorConfig carries executor-level configuration.
type ExecutorConfig struct {
	// MaxOutputTokens is the global truncation limit. Token estimation uses
	// chars/4 as a rough heuristic. Default: 50000.
	MaxOutputTokens int

	// ProjectRoot restricts file tool operations to this directory.
	ProjectRoot string

	// ShellApprovalPatterns require operator approval before matching shell
	// commands execute. Headless callers fail closed until a resume path exists.
	ShellApprovalPatterns []string
}

// Executor dispatches tool call batches with purity-based execution strategy.
// It is the single entry point for all tool dispatch — the agent loop never
// calls tools directly.
type Executor struct {
	registry  *Registry
	recorder  *ToolExecutionRecorder
	tracer    tracepkg.Recorder
	hooks     []Hook
	traceHook Hook
	approval  Hook
	config    ExecutorConfig
	logger    *slog.Logger
	nowFn     func() time.Time // injectable for testing
}

// NewExecutor creates an executor backed by the given registry.
// The recorder is optional — pass nil to skip tool_executions persistence.
func NewExecutor(registry *Registry, config ExecutorConfig, logger *slog.Logger) *Executor {
	if logger == nil {
		logger = slog.Default()
	}
	executor := &Executor{
		registry: registry,
		config:   config,
		logger:   logger,
		nowFn:    time.Now,
	}
	if approval := NewShellApprovalHook(config.ShellApprovalPatterns); approval != nil {
		executor.approval = approval
	}
	return executor
}

// SetRecorder attaches a tool execution recorder for analytics persistence.
// Safe to call before any Execute calls. Passing nil disables persistence.
func (e *Executor) SetRecorder(recorder *ToolExecutionRecorder) {
	e.recorder = recorder
}

func (e *Executor) SetTraceRecorder(recorder tracepkg.Recorder) {
	e.tracer = recorder
	if recorder == nil {
		e.traceHook = nil
		return
	}
	e.traceHook = traceToolHook{recorder: recorder}
}

func (e *Executor) SetHooks(hooks ...Hook) {
	e.hooks = append([]Hook(nil), hooks...)
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

	availableNames := ""
	availableNamesLoaded := false

	for i, call := range calls {
		t, ok := e.registry.Get(call.Name)
		if !ok {
			if !availableNamesLoaded {
				availableNames = strings.Join(e.registry.Names(), ", ")
				availableNamesLoaded = true
			}
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

	batchCtx, batchSpan := tracepkg.StartSpan(ctx, e.tracer, tracepkg.SpanStart{
		Name: "tool.batch",
		Kind: tracepkg.KindToolBatch,
		Attributes: map[string]any{
			"call_count":     len(calls),
			"pure_count":     len(pureCalls),
			"mutating_count": len(mutatingCalls),
		},
	})
	ctx = batchCtx

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
			results[i].OutputSize = len(results[i].Content)
			results[i].Content = NormalizeToolResult(calls[i].Name, results[i].Content)
			results[i].NormalizedSize = len(results[i].Content)
			limit := e.config.MaxOutputTokens
			if t, ok := e.registry.Get(calls[i].Name); ok {
				if ol, ok := t.(OutputLimiter); ok {
					limit = ol.OutputLimit()
				}
			}
			truncated := truncateResult(&results[i], limit, calls[i].Name)
			results[i].Details = provider.MergeToolResultDetails(results[i].Details, map[string]any{
				"original_size":   results[i].OutputSize,
				"normalized_size": results[i].NormalizedSize,
				"returned_size":   len(results[i].Content),
				"truncated":       truncated,
			})
		}
	}

	batchStatus := tracepkg.StatusOK
	var batchErr error
	if ctx.Err() != nil {
		batchStatus = tracepkg.StatusCancelled
		batchErr = ctx.Err()
	} else {
		for _, result := range results {
			if !result.Success {
				batchStatus = tracepkg.StatusError
				batchErr = fmt.Errorf("one or more tool calls failed")
				break
			}
		}
	}
	batchSpan.End(context.Background(), batchStatus, batchErr)

	return results
}

// executeSingle runs a single tool call with panic recovery and timing.
func (e *Executor) executeSingle(ctx context.Context, call ToolCall, t Tool) (result ToolResult) {
	start := e.nowFn()
	hooks := e.executionHooks()
	hookCtx, ran, beforeErr := runBeforeHooks(ctx, hooks, call, t)
	if beforeErr != nil {
		result = blockedBeforeToolResult(call, beforeErr, e.nowFn().Sub(start).Milliseconds())
		return e.finishToolHooks(hookCtx, hooks[:ran], call, result)
	}
	ctx = hookCtx

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
		result = e.finishToolHooks(ctx, hooks[:ran], call, result)
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

func (e *Executor) executionHooks() []Hook {
	hooks := make([]Hook, 0, len(e.hooks)+1)
	if e.traceHook != nil {
		hooks = append(hooks, e.traceHook)
	}
	if e.approval != nil {
		hooks = append(hooks, e.approval)
	}
	hooks = append(hooks, e.hooks...)
	return hooks
}

func blockedBeforeToolResult(call ToolCall, err error, durationMs int64) ToolResult {
	var approvalErr *ApprovalRequiredError
	if errors.As(err, &approvalErr) {
		pending := approvalErr.Pending
		return ToolResult{
			CallID:     call.ID,
			Content:    fmt.Sprintf("Approval required before executing tool %q: %s. The tool was not run.", call.Name, pending.Reason),
			Success:    false,
			Error:      ErrApprovalRequired.Error(),
			DurationMs: durationMs,
			Details:    approvalRequiredDetails(pending),
		}
	}
	return ToolResult{
		CallID:     call.ID,
		Content:    fmt.Sprintf("Tool %q blocked before execution: %v", call.Name, err),
		Success:    false,
		Error:      err.Error(),
		DurationMs: durationMs,
	}
}

func (e *Executor) finishToolHooks(ctx context.Context, hooks []Hook, call ToolCall, result ToolResult) ToolResult {
	next, err := runAfterHooks(ctx, hooks, call, result)
	if err == nil {
		return next
	}
	if next.Success {
		next.Success = false
		next.Content = fmt.Sprintf("Tool %q hook failed: %v", call.Name, err)
	}
	if next.Error == "" {
		next.Error = err.Error()
	}
	return next
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

type traceToolHook struct {
	recorder tracepkg.Recorder
}

type traceToolSpanKey struct{}

func (h traceToolHook) BeforeTool(ctx context.Context, call ToolCall, t Tool) (context.Context, error) {
	spanCtx, span := tracepkg.StartSpan(ctx, h.recorder, tracepkg.SpanStart{
		Name: "tool." + call.Name,
		Kind: tracepkg.KindTool,
		Attributes: map[string]any{
			"tool_name":    call.Name,
			"tool_call_id": call.ID,
			"purity":       t.ToolPurity().String(),
		},
	})
	return context.WithValue(spanCtx, traceToolSpanKey{}, span), nil
}

func (h traceToolHook) AfterTool(ctx context.Context, _ ToolCall, result ToolResult) (ToolResult, error) {
	span, _ := ctx.Value(traceToolSpanKey{}).(*tracepkg.ActiveSpan)
	if span == nil {
		return result, nil
	}
	result.Details = withTraceSpanDetails(result.Details, span.ID())
	status := tracepkg.StatusOK
	var spanErr error
	if !result.Success {
		status = tracepkg.StatusError
		if result.Error != "" {
			spanErr = errors.New(result.Error)
		} else {
			spanErr = errors.New(result.Content)
		}
		if ctx.Err() != nil {
			status = tracepkg.StatusCancelled
			spanErr = ctx.Err()
		}
	}
	span.End(context.Background(), status, spanErr)
	return result, nil
}

func withTraceSpanDetails(details []byte, spanID string) []byte {
	if strings.TrimSpace(spanID) == "" {
		return details
	}
	fields := map[string]any{"trace_span_id": spanID}
	if len(details) == 0 {
		return provider.NewToolResultDetails("tool_execution", fields)
	}
	return provider.MergeToolResultDetails(details, fields)
}
