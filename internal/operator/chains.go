package operator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
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
	detail := ChainDetail{Chain: *ch, Steps: steps, Receipts: receipts, RecentEvents: events}
	report := summarizeChainMetrics(detail)
	detail.Health = report.Health
	detail.Warnings = cloneRuntimeWarnings(report.Warnings)
	return detail, nil
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
	changedFileEventsByStep := map[string]bool{}
	flowAnalysis := chain.AnalyzeFlow(chain.FlowAnalysisInput{Chain: ch, Steps: detail.Steps, Events: detail.RecentEvents})
	report.OpenFindingIDs = append([]string(nil), flowAnalysis.Findings.OpenIDs...)
	report.ClosedFindingIDs = append([]string(nil), flowAnalysis.Findings.ClosedIDs...)
	report.AddressedFindingIDs = append([]string(nil), flowAnalysis.Findings.AddressedIDs...)
	report.ReopenedFindingIDs = append([]string(nil), flowAnalysis.Findings.ReopenedIDs...)
	report.RepeatedResolverFindingIDs = append([]string(nil), flowAnalysis.Findings.RepeatedResolverIDs...)
	report.OpenFindingCount = len(report.OpenFindingIDs)
	report.ClosedFindingCount = len(report.ClosedFindingIDs)
	report.AddressedFindingCount = len(report.AddressedFindingIDs)

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
		case chain.EventStepChangedFiles:
			report.ChangedFileEvents++
			if event.StepID != "" {
				changedFileEventsByStep[event.StepID] = true
			}
		case chain.EventStepFailed:
			report.StepFailedEvents++
			failHealth = true
		case chain.EventReceiptValidation:
			report.ReceiptWarningEvents++
			attentionHealth = true
		case chain.EventReceiptFindings:
			report.ReceiptFindingEvents++
			findingEvent, parseErr := parseReceiptFindingEvent(event.EventData)
			if parseErr == nil {
				if findingEvent.OpenCount > 0 && findingEvent.Verdict != "fix_required" {
					attentionHealth = true
					report.addWarning(fmt.Sprintf("%s reported %d open finding(s) with verdict %s", valueOrUnknown(findingEvent.Role), findingEvent.OpenCount, valueOrUnknown(findingEvent.Verdict)))
				}
				if findingEvent.Role == "resolver" && findingEvent.AddressedCount == 0 {
					attentionHealth = true
					report.addWarning("resolver receipt did not address any finding IDs")
				}
			}
		case chain.EventSourceWriterBlocked:
			report.SourceWriterBlocks++
			failHealth = true
		case chain.EventSourceWriterLockAcquired:
			report.SourceWriterLockAcquires++
		case chain.EventSourceWriterLockReleased:
			report.SourceWriterLockReleases++
		case chain.EventSourceWriterLockForceReleased:
			report.SourceWriterLockForceReleases++
			attentionHealth = true
		case chain.EventSourceWriterLockReleaseFailed:
			report.SourceWriterLockReleaseFailures++
			failHealth = true
		case chain.EventSourceWriterLockHeartbeatFailed:
			report.SourceWriterLockHeartbeatFailures++
			failHealth = true
		case chain.EventSourceWriterLockStaleReplaced:
			report.SourceWriterLockStaleReplacements++
			attentionHealth = true
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
	if report.ReceiptWarningEvents > 0 {
		report.addWarning(fmt.Sprintf("chain has %d receipt_validation_warning event(s)", report.ReceiptWarningEvents))
	}
	if len(report.OpenFindingIDs) > 0 {
		attentionHealth = true
		report.addWarning(fmt.Sprintf("open audit findings: %s", strings.Join(report.OpenFindingIDs, ",")))
	}
	if len(report.ReopenedFindingIDs) > 0 {
		attentionHealth = true
		report.addWarning(fmt.Sprintf("reopened audit findings: %s", strings.Join(report.ReopenedFindingIDs, ",")))
	}
	if report.SourceWriterBlocks > 0 {
		report.addWarning(fmt.Sprintf("source writer guard blocked %d spawn attempt(s)", report.SourceWriterBlocks))
	}
	if report.SourceWriterLockForceReleases > 0 {
		report.addWarning(fmt.Sprintf("source writer lock force released %d time(s)", report.SourceWriterLockForceReleases))
	}
	if report.SourceWriterLockReleaseFailures > 0 {
		report.addWarning(fmt.Sprintf("source writer lock release failed %d time(s)", report.SourceWriterLockReleaseFailures))
	}
	if report.SourceWriterLockHeartbeatFailures > 0 {
		report.addWarning(fmt.Sprintf("source writer lock heartbeat failed %d time(s)", report.SourceWriterLockHeartbeatFailures))
	}
	if report.SourceWriterLockStaleReplacements > 0 {
		report.addWarning(fmt.Sprintf("source writer stale lock replaced %d time(s)", report.SourceWriterLockStaleReplacements))
	}
	if report.SafetyLimitEvents > 0 {
		report.addWarning(fmt.Sprintf("chain has %d safety_limit_hit event(s)", report.SafetyLimitEvents))
	}
	for _, step := range detail.Steps {
		if step.Status == "completed" && appconfig.IsSourceWritingRole(step.Role, appconfig.AgentRoleConfig{}) && !changedFileEventsByStep[step.ID] {
			attentionHealth = true
			report.addWarning(fmt.Sprintf("step %d source-writing role %s completed without changed-file manifest", step.SequenceNum, step.Role))
		}
	}
	if isTerminalChainStatus(ch.Status) && report.ProcessStartedEvents != report.ProcessExitedEvents {
		attentionHealth = true
		report.addWarning(fmt.Sprintf("process events show started=%d exited=%d", report.ProcessStartedEvents, report.ProcessExitedEvents))
	}
	if len(flowAnalysis.Warnings) > 0 {
		attentionHealth = true
		for _, warning := range flowAnalysis.Warnings {
			report.addWarning(warning.Message)
		}
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

type receiptFindingEvent struct {
	Role             string   `json:"role"`
	Verdict          string   `json:"verdict"`
	OpenCount        int      `json:"open_count"`
	AddressedCount   int      `json:"addressed_count"`
	OpenFindingIDs   []string `json:"open_finding_ids"`
	ClosedFindingIDs []string `json:"closed_finding_ids"`
	AddressedIDs     []string `json:"addressed_ids"`
}

func parseReceiptFindingEvent(data string) (receiptFindingEvent, error) {
	var event receiptFindingEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return receiptFindingEvent{}, err
	}
	event.Role = strings.TrimSpace(event.Role)
	event.Verdict = strings.TrimSpace(event.Verdict)
	event.OpenFindingIDs = compactStrings(event.OpenFindingIDs)
	event.ClosedFindingIDs = compactStrings(event.ClosedFindingIDs)
	event.AddressedIDs = compactStrings(event.AddressedIDs)
	if event.OpenCount == 0 {
		event.OpenCount = len(event.OpenFindingIDs)
	}
	if event.AddressedCount == 0 {
		event.AddressedCount = len(event.AddressedIDs)
	}
	return event, nil
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<unknown>"
	}
	return value
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
