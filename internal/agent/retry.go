package agent

import (
	stdctx "context"
	"errors"
	"fmt"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

const (
	defaultRetryMaxAttempts = 3
	defaultRetryBaseDelay   = 1 * time.Second
	defaultRetryBackoffMult = 2
)

// streamWithRetry performs the LLM call (Stream + consumeStream) with retry
// logic for retriable errors (429, 500/502/503). Auth failures and context
// overflow errors are NOT retried — they fail immediately.
//
// This method does NOT increment the iteration counter on retries — a retry is
// not a new iteration.
//
// On each retriable failure, it emits an ErrorEvent(Recoverable=true), logs the
// attempt, waits with exponential backoff, and retries. If all attempts are
// exhausted, it emits ErrorEvent(Recoverable=false) and returns the error.
func (l *AgentLoop) streamWithRetry(
	ctx stdctx.Context,
	req *provider.Request,
	iteration int,
	conversationID string,
) (*streamResult, error) {
	var lastClassification streamErrorClassification
	delay := defaultRetryBaseDelay

	for attempt := 1; attempt <= defaultRetryMaxAttempts; attempt++ {
		// Try to stream.
		result, err := l.doStreamAttempt(ctx, req, iteration)
		if err == nil {
			return result, nil
		}

		if errors.Is(err, stdctx.Canceled) || errors.Is(err, stdctx.DeadlineExceeded) {
			return result, err
		}

		// Classify the error.
		classification := classifyStreamError(err)
		lastClassification = classification

		// Non-retriable: fail immediately.
		if !classification.Retriable {
			l.emit(ErrorEvent{
				ErrorCode:   classification.Code,
				Message:     classification.Message,
				Recoverable: false,
				Time:        l.now(),
			})
			return result, fmt.Errorf("agent loop: stream for iteration %d: %w", iteration, err)
		}

		// Retriable: log, emit, and wait.
		l.logger.Warn("retriable stream error, will retry",
			"conversation_id", conversationID,
			"iteration", iteration,
			"attempt", attempt,
			"max_attempts", defaultRetryMaxAttempts,
			"error_code", classification.Code,
			"delay", delay,
			"error", err,
		)

		l.emit(ErrorEvent{
			ErrorCode:   classification.Code,
			Message:     fmt.Sprintf("Attempt %d/%d: %s", attempt, defaultRetryMaxAttempts, classification.Message),
			Recoverable: true,
			Time:        l.now(),
		})

		// Last attempt — don't sleep, just fail.
		if attempt == defaultRetryMaxAttempts {
			break
		}

		// Wait with backoff, respecting server-suggested Retry-After.
		sleepDelay := delay
		if classification.ProviderError != nil && classification.ProviderError.RetryAfter > 0 {
			sleepDelay = max(sleepDelay, classification.ProviderError.RetryAfter)
		}
		if err := l.sleep(ctx, sleepDelay); err != nil {
			// Context cancelled during sleep.
			return nil, err
		}
		delay = time.Duration(int64(delay) * int64(defaultRetryBackoffMult))
	}

	// All retries exhausted.
	l.emit(ErrorEvent{
		ErrorCode:   lastClassification.Code,
		Message:     fmt.Sprintf("All %d retry attempts exhausted. %s", defaultRetryMaxAttempts, lastClassification.Message),
		Recoverable: false,
		Time:        l.now(),
	})
	return nil, fmt.Errorf("agent loop: stream for iteration %d: all %d attempts exhausted: %s",
		iteration, defaultRetryMaxAttempts, lastClassification.Message)
}

// doStreamAttempt performs a single Stream + consumeStream attempt.
func (l *AgentLoop) doStreamAttempt(
	ctx stdctx.Context,
	req *provider.Request,
	iteration int,
) (*streamResult, error) {
	streamCh, err := l.providerRouter.Stream(ctx, req)
	if err != nil {
		return nil, err
	}

	result, err := consumeStream(ctx, streamCh, l.emit, func() string {
		return l.now().UTC().Format(time.RFC3339)
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

// sleep waits for the specified duration, or returns early if the context is
// cancelled. Uses the AgentLoop's sleepFn for testability.
func (l *AgentLoop) sleep(ctx stdctx.Context, d time.Duration) error {
	sleepFn := l.sleepFn
	if sleepFn == nil {
		sleepFn = func(ctx stdctx.Context, d time.Duration) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
				return nil
			}
		}
	}
	return sleepFn(ctx, d)
}
