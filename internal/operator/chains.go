package operator

import (
	"context"
	"errors"

	"github.com/ponchione/sodoryard/internal/chain"
)

const defaultChainListLimit = 20

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
	return ChainDetail{Chain: *ch, Steps: steps, RecentEvents: events}, nil
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
