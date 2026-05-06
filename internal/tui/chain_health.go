package tui

import (
	"fmt"
	"strings"

	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/operator"
)

const chainBudgetAttentionPct = 80.0

func chainDetailHealth(detail *operator.ChainDetail) readinessState {
	if detail == nil {
		return readinessAttention
	}
	fail := false
	attention := false
	switch detail.Chain.Status {
	case "completed", "dry_run":
	case "failed", "cancelled":
		fail = true
	case "partial", "paused", "pause_requested", "cancel_requested", "running":
		attention = true
	default:
		if strings.TrimSpace(detail.Chain.Status) != "" {
			attention = true
		}
	}
	for _, step := range detail.Steps {
		switch step.Status {
		case "failed":
			fail = true
		case "running", "pending":
			attention = true
		case "completed", "dry_run":
			if strings.TrimSpace(step.ReceiptPath) == "" {
				attention = true
			}
		}
		switch step.Verdict {
		case "fix_required", "blocked", "escalate", "safety_limit":
			fail = true
		case "completed_with_concerns", "completed_no_receipt":
			attention = true
		}
		if step.ExitCode != nil && *step.ExitCode != 0 {
			fail = true
		}
		if strings.TrimSpace(step.ErrorMessage) != "" {
			fail = true
		}
	}
	if budgetPct(maxInt(detail.Chain.TotalSteps, len(detail.Steps)), detail.Chain.MaxSteps) >= chainBudgetAttentionPct {
		attention = true
	}
	if budgetPct(detail.Chain.TotalTokens, detail.Chain.TokenBudget) >= chainBudgetAttentionPct {
		attention = true
	}
	if budgetPct(detail.Chain.TotalDurationSecs, detail.Chain.MaxDurationSecs) >= chainBudgetAttentionPct {
		attention = true
	}
	if detail.Chain.MaxResolverLoops > 0 && detail.Chain.ResolverLoops >= detail.Chain.MaxResolverLoops {
		attention = true
	}
	if fail {
		return readinessFailing
	}
	if attention {
		return readinessAttention
	}
	return readinessOK
}

func renderChainHealth(styles styles, state readinessState) string {
	switch state {
	case readinessOK:
		return styles.success.Render("ok")
	case readinessFailing:
		return styles.error.Render("failing")
	default:
		return styles.warning.Render("attention")
	}
}

func renderChainBudgetLine(ch chain.Chain, stepRows int) string {
	parts := []string{
		budgetPart("steps", maxInt(ch.TotalSteps, stepRows), ch.MaxSteps, ""),
		budgetPart("tokens", ch.TotalTokens, ch.TokenBudget, ""),
		budgetPart("duration", ch.TotalDurationSecs, ch.MaxDurationSecs, "s"),
		budgetPart("resolver", ch.ResolverLoops, ch.MaxResolverLoops, ""),
	}
	return strings.Join(parts, "  ")
}

func budgetPart(label string, used int, budget int, unit string) string {
	if budget <= 0 {
		return fmt.Sprintf("%s %d%s", label, used, unit)
	}
	return fmt.Sprintf("%s %d%s/%d%s (%.0f%%)", label, used, unit, budget, unit, budgetPct(used, budget))
}

func budgetPct(used int, budget int) float64 {
	if budget <= 0 {
		return 0
	}
	return float64(used) * 100 / float64(budget)
}

func renderCurrentStep(step *operator.StepSummary) string {
	if step == nil {
		return "none"
	}
	parts := []string{
		fmt.Sprintf("#%d", step.SequenceNum),
		valueOrUnknown(step.Role),
		valueOrUnknown(step.Status),
	}
	if strings.TrimSpace(step.Verdict) != "" {
		parts = append(parts, "verdict="+step.Verdict)
	}
	if strings.TrimSpace(step.ReceiptPath) != "" {
		parts = append(parts, "receipt="+step.ReceiptPath)
	}
	return strings.Join(parts, " ")
}

func currentStepSummary(steps []chain.Step) *operator.StepSummary {
	for i := len(steps) - 1; i >= 0; i-- {
		switch steps[i].Status {
		case "running", "pending":
			return stepSummaryFromChainStep(steps[i])
		}
	}
	if len(steps) == 0 {
		return nil
	}
	return stepSummaryFromChainStep(steps[len(steps)-1])
}

func stepSummaryFromChainStep(step chain.Step) *operator.StepSummary {
	return &operator.StepSummary{
		ID:          step.ID,
		SequenceNum: step.SequenceNum,
		Role:        step.Role,
		Status:      step.Status,
		Verdict:     step.Verdict,
		ReceiptPath: step.ReceiptPath,
		TokensUsed:  step.TokensUsed,
		StartedAt:   step.StartedAt,
		CompletedAt: step.CompletedAt,
	}
}

func renderStepLine(step chain.Step) string {
	parts := []string{
		fmt.Sprintf("%d", step.SequenceNum),
		fmt.Sprintf("%-18s", valueOrUnknown(step.Role)),
		fmt.Sprintf("%-12s", valueOrUnknown(step.Status)),
	}
	if strings.TrimSpace(step.Verdict) != "" {
		parts = append(parts, "verdict="+step.Verdict)
	}
	if step.TokensUsed > 0 {
		parts = append(parts, fmt.Sprintf("tokens=%d", step.TokensUsed))
	}
	if step.TurnsUsed > 0 {
		parts = append(parts, fmt.Sprintf("turns=%d", step.TurnsUsed))
	}
	if step.DurationSecs > 0 {
		parts = append(parts, fmt.Sprintf("duration=%ds", step.DurationSecs))
	}
	if strings.TrimSpace(step.ReceiptPath) != "" {
		parts = append(parts, "receipt="+step.ReceiptPath)
	}
	if strings.TrimSpace(step.ErrorMessage) != "" {
		parts = append(parts, "error="+trimOneLine(step.ErrorMessage, 40))
	}
	return strings.Join(parts, " ")
}
