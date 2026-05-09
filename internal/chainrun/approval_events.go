package chainrun

import (
	"context"
	"strings"

	"github.com/ponchione/sodoryard/internal/agent"
	"github.com/ponchione/sodoryard/internal/approval"
	"github.com/ponchione/sodoryard/internal/chain"
)

type approvalEventSink struct {
	ctx     context.Context
	store   *chain.Store
	chainID string
	wait    bool
	cancel  context.CancelFunc
}

type approvalEventSinkOptions struct {
	Wait   bool
	Cancel context.CancelFunc
}

func newApprovalEventSink(ctx context.Context, store *chain.Store, chainID string, opts approvalEventSinkOptions) agent.EventSink {
	return &approvalEventSink{ctx: ctx, store: store, chainID: chainID, wait: opts.Wait, cancel: opts.Cancel}
}

func (s *approvalEventSink) Emit(event agent.Event) {
	if s == nil || s.store == nil {
		return
	}
	toolEnd, ok := event.(agent.ToolCallEndEvent)
	if !ok {
		return
	}
	payload, ok := approval.PayloadFromToolResultDetails(toolEnd.Details)
	if !ok {
		return
	}
	payload = approval.WithDefaultChainStep(payload, s.chainID, "")
	chainID := strings.TrimSpace(s.chainID)
	if chainID == "" {
		chainID, _ = payload["chain_id"].(string)
		chainID = strings.TrimSpace(chainID)
	}
	if chainID == "" {
		return
	}
	ctx := context.Background()
	if s.ctx != nil {
		ctx = context.WithoutCancel(s.ctx)
	}
	_ = s.store.LogEvent(ctx, chainID, approval.StepID(payload), chain.EventApprovalRequired, payload)
	if s.wait {
		_ = chain.ApplyTerminalChainClosure(ctx, s.store, chainID, chain.TerminalChainClosure{
			Status:    chain.StatusWaitingApproval,
			EventType: chain.EventChainWaitingApproval,
			Extra: map[string]any{
				"approval_id": payload["approval_id"],
				"tool_name":   payload["tool_name"],
				"status":      chain.StatusWaitingApproval,
			},
		})
		if s.cancel != nil {
			s.cancel()
		}
	}
}

func (s *approvalEventSink) Close() {}
