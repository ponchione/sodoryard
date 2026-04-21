package main

import (
	"context"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/brain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/headless"
	"github.com/ponchione/sodoryard/internal/receipt"
)

type receiptFrontmatter = receipt.Receipt

func resolveReceiptPath(role string, chainID string, override string) string {
	return headless.ResolveReceiptPath(role, chainID, override)
}

func validateReceiptContent(content string) (*receiptFrontmatter, error) {
	return headless.ValidateReceiptContent(content)
}

func ensureReceipt(ctx context.Context, backend brain.Backend, brainCfg appconfig.BrainConfig, role string, chainID string, receiptPath string, verdict string, finalText string, turnResult *agent.TurnResult) (string, *receiptFrontmatter, error) {
	return headless.EnsureReceipt(ctx, backend, brainCfg, role, chainID, receiptPath, verdict, finalText, turnResult)
}

func formatFallbackReceipt(role string, chainID string, receiptPath string, verdict string, finalText string, turnResult *agent.TurnResult) (string, *receiptFrontmatter) {
	return headless.FormatFallbackReceipt(role, chainID, receiptPath, verdict, finalText, turnResult)
}

func fallbackReceiptStep(receiptPath string) int {
	return receipt.StepFromPath(receiptPath)
}
