package tracking

import (
	"context"
	"log/slog"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

// TrackedProvider implements provider.Provider, wrapping an inner provider to
// record every LLM invocation to the SubCallStore. Tracking failures (SQLite
// write errors) are logged but never block inference.
type TrackedProvider struct {
	inner  provider.Provider
	store  SubCallStore
	logger *slog.Logger
}

// Compile-time interface check.
var _ provider.Provider = (*TrackedProvider)(nil)

// NewTrackedProvider creates a TrackedProvider wrapping the given inner provider.
func NewTrackedProvider(inner provider.Provider, store SubCallStore, logger *slog.Logger) *TrackedProvider {
	return &TrackedProvider{
		inner:  inner,
		store:  store,
		logger: logger,
	}
}

// Name delegates directly to the inner provider.
func (tp *TrackedProvider) Name() string {
	return tp.inner.Name()
}

// Models delegates directly to the inner provider. No tracking is performed.
func (tp *TrackedProvider) Models(ctx context.Context) ([]provider.Model, error) {
	return tp.inner.Models(ctx)
}

// Ping delegates to the inner provider if it implements provider.Pinger.
// This allows the router to use lightweight reachability checks even when
// the provider is wrapped with tracking.
func (tp *TrackedProvider) Ping(ctx context.Context) error {
	if pinger, ok := tp.inner.(provider.Pinger); ok {
		return pinger.Ping(ctx)
	}
	// Inner provider does not implement Pinger — the router should fall back
	// to Models() on its own, but to satisfy the interface we return nil.
	return nil
}

// Compile-time check that TrackedProvider satisfies Pinger.
var _ provider.Pinger = (*TrackedProvider)(nil)
var _ provider.AuthStatusReporter = (*TrackedProvider)(nil)

// AuthStatus delegates to the inner provider if it implements
// provider.AuthStatusReporter.
func (tp *TrackedProvider) AuthStatus(ctx context.Context) (*provider.AuthStatus, error) {
	reporter, ok := tp.inner.(provider.AuthStatusReporter)
	if !ok {
		return nil, nil
	}
	return reporter.AuthStatus(ctx)
}

// Complete delegates to the inner provider, measures wall-clock latency, extracts
// usage from the response, and writes a sub_calls row via the SubCallStore.
// Tracking failures are logged but never block inference.
func (tp *TrackedProvider) Complete(ctx context.Context, req *provider.Request) (*provider.Response, error) {
	start := time.Now()
	resp, err := tp.inner.Complete(ctx, req)
	latencyMs := time.Since(start).Milliseconds()

	params := tp.buildParams(req, latencyMs)

	if err != nil {
		// Failed call.
		params.Success = 0
		errMsg := err.Error()
		params.ErrorMessage = &errMsg
		params.Model = req.Model // Use requested model since there's no response.

		// If a partial response is available, extract usage from it.
		if resp != nil {
			params.TokensIn = resp.Usage.InputTokens
			params.TokensOut = resp.Usage.OutputTokens
			params.CacheReadTokens = resp.Usage.CacheReadTokens
			params.CacheCreationTokens = resp.Usage.CacheCreationTokens
		}
	} else {
		// Successful call.
		params.Success = 1
		params.Model = resp.Model
		params.TokensIn = resp.Usage.InputTokens
		params.TokensOut = resp.Usage.OutputTokens
		params.CacheReadTokens = resp.Usage.CacheReadTokens
		params.CacheCreationTokens = resp.Usage.CacheCreationTokens
	}

	if storeErr := tp.store.InsertSubCall(ctx, params); storeErr != nil {
		tp.logger.Error("failed to record sub-call",
			"err", storeErr,
			"provider", params.Provider,
			"model", params.Model,
			"purpose", params.Purpose,
			"tokens_in", params.TokensIn,
			"tokens_out", params.TokensOut,
			"latency_ms", params.LatencyMs,
		)
	}

	return resp, err
}

// Stream wraps the inner provider's stream channel with a tracking channel.
// The wrapper passes all events through unchanged while intercepting usage and
// error events. When the inner channel closes, a sub_calls row is written.
// Tracking failures are logged but never block inference.
func (tp *TrackedProvider) Stream(ctx context.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	start := time.Now()
	ch, err := tp.inner.Stream(ctx, req)
	if err != nil {
		// Inner Stream call failed immediately.
		latencyMs := time.Since(start).Milliseconds()
		params := tp.buildParams(req, latencyMs)
		params.Success = 0
		params.Model = req.Model
		errMsg := err.Error()
		params.ErrorMessage = &errMsg

		if storeErr := tp.store.InsertSubCall(ctx, params); storeErr != nil {
			tp.logger.Error("failed to record sub-call for stream setup error",
				"err", storeErr,
				"provider", params.Provider,
				"model", params.Model,
				"purpose", params.Purpose,
				"tokens_in", params.TokensIn,
				"tokens_out", params.TokensOut,
				"latency_ms", params.LatencyMs,
			)
		}

		return nil, err
	}

	out := make(chan provider.StreamEvent)

	go func() {
		var finalUsage provider.Usage
		var streamErr error
		success := true

		for event := range ch {
			switch e := event.(type) {
			case provider.StreamUsage:
				finalUsage = e.Usage
			case provider.StreamDone:
				finalUsage = e.Usage
			case provider.StreamError:
				if e.Fatal {
					streamErr = e.Err
					success = false
				}
			}
			select {
			case out <- event:
			case <-ctx.Done():
				success = false
				streamErr = ctx.Err()
				goto done
			}
		}
	done:

		// If no usage events were received, mark as failure.
		if finalUsage == (provider.Usage{}) && success {
			success = false
		}

		latencyMs := time.Since(start).Milliseconds()
		params := tp.buildParams(req, latencyMs)
		params.Model = req.Model
		params.TokensIn = finalUsage.InputTokens
		params.TokensOut = finalUsage.OutputTokens
		params.CacheReadTokens = finalUsage.CacheReadTokens
		params.CacheCreationTokens = finalUsage.CacheCreationTokens

		if success {
			params.Success = 1
		} else {
			params.Success = 0
		}

		if streamErr != nil {
			errMsg := streamErr.Error()
			params.ErrorMessage = &errMsg
		}

		// Use context.Background() because the request ctx may be cancelled.
		if storeErr := tp.store.InsertSubCall(context.Background(), params); storeErr != nil {
			tp.logger.Error("failed to record sub-call for stream",
				"err", storeErr,
				"provider", params.Provider,
				"model", params.Model,
				"purpose", params.Purpose,
				"tokens_in", params.TokensIn,
				"tokens_out", params.TokensOut,
				"cache_read_tokens", params.CacheReadTokens,
				"cache_creation_tokens", params.CacheCreationTokens,
				"latency_ms", params.LatencyMs,
			)
		}

		close(out)
	}()

	return out, nil
}

// buildParams creates a base InsertSubCallParams with common fields populated
// from the request. Caller must set Success, Model, token counts, and ErrorMessage.
func (tp *TrackedProvider) buildParams(req *provider.Request, latencyMs int64) InsertSubCallParams {
	params := InsertSubCallParams{
		Provider:  tp.inner.Name(),
		Purpose:   req.Purpose,
		LatencyMs: latencyMs,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if req.ConversationID != "" {
		convID := req.ConversationID
		params.ConversationID = &convID
	}
	if req.TurnNumber != 0 {
		tn := req.TurnNumber
		params.TurnNumber = &tn
	}
	if req.Iteration != 0 {
		iter := req.Iteration
		params.Iteration = &iter
	}

	return params
}
