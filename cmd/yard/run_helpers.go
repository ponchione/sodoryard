package main

import (
	"context"
	"io"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/brain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/headless"
	"github.com/ponchione/sodoryard/internal/receipt"
)

type yardReceiptFrontmatter = receipt.Receipt

func yardResolveReceiptPath(role string, chainID string, override string) string {
	return headless.ResolveReceiptPath(role, chainID, override)
}

func yardValidateReceiptContent(content string) (*yardReceiptFrontmatter, error) {
	return headless.ValidateReceiptContent(content)
}

func yardEnsureReceipt(ctx context.Context, backend brain.Backend, brainCfg appconfig.BrainConfig, role string, chainID string, receiptPath string, verdict string, finalText string, turnResult *agent.TurnResult) (string, *yardReceiptFrontmatter, error) {
	return headless.EnsureReceipt(ctx, backend, brainCfg, role, chainID, receiptPath, verdict, finalText, turnResult)
}

func yardFormatFallbackReceipt(role string, chainID string, receiptPath string, verdict string, finalText string, turnResult *agent.TurnResult) (string, *yardReceiptFrontmatter) {
	return headless.FormatFallbackReceipt(role, chainID, receiptPath, verdict, finalText, turnResult)
}

func yardFallbackReceiptStep(receiptPath string) int {
	return receipt.StepFromPath(receiptPath)
}

type yardRunProgressSink struct{ *headless.ProgressSink }

func newYardRunProgressSink(out io.Writer) *yardRunProgressSink {
	return &yardRunProgressSink{ProgressSink: headless.NewProgressSink(out)}
}

func (s *yardRunProgressSink) formatEvent(event agent.Event) string {
	return headless.FormatEvent(event)
}

func yardExceededMaxTokens(turnResult *agent.TurnResult, maxTokens int) bool {
	return headless.ExceededMaxTokens(turnResult, maxTokens)
}

func yardFinalText(turnResult *agent.TurnResult) string {
	return headless.FinalText(turnResult)
}
