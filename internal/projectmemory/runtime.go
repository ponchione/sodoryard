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
