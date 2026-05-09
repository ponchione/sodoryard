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
}

func newApprovalEventSink(ctx context.Context, store *chain.Store, chainID string) agent.EventSink {
	return &approvalEventSink{ctx: ctx, store: store, chainID: chainID}
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
}

func (s *approvalEventSink) Close() {}
