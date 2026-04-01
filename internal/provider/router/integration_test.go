package router

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ponchione/sirtopham/internal/provider"
)

func TestIntegration_FullLifecycle(t *testing.T) {
	cfg := RouterConfig{
		Default:  RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		Fallback: &RouteTarget{Provider: "local", Model: "qwen2.5-coder-7b"},
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
	localResp := &provider.Response{Model: "qwen2.5-coder-7b"}
	localMock := &mockProvider{
		name:         "local",
		completeResp: localResp,
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	// Step 5: Complete routes to anthropic.
	resp, err := r.Complete(context.Background(), &provider.Request{})
	if err != nil {
		t.Fatalf("step 5: unexpected error: %v", err)
	}
	if resp != anthropicResp {
		t.Fatal("step 5: expected anthropic response")
	}

	// Step 6: Health check.
	health := r.ProviderHealthMap()
	if !health["anthropic"].Healthy {
		t.Fatal("step 6: expected anthropic healthy")
	}
	if health["anthropic"].LastSuccessAt.IsZero() {
		t.Fatal("step 6: expected LastSuccessAt set")
	}

	// Step 7: Reconfigure anthropic to fail with retriable error.
	anthropicMock.mu.Lock()
	anthropicMock.completeResp = nil
	anthropicMock.completeErr = &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 502,
		Message:    "bad gateway",
		Retriable:  true,
	}
	anthropicMock.mu.Unlock()

	// Step 8: Complete falls back to local.
	resp, err = r.Complete(context.Background(), &provider.Request{})
	if err != nil {
		t.Fatalf("step 8: unexpected error: %v", err)
	}
	if resp != localResp {
		t.Fatal("step 8: expected local response from fallback")
	}

	// Step 9: Anthropic should be unhealthy.
	health = r.ProviderHealthMap()
	if health["anthropic"].Healthy {
		t.Fatal("step 9: expected anthropic unhealthy")
	}
	if health["anthropic"].LastError == nil {
		t.Fatal("step 9: expected LastError set")
	}

	// Step 10: Local should be healthy.
	if !health["local"].Healthy {
		t.Fatal("step 10: expected local healthy")
	}

	// Step 11: Reconfigure anthropic back to healthy.
	anthropicMock.mu.Lock()
	anthropicMock.completeResp = anthropicResp
	anthropicMock.completeErr = nil
	anthropicMock.mu.Unlock()

	// Step 12: Complete routes to anthropic again (always tries default first).
	resp, err = r.Complete(context.Background(), &provider.Request{})
	if err != nil {
		t.Fatalf("step 12: unexpected error: %v", err)
	}
	if resp != anthropicResp {
		t.Fatal("step 12: expected anthropic response")
	}

	// Step 13: Anthropic should be healthy again.
	health = r.ProviderHealthMap()
	if !health["anthropic"].Healthy {
		t.Fatal("step 13: expected anthropic healthy")
	}
}

func TestIntegration_PerRequestOverrideWithFallback(t *testing.T) {
	cfg := RouterConfig{
		Default:  RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		Fallback: &RouteTarget{Provider: "local", Model: "qwen2.5-coder-7b"},
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

	// Step 3: Override routes to local directly.
	req := &provider.Request{Model: "qwen2.5-coder-7b"}
	resp, err := r.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("step 3: unexpected error: %v", err)
	}
	if resp != localResp {
		t.Fatal("step 3: expected local response from override")
	}
	if anthropicMock.getCompleteCalls() != 0 {
		t.Fatal("step 3: anthropic should not have been called")
	}

	// Step 4: Configure local to fail with retriable error.
	retriableErr := &provider.ProviderError{
		Provider:   "local",
		StatusCode: 503,
		Message:    "service unavailable",
		Retriable:  true,
	}
	localMock.mu.Lock()
	localMock.completeResp = nil
	localMock.completeErr = retriableErr
	localMock.completeCalls = 0 // Reset counter for this phase.
	localMock.mu.Unlock()

	// Step 5: Override routes to local (fails), falls back to configured fallback (also local).
	req = &provider.Request{Model: "qwen2.5-coder-7b"}
	_, err = r.Complete(context.Background(), req)
	// Both the override attempt and the fallback attempt go to "local", both fail.
	if err == nil {
		t.Fatal("step 5: expected error when both override and fallback fail")
	}
	if localMock.getCompleteCalls() != 2 {
		t.Fatalf("step 5: expected 2 local calls (override + fallback), got %d", localMock.getCompleteCalls())
	}
}

func TestIntegration_AuthErrorSurfacesImmediately(t *testing.T) {
	cfg := RouterConfig{
		Default:  RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		Fallback: &RouteTarget{Provider: "local", Model: "qwen2.5-coder-7b"},
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
	localMock := &mockProvider{
		name:         "local",
		completeResp: &provider.Response{Model: "qwen2.5-coder-7b"},
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	// Step 3: First call.
	_, err := r.Complete(context.Background(), &provider.Request{})
	if err == nil {
		t.Fatal("expected auth error")
	}
	errMsg := err.Error()
	if !contains(errMsg, "authentication failed for provider anthropic (HTTP 401)") {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if !contains(errMsg, "Check your API key") {
		t.Fatalf("missing remediation message: %s", errMsg)
	}

	// Step 4: Local never called.
	if localMock.getCompleteCalls() != 0 {
		t.Fatal("local should not have been called")
	}

	// Step 5: Anthropic should be unhealthy.
	health := r.ProviderHealthMap()
	if health["anthropic"].Healthy {
		t.Fatal("expected anthropic unhealthy")
	}

	// Step 6: Second call, same auth error.
	_, err = r.Complete(context.Background(), &provider.Request{})
	if err == nil {
		t.Fatal("expected auth error on second call")
	}
	if !contains(err.Error(), "authentication failed") {
		t.Fatalf("unexpected error on second call: %s", err.Error())
	}
	if localMock.getCompleteCalls() != 0 {
		t.Fatal("local should still not have been called")
	}
}

func TestIntegration_BothProvidersFail(t *testing.T) {
	cfg := RouterConfig{
		Default:  RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		Fallback: &RouteTarget{Provider: "local", Model: "qwen2.5-coder-7b"},
	}
	r, _ := NewRouter(cfg, nil, nil)

	anthropicMock := &mockProvider{
		name: "anthropic",
		completeErr: &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: 503,
			Message:    "service unavailable",
			Retriable:  true,
		},
	}
	localErr := &provider.ProviderError{
		Provider:   "local",
		StatusCode: 500,
		Message:    "internal error",
		Retriable:  true,
	}
	localMock := &mockProvider{
		name:        "local",
		completeErr: localErr,
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	// Step 3: Returns local's error, not anthropic's.
	_, err := r.Complete(context.Background(), &provider.Request{})
	if err != localErr {
		t.Fatalf("expected local error, got: %v", err)
	}

	// Step 4-5: Both unhealthy.
	health := r.ProviderHealthMap()
	if health["anthropic"].Healthy {
		t.Fatal("expected anthropic unhealthy")
	}
	if health["local"].Healthy {
		t.Fatal("expected local unhealthy")
	}
}

func TestIntegration_StreamFallback(t *testing.T) {
	cfg := RouterConfig{
		Default:  RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		Fallback: &RouteTarget{Provider: "local", Model: "qwen2.5-coder-7b"},
	}
	r, _ := NewRouter(cfg, nil, nil)

	anthropicMock := &mockProvider{
		name: "anthropic",
		streamErr: &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: 502,
			Message:    "bad gateway",
			Retriable:  true,
		},
	}

	ch := make(chan provider.StreamEvent, 2)
	ch <- provider.TokenDelta{Text: "hello"}
	ch <- provider.StreamDone{StopReason: provider.StopReasonEndTurn}
	close(ch)

	localMock := &mockProvider{
		name:     "local",
		streamCh: ch,
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	// Step 3: Stream falls back to local.
	got, err := r.Stream(context.Background(), &provider.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Step 4: Read events.
	event1 := <-got
	td, ok := event1.(provider.TokenDelta)
	if !ok {
		t.Fatal("expected TokenDelta")
	}
	if td.Text != "hello" {
		t.Fatalf("expected 'hello', got %s", td.Text)
	}

	event2 := <-got
	_, ok = event2.(provider.StreamDone)
	if !ok {
		t.Fatal("expected StreamDone")
	}

	// Step 5-6: Health checks.
	health := r.ProviderHealthMap()
	if health["anthropic"].Healthy {
		t.Fatal("expected anthropic unhealthy")
	}
	if !health["local"].Healthy {
		t.Fatal("expected local healthy")
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
	if !contains(err.Error(), "no providers configured; add at least one provider to sirtopham.yaml") {
		t.Fatalf("unexpected error: %s", err.Error())
	}
}

func TestIntegration_Validate_DefaultProviderUnavailable(t *testing.T) {
	cfg := RouterConfig{
		Default: RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
	r, _ := NewRouter(cfg, nil, nil)

	// Register both providers.
	anthropicMock := &mockProvider{name: "anthropic"}
	localMock := &mockProvider{
		name:   "local",
		models: []provider.Model{{ID: "local-model"}},
	}
	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	// Simulate anthropic being unregistered before Validate.
	r.mu.Lock()
	delete(r.providers, "anthropic")
	delete(r.health, "anthropic")
	r.mu.Unlock()

	err := r.Validate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default should now point to the remaining provider.
	r.mu.RLock()
	defaultProvider := r.config.Default.Provider
	defaultModel := r.config.Default.Model
	r.mu.RUnlock()

	if defaultProvider != "local" {
		t.Fatalf("expected default to be 'local', got '%s'", defaultProvider)
	}
	if defaultModel != "local-model" {
		t.Fatalf("expected default model to be 'local-model', got '%s'", defaultModel)
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
	localMock := &mockProvider{
		name:         "local",
		completeResp: &provider.Response{Model: "qwen2.5-coder-7b"},
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

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

	start := time.Now()
	const numGoroutines = 20
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = r.Complete(context.Background(), &provider.Request{})
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	// With no simulated delay, 20 concurrent calls should complete very quickly.
	if elapsed > 2*time.Second {
		t.Fatalf("concurrent calls took too long: %v", elapsed)
	}
}
