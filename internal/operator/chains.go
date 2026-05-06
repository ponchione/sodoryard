package operator

import (
	"context"
	"errors"
	"fmt"

	"github.com/ponchione/sodoryard/internal/chain"
)

const defaultChainListLimit = 20
const chainMetricsBudgetWarningPct = 80.0

func (s *Service) ListChains(ctx context.Context, limit int) ([]ChainSummary, error) {
	chains, err := s.listChains(ctx, normalizeLimit(limit))
	if err != nil {
		return nil, err
	}
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	summaries := make([]ChainSummary, 0, len(chains))
	for _, ch := range chains {
		steps, err := store.ListSteps(ctx, ch.ID)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summarizeChain(ch, steps))
	}
	return summaries, nil
}

func (s *Service) GetChainDetail(ctx context.Context, chainID string) (ChainDetail, error) {
	store, err := s.store()
	if err != nil {
		return ChainDetail{}, err
	}
	ch, err := store.GetChain(ctx, chainID)
	if err != nil {
		return ChainDetail{}, err
	}
	steps, err := store.ListSteps(ctx, chainID)
	if err != nil {
		return ChainDetail{}, err
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		return ChainDetail{}, err
	}
	receipts := s.receiptSummaries(ctx, chainID, steps)
	return ChainDetail{Chain: *ch, Steps: steps, Receipts: receipts, RecentEvents: events}, nil
}

func (s *Service) GetChainMetrics(ctx context.Context, chainID string) (ChainMetricsReport, error) {
	detail, err := s.GetChainDetail(ctx, chainID)
	if err != nil {
		return ChainMetricsReport{}, err
	}
	return summarizeChainMetrics(detail), nil
}

func (s *Service) ListEvents(ctx context.Context, chainID string) ([]chain.Event, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	return store.ListEvents(ctx, chainID)
}

func (s *Service) ListEventsSince(ctx context.Context, chainID string, afterID int64) ([]chain.Event, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	return store.ListEventsSince(ctx, chainID, afterID)
}

func (s *Service) listChains(ctx context.Context, limit int) ([]chain.Chain, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	return store.ListChains(ctx, normalizeLimit(limit))
}

func (s *Service) store() (*chain.Store, error) {
	if s == nil || s.rt == nil {
		return nil, errors.New("operator service is closed")
	}
	if s.rt.ChainStore == nil {
		return nil, errors.New("operator runtime chain store is nil")
	}
	return s.rt.ChainStore, nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultChainListLimit
	}
	return limit
}

func summarizeChain(ch chain.Chain, steps []chain.Step) ChainSummary {
	return ChainSummary{
		ID:          ch.ID,
		Status:      ch.Status,
		SourceTask:  ch.SourceTask,
		SourceSpecs: append([]string(nil), ch.SourceSpecs...),
		TotalSteps:  ch.TotalSteps,
		TotalTokens: ch.TotalTokens,
		StartedAt:   ch.StartedAt,
		UpdatedAt:   ch.UpdatedAt,
		CurrentStep: summarizeCurrentStep(steps),
	}
}

func summarizeChainMetrics(detail ChainDetail) ChainMetricsReport {
	ch := detail.Chain
	report := ChainMetricsReport{
		ChainID:           ch.ID,
		Status:            ch.Status,
		Health:            "ok",
		TotalSteps:        ch.TotalSteps,
		StepRows:          len(detail.Steps),
		MaxSteps:          ch.MaxSteps,
		TotalTokens:       ch.TotalTokens,
		TokenBudget:       ch.TokenBudget,
		TotalDurationSecs: ch.TotalDurationSecs,
		MaxDurationSecs:   ch.MaxDurationSecs,
		ResolverLoops:     ch.ResolverLoops,
		MaxResolverLoops:  ch.MaxResolverLoops,
		EventTotal:        len(detail.RecentEvents),
		Steps:             make([]ChainStepMetric, 0, len(detail.Steps)),
	}
	report.StepBudgetPct = pct(maxInt(ch.TotalSteps, len(detail.Steps)), ch.MaxSteps)
	failHealth := false
	attentionHealth := false

	switch ch.Status {
	case "completed", "dry_run":
	case "failed", "cancelled":
		failHealth = true
		report.addWarning(fmt.Sprintf("chain status is %s", ch.Status))
	default:
		attentionHealth = true
		report.addWarning(fmt.Sprintf("chain status is %s", ch.Status))
	}

	for _, step := range detail.Steps {
		report.StepTokenTotal += step.TokensUsed
		report.StepTurnTotal += step.TurnsUsed
		report.StepDurationSecs += step.DurationSecs
		report.Steps = append(report.Steps, ChainStepMetric{
			SequenceNum:  step.SequenceNum,
			Role:         step.Role,
			Status:       step.Status,
			Verdict:      step.Verdict,
			ReceiptPath:  step.ReceiptPath,
			TokensUsed:   step.TokensUsed,
			TurnsUsed:    step.TurnsUsed,
			DurationSecs: step.DurationSecs,
			ExitCode:     step.ExitCode,
			ErrorMessage: step.ErrorMessage,
		})
		switch step.Status {
		case "completed":
			report.CompletedSteps++
			if step.ReceiptPath == "" {
				attentionHealth = true
				report.addWarning(fmt.Sprintf("step %d completed without a receipt path", step.SequenceNum))
			}
			if step.TokensUsed == 0 {
				attentionHealth = true
				report.addWarning(fmt.Sprintf("step %d completed without token usage", step.SequenceNum))
			}
			if step.TurnsUsed == 0 {
				attentionHealth = true
				report.addWarning(fmt.Sprintf("step %d completed without turn count", step.SequenceNum))
			}
		case "failed":
			report.FailedSteps++
			failHealth = true
			report.addWarning(fmt.Sprintf("step %d failed", step.SequenceNum))
		case "running":
			report.RunningSteps++
		case "pending":
			report.PendingSteps++
		}
		if step.ExitCode != nil && *step.ExitCode != 0 {
			failHealth = true
			report.addWarning(fmt.Sprintf("step %d exited with code %d", step.SequenceNum, *step.ExitCode))
		}
		if step.ErrorMessage != "" {
			failHealth = true
			report.addWarning(fmt.Sprintf("step %d recorded error: %s", step.SequenceNum, step.ErrorMessage))
		}
	}

	report.TokenBudgetPct = pct(maxInt(ch.TotalTokens, report.StepTokenTotal), ch.TokenBudget)
	report.DurationBudgetPct = pct(maxInt(ch.TotalDurationSecs, report.StepDurationSecs), ch.MaxDurationSecs)
	report.ResolverLoopPct = pct(ch.ResolverLoops, ch.MaxResolverLoops)
	if len(detail.Steps) != ch.TotalSteps {
		attentionHealth = true
		report.addWarning(fmt.Sprintf("chain total_steps=%d but step rows=%d", ch.TotalSteps, len(detail.Steps)))
	}
	if report.StepTokenTotal != ch.TotalTokens {
		attentionHealth = true
		report.addWarning(fmt.Sprintf("chain total_tokens=%d but step token sum=%d", ch.TotalTokens, report.StepTokenTotal))
	}
	if report.StepDurationSecs != ch.TotalDurationSecs {
		attentionHealth = true
		report.addWarning(fmt.Sprintf("chain total_duration_secs=%d but step duration sum=%d", ch.TotalDurationSecs, report.StepDurationSecs))
	}
	if report.TokenBudgetPct >= chainMetricsBudgetWarningPct {
		attentionHealth = true
		report.addWarning(fmt.Sprintf("token budget %.1f%% used", report.TokenBudgetPct))
	}
	if report.DurationBudgetPct >= chainMetricsBudgetWarningPct {
		attentionHealth = true
		report.addWarning(fmt.Sprintf("duration budget %.1f%% used", report.DurationBudgetPct))
	}
	if ch.MaxResolverLoops > 0 && ch.ResolverLoops >= ch.MaxResolverLoops {
		attentionHealth = true
		report.addWarning("resolver loop budget exhausted")
	}

	for _, event := range detail.RecentEvents {
		switch event.EventType {
		case chain.EventStepOutput:
			report.OutputEvents++
		case chain.EventStepFailed:
			report.StepFailedEvents++
			failHealth = true
		case chain.EventSafetyLimitHit:
			report.SafetyLimitEvents++
			failHealth = true
		case chain.EventReindexStarted:
			report.ReindexStartedEvents++
		case chain.EventReindexCompleted:
			report.ReindexDoneEvents++
		case chain.EventStepProcessStarted:
			report.ProcessStartedEvents++
		case chain.EventStepProcessExited:
			report.ProcessExitedEvents++
		}
	}
	if report.StepFailedEvents > 0 {
		report.addWarning(fmt.Sprintf("chain has %d step_failed event(s)", report.StepFailedEvents))
	}
	if report.SafetyLimitEvents > 0 {
		report.addWarning(fmt.Sprintf("chain has %d safety_limit_hit event(s)", report.SafetyLimitEvents))
	}
	if isTerminalChainStatus(ch.Status) && report.ProcessStartedEvents != report.ProcessExitedEvents {
		attentionHealth = true
		report.addWarning(fmt.Sprintf("process events show started=%d exited=%d", report.ProcessStartedEvents, report.ProcessExitedEvents))
	}

	if failHealth {
		report.Health = "failing"
	} else if attentionHealth {
		report.Health = "attention"
	}
	return report
}

func summarizeCurrentStep(steps []chain.Step) *StepSummary {
	for i := len(steps) - 1; i >= 0; i-- {
		switch steps[i].Status {
		case "running", "pending":
			return summarizeStep(steps[i])
		}
	}
	if len(steps) == 0 {
		return nil
	}
	return summarizeStep(steps[len(steps)-1])
}

func summarizeStep(step chain.Step) *StepSummary {
	return &StepSummary{
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

func isActiveChainStatus(status string) bool {
	switch status {
	case "running", "pause_requested", "paused", "cancel_requested":
		return true
	default:
		return false
	}
}

func isTerminalChainStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled", "dry_run":
		return true
	default:
		return false
	}
}

func (r *ChainMetricsReport) addWarning(message string) {
	r.Warnings = append(r.Warnings, RuntimeWarning{Message: message})
}

func pct(used int, budget int) float64 {
	if budget <= 0 {
		return 0
	}
	return float64(used) * 100 / float64(budget)
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
