package projectmemory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ponchione/shunter"
)

type Config struct {
	DataDir    string
	DurableAck bool
}

type Runtime struct {
	rt         *shunter.Runtime
	durableAck bool
}

type SubCallRecorder interface {
	RecordSubCall(ctx context.Context, args RecordSubCallArgs) error
}

type ToolExecutionRecorder interface {
	RecordToolExecution(ctx context.Context, args RecordToolExecutionArgs) error
}

type ContextReportStore interface {
	StoreContextReport(ctx context.Context, args StoreContextReportArgs) error
	ReadContextReport(ctx context.Context, conversationID string, turnNumber uint32) (ContextReport, bool, error)
	UpdateContextReportQuality(ctx context.Context, args UpdateContextReportQualityArgs) error
}

type ChainStore interface {
	StartChain(ctx context.Context, args StartChainArgs) error
	StartStep(ctx context.Context, args StartStepArgs) error
	StepRunning(ctx context.Context, args StepRunningArgs) error
	CompleteStep(ctx context.Context, args CompleteStepArgs) error
	CompleteChain(ctx context.Context, args CompleteChainArgs) error
	UpdateChainMetrics(ctx context.Context, args UpdateChainMetricsArgs) error
	SetChainStatus(ctx context.Context, args SetChainStatusArgs) error
	LogChainEvent(ctx context.Context, args LogChainEventArgs) error
	ReadChain(ctx context.Context, id string) (Chain, bool, error)
	ListChains(ctx context.Context, limit int) ([]Chain, error)
	ReadStep(ctx context.Context, id string) (ChainStep, bool, error)
	ListChainSteps(ctx context.Context, chainID string) ([]ChainStep, error)
	ListChainEvents(ctx context.Context, chainID string) ([]ChainEvent, error)
	ListChainEventsSince(ctx context.Context, chainID string, afterSequence uint64) ([]ChainEvent, error)
}

func Open(ctx context.Context, cfg Config) (*Runtime, error) {
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("project memory shunter data dir is required")
	}
	rt, err := shunter.Build(NewModule(), shunter.Config{DataDir: cfg.DataDir})
	if err != nil {
		return nil, fmt.Errorf("build project memory runtime: %w", err)
	}
	if err := rt.Start(ctx); err != nil {
		_ = rt.Close()
		return nil, fmt.Errorf("start project memory runtime: %w", err)
	}
	return &Runtime{rt: rt, durableAck: cfg.DurableAck}, nil
}

func (r *Runtime) Close() error {
	if r == nil || r.rt == nil {
		return nil
	}
	return r.rt.Close()
}

func (r *Runtime) callReducerJSON(ctx context.Context, name string, args any) ([]byte, error) {
	if r == nil || r.rt == nil {
		return nil, fmt.Errorf("project memory runtime is not open")
	}
	payload, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("encode %s args: %w", name, err)
	}
	res, err := r.rt.CallReducer(ctx, name, payload)
	if err != nil {
		return nil, err
	}
	if res.Status != shunter.StatusCommitted {
		if res.Error != nil {
			return nil, fmt.Errorf("%s failed: %w", name, res.Error)
		}
		return nil, fmt.Errorf("%s failed with status %v", name, res.Status)
	}
	if r.durableAck {
		if err := r.rt.WaitUntilDurable(ctx, res.TxID); err != nil {
			return nil, fmt.Errorf("wait for durable %s tx %d: %w", name, res.TxID, err)
		}
	}
	return res.ReturnBSATN, nil
}

func (r *Runtime) WriteDocument(ctx context.Context, args WriteDocumentArgs) error {
	_, err := r.callReducerJSON(ctx, "write_document", args)
	return err
}

func (r *Runtime) PatchDocument(ctx context.Context, args PatchDocumentArgs) error {
	_, err := r.callReducerJSON(ctx, "patch_document", args)
	return err
}

func (r *Runtime) DeleteDocument(ctx context.Context, args DeleteDocumentArgs) error {
	_, err := r.callReducerJSON(ctx, "delete_document", args)
	return err
}

func (r *Runtime) MarkBrainIndexDirty(ctx context.Context, args MarkBrainIndexDirtyArgs) error {
	_, err := r.callReducerJSON(ctx, "mark_brain_index_dirty", args)
	return err
}

func (r *Runtime) MarkBrainIndexClean(ctx context.Context, args MarkBrainIndexCleanArgs) error {
	_, err := r.callReducerJSON(ctx, "mark_brain_index_clean", args)
	return err
}

func (r *Runtime) MarkCodeIndexDirty(ctx context.Context, args MarkCodeIndexDirtyArgs) error {
	_, err := r.callReducerJSON(ctx, "mark_code_index_dirty", args)
	return err
}

func (r *Runtime) MarkCodeIndexClean(ctx context.Context, args MarkCodeIndexCleanArgs) error {
	_, err := r.callReducerJSON(ctx, "mark_code_index_clean", args)
	return err
}

func (r *Runtime) CreateConversation(ctx context.Context, args CreateConversationArgs) error {
	_, err := r.callReducerJSON(ctx, "create_conversation", args)
	return err
}

func (r *Runtime) DeleteConversation(ctx context.Context, args DeleteConversationArgs) error {
	_, err := r.callReducerJSON(ctx, "delete_conversation", args)
	return err
}

func (r *Runtime) SetConversationTitle(ctx context.Context, args SetConversationTitleArgs) error {
	_, err := r.callReducerJSON(ctx, "set_conversation_title", args)
	return err
}

func (r *Runtime) SetRuntimeDefaults(ctx context.Context, args SetRuntimeDefaultsArgs) error {
	_, err := r.callReducerJSON(ctx, "set_runtime_defaults", args)
	return err
}

func (r *Runtime) AppendUserMessage(ctx context.Context, args AppendUserMessageArgs) error {
	_, err := r.callReducerJSON(ctx, "append_user_message", args)
	return err
}

func (r *Runtime) PersistIteration(ctx context.Context, args PersistIterationArgs) error {
	_, err := r.callReducerJSON(ctx, "persist_iteration", args)
	return err
}

func (r *Runtime) CancelIteration(ctx context.Context, args CancelIterationArgs) error {
	_, err := r.callReducerJSON(ctx, "cancel_iteration", args)
	return err
}

func (r *Runtime) DiscardTurn(ctx context.Context, args DiscardTurnArgs) error {
	_, err := r.callReducerJSON(ctx, "discard_turn", args)
	return err
}

func (r *Runtime) RecordSubCall(ctx context.Context, args RecordSubCallArgs) error {
	_, err := r.callReducerJSON(ctx, "record_sub_call", args)
	return err
}

func (r *Runtime) RecordToolExecution(ctx context.Context, args RecordToolExecutionArgs) error {
	_, err := r.callReducerJSON(ctx, "record_tool_execution", args)
	return err
}

func (r *Runtime) StoreContextReport(ctx context.Context, args StoreContextReportArgs) error {
	_, err := r.callReducerJSON(ctx, "store_context_report", args)
	return err
}

func (r *Runtime) UpdateContextReportQuality(ctx context.Context, args UpdateContextReportQualityArgs) error {
	_, err := r.callReducerJSON(ctx, "update_context_report_quality", args)
	return err
}

func (r *Runtime) StartChain(ctx context.Context, args StartChainArgs) error {
	_, err := r.callReducerJSON(ctx, "start_chain", args)
	return err
}

func (r *Runtime) StartStep(ctx context.Context, args StartStepArgs) error {
	_, err := r.callReducerJSON(ctx, "start_step", args)
	return err
}

func (r *Runtime) StepRunning(ctx context.Context, args StepRunningArgs) error {
	_, err := r.callReducerJSON(ctx, "step_running", args)
	return err
}

func (r *Runtime) CompleteStep(ctx context.Context, args CompleteStepArgs) error {
	_, err := r.callReducerJSON(ctx, "complete_step", args)
	return err
}

func (r *Runtime) CompleteChain(ctx context.Context, args CompleteChainArgs) error {
	_, err := r.callReducerJSON(ctx, "complete_chain", args)
	return err
}

func (r *Runtime) UpdateChainMetrics(ctx context.Context, args UpdateChainMetricsArgs) error {
	_, err := r.callReducerJSON(ctx, "update_chain_metrics", args)
	return err
}

func (r *Runtime) SetChainStatus(ctx context.Context, args SetChainStatusArgs) error {
	_, err := r.callReducerJSON(ctx, "set_chain_status", args)
	return err
}

func (r *Runtime) LogChainEvent(ctx context.Context, args LogChainEventArgs) error {
	_, err := r.callReducerJSON(ctx, "log_chain_event", args)
	return err
}
