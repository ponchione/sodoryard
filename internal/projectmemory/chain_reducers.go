package projectmemory

import (
	"fmt"
	"strings"

	"github.com/ponchione/shunter/schema"
	"github.com/ponchione/shunter/types"
)

type StartChainArgs struct {
	ID               string `json:"id"`
	SourceSpecsJSON  string `json:"source_specs_json"`
	SourceTask       string `json:"source_task"`
	MaxSteps         uint64 `json:"max_steps"`
	MaxResolverLoops uint64 `json:"max_resolver_loops"`
	MaxDurationSecs  uint64 `json:"max_duration_secs"`
	TokenBudget      uint64 `json:"token_budget"`
	CreatedAtUS      uint64 `json:"created_at_us"`
}

type StartStepArgs struct {
	ID          string `json:"id"`
	ChainID     string `json:"chain_id"`
	Sequence    uint32 `json:"sequence"`
	Role        string `json:"role"`
	Task        string `json:"task"`
	TaskContext string `json:"task_context"`
	CreatedAtUS uint64 `json:"created_at_us"`
}

type StepRunningArgs struct {
	ID          string `json:"id"`
	StartedAtUS uint64 `json:"started_at_us"`
}

type CompleteStepArgs struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	Verdict       string `json:"verdict"`
	ReceiptPath   string `json:"receipt_path"`
	TokensUsed    uint64 `json:"tokens_used"`
	TurnsUsed     uint64 `json:"turns_used"`
	DurationSecs  uint64 `json:"duration_secs"`
	ExitCode      int64  `json:"exit_code"`
	HasExitCode   bool   `json:"has_exit_code"`
	Error         string `json:"error"`
	CompletedAtUS uint64 `json:"completed_at_us"`
}

type CompleteChainArgs struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	Summary       string `json:"summary"`
	CompletedAtUS uint64 `json:"completed_at_us"`
}

type UpdateChainMetricsArgs struct {
	ID                string `json:"id"`
	TotalSteps        uint64 `json:"total_steps"`
	TotalTokens       uint64 `json:"total_tokens"`
	TotalDurationSecs uint64 `json:"total_duration_secs"`
	ResolverLoops     uint64 `json:"resolver_loops"`
	UpdatedAtUS       uint64 `json:"updated_at_us"`
}

type SetChainStatusArgs struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	UpdatedAtUS uint64 `json:"updated_at_us"`
}

type LogChainEventArgs struct {
	ID          string `json:"id"`
	ChainID     string `json:"chain_id"`
	StepID      string `json:"step_id"`
	EventType   string `json:"event_type"`
	PayloadJSON string `json:"payload_json"`
	CreatedAtUS uint64 `json:"created_at_us"`
}

type chainMetricsPayload struct {
	TotalSteps        uint64 `json:"total_steps"`
	TotalTokens       uint64 `json:"total_tokens"`
	TotalDurationSecs uint64 `json:"total_duration_secs"`
	ResolverLoops     uint64 `json:"resolver_loops"`
}

type chainLimitsPayload struct {
	MaxSteps         uint64 `json:"max_steps"`
	MaxResolverLoops uint64 `json:"max_resolver_loops"`
	MaxDurationSecs  uint64 `json:"max_duration_secs"`
	TokenBudget      uint64 `json:"token_budget"`
}

func startChainReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args StartChainArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(args.ID)
	if id == "" {
		return nil, fmt.Errorf("chain id is required")
	}
	if _, _, found := findChainByID(ctx.DB, id); found {
		return nil, fmt.Errorf("chain already exists: %s", id)
	}
	nowUS := nonZeroUS(args.CreatedAtUS, reducerNowUS(ctx))
	if _, err := ctx.DB.Insert(uint32(tableChains), chainRow(Chain{
		ID:              id,
		SourceSpecsJSON: defaultString(args.SourceSpecsJSON, emptyJSONArray),
		SourceTask:      args.SourceTask,
		Status:          "running",
		CreatedAtUS:     nowUS,
		UpdatedAtUS:     nowUS,
		StartedAtUS:     nowUS,
		MetricsJSON:     mustJSON(chainMetricsPayload{}),
		LimitsJSON: mustJSON(chainLimitsPayload{
			MaxSteps:         args.MaxSteps,
			MaxResolverLoops: args.MaxResolverLoops,
			MaxDurationSecs:  args.MaxDurationSecs,
			TokenBudget:      args.TokenBudget,
		}),
		ControlJSON: emptyJSONObject,
	})); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{OperationID: id})
}

func startStepReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args StartStepArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(args.ID)
	if id == "" {
		id = ChainStepID(args.ChainID, fmt.Sprint(args.Sequence), args.Role, args.Task)
	}
	chainID := strings.TrimSpace(args.ChainID)
	if chainID == "" {
		return nil, fmt.Errorf("step chain id is required")
	}
	if _, chain, found := findChainByID(ctx.DB, chainID); !found || chain.Status == "" {
		return nil, fmt.Errorf("chain not found: %s", chainID)
	}
	if args.Sequence == 0 {
		return nil, fmt.Errorf("step sequence is required")
	}
	role := strings.TrimSpace(args.Role)
	if role == "" {
		return nil, fmt.Errorf("step role is required")
	}
	task := strings.TrimSpace(args.Task)
	if task == "" {
		return nil, fmt.Errorf("step task is required")
	}
	if _, _, found := firstRow(ctx.DB.SeekIndex(uint32(tableSteps), uint32(indexStepsPrimary), types.NewString(id))); found {
		return nil, fmt.Errorf("step already exists: %s", id)
	}
	if _, err := ctx.DB.Insert(uint32(tableSteps), chainStepRow(ChainStep{
		ID:          id,
		ChainID:     chainID,
		Sequence:    args.Sequence,
		Role:        role,
		Task:        task,
		TaskContext: args.TaskContext,
		Status:      "pending",
		CreatedAtUS: nonZeroUS(args.CreatedAtUS, reducerNowUS(ctx)),
	})); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{OperationID: id})
}

func stepRunningReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args StepRunningArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	rowID, step, found := findStepByID(ctx.DB, strings.TrimSpace(args.ID))
	if !found {
		return nil, fmt.Errorf("step not found: %s", args.ID)
	}
	step.Status = "running"
	step.StartedAtUS = nonZeroUS(args.StartedAtUS, reducerNowUS(ctx))
	if _, err := ctx.DB.Update(uint32(tableSteps), rowID, chainStepRow(step)); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{OperationID: step.ID})
}

func completeStepReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args CompleteStepArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	rowID, step, found := findStepByID(ctx.DB, strings.TrimSpace(args.ID))
	if !found {
		return nil, fmt.Errorf("step not found: %s", args.ID)
	}
	status := strings.TrimSpace(args.Status)
	if status == "" {
		status = "completed"
	}
	step.Status = status
	step.Verdict = args.Verdict
	step.ReceiptPath = args.ReceiptPath
	step.TokensUsed = args.TokensUsed
	step.TurnsUsed = args.TurnsUsed
	step.DurationSecs = args.DurationSecs
	step.ExitCode = args.ExitCode
	step.HasExitCode = args.HasExitCode
	step.Error = args.Error
	step.CompletedAtUS = nonZeroUS(args.CompletedAtUS, reducerNowUS(ctx))
	if _, err := ctx.DB.Update(uint32(tableSteps), rowID, chainStepRow(step)); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{OperationID: step.ID})
}

func completeChainReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args CompleteChainArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	rowID, chain, found := findChainByID(ctx.DB, strings.TrimSpace(args.ID))
	if !found {
		return nil, fmt.Errorf("chain not found: %s", args.ID)
	}
	status := strings.TrimSpace(args.Status)
	if status == "" {
		return nil, fmt.Errorf("chain status is required")
	}
	completedAtUS := nonZeroUS(args.CompletedAtUS, reducerNowUS(ctx))
	chain.Status = status
	chain.Summary = args.Summary
	chain.CompletedAtUS = completedAtUS
	chain.UpdatedAtUS = completedAtUS
	if _, err := ctx.DB.Update(uint32(tableChains), rowID, chainRow(chain)); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{OperationID: chain.ID})
}

func updateChainMetricsReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args UpdateChainMetricsArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	rowID, chain, found := findChainByID(ctx.DB, strings.TrimSpace(args.ID))
	if !found {
		return nil, fmt.Errorf("chain not found: %s", args.ID)
	}
	chain.MetricsJSON = mustJSON(chainMetricsPayload{
		TotalSteps:        args.TotalSteps,
		TotalTokens:       args.TotalTokens,
		TotalDurationSecs: args.TotalDurationSecs,
		ResolverLoops:     args.ResolverLoops,
	})
	chain.UpdatedAtUS = nonZeroUS(args.UpdatedAtUS, reducerNowUS(ctx))
	if _, err := ctx.DB.Update(uint32(tableChains), rowID, chainRow(chain)); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{OperationID: chain.ID})
}

func setChainStatusReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args SetChainStatusArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	rowID, chain, found := findChainByID(ctx.DB, strings.TrimSpace(args.ID))
	if !found {
		return nil, fmt.Errorf("chain not found: %s", args.ID)
	}
	status := strings.TrimSpace(args.Status)
	if status == "" {
		return nil, fmt.Errorf("chain status is required")
	}
	chain.Status = status
	chain.UpdatedAtUS = nonZeroUS(args.UpdatedAtUS, reducerNowUS(ctx))
	if _, err := ctx.DB.Update(uint32(tableChains), rowID, chainRow(chain)); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{OperationID: chain.ID})
}

func logChainEventReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args LogChainEventArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	chainID := strings.TrimSpace(args.ChainID)
	if chainID == "" {
		return nil, fmt.Errorf("event chain id is required")
	}
	if _, _, found := findChainByID(ctx.DB, chainID); !found {
		return nil, fmt.Errorf("chain not found: %s", chainID)
	}
	eventType := strings.TrimSpace(args.EventType)
	if eventType == "" {
		return nil, fmt.Errorf("event type is required")
	}
	sequence := nextChainEventSequence(ctx.DB, chainID)
	id := strings.TrimSpace(args.ID)
	if id == "" {
		id = ChainEventID(chainID, sequence)
	}
	if _, _, found := firstRow(ctx.DB.SeekIndex(uint32(tableEvents), uint32(indexEventsPrimary), types.NewString(id))); found {
		return nil, fmt.Errorf("event already exists: %s", id)
	}
	if _, err := ctx.DB.Insert(uint32(tableEvents), chainEventRow(ChainEvent{
		ID:          id,
		ChainID:     chainID,
		StepID:      strings.TrimSpace(args.StepID),
		Sequence:    sequence,
		EventType:   eventType,
		CreatedAtUS: nonZeroUS(args.CreatedAtUS, reducerNowUS(ctx)),
		PayloadJSON: defaultString(args.PayloadJSON, emptyJSONObject),
	})); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{OperationID: id})
}

func findChainByID(db types.ReducerDB, id string) (types.RowID, Chain, bool) {
	rowID, row, ok := firstRow(db.SeekIndex(uint32(tableChains), uint32(indexChainsPrimary), types.NewString(id)))
	if !ok {
		return 0, Chain{}, false
	}
	return rowID, decodeChainRow(row), true
}

func findStepByID(db types.ReducerDB, id string) (types.RowID, ChainStep, bool) {
	rowID, row, ok := firstRow(db.SeekIndex(uint32(tableSteps), uint32(indexStepsPrimary), types.NewString(id)))
	if !ok {
		return 0, ChainStep{}, false
	}
	return rowID, decodeChainStepRow(row), true
}

func nextChainEventSequence(db types.ReducerDB, chainID string) uint64 {
	var maxSequence uint64
	for _, row := range db.SeekIndex(uint32(tableEvents), uint32(indexEventsChain), types.NewString(chainID)) {
		sequence := row[3].AsUint64()
		if sequence > maxSequence {
			maxSequence = sequence
		}
	}
	return maxSequence + 1
}
