package chain

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ponchione/sodoryard/internal/chaininput"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/id"
)

type Store struct {
	q      *appdb.Queries
	db     *sql.DB
	memory storeBackend
	clock  func() time.Time
}

type storeBackend interface {
	StartChain(ctx context.Context, spec ChainSpec) (string, error)
	StartStep(ctx context.Context, spec StepSpec) (string, error)
	StepRunning(ctx context.Context, stepID string) error
	CompleteStep(ctx context.Context, params CompleteStepParams) error
	CompleteChain(ctx context.Context, chainID, status, summary string) error
	UpdateChainMetrics(ctx context.Context, chainID string, metrics ChainMetrics) error
	GetChain(ctx context.Context, chainID string) (*Chain, error)
	ListChains(ctx context.Context, limit int) ([]Chain, error)
	GetStep(ctx context.Context, stepID string) (*Step, error)
	ListSteps(ctx context.Context, chainID string) ([]Step, error)
	SetChainStatus(ctx context.Context, chainID, status string) error
	CountResolverStepsForContext(ctx context.Context, chainID, taskContext string) (int, error)
	LogEvent(ctx context.Context, chainID string, stepID string, eventType EventType, eventData any) error
	ListEvents(ctx context.Context, chainID string) ([]Event, error)
	ListEventsSince(ctx context.Context, chainID string, afterID int64) ([]Event, error)
}

type ChainSpec struct {
	ChainID          string
	SourceSpecs      []string
	SourceTask       string
	MaxSteps         int
	MaxResolverLoops int
	MaxDuration      time.Duration
	TokenBudget      int
}

type StepSpec struct {
	StepID      string
	ChainID     string
	SequenceNum int
	Role        string
	Task        string
	TaskContext string
}

type ChainMetrics struct {
	TotalSteps        int
	TotalTokens       int
	TotalDurationSecs int
	ResolverLoops     int
}

type CompleteStepParams struct {
	StepID       string
	Status       string
	Verdict      string
	ReceiptPath  string
	TokensUsed   int
	TurnsUsed    int
	DurationSecs int
	ExitCode     *int
	ErrorMessage string
}

type Chain struct {
	ID                string
	SourceSpecs       []string
	SourceTask        string
	Status            string
	Summary           string
	TotalSteps        int
	TotalTokens       int
	TotalDurationSecs int
	ResolverLoops     int
	MaxSteps          int
	MaxResolverLoops  int
	MaxDurationSecs   int
	TokenBudget       int
	StartedAt         time.Time
	CompletedAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Step struct {
	ID           string
	ChainID      string
	SequenceNum  int
	Role         string
	Task         string
	TaskContext  string
	Status       string
	Verdict      string
	ReceiptPath  string
	TokensUsed   int
	TurnsUsed    int
	DurationSecs int
	ExitCode     *int
	ErrorMessage string
	StartedAt    *time.Time
	CompletedAt  *time.Time
	CreatedAt    time.Time
}

type Event struct {
	ID        int64
	ChainID   string
	StepID    string
	EventType EventType
	EventData string
	CreatedAt time.Time
}

func NewStore(db *sql.DB) *Store {
	return &Store{q: appdb.New(db), db: db, clock: time.Now}
}

func StoreWithClock(db *sql.DB, clk func() time.Time) *Store {
	s := NewStore(db)
	if clk != nil {
		s.clock = clk
	}
	return s
}

func (s *Store) StartChain(ctx context.Context, spec ChainSpec) (string, error) {
	chainID := spec.ChainID
	if chainID == "" {
		chainID = id.New()
	}
	limits := chaininput.NormalizeLimits(chaininput.Limits{
		MaxSteps:         spec.MaxSteps,
		MaxResolverLoops: spec.MaxResolverLoops,
		MaxDuration:      spec.MaxDuration,
		TokenBudget:      spec.TokenBudget,
	})
	maxDurationSecs := int(limits.MaxDuration / time.Second)
	maxSteps := limits.MaxSteps
	maxResolverLoops := limits.MaxResolverLoops
	tokenBudget := limits.TokenBudget
	if s != nil && s.memory != nil {
		spec.ChainID = chainID
		spec.MaxSteps = maxSteps
		spec.MaxResolverLoops = maxResolverLoops
		spec.MaxDuration = time.Duration(maxDurationSecs) * time.Second
		spec.TokenBudget = tokenBudget
		return s.memory.StartChain(ctx, spec)
	}
	if err := s.q.CreateChain(ctx, appdb.CreateChainParams{
		ID:               chainID,
		SourceSpecs:      marshalStrings(spec.SourceSpecs),
		SourceTask:       nullableString(spec.SourceTask),
		MaxSteps:         int64(maxSteps),
		MaxResolverLoops: int64(maxResolverLoops),
		MaxDurationSecs:  int64(maxDurationSecs),
		TokenBudget:      int64(tokenBudget),
	}); err != nil {
		return "", fmt.Errorf("start chain: %w", err)
	}
	return chainID, nil
}

func (s *Store) StartStep(ctx context.Context, spec StepSpec) (string, error) {
	stepID := spec.StepID
	if stepID == "" {
		stepID = id.New()
	}
	if s != nil && s.memory != nil {
		spec.StepID = stepID
		return s.memory.StartStep(ctx, spec)
	}
	if err := s.q.CreateStep(ctx, appdb.CreateStepParams{
		ID:          stepID,
		ChainID:     spec.ChainID,
		SequenceNum: int64(spec.SequenceNum),
		Role:        spec.Role,
		Task:        spec.Task,
		TaskContext: nullableString(spec.TaskContext),
	}); err != nil {
		return "", fmt.Errorf("start step: %w", err)
	}
	return stepID, nil
}

func (s *Store) StepRunning(ctx context.Context, stepID string) error {
	if s != nil && s.memory != nil {
		return s.memory.StepRunning(ctx, stepID)
	}
	if err := s.q.StartStep(ctx, stepID); err != nil {
		return fmt.Errorf("step running: %w", err)
	}
	return nil
}

func (s *Store) CompleteStep(ctx context.Context, params CompleteStepParams) error {
	if s != nil && s.memory != nil {
		return s.memory.CompleteStep(ctx, params)
	}
	if err := s.q.CompleteStep(ctx, appdb.CompleteStepParams{
		Status:       params.Status,
		Verdict:      nullableString(params.Verdict),
		ReceiptPath:  nullableString(params.ReceiptPath),
		TokensUsed:   int64(params.TokensUsed),
		TurnsUsed:    int64(params.TurnsUsed),
		DurationSecs: int64(params.DurationSecs),
		ExitCode:     nullableInt(params.ExitCode),
		ErrorMessage: nullableString(params.ErrorMessage),
		ID:           params.StepID,
	}); err != nil {
		return fmt.Errorf("complete step: %w", err)
	}
	return nil
}

func (s *Store) FailStep(ctx context.Context, params CompleteStepParams) error {
	params.Status = "failed"
	return s.CompleteStep(ctx, params)
}

func (s *Store) CompleteChain(ctx context.Context, chainID, status, summary string) error {
	if s != nil && s.memory != nil {
		return s.memory.CompleteChain(ctx, chainID, status, summary)
	}
	if err := s.q.CompleteChain(ctx, appdb.CompleteChainParams{Status: status, Summary: nullableString(summary), ID: chainID}); err != nil {
		return fmt.Errorf("complete chain: %w", err)
	}
	return nil
}

func (s *Store) UpdateChainMetrics(ctx context.Context, chainID string, metrics ChainMetrics) error {
	if s != nil && s.memory != nil {
		return s.memory.UpdateChainMetrics(ctx, chainID, metrics)
	}
	if err := s.q.UpdateChainMetrics(ctx, appdb.UpdateChainMetricsParams{
		TotalSteps:        int64(metrics.TotalSteps),
		TotalTokens:       int64(metrics.TotalTokens),
		TotalDurationSecs: int64(metrics.TotalDurationSecs),
		ResolverLoops:     int64(metrics.ResolverLoops),
		ID:                chainID,
	}); err != nil {
		return fmt.Errorf("update chain metrics: %w", err)
	}
	return nil
}

func (s *Store) GetChain(ctx context.Context, chainID string) (*Chain, error) {
	if s != nil && s.memory != nil {
		return s.memory.GetChain(ctx, chainID)
	}
	row, err := s.q.GetChain(ctx, chainID)
	if err != nil {
		return nil, fmt.Errorf("get chain: %w", err)
	}
	mapped, err := mapChain(row)
	if err != nil {
		return nil, fmt.Errorf("get chain: %w", err)
	}
	return &mapped, nil
}

func (s *Store) ListChains(ctx context.Context, limit int) ([]Chain, error) {
	if s != nil && s.memory != nil {
		return s.memory.ListChains(ctx, limit)
	}
	rows, err := s.q.ListChains(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("list chains: %w", err)
	}
	chains := make([]Chain, 0, len(rows))
	for _, row := range rows {
		mapped, err := mapChain(row)
		if err != nil {
			return nil, fmt.Errorf("list chains: %w", err)
		}
		chains = append(chains, mapped)
	}
	return chains, nil
}

func (s *Store) GetStep(ctx context.Context, stepID string) (*Step, error) {
	if s != nil && s.memory != nil {
		return s.memory.GetStep(ctx, stepID)
	}
	row, err := s.q.GetStep(ctx, stepID)
	if err != nil {
		return nil, fmt.Errorf("get step: %w", err)
	}
	mapped, err := mapStep(row)
	if err != nil {
		return nil, fmt.Errorf("get step: %w", err)
	}
	return &mapped, nil
}

func (s *Store) ListSteps(ctx context.Context, chainID string) ([]Step, error) {
	if s != nil && s.memory != nil {
		return s.memory.ListSteps(ctx, chainID)
	}
	rows, err := s.q.ListStepsByChain(ctx, chainID)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}
	steps := make([]Step, 0, len(rows))
	for _, row := range rows {
		mapped, err := mapStep(row)
		if err != nil {
			return nil, fmt.Errorf("list steps: %w", err)
		}
		steps = append(steps, mapped)
	}
	return steps, nil
}

func (s *Store) SetChainStatus(ctx context.Context, chainID, status string) error {
	if s != nil && s.memory != nil {
		return s.memory.SetChainStatus(ctx, chainID, status)
	}
	if err := s.q.UpdateChainStatus(ctx, appdb.UpdateChainStatusParams{Status: status, ID: chainID}); err != nil {
		return fmt.Errorf("set chain status: %w", err)
	}
	return nil
}

func (s *Store) CountResolverStepsForContext(ctx context.Context, chainID, taskContext string) (int, error) {
	if s != nil && s.memory != nil {
		return s.memory.CountResolverStepsForContext(ctx, chainID, taskContext)
	}
	count, err := s.q.CountResolverStepsForTaskContext(ctx, appdb.CountResolverStepsForTaskContextParams{ChainID: chainID, TaskContext: nullableString(taskContext)})
	if err != nil {
		return 0, fmt.Errorf("count resolver steps: %w", err)
	}
	return int(count), nil
}

func (s *Store) ListEvents(ctx context.Context, chainID string) ([]Event, error) {
	if s != nil && s.memory != nil {
		return s.memory.ListEvents(ctx, chainID)
	}
	rows, err := s.q.ListEventsByChain(ctx, chainID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	events := make([]Event, 0, len(rows))
	for _, row := range rows {
		mapped, err := mapEvent(row)
		if err != nil {
			return nil, fmt.Errorf("list events: %w", err)
		}
		events = append(events, mapped)
	}
	return events, nil
}

func mapChain(row appdb.Chain) (Chain, error) {
	startedAt, err := parseTime(row.StartedAt)
	if err != nil {
		return Chain{}, fmt.Errorf("parse started_at: %w", err)
	}
	createdAt, err := parseTime(row.CreatedAt)
	if err != nil {
		return Chain{}, fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := parseTime(row.UpdatedAt)
	if err != nil {
		return Chain{}, fmt.Errorf("parse updated_at: %w", err)
	}
	completedAt, err := parseNullableTime(row.CompletedAt)
	if err != nil {
		return Chain{}, fmt.Errorf("parse completed_at: %w", err)
	}
	return Chain{
		ID:                row.ID,
		SourceSpecs:       unmarshalStrings(row.SourceSpecs),
		SourceTask:        row.SourceTask.String,
		Status:            row.Status,
		Summary:           row.Summary.String,
		TotalSteps:        int(row.TotalSteps),
		TotalTokens:       int(row.TotalTokens),
		TotalDurationSecs: int(row.TotalDurationSecs),
		ResolverLoops:     int(row.ResolverLoops),
		MaxSteps:          int(row.MaxSteps),
		MaxResolverLoops:  int(row.MaxResolverLoops),
		MaxDurationSecs:   int(row.MaxDurationSecs),
		TokenBudget:       int(row.TokenBudget),
		StartedAt:         startedAt,
		CompletedAt:       completedAt,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
	}, nil
}

func mapStep(row appdb.Step) (Step, error) {
	createdAt, err := parseTime(row.CreatedAt)
	if err != nil {
		return Step{}, fmt.Errorf("parse created_at: %w", err)
	}
	startedAt, err := parseNullableTime(row.StartedAt)
	if err != nil {
		return Step{}, fmt.Errorf("parse started_at: %w", err)
	}
	completedAt, err := parseNullableTime(row.CompletedAt)
	if err != nil {
		return Step{}, fmt.Errorf("parse completed_at: %w", err)
	}
	var exitCode *int
	if row.ExitCode.Valid {
		v := int(row.ExitCode.Int64)
		exitCode = &v
	}
	return Step{
		ID:           row.ID,
		ChainID:      row.ChainID,
		SequenceNum:  int(row.SequenceNum),
		Role:         row.Role,
		Task:         row.Task,
		TaskContext:  row.TaskContext.String,
		Status:       row.Status,
		Verdict:      row.Verdict.String,
		ReceiptPath:  row.ReceiptPath.String,
		TokensUsed:   int(row.TokensUsed),
		TurnsUsed:    int(row.TurnsUsed),
		DurationSecs: int(row.DurationSecs),
		ExitCode:     exitCode,
		ErrorMessage: row.ErrorMessage.String,
		StartedAt:    startedAt,
		CompletedAt:  completedAt,
		CreatedAt:    createdAt,
	}, nil
}

func mapEvent(row appdb.Event) (Event, error) {
	createdAt, err := parseTime(row.CreatedAt)
	if err != nil {
		return Event{}, fmt.Errorf("parse created_at: %w", err)
	}
	return Event{ID: row.ID, ChainID: row.ChainID, StepID: row.StepID.String, EventType: EventType(row.EventType), EventData: row.EventData.String, CreatedAt: createdAt}, nil
}

func nullableString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func nullableInt(value *int) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*value), Valid: true}
}

func marshalStrings(values []string) sql.NullString {
	if len(values) == 0 {
		return sql.NullString{}
	}
	b, _ := json.Marshal(values)
	return sql.NullString{String: string(b), Valid: true}
}

func unmarshalStrings(value sql.NullString) []string {
	if !value.Valid || value.String == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(value.String), &out); err != nil {
		return []string{value.String}
	}
	return out
}

func parseTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp %q", value)
}

func parseNullableTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	ts, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &ts, nil
}
