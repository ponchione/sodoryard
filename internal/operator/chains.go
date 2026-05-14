package operator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
)

const defaultChainListLimit = 20
const chainMetricsBudgetWarningPct = 80.0
const smallSingleStepTurnWarningThreshold = 6
const smallSingleStepTokenWarningThreshold = 100_000

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
	approvals := make([]ApprovalView, 0)
	for _, approval := range chain.ApprovalsFromEvents(events) {
		approvals = append(approvals, approvalViewFromChain(approval))
	}
	receipts := s.receiptSummaries(ctx, chainID, steps)
	timeline := s.buildChainTimeline(ctx, chainID, events)
	detail := ChainDetail{Chain: *ch, Steps: steps, Receipts: receipts, Approvals: approvals, RecentEvents: events, Timeline: timeline}
	report := summarizeChainMetrics(detail)
	detail.Health = report.Health
	detail.Warnings = cloneRuntimeWarnings(report.Warnings)
	detail.Guardrails = summarizeChainGuardrails(*ch, steps, events)
	detail.Metrics = report
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
		ID:                ch.ID,
		Status:            ch.Status,
		SourceTask:        ch.SourceTask,
		SourceSpecs:       append([]string(nil), ch.SourceSpecs...),
		TotalSteps:        ch.TotalSteps,
		TotalTokens:       ch.TotalTokens,
		TotalDurationSecs: ch.TotalDurationSecs,
		ReceiptCount:      countStepReceipts(steps),
		StartedAt:         ch.StartedAt,
		UpdatedAt:         ch.UpdatedAt,
		CurrentStep:       summarizeCurrentStep(steps),
	}
}

func countStepReceipts(steps []chain.Step) int {
	count := 0
	for _, step := range steps {
		if strings.TrimSpace(step.ReceiptPath) != "" {
			count++
		}
	}
	return count
}

func findingLifecycleMetrics(entries []chain.FindingLifecycleEntry) []FindingLifecycleMetric {
	out := make([]FindingLifecycleMetric, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) == "" {
			continue
		}
		out = append(out, FindingLifecycleMetric{
			ID:              entry.ID,
			SourceRole:      entry.SourceRole,
			Status:          entry.Status,
			Severity:        entry.Severity,
			Evidence:        entry.Evidence,
			Summary:         entry.Summary,
			RequiredFix:     entry.RequiredFix,
			Resolution:      entry.Resolution,
			FilesChanged:    append([]string(nil), entry.FilesChanged...),
			Validation:      append([]string(nil), entry.Validation...),
			AddressedCount:  entry.AddressedCount,
			ClosedCount:     entry.ClosedCount,
			ReopenedCount:   entry.ReopenedCount,
			FirstSeenStep:   entry.FirstSeenStep,
			LastUpdatedStep: entry.LastUpdatedStep,
		})
	}
	return out
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
	guardrailFactsByStep := map[string]stepGuardrailFactsEvent{}
	launchFacts := chainMetricsLaunchFactsFromEvents(detail.RecentEvents)
	report.LaunchMode = launchFacts.Mode
	report.StepMaxTurns = launchFacts.StepMaxTurns
	report.StepMaxTokens = launchFacts.StepMaxTokens
	report.HasStepMaxTurns = launchFacts.HasStepMaxTurns
	report.HasStepMaxTokens = launchFacts.HasStepMaxTokens
	flowAnalysis := chain.AnalyzeFlow(chain.FlowAnalysisInput{Chain: ch, Steps: detail.Steps, Events: detail.RecentEvents})
	report.OpenFindingIDs = append([]string(nil), flowAnalysis.Findings.OpenIDs...)
	report.ClosedFindingIDs = append([]string(nil), flowAnalysis.Findings.ClosedIDs...)
	report.AddressedFindingIDs = append([]string(nil), flowAnalysis.Findings.AddressedIDs...)
	report.ReopenedFindingIDs = append([]string(nil), flowAnalysis.Findings.ReopenedIDs...)
	report.RepeatedResolverFindingIDs = append([]string(nil), flowAnalysis.Findings.RepeatedResolverIDs...)
	report.FindingLifecycle = findingLifecycleMetrics(flowAnalysis.Findings.Findings)
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
	if ch.Status == "completed" && isSmallSingleStepLaunch(launchFacts.Mode, ch, detail.Steps, report.CompletedSteps) {
		tokensUsed := maxInt(ch.TotalTokens, report.StepTokenTotal)
		if report.StepTurnTotal > smallSingleStepTurnWarningThreshold || tokensUsed > smallSingleStepTokenWarningThreshold {
			attentionHealth = true
			report.addWarning(fmt.Sprintf("%s used %d turns and %d tokens in a single completed step; %s; expected small one-step chains to stay within %d turns and %d tokens, so consider narrowing the task, using a roster for broad work, or setting --step-max-turns/--step-max-tokens for bounded probes",
				launchFacts.Mode,
				report.StepTurnTotal,
				tokensUsed,
				singleStepCapSummary(launchFacts),
				smallSingleStepTurnWarningThreshold,
				smallSingleStepTokenWarningThreshold,
			))
		}
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
		case chain.EventStepGuardrailFacts:
			report.StepGuardrailFactEvents++
			facts, parseErr := parseStepGuardrailFactsEvent(event.EventData)
			if parseErr == nil {
				guardrailFactsByStep[event.StepID] = facts
				if facts.SourceMutating && facts.ChangedFileManifestPresent && event.StepID != "" {
					changedFileEventsByStep[event.StepID] = true
				}
				if facts.SourceMutating && facts.ChangedFileClaimPresent && !facts.ChangedFileClaimMatchesManifest {
					attentionHealth = true
					report.addWarning(fmt.Sprintf("step %d changed-file receipt claim differs from harness manifest: extra=%s unclaimed=%s",
						facts.Sequence,
						joinValuesOrNone(facts.ChangedFileClaimExtra),
						joinValuesOrNone(facts.ChangedFileManifestUnclaimed),
					))
				}
				if facts.SourceMutating && facts.ChangedFileCount > 0 && facts.CodeIndexStateFound && !facts.CodeIndexDirty {
					attentionHealth = true
					report.addWarning(fmt.Sprintf("step %d changed files but code index state was not marked stale", facts.Sequence))
				}
				if facts.SourceMutating && facts.ChangedFileCount > 0 && !facts.CodeIndexDirtyMarkSupported && !facts.CodeIndexStateFound {
					attentionHealth = true
					report.addWarning(fmt.Sprintf("step %d changed files but code index dirty marking is unavailable", facts.Sequence))
				}
				if facts.SourceMutating && facts.ChangedFileCount > 0 && facts.CodeIndexDirtyMarkSupported && !facts.CodeIndexDirtyMarkAttempted {
					attentionHealth = true
					report.addWarning(fmt.Sprintf("step %d changed files but code index dirty marking was not attempted", facts.Sequence))
				}
				if facts.CodeIndexDirtyMarkAttempted && !facts.CodeIndexDirtyMarked {
					attentionHealth = true
					reason := strings.TrimSpace(facts.CodeIndexDirtyMarkError)
					if reason == "" {
						reason = "mark not confirmed"
					}
					report.addWarning(fmt.Sprintf("step %d failed to mark code index stale: %s", facts.Sequence, reason))
				}
				if !facts.ReceiptValid {
					attentionHealth = true
					reason := strings.TrimSpace(facts.ReceiptError)
					if reason == "" {
						reason = "receipt did not pass guardrail validation"
					}
					report.addWarning(fmt.Sprintf("step %d receipt guardrail facts show invalid receipt: %s", facts.Sequence, reason))
				}
				if facts.SourceMutating && !facts.SourceWriterLockReleaseAttempted {
					failHealth = true
					report.addWarning(fmt.Sprintf("step %d source writer lock release was not attempted", facts.Sequence))
				}
				if facts.SourceWriterLockReleaseAttempted && !facts.SourceWriterLockReleased {
					failHealth = true
					reason := strings.TrimSpace(facts.SourceWriterLockReleaseError)
					if reason == "" {
						reason = "release not confirmed"
					}
					report.addWarning(fmt.Sprintf("step %d source writer lock release failed: %s", facts.Sequence, reason))
				}
				if facts.SuspiciousVerdictFindingCombination {
					attentionHealth = true
					reason := strings.TrimSpace(facts.SuspiciousVerdictFindingReason)
					if reason == "" {
						reason = "suspicious verdict/finding combination"
					}
					report.addWarning(reason)
				}
			}
		case chain.EventStepFailed:
			report.StepFailedEvents++
			failHealth = true
		case chain.EventReceiptValidation:
			report.ReceiptWarningEvents++
			attentionHealth = true
			if validationEvent, parseErr := parseReceiptValidationEvent(event.EventData); parseErr == nil {
				message := validationEvent.Message()
				if message != "" {
					report.addWarning(message)
				}
			}
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
		case chain.EventFindingLifecycleFacts:
			report.FindingLifecycleFactEvents++
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
		facts, hasGuardrailFacts := guardrailFactsByStep[step.ID]
		if step.Status == "completed" && appconfig.IsSourceWritingRole(step.Role, appconfig.AgentRoleConfig{}) && !changedFileEventsByStep[step.ID] {
			attentionHealth = true
			report.addWarning(fmt.Sprintf("step %d source-writing role %s completed without changed-file manifest", step.SequenceNum, step.Role))
		}
		if hasGuardrailFacts && facts.SourceMutating && facts.ChangedFileManifestPresent && strings.TrimSpace(facts.ChangedFileManifestError) != "" {
			attentionHealth = true
			report.addWarning(fmt.Sprintf("step %d changed-file manifest capture reported: %s", step.SequenceNum, facts.ChangedFileManifestError))
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
	case "running", "pause_requested", "paused", "cancel_requested", chain.StatusWaitingApproval:
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

type chainMetricsLaunchFacts struct {
	Mode             string
	StepMaxTurns     int
	StepMaxTokens    int
	HasStepMaxTurns  bool
	HasStepMaxTokens bool
}

func chainMetricsLaunchFactsFromEvents(events []chain.Event) chainMetricsLaunchFacts {
	var facts chainMetricsLaunchFacts
	for _, event := range events {
		switch event.EventType {
		case chain.EventChainStarted, chain.EventChainCompleted:
		default:
			continue
		}
		var payload struct {
			Mode          string `json:"mode"`
			StepMaxTurns  *int   `json:"step_max_turns"`
			StepMaxTokens *int   `json:"step_max_tokens"`
		}
		if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil {
			continue
		}
		if trimmed := strings.TrimSpace(payload.Mode); trimmed != "" {
			facts.Mode = trimmed
		}
		if payload.StepMaxTurns != nil {
			facts.StepMaxTurns = *payload.StepMaxTurns
			facts.HasStepMaxTurns = true
		}
		if payload.StepMaxTokens != nil {
			facts.StepMaxTokens = *payload.StepMaxTokens
			facts.HasStepMaxTokens = true
		}
	}
	return facts
}

func isSmallSingleStepLaunch(mode string, ch chain.Chain, steps []chain.Step, completedSteps int) bool {
	switch strings.TrimSpace(mode) {
	case "one_step_chain", "manual_roster":
	default:
		return false
	}
	return maxInt(ch.TotalSteps, len(steps)) == 1 && len(steps) == 1 && completedSteps == 1
}

func singleStepCapSummary(facts chainMetricsLaunchFacts) string {
	hasTurnCap := facts.HasStepMaxTurns && facts.StepMaxTurns > 0
	hasTokenCap := facts.HasStepMaxTokens && facts.StepMaxTokens > 0
	if !hasTurnCap && !hasTokenCap {
		if facts.HasStepMaxTurns || facts.HasStepMaxTokens {
			return "no per-step turn/token cap was set"
		}
		return "no per-step turn/token cap was recorded"
	}
	parts := make([]string, 0, 2)
	if hasTurnCap {
		parts = append(parts, fmt.Sprintf("step_max_turns=%d", facts.StepMaxTurns))
	}
	if hasTokenCap {
		parts = append(parts, fmt.Sprintf("step_max_tokens=%d", facts.StepMaxTokens))
	}
	return "recorded per-step caps: " + strings.Join(parts, ", ")
}

func (r *ChainMetricsReport) addWarning(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	for _, warning := range r.Warnings {
		if warning.Message == message {
			return
		}
	}
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

type receiptValidationEvent struct {
	Role          string `json:"role"`
	ReceiptPath   string `json:"receipt_path"`
	SchemaVersion string `json:"schema_version"`
	Warning       string `json:"warning"`
	Error         string `json:"error"`
}

type stepGuardrailFactsEvent struct {
	Role                                string   `json:"role"`
	Sequence                            int      `json:"sequence"`
	ReceiptPath                         string   `json:"receipt_path"`
	SourceMutating                      bool     `json:"source_mutating"`
	ExitCode                            int      `json:"exit_code"`
	DurationSecs                        int      `json:"duration_secs"`
	ReceiptPresent                      bool     `json:"receipt_present"`
	SyntheticReceiptWritten             bool     `json:"synthetic_receipt_written"`
	ReceiptSchemaValid                  bool     `json:"receipt_schema_valid"`
	ReceiptStepValid                    bool     `json:"receipt_step_valid"`
	ReceiptSectionsValid                bool     `json:"receipt_sections_valid"`
	ReceiptValid                        bool     `json:"receipt_valid"`
	ReceiptError                        string   `json:"receipt_error"`
	ParsedVerdict                       string   `json:"parsed_verdict"`
	TokensUsed                          int      `json:"tokens_used"`
	TurnsUsed                           int      `json:"turns_used"`
	ReceiptDurationSeconds              int      `json:"receipt_duration_seconds"`
	ClaimedValidationCommands           []string `json:"claimed_validation_commands"`
	ChangedFileClaimPresent             bool     `json:"changed_file_claim_present"`
	ClaimedChangedFiles                 []string `json:"claimed_changed_files"`
	ChangedFileClaimMatchesManifest     bool     `json:"changed_file_claim_matches_manifest"`
	ChangedFileClaimExtra               []string `json:"changed_file_claim_extra"`
	ChangedFileManifestUnclaimed        []string `json:"changed_file_manifest_unclaimed"`
	ChangedFileManifestPresent          bool     `json:"changed_file_manifest_present"`
	ChangedFileManifestError            string   `json:"changed_file_manifest_error"`
	ChangedFileCount                    int      `json:"changed_file_count"`
	ChangedFiles                        []string `json:"changed_files"`
	CodeIndexStateSupported             bool     `json:"code_index_state_supported"`
	CodeIndexStateFound                 bool     `json:"code_index_state_found"`
	CodeIndexDirtyMarkSupported         bool     `json:"code_index_dirty_mark_supported"`
	CodeIndexDirtyMarkAttempted         bool     `json:"code_index_dirty_mark_attempted"`
	CodeIndexDirtyMarked                bool     `json:"code_index_dirty_marked"`
	CodeIndexDirtyMarkError             string   `json:"code_index_dirty_mark_error"`
	CodeIndexDirty                      bool     `json:"code_index_dirty"`
	CodeIndexDirtyReason                string   `json:"code_index_dirty_reason"`
	CodeIndexStateError                 string   `json:"code_index_state_error"`
	BrainIndexStateSupported            bool     `json:"brain_index_state_supported"`
	BrainIndexStateFound                bool     `json:"brain_index_state_found"`
	BrainIndexDirty                     bool     `json:"brain_index_dirty"`
	BrainIndexDirtyReason               string   `json:"brain_index_dirty_reason"`
	BrainIndexStateError                string   `json:"brain_index_state_error"`
	SourceWriterLockReleaseAttempted    bool     `json:"source_writer_lock_release_attempted"`
	SourceWriterLockReleased            bool     `json:"source_writer_lock_released"`
	SourceWriterLockReleaseError        string   `json:"source_writer_lock_release_error"`
	FindingCount                        int      `json:"finding_count"`
	OpenFindingCount                    int      `json:"open_finding_count"`
	ClosedFindingCount                  int      `json:"closed_finding_count"`
	AddressedFindingCount               int      `json:"addressed_finding_count"`
	FindingIDs                          []string `json:"finding_ids"`
	SuspiciousVerdictFindingCombination bool     `json:"suspicious_verdict_finding_combination"`
	SuspiciousVerdictFindingReason      string   `json:"suspicious_verdict_finding_reason"`
	OpenFindingIDs                      []string `json:"open_finding_ids"`
	ClosedFindingIDs                    []string `json:"closed_finding_ids"`
	AddressedIDs                        []string `json:"addressed_ids"`
	RunError                            string   `json:"run_error"`
}

type changedFileManifestEvent struct {
	Paths []string `json:"paths"`
	Error string   `json:"error"`
}

func (e receiptValidationEvent) Message() string {
	message := strings.TrimSpace(e.Warning)
	if message == "" {
		message = strings.TrimSpace(e.Error)
	}
	if message == "" {
		return ""
	}
	prefix := strings.TrimSpace(e.Role)
	if e.ReceiptPath != "" {
		if prefix != "" {
			prefix += " "
		}
		prefix += e.ReceiptPath
	}
	if prefix != "" {
		return prefix + ": " + message
	}
	return message
}

func parseReceiptValidationEvent(data string) (receiptValidationEvent, error) {
	var event receiptValidationEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return receiptValidationEvent{}, err
	}
	event.Role = strings.TrimSpace(event.Role)
	event.ReceiptPath = strings.TrimSpace(event.ReceiptPath)
	event.SchemaVersion = strings.TrimSpace(event.SchemaVersion)
	event.Warning = strings.TrimSpace(event.Warning)
	event.Error = strings.TrimSpace(event.Error)
	return event, nil
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

func parseStepGuardrailFactsEvent(data string) (stepGuardrailFactsEvent, error) {
	var event stepGuardrailFactsEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return stepGuardrailFactsEvent{}, err
	}
	event.Role = strings.TrimSpace(event.Role)
	event.ReceiptPath = strings.TrimSpace(event.ReceiptPath)
	event.ReceiptError = strings.TrimSpace(event.ReceiptError)
	event.ParsedVerdict = strings.TrimSpace(event.ParsedVerdict)
	event.ChangedFileManifestError = strings.TrimSpace(event.ChangedFileManifestError)
	event.CodeIndexDirtyMarkError = strings.TrimSpace(event.CodeIndexDirtyMarkError)
	event.CodeIndexDirtyReason = strings.TrimSpace(event.CodeIndexDirtyReason)
	event.CodeIndexStateError = strings.TrimSpace(event.CodeIndexStateError)
	event.BrainIndexDirtyReason = strings.TrimSpace(event.BrainIndexDirtyReason)
	event.BrainIndexStateError = strings.TrimSpace(event.BrainIndexStateError)
	event.SourceWriterLockReleaseError = strings.TrimSpace(event.SourceWriterLockReleaseError)
	event.SuspiciousVerdictFindingReason = strings.TrimSpace(event.SuspiciousVerdictFindingReason)
	event.RunError = strings.TrimSpace(event.RunError)
	event.OpenFindingIDs = compactStrings(event.OpenFindingIDs)
	event.ClosedFindingIDs = compactStrings(event.ClosedFindingIDs)
	event.AddressedIDs = compactStrings(event.AddressedIDs)
	event.FindingIDs = compactStrings(event.FindingIDs)
	event.ClaimedValidationCommands = compactStrings(event.ClaimedValidationCommands)
	event.ClaimedChangedFiles = compactStrings(event.ClaimedChangedFiles)
	event.ChangedFileClaimExtra = compactStrings(event.ChangedFileClaimExtra)
	event.ChangedFileManifestUnclaimed = compactStrings(event.ChangedFileManifestUnclaimed)
	event.ChangedFiles = compactStrings(event.ChangedFiles)
	if event.ChangedFileCount == 0 {
		event.ChangedFileCount = len(event.ChangedFiles)
	}
	if event.FindingCount == 0 {
		event.FindingCount = len(event.FindingIDs)
	}
	if event.OpenFindingCount == 0 {
		event.OpenFindingCount = len(event.OpenFindingIDs)
	}
	if event.ClosedFindingCount == 0 {
		event.ClosedFindingCount = len(event.ClosedFindingIDs)
	}
	if event.AddressedFindingCount == 0 {
		event.AddressedFindingCount = len(event.AddressedIDs)
	}
	return event, nil
}

func summarizeChainGuardrails(ch chain.Chain, steps []chain.Step, events []chain.Event) ChainGuardrailDetails {
	analysis := chain.AnalyzeFlow(chain.FlowAnalysisInput{Chain: ch, Steps: steps, Events: events})
	details := ChainGuardrailDetails{
		OpenFindingIDs:             append([]string(nil), analysis.Findings.OpenIDs...),
		ClosedFindingIDs:           append([]string(nil), analysis.Findings.ClosedIDs...),
		AddressedFindingIDs:        append([]string(nil), analysis.Findings.AddressedIDs...),
		ReopenedFindingIDs:         append([]string(nil), analysis.Findings.ReopenedIDs...),
		RepeatedResolverFindingIDs: append([]string(nil), analysis.Findings.RepeatedResolverIDs...),
		Findings:                   findingLifecycleMetrics(analysis.Findings.Findings),
	}
	stepsByID := map[string]chain.Step{}
	for _, step := range steps {
		stepsByID[step.ID] = step
	}
	for _, event := range events {
		step := stepsByID[event.StepID]
		switch event.EventType {
		case chain.EventStepChangedFiles:
			manifest, err := parseChangedFileManifestEvent(event.EventData)
			if err != nil {
				manifest.Error = err.Error()
			}
			details.ChangedFiles = append(details.ChangedFiles, ChangedFileManifest{
				StepID:      event.StepID,
				SequenceNum: step.SequenceNum,
				Role:        step.Role,
				Paths:       append([]string(nil), manifest.Paths...),
				Error:       manifest.Error,
			})
		case chain.EventStepGuardrailFacts:
			facts, err := parseStepGuardrailFactsEvent(event.EventData)
			if err != nil {
				continue
			}
			seq := facts.Sequence
			role := facts.Role
			if seq == 0 {
				seq = step.SequenceNum
			}
			if role == "" {
				role = step.Role
			}
			details.StepFacts = append(details.StepFacts, StepGuardrailFactSummary{
				StepID:                              event.StepID,
				SequenceNum:                         seq,
				Role:                                role,
				ReceiptPath:                         facts.ReceiptPath,
				SourceMutating:                      facts.SourceMutating,
				ExitCode:                            facts.ExitCode,
				DurationSecs:                        facts.DurationSecs,
				ReceiptPresent:                      facts.ReceiptPresent,
				SyntheticReceiptWritten:             facts.SyntheticReceiptWritten,
				ReceiptValid:                        facts.ReceiptValid,
				ReceiptSchemaValid:                  facts.ReceiptSchemaValid,
				ReceiptStepValid:                    facts.ReceiptStepValid,
				ReceiptSectionsValid:                facts.ReceiptSectionsValid,
				ReceiptError:                        facts.ReceiptError,
				ParsedVerdict:                       facts.ParsedVerdict,
				TokensUsed:                          facts.TokensUsed,
				TurnsUsed:                           facts.TurnsUsed,
				ReceiptDurationSeconds:              facts.ReceiptDurationSeconds,
				ClaimedValidationCommands:           append([]string(nil), facts.ClaimedValidationCommands...),
				ChangedFileClaimPresent:             facts.ChangedFileClaimPresent,
				ClaimedChangedFiles:                 append([]string(nil), facts.ClaimedChangedFiles...),
				ChangedFileClaimMatchesManifest:     facts.ChangedFileClaimMatchesManifest,
				ChangedFileClaimExtra:               append([]string(nil), facts.ChangedFileClaimExtra...),
				ChangedFileManifestUnclaimed:        append([]string(nil), facts.ChangedFileManifestUnclaimed...),
				ChangedFileManifestPresent:          facts.ChangedFileManifestPresent,
				ChangedFileManifestError:            facts.ChangedFileManifestError,
				ChangedFileCount:                    facts.ChangedFileCount,
				ChangedFiles:                        append([]string(nil), facts.ChangedFiles...),
				CodeIndexStateSupported:             facts.CodeIndexStateSupported,
				CodeIndexStateFound:                 facts.CodeIndexStateFound,
				CodeIndexDirtyMarkSupported:         facts.CodeIndexDirtyMarkSupported,
				CodeIndexDirtyMarkAttempted:         facts.CodeIndexDirtyMarkAttempted,
				CodeIndexDirtyMarked:                facts.CodeIndexDirtyMarked,
				CodeIndexDirtyMarkError:             facts.CodeIndexDirtyMarkError,
				CodeIndexDirty:                      facts.CodeIndexDirty,
				CodeIndexDirtyReason:                facts.CodeIndexDirtyReason,
				CodeIndexStateError:                 facts.CodeIndexStateError,
				BrainIndexStateSupported:            facts.BrainIndexStateSupported,
				BrainIndexStateFound:                facts.BrainIndexStateFound,
				BrainIndexDirty:                     facts.BrainIndexDirty,
				BrainIndexDirtyReason:               facts.BrainIndexDirtyReason,
				BrainIndexStateError:                facts.BrainIndexStateError,
				SourceWriterLockReleaseAttempted:    facts.SourceWriterLockReleaseAttempted,
				SourceWriterLockReleased:            facts.SourceWriterLockReleased,
				SourceWriterLockReleaseError:        facts.SourceWriterLockReleaseError,
				FindingCount:                        facts.FindingCount,
				OpenFindingCount:                    facts.OpenFindingCount,
				ClosedFindingCount:                  facts.ClosedFindingCount,
				AddressedFindingCount:               facts.AddressedFindingCount,
				FindingIDs:                          append([]string(nil), facts.FindingIDs...),
				OpenFindingIDs:                      append([]string(nil), facts.OpenFindingIDs...),
				ClosedFindingIDs:                    append([]string(nil), facts.ClosedFindingIDs...),
				AddressedIDs:                        append([]string(nil), facts.AddressedIDs...),
				SuspiciousVerdictFindingCombination: facts.SuspiciousVerdictFindingCombination,
				SuspiciousVerdictFindingReason:      facts.SuspiciousVerdictFindingReason,
				RunError:                            facts.RunError,
			})
		case chain.EventSourceWriterBlocked:
			details.LockHealth.Blocked++
		case chain.EventSourceWriterLockAcquired:
			details.LockHealth.Acquired++
		case chain.EventSourceWriterLockReleased:
			details.LockHealth.Released++
		case chain.EventSourceWriterLockForceReleased:
			details.LockHealth.ForceReleased++
		case chain.EventSourceWriterLockReleaseFailed:
			details.LockHealth.ReleaseFailed++
		case chain.EventSourceWriterLockHeartbeatFailed:
			details.LockHealth.HeartbeatFailed++
		case chain.EventSourceWriterLockStaleReplaced:
			details.LockHealth.StaleReplaced++
		}
	}
	details.LockHealth.UnreleasedWriters = details.LockHealth.Acquired - details.LockHealth.Released - details.LockHealth.ForceReleased
	if details.LockHealth.UnreleasedWriters < 0 {
		details.LockHealth.UnreleasedWriters = 0
	}
	sort.SliceStable(details.ChangedFiles, func(i, j int) bool {
		if details.ChangedFiles[i].SequenceNum == details.ChangedFiles[j].SequenceNum {
			return details.ChangedFiles[i].StepID < details.ChangedFiles[j].StepID
		}
		return details.ChangedFiles[i].SequenceNum < details.ChangedFiles[j].SequenceNum
	})
	sort.SliceStable(details.StepFacts, func(i, j int) bool {
		if details.StepFacts[i].SequenceNum == details.StepFacts[j].SequenceNum {
			return details.StepFacts[i].StepID < details.StepFacts[j].StepID
		}
		return details.StepFacts[i].SequenceNum < details.StepFacts[j].SequenceNum
	})
	return details
}

func parseChangedFileManifestEvent(data string) (changedFileManifestEvent, error) {
	var event changedFileManifestEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return changedFileManifestEvent{}, err
	}
	event.Paths = compactStrings(event.Paths)
	event.Error = strings.TrimSpace(event.Error)
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

func joinValuesOrNone(values []string) string {
	values = compactStrings(values)
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ",")
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
