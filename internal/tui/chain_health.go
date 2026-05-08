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
	switch strings.TrimSpace(detail.Health) {
	case "ok":
		if len(detail.Warnings) == 0 {
			return readinessOK
		}
		return readinessAttention
	case "attention":
		return readinessAttention
	case "failing":
		return readinessFailing
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
	if len(detail.Warnings) > 0 {
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

func renderChainWarnings(warnings []operator.RuntimeWarning, limit int) []string {
	if len(warnings) == 0 {
		return nil
	}
	if limit <= 0 || limit > len(warnings) {
		limit = len(warnings)
	}
	lines := []string{"", "Warnings"}
	for i := 0; i < limit; i++ {
		lines = append(lines, "- "+warnings[i].Message)
	}
	if len(warnings) > limit {
		lines = append(lines, fmt.Sprintf("- %d more warning(s)", len(warnings)-limit))
	}
	return lines
}

func renderGuardrailDetails(details operator.ChainGuardrailDetails) []string {
	if guardrailDetailsEmpty(details) {
		return nil
	}
	lines := []string{"", "Guardrails"}
	lines = append(lines, fmt.Sprintf("- findings open=%s addressed=%s closed=%s reopened=%s repeated_resolver=%s",
		joinOrNone(details.OpenFindingIDs),
		joinOrNone(details.AddressedFindingIDs),
		joinOrNone(details.ClosedFindingIDs),
		joinOrNone(details.ReopenedFindingIDs),
		joinOrNone(details.RepeatedResolverFindingIDs),
	))
	for i, finding := range details.Findings {
		if i >= 3 {
			lines = append(lines, fmt.Sprintf("- %d more finding lifecycle record(s)", len(details.Findings)-i))
			break
		}
		lines = append(lines, fmt.Sprintf("- finding id=%s source=%s status=%s addressed=%d closed=%d reopened=%d severity=%s evidence=%s resolution=%s files=%s validation=%s",
			valueOrUnknown(finding.ID),
			valueOrUnknown(finding.SourceRole),
			valueOrUnknown(finding.Status),
			finding.AddressedCount,
			finding.ClosedCount,
			finding.ReopenedCount,
			valueOrNone(finding.Severity),
			valueOrNone(finding.Evidence),
			valueOrNone(finding.Resolution),
			joinOrNone(finding.FilesChanged),
			joinOrNone(finding.Validation),
		))
	}
	lock := details.LockHealth
	lines = append(lines, fmt.Sprintf("- source_writer_lock acquired=%d released=%d unreleased=%d blocked=%d release_failed=%d heartbeat_failed=%d",
		lock.Acquired,
		lock.Released,
		lock.UnreleasedWriters,
		lock.Blocked,
		lock.ReleaseFailed,
		lock.HeartbeatFailed,
	))
	for i, manifest := range details.ChangedFiles {
		if i >= 3 {
			lines = append(lines, fmt.Sprintf("- %d more changed-file manifest(s)", len(details.ChangedFiles)-i))
			break
		}
		lines = append(lines, fmt.Sprintf("- changed_files step=%d role=%s paths=%s error=%s",
			manifest.SequenceNum,
			valueOrUnknown(manifest.Role),
			joinOrNone(manifest.Paths),
			valueOrNone(manifest.Error),
		))
	}
	for i, facts := range details.StepFacts {
		if i >= 3 {
			lines = append(lines, fmt.Sprintf("- %d more post-step fact event(s)", len(details.StepFacts)-i))
			break
		}
		lines = append(lines, fmt.Sprintf("- facts step=%d role=%s receipt_valid=%t manifest=%t changed=%d claim_present=%t claim_matches=%t claimed=%s extra=%s unclaimed=%s code_index_dirty=%t brain_index_dirty=%t lock_released=%t open=%s addressed=%s",
			facts.SequenceNum,
			valueOrUnknown(facts.Role),
			facts.ReceiptValid,
			facts.ChangedFileManifestPresent,
			facts.ChangedFileCount,
			facts.ChangedFileClaimPresent,
			facts.ChangedFileClaimMatchesManifest,
			joinOrNone(facts.ClaimedChangedFiles),
			joinOrNone(facts.ChangedFileClaimExtra),
			joinOrNone(facts.ChangedFileManifestUnclaimed),
			facts.CodeIndexDirty,
			facts.BrainIndexDirty,
			facts.SourceWriterLockReleased,
			joinOrNone(facts.OpenFindingIDs),
			joinOrNone(facts.AddressedIDs),
		))
	}
	return lines
}

func guardrailDetailsEmpty(details operator.ChainGuardrailDetails) bool {
	return len(details.OpenFindingIDs) == 0 &&
		len(details.ClosedFindingIDs) == 0 &&
		len(details.AddressedFindingIDs) == 0 &&
		len(details.ReopenedFindingIDs) == 0 &&
		len(details.RepeatedResolverFindingIDs) == 0 &&
		len(details.Findings) == 0 &&
		len(details.ChangedFiles) == 0 &&
		len(details.StepFacts) == 0 &&
		details.LockHealth == (operator.GuardrailLockHealth{})
}

func valueOrNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return trimOneLine(value, 40)
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ",")
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
