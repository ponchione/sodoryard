package eval

import (
	"context"
	"encoding/json"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/provider"
	yardtool "github.com/ponchione/sodoryard/internal/tool"
)

type toolContractSuite struct{}

type toolContractShell struct {
	executed *bool
}

func (toolContractSuite) Info() SuiteInfo {
	return SuiteInfo{
		Name:        "tool-contract",
		Description: "Evaluate deterministic tool executor behavior for approval-required calls, allowed calls, and repeated call loops.",
	}
}

func (s toolContractSuite) Run(ctx context.Context) (Report, error) {
	select {
	case <-ctx.Done():
		return Report{}, ctx.Err()
	default:
	}
	report := newReport(s.Info())
	report.addCase(evaluateApprovalRequiredShell())
	report.addCase(evaluateAllowedShell())
	report.addCase(evaluateRepeatedFailingToolLoop())
	report.finalize()
	return report, nil
}

func evaluateApprovalRequiredShell() CaseResult {
	result := newCase("approval-required-shell")
	var executed bool
	executor := toolContractExecutor(&executed)
	results := executor.Execute(context.Background(), []yardtool.ToolCall{{
		ID:        "tc-approval",
		Name:      "shell",
		Arguments: json.RawMessage(`{"command":"git push --force origin main"}`),
	}})
	toolResult := firstToolContractResult(results)
	result.Details["executed"] = executed
	result.Details["error"] = toolResult.Error

	assertEqual(&result, "tool did not execute", executed, false)
	assertEqual(&result, "approval error", toolResult.Error, yardtool.ErrApprovalRequired.Error())
	assertEqual(&result, "tool result failed", toolResult.Success, false)

	details := decodeToolContractDetails(toolResult.Details)
	result.Details["detail_kind"] = details["kind"]
	result.Details["approval_status"] = details["status"]
	assertEqual(&result, "approval detail kind", stringValue(details["kind"]), "approval_required")
	assertEqual(&result, "approval status", stringValue(details["status"]), yardtool.ApprovalStatusPending)
	assertEqual(&result, "approval tool name", stringValue(details["tool_name"]), "shell")
	return result
}

func evaluateAllowedShell() CaseResult {
	result := newCase("allowed-shell")
	var executed bool
	executor := toolContractExecutor(&executed)
	results := executor.Execute(context.Background(), []yardtool.ToolCall{{
		ID:        "tc-safe",
		Name:      "shell",
		Arguments: json.RawMessage(`{"command":"git status --short"}`),
	}})
	toolResult := firstToolContractResult(results)
	result.Details["executed"] = executed
	result.Details["success"] = toolResult.Success

	assertEqual(&result, "tool executed", executed, true)
	assertEqual(&result, "tool result passed", toolResult.Success, true)
	assertEqual(&result, "tool output", toolResult.Content, "executed")
	return result
}

func evaluateRepeatedFailingToolLoop() CaseResult {
	result := newCase("repeated-failing-tool-loop")
	iterations := [][]provider.ToolCall{
		{{ID: "tc-loop-1", Name: "read_file", Input: json.RawMessage(`{"path":"missing.go"}`)}},
		{{ID: "tc-loop-2", Name: "read_file", Input: json.RawMessage(`{"path":"missing.go"}`)}},
		{{ID: "tc-loop-3", Name: "read_file", Input: json.RawMessage(`{"path":"missing.go"}`)}},
	}
	detected := agent.RepeatedToolCallLoopDetected(3, iterations)
	result.Details["threshold"] = 3
	result.Details["iterations"] = len(iterations)
	result.Details["repeated_tool"] = "read_file"
	result.Details["tool_errors"] = len(iterations)
	result.Details["loop_detected"] = detected

	assertEqual(&result, "loop detected", detected, true)
	return result
}

func toolContractExecutor(executed *bool) *yardtool.Executor {
	registry := yardtool.NewRegistry()
	registry.Register(toolContractShell{executed: executed})
	return yardtool.NewExecutor(registry, yardtool.ExecutorConfig{ShellApprovalPatterns: []string{"git push --force"}}, nil)
}

func firstToolContractResult(results []yardtool.ToolResult) yardtool.ToolResult {
	if len(results) == 0 {
		return yardtool.ToolResult{Success: false, Error: "missing tool result"}
	}
	return results[0]
}

func decodeToolContractDetails(raw json.RawMessage) map[string]any {
	var details map[string]any
	if len(raw) == 0 {
		return details
	}
	_ = json.Unmarshal(raw, &details)
	return details
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func (t toolContractShell) Name() string {
	return "shell"
}

func (t toolContractShell) Description() string {
	return "deterministic shell test tool"
}

func (t toolContractShell) ToolPurity() yardtool.Purity {
	return yardtool.Mutating
}

func (t toolContractShell) Schema() json.RawMessage {
	return json.RawMessage(`{"name":"shell","input_schema":{"type":"object"}}`)
}

func (t toolContractShell) Execute(_ context.Context, _ string, _ json.RawMessage) (*yardtool.ToolResult, error) {
	if t.executed != nil {
		*t.executed = true
	}
	return &yardtool.ToolResult{Success: true, Content: "executed"}, nil
}
