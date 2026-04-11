package chain

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrMaxStepsExceeded    = errors.New("chain limit: max_steps exceeded")
	ErrTokenBudgetExceeded = errors.New("chain limit: token_budget exceeded")
	ErrMaxDurationExceeded = errors.New("chain limit: max_duration exceeded")
	ErrResolverLoopCapHit  = errors.New("chain limit: max_resolver_loops exceeded")
	ErrChainNotRunning     = errors.New("chain limit: chain is not in running state")
)

type LimitCheckInput struct {
	Role        string
	TaskContext string
}

func (s *Store) CheckLimits(ctx context.Context, chainID string, in LimitCheckInput) error {
	ch, err := s.GetChain(ctx, chainID)
	if err != nil {
		return fmt.Errorf("limit check: load chain: %w", err)
	}
	if ch.Status != "running" {
		return ErrChainNotRunning
	}
	if ch.TotalSteps >= ch.MaxSteps {
		return fmt.Errorf("%w (current=%d max=%d)", ErrMaxStepsExceeded, ch.TotalSteps, ch.MaxSteps)
	}
	if ch.TotalTokens >= ch.TokenBudget {
		return fmt.Errorf("%w (current=%d budget=%d)", ErrTokenBudgetExceeded, ch.TotalTokens, ch.TokenBudget)
	}
	if !ch.StartedAt.IsZero() && s.clock().Sub(ch.StartedAt) >= time.Duration(ch.MaxDurationSecs)*time.Second {
		return fmt.Errorf("%w (elapsed=%s max=%ds)", ErrMaxDurationExceeded, s.clock().Sub(ch.StartedAt), ch.MaxDurationSecs)
	}
	if in.Role == "resolver" && in.TaskContext != "" {
		count, err := s.CountResolverStepsForContext(ctx, chainID, in.TaskContext)
		if err != nil {
			return fmt.Errorf("limit check: resolver count: %w", err)
		}
		if count >= ch.MaxResolverLoops {
			return fmt.Errorf("%w (task_context=%q count=%d max=%d)", ErrResolverLoopCapHit, in.TaskContext, count, ch.MaxResolverLoops)
		}
	}
	return nil
}
