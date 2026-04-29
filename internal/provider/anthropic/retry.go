package anthropic

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

// classifyError returns a ProviderError based on the HTTP status code,
// response body, and Retry-After header value.
func classifyError(statusCode int, body []byte, retryAfterHeader string) *provider.ProviderError {
	retryAfter := provider.ParseRetryAfter(retryAfterHeader, time.Now())

	switch statusCode {
	case 401, 403:
		return &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: statusCode,
			Message:    "Anthropic authentication failed. Run 'claude login' to re-authenticate.",
			Retriable:  false,
		}
	case 429:
		return &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: statusCode,
			Message:    "Anthropic rate limit exceeded",
			Retriable:  true,
			RetryAfter: retryAfter,
		}
	case 400:
		bodyStr := string(body)
		if len(bodyStr) > 1024 {
			bodyStr = bodyStr[:1024]
		}
		return &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: statusCode,
			Message:    fmt.Sprintf("Anthropic bad request: %s", bodyStr),
			Retriable:  false,
		}
	case 500:
		return &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: statusCode,
			Message:    "Anthropic internal server error",
			Retriable:  true,
			RetryAfter: retryAfter,
		}
	case 502:
		return &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: statusCode,
			Message:    "Anthropic bad gateway",
			Retriable:  true,
			RetryAfter: retryAfter,
		}
	case 503:
		return &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: statusCode,
			Message:    "Anthropic service unavailable",
			Retriable:  true,
			RetryAfter: retryAfter,
		}
	default:
		bodyStr := string(body)
		if len(bodyStr) > 512 {
			bodyStr = bodyStr[:512]
		}
		return &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: statusCode,
			Message:    fmt.Sprintf("Anthropic API error (%d): %s", statusCode, bodyStr),
			Retriable:  false,
		}
	}
}

// classifyNetworkError wraps a network/connection error as a retriable
// ProviderError.
func classifyNetworkError(err error) *provider.ProviderError {
	return &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 0,
		Message:    fmt.Sprintf("Anthropic network error: %s", err),
		Retriable:  true,
		Err:        err,
	}
}

// doWithRetry executes fn up to 3 times with exponential backoff on retriable
// errors. Non-retriable errors are returned immediately.
func (p *AnthropicProvider) doWithRetry(ctx context.Context, fn func() (*http.Response, error)) (*http.Response, error) {
	const maxAttempts = 3
	baseDelay := 1 * time.Second

	var lastErr *provider.ProviderError

	for attempt := 0; attempt < maxAttempts; attempt++ {
		resp, err := fn()

		if err != nil {
			// Check if this is already a ProviderError (e.g., from buildHTTPRequest).
			var pe *provider.ProviderError
			if errors.As(err, &pe) {
				lastErr = pe
			} else {
				// Network-level error (no response received).
				lastErr = classifyNetworkError(err)
			}
			if !lastErr.Retriable || attempt == maxAttempts-1 {
				return nil, lastErr
			}
			if !p.backoff(ctx, baseDelay, attempt) {
				return nil, &provider.ProviderError{
					Provider:  "anthropic",
					Message:   "retry cancelled",
					Retriable: false,
					Err:       ctx.Err(),
				}
			}
			continue
		}

		if resp.StatusCode == 200 {
			return resp, nil
		}

		// Non-200: read body for error classification.
		retryAfterHeader := resp.Header.Get("Retry-After")
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()

		lastErr = classifyError(resp.StatusCode, body, retryAfterHeader)
		if !lastErr.Retriable || attempt == maxAttempts-1 {
			return nil, lastErr
		}

		if !p.backoff(ctx, baseDelay, attempt) {
			return nil, &provider.ProviderError{
				Provider:  "anthropic",
				Message:   "retry cancelled",
				Retriable: false,
				Err:       ctx.Err(),
			}
		}
	}

	return nil, lastErr
}

// backoff sleeps for an exponential backoff delay with jitter, respecting
// context cancellation. Returns true if the sleep completed, false if the
// context was cancelled.
func (p *AnthropicProvider) backoff(ctx context.Context, baseDelay time.Duration, attempt int) bool {
	delay := baseDelay * time.Duration(1<<uint(attempt)) // 1s, 2s, 4s
	jitter := time.Duration(rand.Int63n(int64(delay / 2)))
	totalDelay := delay + jitter

	if p.sleep == nil {
		return provider.SleepWithContext(ctx, totalDelay)
	}
	return p.sleep(ctx, totalDelay)
}
