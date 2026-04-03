package router

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ponchione/sirtopham/internal/provider"
)

func TestIntegration_FullLifecycle(t *testing.T) {
	cfg := RouterConfig{
		Default: RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
	r, err := NewRouter(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	anthropicResp := &provider.Response{Model: "claude-sonnet-4-6"}
	anthropicMock := &mockProvider{
		name:         "anthropic",
		completeResp: anthropicResp,
	}

	_ = r.RegisterProvider(anthropicMock)

	// Step 1: Complete routes to anthropic.
	resp, err := r.Complete(context.Background(), &provider.Request{})
	if err != nil {
		t.Fatalf("step 1: unexpected error: %v", err)
	}
	if resp != anthropicResp {
		t.Fatal("step 1: expected anthropic response")
	}

	// Step 2: Health check.
	health := r.ProviderHealthMap()
	if !health["anthropic"].Healthy {
		t.Fatal("step 2: expected anthropic healthy")
	}
	if health["anthropic"].LastSuccessAt.IsZero() {
		t.Fatal("step 2: expected LastSuccessAt set")
	}

	// Step 3: Reconfigure anthropic to fail with retriable error.
	anthropicMock.mu.Lock()
	anthropicMock.completeResp = nil
	anthropicMock.completeErr = &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 502,
		Message:    "bad gateway",
		Retriable:  true,
	}
	anthropicMock.mu.Unlock()

	// Step 4: Complete returns error directly — no fallback.
	_, err = r.Complete(context.Background(), &provider.Request{})
	if err == nil {
		t.Fatal("step 4: expected error from failed provider")
	}

	// Step 5: Anthropic should be unhealthy.
	health = r.ProviderHealthMap()
	if health["anthropic"].Healthy {
		t.Fatal("step 5: expected anthropic unhealthy")
	}
	if health["anthropic"].LastError == nil {
		t.Fatal("step 5: expected LastError set")
	}

	// Step 6: Reconfigure anthropic back to healthy.
	anthropicMock.mu.Lock()
	anthropicMock.completeResp = anthropicResp
	anthropicMock.completeErr = nil
	anthropicMock.mu.Unlock()

	// Step 7: Complete routes to anthropic again.
	resp, err = r.Complete(context.Background(), &provider.Request{})
	if err != nil {
		t.Fatalf("step 7: unexpected error: %v", err)
	}
	if resp != anthropicResp {
		t.Fatal("step 7: expected anthropic response")
	}

	// Step 8: Anthropic should be healthy again.
	health = r.ProviderHealthMap()
	if !health["anthropic"].Healthy {
		t.Fatal("step 8: expected anthropic healthy")
	}
}

func TestIntegration_PerRequestOverride(t *testing.T) {
	cfg := RouterConfig{
		Default: RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
	r, _ := NewRouter(cfg, nil, nil)

	anthropicMock := &mockProvider{
		name:         "anthropic",
		completeResp: &provider.Response{Model: "claude-sonnet-4-6"},
		models:       []provider.Model{{ID: "claude-sonnet-4-6"}},
	}
	localResp := &provider.Response{Model: "qwen2.5-coder-7b"}
	localMock := &mockProvider{
		name:         "local",
		completeResp: localResp,
		models:       []provider.Model{{ID: "qwen2.5-coder-7b"}},
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	// Override routes to local directly.
	req := &provider.Request{Model: "qwen2.5-coder-7b"}
	resp, err := r.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != localResp {
		t.Fatal("expected local response from override")
	}
	if anthropicMock.getCompleteCalls() != 0 {
		t.Fatal("anthropic should not have been called")
	}

	// Configure local to fail — error returned directly, no fallback.
	localMock.mu.Lock()
	localMock.completeResp = nil
	localMock.completeErr = &provider.ProviderError{
		Provider:   "local",
		StatusCode: 503,
		Message:    "service unavailable",
		Retriable:  true,
	}
	localMock.completeCalls = 0
	localMock.mu.Unlock()

	req = &provider.Request{Model: "qwen2.5-coder-7b"}
	_, err = r.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when override provider fails")
	}
	if localMock.getCompleteCalls() != 1 {
		t.Fatalf("expected 1 local call, got %d", localMock.getCompleteCalls())
	}
}

func TestIntegration_AuthErrorSurfacesImmediately(t *testing.T) {
	cfg := RouterConfig{
		Default: RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
	r, _ := NewRouter(cfg, nil, nil)

	anthropicMock := &mockProvider{
		name: "anthropic",
		completeErr: &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: 401,
			Message:    "invalid api key",
			Retriable:  false,
		},
	}

	_ = r.RegisterProvider(anthropicMock)

	// First call — auth error surfaces immediately.
	_, err := r.Complete(context.Background(), &provider.Request{})
	if err == nil {
		t.Fatal("expected auth error")
	}
	errMsg := err.Error()
	if !contains(errMsg, "authentication failed for provider anthropic (HTTP 401)") {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if !contains(errMsg, "ANTHROPIC_API_KEY") && !contains(errMsg, "claude login") {
		t.Fatalf("missing remediation message: %s", errMsg)
	}

	// Anthropic should be unhealthy.
	health := r.ProviderHealthMap()
	if health["anthropic"].Healthy {
		t.Fatal("expected anthropic unhealthy")
	}

	// Second call, same auth error.
	_, err = r.Complete(context.Background(), &provider.Request{})
	if err == nil {
		t.Fatal("expected auth error on second call")
	}
	if !contains(err.Error(), "authentication failed") {
		t.Fatalf("unexpected error on second call: %s", err.Error())
	}
}

func TestIntegration_ProviderFailureReturnsError(t *testing.T) {
	cfg := RouterConfig{
		Default: RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
	r, _ := NewRouter(cfg, nil, nil)

	anthropicErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 503,
		Message:    "service unavailable",
		Retriable:  true,
	}
	anthropicMock := &mockProvider{
		name:        "anthropic",
		completeErr: anthropicErr,
	}

	_ = r.RegisterProvider(anthropicMock)

	// Error returned directly — no fallback.
	_, err := r.Complete(context.Background(), &provider.Request{})
	if err != anthropicErr {
		t.Fatalf("expected anthropic error returned directly, got: %v", err)
	}

	// Provider should be unhealthy.
	health := r.ProviderHealthMap()
	if health["anthropic"].Healthy {
		t.Fatal("expected anthropic unhealthy")
	}
}

func TestIntegration_StreamErrorReturnsDirectly(t *testing.T) {
	cfg := RouterConfig{
		Default: RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
	r, _ := NewRouter(cfg, nil, nil)

	streamErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 502,
		Message:    "bad gateway",
		Retriable:  true,
	}
	anthropicMock := &mockProvider{
		name:      "anthropic",
		streamErr: streamErr,
	}

	_ = r.RegisterProvider(anthropicMock)

	// Error returned directly — no fallback.
	_, err := r.Stream(context.Background(), &provider.Request{})
	if err != streamErr {
		t.Fatalf("expected stream error returned directly, got: %v", err)
	}

	// Health checks.
	health := r.ProviderHealthMap()
	if health["anthropic"].Healthy {
		t.Fatal("expected anthropic unhealthy")
	}
}

func TestIntegration_Validate_NoProviders(t *testing.T) {
	cfg := RouterConfig{
		Default: RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
	r, _ := NewRouter(cfg, nil, nil)

	err := r.Validate(context.Background())
	if err == nil {
		t.Fatal("expected error for no providers")
	}
	if !contains(err.Error(), "no providers configured; add at least one provider to the project's YAML config") {
		t.Fatalf("unexpected error: %s", err.Error())
	}
}

func TestIntegration_Validate_DefaultProviderUnavailable(t *testing.T) {
	cfg := RouterConfig{
		Default: RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
	r, _ := NewRouter(cfg, nil, nil)

	// Register both providers but anthropic fails.
	anthropicMock := &mockProvider{
		name:      "anthropic",
		modelsErr: fmt.Errorf("unauthorized"),
	}
	localMock := &mockProvider{
		name:   "local",
		models: []provider.Model{{ID: "local-model"}},
	}
	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	// Hard stop — no fallback.
	err := r.Validate(context.Background())
	if err == nil {
		t.Fatal("expected hard error when default provider is unavailable")
	}
	if !contains(err.Error(), "default provider") {
		t.Fatalf("unexpected error: %s", err.Error())
	}
}

func TestIntegration_ModelsAggregation(t *testing.T) {
	cfg := RouterConfig{
		Default: RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
	r, _ := NewRouter(cfg, nil, nil)

	anthropicMock := &mockProvider{
		name: "anthropic",
		models: []provider.Model{
			{ID: "claude-sonnet-4-6"},
			{ID: "claude-haiku-3.5"},
		},
	}
	localMock := &mockProvider{
		name: "local",
		models: []provider.Model{
			{ID: "qwen2.5-coder-7b"},
			{ID: "llama3-8b"},
		},
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	models, err := r.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 4 {
		t.Fatalf("expected 4 models, got %d", len(models))
	}

	ids := make(map[string]bool)
	for _, m := range models {
		ids[m.ID] = true
	}
	for _, expected := range []string{"claude-sonnet-4-6", "claude-haiku-3.5", "qwen2.5-coder-7b", "llama3-8b"} {
		if !ids[expected] {
			t.Fatalf("missing model: %s", expected)
		}
	}
}

func TestIntegration_ConcurrentRequests(t *testing.T) {
	cfg := RouterConfig{
		Default: RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
	r, _ := NewRouter(cfg, nil, nil)

	anthropicMock := &mockProvider{
		name:         "anthropic",
		completeResp: &provider.Response{Model: "claude-sonnet-4-6"},
	}

	_ = r.RegisterProvider(anthropicMock)

	const numGoroutines = 20
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errCh := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			// Each goroutine creates its own request to avoid data races on req.Model.
			req := &provider.Request{}
			_, err := r.Complete(context.Background(), req)
			if err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent call failed: %v", err)
	}

	if anthropicMock.getCompleteCalls() != numGoroutines {
		t.Fatalf("expected %d anthropic calls, got %d", numGoroutines, anthropicMock.getCompleteCalls())
	}

	health := r.ProviderHealthMap()
	if !health["anthropic"].Healthy {
		t.Fatal("expected anthropic healthy after concurrent calls")
	}
}

func TestIntegration_ConcurrentRequests_Timing(t *testing.T) {
	// Verify that concurrent requests complete in reasonable time,
	// demonstrating that locks don't serialize unnecessarily.
	cfg := RouterConfig{
		Default: RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
	r, _ := NewRouter(cfg, nil, nil)

	anthropicMock := &mockProvider{
		name:         "anthropic",
		completeResp: &provider.Response{Model: "claude-sonnet-4-6"},
	}
	_ = r.RegisterProvider(anthropicMock)

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = r.Complete(context.Background(), &provider.Request{})
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("concurrent requests took %v, expected < 2s (possible lock contention)", elapsed)
	}
}
