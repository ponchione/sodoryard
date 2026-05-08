package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
)

type chainFlowSuite struct{}

type chainFlowFixture struct {
	Name                          string      `json:"name"`
	Chain                         flowChain   `json:"chain"`
	Steps                         []flowStep  `json:"steps"`
	Events                        []flowEvent `json:"events"`
	ExpectedWarningCodes          []string    `json:"expected_warning_codes"`
	ExpectedOpenFindingIDs        []string    `json:"expected_open_finding_ids"`
	ExpectedClosedFindingIDs      []string    `json:"expected_closed_finding_ids"`
	ExpectedAddressedFindingIDs   []string    `json:"expected_addressed_finding_ids"`
	ExpectedReopenedFindingIDs    []string    `json:"expected_reopened_finding_ids"`
	ExpectedRepeatedResolverIDs   []string    `json:"expected_repeated_resolver_ids"`
	ExpectedSourceWriterConflicts []string    `json:"expected_source_writer_conflicts"`
}

type flowChain struct {
	ID                string `json:"id"`
	Status            string `json:"status"`
	TotalSteps        int    `json:"total_steps"`
	TotalTokens       int    `json:"total_tokens"`
	TotalDurationSecs int    `json:"total_duration_secs"`
	MaxSteps          int    `json:"max_steps"`
	MaxResolverLoops  int    `json:"max_resolver_loops"`
	MaxDurationSecs   int    `json:"max_duration_secs"`
	TokenBudget       int    `json:"token_budget"`
}

type flowStep struct {
	ID           string `json:"id"`
	ChainID      string `json:"chain_id"`
	SequenceNum  int    `json:"sequence_num"`
	Role         string `json:"role"`
	Task         string `json:"task"`
	TaskContext  string `json:"task_context"`
	Status       string `json:"status"`
	Verdict      string `json:"verdict"`
	ReceiptPath  string `json:"receipt_path"`
	TokensUsed   int    `json:"tokens_used"`
	TurnsUsed    int    `json:"turns_used"`
	DurationSecs int    `json:"duration_secs"`
	ExitCode     *int   `json:"exit_code"`
	ErrorMessage string `json:"error_message"`
	StartedAt    string `json:"started_at"`
	CompletedAt  string `json:"completed_at"`
}

type flowEvent struct {
	ID        int64           `json:"id"`
	ChainID   string          `json:"chain_id"`
	StepID    string          `json:"step_id"`
	EventType string          `json:"event_type"`
	EventData json.RawMessage `json:"event_data"`
	CreatedAt string          `json:"created_at"`
}

var chainFlowFixturePaths = []string{
	"fixtures/chain-flow/valid-sequential.json",
	"fixtures/chain-flow/guardrail-warnings.json",
	"fixtures/chain-flow/finding-lifecycle.json",
	"fixtures/chain-flow/source-writer-conflict.json",
}

func (chainFlowSuite) Info() SuiteInfo {
	return SuiteInfo{
		Name:        "chain-flow",
		Description: "Evaluate deterministic chain step/event fixtures against spec 22 sequencing and finding-flow invariants.",
	}
}

func (s chainFlowSuite) Run(ctx context.Context) (Report, error) {
	select {
	case <-ctx.Done():
		return Report{}, ctx.Err()
	default:
	}
	report := newReport(s.Info())
	for _, path := range chainFlowFixturePaths {
		result, err := evaluateChainFlowFixture(path)
		if err != nil {
			c := newCase(path)
			c.addAssertion("evaluate fixture", false, err.Error(), path, err.Error())
			report.addCase(c)
			continue
		}
		report.addCase(result)
	}
	report.finalize()
	return report, nil
}

func evaluateChainFlowFixture(path string) (CaseResult, error) {
	data, err := fixtureFS.ReadFile(path)
	if err != nil {
		return CaseResult{}, err
	}
	var fixture chainFlowFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return CaseResult{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if strings.TrimSpace(fixture.Name) == "" {
		fixture.Name = path
	}
	result := newCase(fixture.Name)
	result.addAssertion("read fixture", true, "", path, path)

	ch := fixture.Chain.toChain()
	steps, err := fixture.toSteps(ch.ID)
	if err != nil {
		result.addAssertion("decode steps", false, err.Error(), "valid steps", err.Error())
		return result, nil
	}
	result.addAssertion("decode steps", true, "", "valid steps", "valid steps")
	events, err := fixture.toEvents(ch.ID)
	if err != nil {
		result.addAssertion("decode events", false, err.Error(), "valid events", err.Error())
		return result, nil
	}
	result.addAssertion("decode events", true, "", "valid events", "valid events")

	analysis := chain.AnalyzeFlow(chain.FlowAnalysisInput{Chain: ch, Steps: steps, Events: events})
	warningCodes := make([]string, 0, len(analysis.Warnings))
	for _, warning := range analysis.Warnings {
		warningCodes = append(warningCodes, warning.Code)
		result.Warnings = append(result.Warnings, warning.Message)
	}
	sourceWriterConflicts := runningSourceWriterConflicts(steps)
	result.Warnings = append(result.Warnings, sourceWriterConflicts...)
	result.Details["warning_codes"] = uniqueSortedStrings(warningCodes)
	result.Details["source_writer_conflicts"] = uniqueSortedStrings(sourceWriterConflicts)
	result.Details["open_finding_ids"] = analysis.Findings.OpenIDs
	result.Details["closed_finding_ids"] = analysis.Findings.ClosedIDs
	result.Details["addressed_finding_ids"] = analysis.Findings.AddressedIDs
	result.Details["reopened_finding_ids"] = analysis.Findings.ReopenedIDs
	result.Details["repeated_resolver_ids"] = analysis.Findings.RepeatedResolverIDs

	for _, finding := range analysis.Findings.Findings {
		result.Findings = append(result.Findings, FindingResult{
			ID:         finding.ID,
			SourceRole: finding.SourceRole,
			Status:     finding.Status,
			Severity:   finding.Severity,
			Summary:    finding.Summary,
		})
	}

	assertStringSet(&result, "flow warning codes", warningCodes, fixture.ExpectedWarningCodes)
	assertStringSet(&result, "open finding ids", analysis.Findings.OpenIDs, fixture.ExpectedOpenFindingIDs)
	assertStringSet(&result, "closed finding ids", analysis.Findings.ClosedIDs, fixture.ExpectedClosedFindingIDs)
	assertStringSet(&result, "addressed finding ids", analysis.Findings.AddressedIDs, fixture.ExpectedAddressedFindingIDs)
	assertStringSet(&result, "reopened finding ids", analysis.Findings.ReopenedIDs, fixture.ExpectedReopenedFindingIDs)
	assertStringSet(&result, "repeated resolver ids", analysis.Findings.RepeatedResolverIDs, fixture.ExpectedRepeatedResolverIDs)
	assertStringSet(&result, "source writer conflicts", sourceWriterConflicts, fixture.ExpectedSourceWriterConflicts)
	return result, nil
}

func (f flowChain) toChain() chain.Chain {
	return chain.Chain{
		ID:                strings.TrimSpace(f.ID),
		Status:            strings.TrimSpace(f.Status),
		TotalSteps:        f.TotalSteps,
		TotalTokens:       f.TotalTokens,
		TotalDurationSecs: f.TotalDurationSecs,
		MaxSteps:          f.MaxSteps,
		MaxResolverLoops:  f.MaxResolverLoops,
		MaxDurationSecs:   f.MaxDurationSecs,
		TokenBudget:       f.TokenBudget,
	}
}

func (f chainFlowFixture) toSteps(chainID string) ([]chain.Step, error) {
	steps := make([]chain.Step, 0, len(f.Steps))
	for _, fixtureStep := range f.Steps {
		startedAt, err := parseOptionalTime(fixtureStep.StartedAt)
		if err != nil {
			return nil, err
		}
		completedAt, err := parseOptionalTime(fixtureStep.CompletedAt)
		if err != nil {
			return nil, err
		}
		stepChainID := strings.TrimSpace(fixtureStep.ChainID)
		if stepChainID == "" {
			stepChainID = chainID
		}
		steps = append(steps, chain.Step{
			ID:           strings.TrimSpace(fixtureStep.ID),
			ChainID:      stepChainID,
			SequenceNum:  fixtureStep.SequenceNum,
			Role:         strings.TrimSpace(fixtureStep.Role),
			Task:         strings.TrimSpace(fixtureStep.Task),
			TaskContext:  strings.TrimSpace(fixtureStep.TaskContext),
			Status:       strings.TrimSpace(fixtureStep.Status),
			Verdict:      strings.TrimSpace(fixtureStep.Verdict),
			ReceiptPath:  strings.TrimSpace(fixtureStep.ReceiptPath),
			TokensUsed:   fixtureStep.TokensUsed,
			TurnsUsed:    fixtureStep.TurnsUsed,
			DurationSecs: fixtureStep.DurationSecs,
			ExitCode:     fixtureStep.ExitCode,
			ErrorMessage: strings.TrimSpace(fixtureStep.ErrorMessage),
			StartedAt:    startedAt,
			CompletedAt:  completedAt,
		})
	}
	return steps, nil
}

func (f chainFlowFixture) toEvents(chainID string) ([]chain.Event, error) {
	events := make([]chain.Event, 0, len(f.Events))
	for _, fixtureEvent := range f.Events {
		createdAt, err := parseFixtureTime(fixtureEvent.CreatedAt)
		if err != nil {
			return nil, err
		}
		eventChainID := strings.TrimSpace(fixtureEvent.ChainID)
		if eventChainID == "" {
			eventChainID = chainID
		}
		eventData := strings.TrimSpace(string(fixtureEvent.EventData))
		if eventData == "" || eventData == "null" {
			eventData = "{}"
		}
		events = append(events, chain.Event{
			ID:        fixtureEvent.ID,
			ChainID:   eventChainID,
			StepID:    strings.TrimSpace(fixtureEvent.StepID),
			EventType: chain.EventType(strings.TrimSpace(fixtureEvent.EventType)),
			EventData: eventData,
			CreatedAt: createdAt,
		})
	}
	return events, nil
}

func parseOptionalTime(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("parse time %q: %w", value, err)
	}
	return &parsed, nil
}

func parseFixtureTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", value, err)
	}
	return parsed, nil
}

func runningSourceWriterConflicts(steps []chain.Step) []string {
	running := make([]string, 0)
	for _, step := range steps {
		if strings.TrimSpace(step.Status) != "running" {
			continue
		}
		if !appconfig.IsSourceWritingRole(step.Role, appconfig.AgentRoleConfig{}) {
			continue
		}
		running = append(running, fmt.Sprintf("step %d %s", step.SequenceNum, step.Role))
	}
	if len(running) <= 1 {
		return nil
	}
	return []string{fmt.Sprintf("multiple source-writing steps running: %s", strings.Join(running, ", "))}
}
