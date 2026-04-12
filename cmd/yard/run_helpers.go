package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/brain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/receipt"
	toolpkg "github.com/ponchione/sodoryard/internal/tool"
)

type yardReceiptFrontmatter = receipt.Receipt

type yardReceiptMetrics struct {
	TurnsUsed       int
	TokensUsed      int
	DurationSeconds int
}

var yardFallbackReceiptStepPattern = regexp.MustCompile(`-step-(\d+)\.md$`)

func yardResolveReceiptPath(role string, chainID string, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	return fmt.Sprintf("receipts/%s/%s.md", strings.TrimSpace(role), strings.TrimSpace(chainID))
}

func yardValidateReceiptContent(content string) (*yardReceiptFrontmatter, error) {
	parsed, err := receipt.Parse([]byte(content))
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func yardEnsureReceipt(ctx context.Context, backend brain.Backend, brainCfg appconfig.BrainConfig, role string, chainID string, receiptPath string, verdict string, finalText string, turnResult *agent.TurnResult) (string, *yardReceiptFrontmatter, error) {
	normalizedPath, err := toolpkg.ValidateBrainWritePath(brainCfg, receiptPath)
	if err != nil {
		return "", nil, fmt.Errorf("receipt path policy: %w", err)
	}
	if backend == nil {
		return "", nil, fmt.Errorf("brain backend unavailable")
	}
	content, err := backend.ReadDocument(ctx, normalizedPath)
	if err == nil {
		r, validateErr := yardValidateReceiptContent(content)
		if validateErr != nil {
			return "", nil, fmt.Errorf("invalid receipt at %s: %w", normalizedPath, validateErr)
		}
		return normalizedPath, r, nil
	}
	if !strings.Contains(err.Error(), "Document not found") {
		return "", nil, fmt.Errorf("read receipt %s: %w", normalizedPath, err)
	}
	fallback, r := yardFormatFallbackReceipt(role, chainID, normalizedPath, verdict, finalText, turnResult)
	if err := backend.WriteDocument(ctx, normalizedPath, fallback); err != nil {
		return "", nil, fmt.Errorf("write fallback receipt %s: %w", normalizedPath, err)
	}
	return normalizedPath, r, nil
}

func yardFormatFallbackReceipt(role string, chainID string, receiptPath string, verdict string, finalText string, turnResult *agent.TurnResult) (string, *yardReceiptFrontmatter) {
	metrics := yardReceiptMetrics{}
	if turnResult != nil {
		metrics.TurnsUsed = turnResult.IterationCount
		metrics.TokensUsed = turnResult.TotalUsage.InputTokens + turnResult.TotalUsage.OutputTokens
		metrics.DurationSeconds = int(turnResult.Duration.Round(time.Second) / time.Second)
	}
	now := time.Now().UTC()
	step := yardFallbackReceiptStep(receiptPath)
	r := &yardReceiptFrontmatter{
		Agent:           role,
		ChainID:         chainID,
		Step:            step,
		Verdict:         receipt.Verdict(verdict),
		Timestamp:       now,
		TurnsUsed:       metrics.TurnsUsed,
		TokensUsed:      metrics.TokensUsed,
		DurationSeconds: metrics.DurationSeconds,
	}
	body := strings.TrimSpace(finalText)
	if body == "" {
		body = "No final text was returned."
	}
	content := fmt.Sprintf(`---
agent: %s
chain_id: %s
step: %d
verdict: %s
timestamp: %s
turns_used: %d
tokens_used: %d
duration_seconds: %d
---

## Summary
%s

## Changes
- No agent-authored receipt was found; this fallback receipt was written by the harness.

## Concerns
- Review the final text and session logs if more detail is needed.

## Next Steps
- Inspect the task outcome and decide whether follow-up work is needed.
`, role, chainID, step, verdict, now.Format(time.RFC3339), metrics.TurnsUsed, metrics.TokensUsed, metrics.DurationSeconds, body)
	return content, r
}

func yardFallbackReceiptStep(receiptPath string) int {
	path := filepath.Base(strings.TrimSpace(receiptPath))
	matches := yardFallbackReceiptStepPattern.FindStringSubmatch(path)
	if len(matches) != 2 {
		return 1
	}
	step, err := strconv.Atoi(matches[1])
	if err != nil || step <= 0 {
		return 1
	}
	return step
}

type yardRunProgressSink struct {
	mu  sync.Mutex
	out io.Writer
}

func newYardRunProgressSink(out io.Writer) *yardRunProgressSink {
	return &yardRunProgressSink{out: out}
}

func (s *yardRunProgressSink) Emit(event agent.Event) {
	if s == nil || s.out == nil || event == nil {
		return
	}
	line := s.formatEvent(event)
	if line == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = fmt.Fprintln(s.out, line)
}

func (s *yardRunProgressSink) Close() {}

func (s *yardRunProgressSink) formatEvent(event agent.Event) string {
	switch e := event.(type) {
	case agent.StatusEvent:
		return fmt.Sprintf("status: %s", e.State)
	case agent.ContextDebugEvent:
		if e.Report == nil {
			return "context: assembled"
		}
		return fmt.Sprintf("context: assembled rag=%d brain=%d explicit_files=%d", len(e.Report.RAGResults), len(e.Report.BrainResults), len(e.Report.ExplicitFileResults))
	case agent.ToolCallStartEvent:
		args := ""
		if len(e.Arguments) > 0 {
			var compact map[string]any
			if json.Unmarshal(e.Arguments, &compact) == nil {
				if marshaled, err := json.Marshal(compact); err == nil {
					args = " " + string(marshaled)
				}
			}
		}
		return fmt.Sprintf("tool: start %s%s", e.ToolName, args)
	case agent.ToolCallEndEvent:
		return fmt.Sprintf("tool: end %s success=%t duration=%s", e.ToolCallID, e.Success, e.Duration)
	case agent.TurnCompleteEvent:
		return fmt.Sprintf("complete: iterations=%d input_tokens=%d output_tokens=%d duration=%s", e.IterationCount, e.TotalInputTokens, e.TotalOutputTokens, e.Duration)
	case agent.ErrorEvent:
		return fmt.Sprintf("error: %s", e.Message)
	default:
		return ""
	}
}

func yardExceededMaxTokens(turnResult *agent.TurnResult, maxTokens int) bool {
	if turnResult == nil || maxTokens <= 0 {
		return false
	}
	used := turnResult.TotalUsage.InputTokens + turnResult.TotalUsage.OutputTokens
	return used >= maxTokens
}

func yardFinalText(turnResult *agent.TurnResult) string {
	if turnResult == nil {
		return ""
	}
	return strings.TrimSpace(turnResult.FinalText)
}
