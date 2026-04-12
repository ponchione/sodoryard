package main

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/brain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/receipt"
	toolpkg "github.com/ponchione/sodoryard/internal/tool"
)

type receiptFrontmatter = receipt.Receipt

type receiptMetrics struct {
	TurnsUsed       int
	TokensUsed      int
	DurationSeconds int
}

var fallbackReceiptStepPattern = regexp.MustCompile(`-step-(\d+)\.md$`)

func resolveReceiptPath(role string, chainID string, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	return fmt.Sprintf("receipts/%s/%s.md", strings.TrimSpace(role), strings.TrimSpace(chainID))
}

func validateReceiptContent(content string) (*receiptFrontmatter, error) {
	parsed, err := receipt.Parse([]byte(content))
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func ensureReceipt(ctx context.Context, backend brain.Backend, brainCfg appconfig.BrainConfig, role string, chainID string, receiptPath string, verdict string, finalText string, turnResult *agent.TurnResult) (string, *receiptFrontmatter, error) {
	normalizedPath, err := toolpkg.ValidateBrainWritePath(brainCfg, receiptPath)
	if err != nil {
		return "", nil, fmt.Errorf("receipt path policy: %w", err)
	}
	if backend == nil {
		return "", nil, fmt.Errorf("brain backend unavailable")
	}
	content, err := backend.ReadDocument(ctx, normalizedPath)
	if err == nil {
		receipt, validateErr := validateReceiptContent(content)
		if validateErr != nil {
			return "", nil, fmt.Errorf("invalid receipt at %s: %w", normalizedPath, validateErr)
		}
		return normalizedPath, receipt, nil
	}
	if !strings.Contains(err.Error(), "Document not found") {
		return "", nil, fmt.Errorf("read receipt %s: %w", normalizedPath, err)
	}
	fallback, receipt := formatFallbackReceipt(role, chainID, normalizedPath, verdict, finalText, turnResult)
	if err := backend.WriteDocument(ctx, normalizedPath, fallback); err != nil {
		return "", nil, fmt.Errorf("write fallback receipt %s: %w", normalizedPath, err)
	}
	return normalizedPath, receipt, nil
}

func formatFallbackReceipt(role string, chainID string, receiptPath string, verdict string, finalText string, turnResult *agent.TurnResult) (string, *receiptFrontmatter) {
	metrics := receiptMetrics{}
	if turnResult != nil {
		metrics.TurnsUsed = turnResult.IterationCount
		metrics.TokensUsed = turnResult.TotalUsage.InputTokens + turnResult.TotalUsage.OutputTokens
		metrics.DurationSeconds = int(turnResult.Duration.Round(time.Second) / time.Second)
	}
	now := time.Now().UTC()
	step := fallbackReceiptStep(receiptPath)
	receipt := &receiptFrontmatter{
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
	return content, receipt
}

func fallbackReceiptStep(receiptPath string) int {
	path := filepath.Base(strings.TrimSpace(receiptPath))
	matches := fallbackReceiptStepPattern.FindStringSubmatch(path)
	if len(matches) != 2 {
		return 1
	}
	step, err := strconv.Atoi(matches[1])
	if err != nil || step <= 0 {
		return 1
	}
	return step
}
