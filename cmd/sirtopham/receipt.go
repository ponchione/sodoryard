package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/brain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	toolpkg "github.com/ponchione/sodoryard/internal/tool"
)

type receiptFrontmatter struct {
	Agent           string    `yaml:"agent"`
	ChainID         string    `yaml:"chain_id"`
	Step            int       `yaml:"step"`
	Verdict         string    `yaml:"verdict"`
	Timestamp       time.Time `yaml:"timestamp"`
	TurnsUsed       int       `yaml:"turns_used"`
	TokensUsed      int       `yaml:"tokens_used"`
	DurationSeconds int       `yaml:"duration_seconds"`
}

type receiptMetrics struct {
	TurnsUsed       int
	TokensUsed      int
	DurationSeconds int
}

func resolveReceiptPath(role string, chainID string, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	return fmt.Sprintf("receipts/%s/%s.md", strings.TrimSpace(role), strings.TrimSpace(chainID))
}

func validateReceiptContent(content string) (*receiptFrontmatter, error) {
	frontmatterText, _, ok := splitFrontmatter(content)
	if !ok {
		return nil, fmt.Errorf("receipt missing YAML frontmatter")
	}
	var receipt receiptFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatterText), &receipt); err != nil {
		return nil, fmt.Errorf("parse receipt frontmatter: %w", err)
	}
	if strings.TrimSpace(receipt.Agent) == "" {
		return nil, fmt.Errorf("receipt missing agent")
	}
	if strings.TrimSpace(receipt.ChainID) == "" {
		return nil, fmt.Errorf("receipt missing chain_id")
	}
	if receipt.Step <= 0 {
		return nil, fmt.Errorf("receipt missing step")
	}
	if strings.TrimSpace(receipt.Verdict) == "" {
		return nil, fmt.Errorf("receipt missing verdict")
	}
	if receipt.Timestamp.IsZero() {
		return nil, fmt.Errorf("receipt missing timestamp")
	}
	if receipt.TurnsUsed < 0 {
		return nil, fmt.Errorf("receipt missing turns_used")
	}
	if receipt.TokensUsed < 0 {
		return nil, fmt.Errorf("receipt missing tokens_used")
	}
	if receipt.DurationSeconds < 0 {
		return nil, fmt.Errorf("receipt missing duration_seconds")
	}
	return &receipt, nil
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
	fallback, receipt := formatFallbackReceipt(role, chainID, verdict, finalText, turnResult)
	if err := backend.WriteDocument(ctx, normalizedPath, fallback); err != nil {
		return "", nil, fmt.Errorf("write fallback receipt %s: %w", normalizedPath, err)
	}
	return normalizedPath, receipt, nil
}

func formatFallbackReceipt(role string, chainID string, verdict string, finalText string, turnResult *agent.TurnResult) (string, *receiptFrontmatter) {
	metrics := receiptMetrics{}
	if turnResult != nil {
		metrics.TurnsUsed = turnResult.IterationCount
		metrics.TokensUsed = turnResult.TotalUsage.InputTokens + turnResult.TotalUsage.OutputTokens
		metrics.DurationSeconds = int(turnResult.Duration.Round(time.Second) / time.Second)
	}
	now := time.Now().UTC()
	receipt := &receiptFrontmatter{
		Agent:           role,
		ChainID:         chainID,
		Step:            1,
		Verdict:         verdict,
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
step: 1
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
`, role, chainID, verdict, now.Format(time.RFC3339), metrics.TurnsUsed, metrics.TokensUsed, metrics.DurationSeconds, body)
	return content, receipt
}

func splitFrontmatter(content string) (frontmatter string, body string, ok bool) {
	trimmed := strings.TrimLeft(content, "\n")
	if !strings.HasPrefix(trimmed, "---\n") {
		return "", content, false
	}
	rest := trimmed[len("---\n"):]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return "", content, false
	}
	return rest[:idx], rest[idx+len("\n---\n"):], true
}
