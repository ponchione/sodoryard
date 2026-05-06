package chain

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ponchione/sodoryard/internal/projectmemory"
)

type ProjectMemoryStore interface {
	StartChain(ctx context.Context, args projectmemory.StartChainArgs) error
	StartStep(ctx context.Context, args projectmemory.StartStepArgs) error
	StepRunning(ctx context.Context, args projectmemory.StepRunningArgs) error
	CompleteStep(ctx context.Context, args projectmemory.CompleteStepArgs) error
	CompleteChain(ctx context.Context, args projectmemory.CompleteChainArgs) error
	UpdateChainMetrics(ctx context.Context, args projectmemory.UpdateChainMetricsArgs) error
	SetChainStatus(ctx context.Context, args projectmemory.SetChainStatusArgs) error
	LogChainEvent(ctx context.Context, args projectmemory.LogChainEventArgs) error
	ReadChain(ctx context.Context, id string) (projectmemory.Chain, bool, error)
	ListChains(ctx context.Context, limit int) ([]projectmemory.Chain, error)
	ReadStep(ctx context.Context, id string) (projectmemory.ChainStep, bool, error)
	ListChainSteps(ctx context.Context, chainID string) ([]projectmemory.ChainStep, error)
	ListChainEvents(ctx context.Context, chainID string) ([]projectmemory.ChainEvent, error)
	ListChainEventsSince(ctx context.Context, chainID string, afterSequence uint64) ([]projectmemory.ChainEvent, error)
}

type projectMemoryBackend struct {
	memory ProjectMemoryStore
	now    func() time.Time
}

func NewProjectMemoryStore(memory ProjectMemoryStore) *Store {
	if memory == nil {
		return nil
	}
	return &Store{
		memory: projectMemoryBackend{memory: memory, now: time.Now},
		clock:  time.Now,
	}
}

func (b projectMemoryBackend) StartChain(ctx context.Context, spec ChainSpec) (string, error) {
	if b.memory == nil {
		return "", fmt.Errorf("start chain: project memory store is nil")
	}
	nowUS := b.nowUS()
	sourceSpecsJSON, err := marshalProjectMemoryStrings(spec.SourceSpecs)
	if err != nil {
		return "", fmt.Errorf("start chain: marshal source specs: %w", err)
	}
	if err := b.memory.StartChain(ctx, projectmemory.StartChainArgs{
		ID:               spec.ChainID,
		SourceSpecsJSON:  sourceSpecsJSON,
		SourceTask:       spec.SourceTask,
		MaxSteps:         nonNegativeUint64(spec.MaxSteps),
		MaxResolverLoops: nonNegativeUint64(spec.MaxResolverLoops),
		MaxDurationSecs:  nonNegativeUint64(int(spec.MaxDuration / time.Second)),
		TokenBudget:      nonNegativeUint64(spec.TokenBudget),
		CreatedAtUS:      nowUS,
	}); err != nil {
		return "", fmt.Errorf("start chain: %w", err)
	}
	return spec.ChainID, nil
}

func (b projectMemoryBackend) StartStep(ctx context.Context, spec StepSpec) (string, error) {
	if b.memory == nil {
		return "", fmt.Errorf("start step: project memory store is nil")
	}
	if err := b.memory.StartStep(ctx, projectmemory.StartStepArgs{
		ID:          spec.StepID,
		ChainID:     spec.ChainID,
		Sequence:    uint32(nonNegativeUint64(spec.SequenceNum)),
		Role:        spec.Role,
		Task:        spec.Task,
		TaskContext: spec.TaskContext,
		CreatedAtUS: b.nowUS(),
	}); err != nil {
		return "", fmt.Errorf("start step: %w", err)
	}
	return spec.StepID, nil
}

func (b projectMemoryBackend) StepRunning(ctx context.Context, stepID string) error {
	if err := b.memory.StepRunning(ctx, projectmemory.StepRunningArgs{ID: stepID, StartedAtUS: b.nowUS()}); err != nil {
		return fmt.Errorf("step running: %w", err)
	}
	return nil
}

func (b projectMemoryBackend) CompleteStep(ctx context.Context, params CompleteStepParams) error {
	args := projectmemory.CompleteStepArgs{
		ID:            params.StepID,
		Status:        params.Status,
		Verdict:       params.Verdict,
		ReceiptPath:   params.ReceiptPath,
		TokensUsed:    nonNegativeUint64(params.TokensUsed),
		TurnsUsed:     nonNegativeUint64(params.TurnsUsed),
		DurationSecs:  nonNegativeUint64(params.DurationSecs),
		Error:         params.ErrorMessage,
		CompletedAtUS: b.nowUS(),
	}
	if params.ExitCode != nil {
		args.ExitCode = int64(*params.ExitCode)
		args.HasExitCode = true
	}
	if err := b.memory.CompleteStep(ctx, args); err != nil {
		return fmt.Errorf("complete step: %w", err)
	}
	return nil
}

func (b projectMemoryBackend) CompleteChain(ctx context.Context, chainID, status, summary string) error {
	if err := b.memory.CompleteChain(ctx, projectmemory.CompleteChainArgs{ID: chainID, Status: status, Summary: summary, CompletedAtUS: b.nowUS()}); err != nil {
		return fmt.Errorf("complete chain: %w", err)
	}
	return nil
}

func (b projectMemoryBackend) UpdateChainMetrics(ctx context.Context, chainID string, metrics ChainMetrics) error {
	if err := b.memory.UpdateChainMetrics(ctx, projectmemory.UpdateChainMetricsArgs{
		ID:                chainID,
		TotalSteps:        nonNegativeUint64(metrics.TotalSteps),
		TotalTokens:       nonNegativeUint64(metrics.TotalTokens),
		TotalDurationSecs: nonNegativeUint64(metrics.TotalDurationSecs),
		ResolverLoops:     nonNegativeUint64(metrics.ResolverLoops),
		UpdatedAtUS:       b.nowUS(),
	}); err != nil {
		return fmt.Errorf("update chain metrics: %w", err)
	}
	return nil
}

func (b projectMemoryBackend) GetChain(ctx context.Context, chainID string) (*Chain, error) {
	row, found, err := b.memory.ReadChain(ctx, chainID)
	if err != nil {
		return nil, fmt.Errorf("get chain: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("get chain: %w", sql.ErrNoRows)
	}
	mapped, err := mapProjectMemoryChain(row)
	if err != nil {
		return nil, fmt.Errorf("get chain: %w", err)
	}
	return &mapped, nil
}

func (b projectMemoryBackend) ListChains(ctx context.Context, limit int) ([]Chain, error) {
	rows, err := b.memory.ListChains(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list chains: %w", err)
	}
	return mapRows(rows, "list chains", mapProjectMemoryChain)
}

func (b projectMemoryBackend) GetStep(ctx context.Context, stepID string) (*Step, error) {
	row, found, err := b.memory.ReadStep(ctx, stepID)
	if err != nil {
		return nil, fmt.Errorf("get step: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("get step: %w", sql.ErrNoRows)
	}
	mapped := mapProjectMemoryStep(row)
	return &mapped, nil
}

func (b projectMemoryBackend) ListSteps(ctx context.Context, chainID string) ([]Step, error) {
	rows, err := b.memory.ListChainSteps(ctx, chainID)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}
	steps := make([]Step, 0, len(rows))
	for _, row := range rows {
		steps = append(steps, mapProjectMemoryStep(row))
	}
	return steps, nil
}

func (b projectMemoryBackend) SetChainStatus(ctx context.Context, chainID, status string) error {
	if err := b.memory.SetChainStatus(ctx, projectmemory.SetChainStatusArgs{ID: chainID, Status: status, UpdatedAtUS: b.nowUS()}); err != nil {
		return fmt.Errorf("set chain status: %w", err)
	}
	return nil
}

func (b projectMemoryBackend) CountResolverStepsForContext(ctx context.Context, chainID, taskContext string) (int, error) {
	steps, err := b.ListSteps(ctx, chainID)
	if err != nil {
		return 0, fmt.Errorf("count resolver steps: %w", err)
	}
	var count int
	for _, step := range steps {
		if step.Role == "resolver" && step.TaskContext == taskContext {
			count++
		}
	}
	return count, nil
}

func (b projectMemoryBackend) LogEvent(ctx context.Context, chainID string, stepID string, eventType EventType, eventData any) error {
	payloadJSON := emptyJSONPayload
	if eventData != nil {
		data, err := json.Marshal(eventData)
		if err != nil {
			return fmt.Errorf("log event: marshal payload: %w", err)
		}
		payloadJSON = string(data)
	}
	if err := b.memory.LogChainEvent(ctx, projectmemory.LogChainEventArgs{
		ChainID:     chainID,
		StepID:      stepID,
		EventType:   string(eventType),
		PayloadJSON: payloadJSON,
		CreatedAtUS: b.nowUS(),
	}); err != nil {
		return fmt.Errorf("log event: %w", err)
	}
	return nil
}

func (b projectMemoryBackend) ListEvents(ctx context.Context, chainID string) ([]Event, error) {
	rows, err := b.memory.ListChainEvents(ctx, chainID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	return mapProjectMemoryEvents(rows), nil
}

func (b projectMemoryBackend) ListEventsSince(ctx context.Context, chainID string, afterID int64) ([]Event, error) {
	rows, err := b.memory.ListChainEventsSince(ctx, chainID, nonNegativeUint64(int(afterID)))
	if err != nil {
		return nil, fmt.Errorf("list events since: %w", err)
	}
	return mapProjectMemoryEvents(rows), nil
}

const emptyJSONPayload = "{}"

type projectMemoryChainMetrics struct {
	TotalSteps        int `json:"total_steps"`
	TotalTokens       int `json:"total_tokens"`
	TotalDurationSecs int `json:"total_duration_secs"`
	ResolverLoops     int `json:"resolver_loops"`
}

type projectMemoryChainLimits struct {
	MaxSteps         int `json:"max_steps"`
	MaxResolverLoops int `json:"max_resolver_loops"`
	MaxDurationSecs  int `json:"max_duration_secs"`
	TokenBudget      int `json:"token_budget"`
}

func mapProjectMemoryChain(row projectmemory.Chain) (Chain, error) {
	var metrics projectMemoryChainMetrics
	if err := unmarshalOptionalJSON(row.MetricsJSON, &metrics); err != nil {
		return Chain{}, fmt.Errorf("decode metrics_json: %w", err)
	}
	var limits projectMemoryChainLimits
	if err := unmarshalOptionalJSON(row.LimitsJSON, &limits); err != nil {
		return Chain{}, fmt.Errorf("decode limits_json: %w", err)
	}
	return Chain{
		ID:                row.ID,
		SourceSpecs:       unmarshalProjectMemoryStrings(row.SourceSpecsJSON),
		SourceTask:        row.SourceTask,
		Status:            row.Status,
		Summary:           row.Summary,
		TotalSteps:        metrics.TotalSteps,
		TotalTokens:       metrics.TotalTokens,
		TotalDurationSecs: metrics.TotalDurationSecs,
		ResolverLoops:     metrics.ResolverLoops,
		MaxSteps:          limits.MaxSteps,
		MaxResolverLoops:  limits.MaxResolverLoops,
		MaxDurationSecs:   limits.MaxDurationSecs,
		TokenBudget:       limits.TokenBudget,
		StartedAt:         unixMicro(row.StartedAtUS),
		CompletedAt:       nullableUnixMicro(row.CompletedAtUS),
		CreatedAt:         unixMicro(row.CreatedAtUS),
		UpdatedAt:         unixMicro(row.UpdatedAtUS),
	}, nil
}

func mapProjectMemoryStep(row projectmemory.ChainStep) Step {
	var exitCode *int
	if row.HasExitCode {
		value := int(row.ExitCode)
		exitCode = &value
	}
	return Step{
		ID:           row.ID,
		ChainID:      row.ChainID,
		SequenceNum:  int(row.Sequence),
		Role:         row.Role,
		Task:         row.Task,
		TaskContext:  row.TaskContext,
		Status:       row.Status,
		Verdict:      row.Verdict,
		ReceiptPath:  row.ReceiptPath,
		TokensUsed:   int(row.TokensUsed),
		TurnsUsed:    int(row.TurnsUsed),
		DurationSecs: int(row.DurationSecs),
		ExitCode:     exitCode,
		ErrorMessage: row.Error,
		StartedAt:    nullableUnixMicro(row.StartedAtUS),
		CompletedAt:  nullableUnixMicro(row.CompletedAtUS),
		CreatedAt:    unixMicro(row.CreatedAtUS),
	}
}

func mapProjectMemoryEvents(rows []projectmemory.ChainEvent) []Event {
	events := make([]Event, 0, len(rows))
	for _, row := range rows {
		events = append(events, Event{
			ID:        int64(row.Sequence),
			ChainID:   row.ChainID,
			StepID:    row.StepID,
			EventType: EventType(row.EventType),
			EventData: row.PayloadJSON,
			CreatedAt: unixMicro(row.CreatedAtUS),
		})
	}
	return events
}

func (b projectMemoryBackend) nowUS() uint64 {
	now := b.now
	if now == nil {
		now = time.Now
	}
	return uint64(now().UTC().UnixMicro())
}

func unixMicro(value uint64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.UnixMicro(int64(value)).UTC()
}

func nullableUnixMicro(value uint64) *time.Time {
	if value == 0 {
		return nil
	}
	ts := unixMicro(value)
	return &ts
}

func marshalProjectMemoryStrings(values []string) (string, error) {
	if len(values) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalProjectMemoryStrings(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{raw}
	}
	return out
}

func unmarshalOptionalJSON(raw string, target any) error {
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), target)
}

func nonNegativeUint64(value int) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}
