package router

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ponchione/sirtopham/internal/provider"
	"github.com/ponchione/sirtopham/internal/provider/tracking"
)

// mockProvider implements provider.Provider with controllable behavior.
type mockProvider struct {
	mu            sync.Mutex
	name          string
	models        []provider.Model
	modelsErr     error
	completeResp  *provider.Response
	completeErr   error
	streamCh      <-chan provider.StreamEvent
	streamErr     error
	completeCalls int
	streamCalls   int
	lastReq       *provider.Request // captures last request for inspection
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Complete(_ context.Context, req *provider.Request) (*provider.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completeCalls++
	// Capture a snapshot of the model for verification.
	m.lastReq = &provider.Request{Model: req.Model}
	return m.completeResp, m.completeErr
}

func (m *mockProvider) Stream(_ context.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.streamCalls++
	m.lastReq = &provider.Request{Model: req.Model}
	return m.streamCh, m.streamErr
}

func (m *mockProvider) Models(_ context.Context) ([]provider.Model, error) {
	return m.models, m.modelsErr
}

func (m *mockProvider) getCompleteCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.completeCalls
}

func (m *mockProvider) getStreamCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.streamCalls
}

// validConfig returns a RouterConfig suitable for most tests.
func validConfig() RouterConfig {
	return RouterConfig{
		Default: RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
}

// --- Constructor tests ---

func TestNewRouter_ValidConfig(t *testing.T) {
	r, err := NewRouter(validConfig(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil router")
	}
}

func TestNewRouter_MissingDefaultProvider(t *testing.T) {
	cfg := RouterConfig{Default: RouteTarget{Provider: "", Model: "claude-sonnet-4-6"}}
	_, err := NewRouter(cfg, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing default provider")
	}
	if got := err.Error(); !contains(got, "routing.default.provider is required") {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestNewRouter_MissingDefaultModel(t *testing.T) {
	cfg := RouterConfig{Default: RouteTarget{Provider: "anthropic", Model: ""}}
	_, err := NewRouter(cfg, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing default model")
	}
	if got := err.Error(); !contains(got, "routing.default.model is required") {
		t.Fatalf("unexpected error: %s", got)
	}
}

// --- Registration tests ---

func TestRegisterProvider_Success(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)
	mp := &mockProvider{name: "anthropic"}

	if err := r.RegisterProvider(mp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	health := r.ProviderHealthMap()
	h, ok := health["anthropic"]
	if !ok {
		t.Fatal("expected health entry for anthropic")
	}
	if !h.Healthy {
		t.Fatal("expected provider to be healthy on registration")
	}
}

func TestRegisterProvider_Nil(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)
	err := r.RegisterProvider(nil)
	if err == nil {
		t.Fatal("expected error for nil provider")
	}
	if got := err.Error(); !contains(got, "cannot register nil provider") {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestRegisterProvider_Duplicate(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)
	mp := &mockProvider{name: "anthropic"}
	_ = r.RegisterProvider(mp)

	err := r.RegisterProvider(mp)
	if err == nil {
		t.Fatal("expected error for duplicate provider")
	}
	if got := err.Error(); !contains(got, "provider already registered: anthropic") {
		t.Fatalf("unexpected error: %s", got)
	}
}

// --- Complete routing tests ---

func TestComplete_DefaultRouting(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)
	resp := &provider.Response{Model: "claude-sonnet-4-6"}
	mp := &mockProvider{name: "anthropic", completeResp: resp}
	_ = r.RegisterProvider(mp)

	req := &provider.Request{}
	got, err := r.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != resp {
		t.Fatal("expected response from anthropic mock")
	}
	if mp.getCompleteCalls() != 1 {
		t.Fatalf("expected 1 complete call, got %d", mp.getCompleteCalls())
	}
	// Verify the model was set during the call.
	if mp.lastReq.Model != "claude-sonnet-4-6" {
		t.Fatalf("expected model claude-sonnet-4-6, got %s", mp.lastReq.Model)
	}
	// Verify the caller's request was not mutated.
	if req.Model != "" {
		t.Fatalf("expected req.Model to be restored to empty, got %s", req.Model)
	}
}

func TestComplete_PerRequestOverride(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)

	anthropicResp := &provider.Response{Model: "claude-sonnet-4-6"}
	anthropicMock := &mockProvider{
		name:         "anthropic",
		completeResp: anthropicResp,
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

	req := &provider.Request{Model: "qwen2.5-coder-7b"}
	got, err := r.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != localResp {
		t.Fatal("expected response from local mock")
	}
	if localMock.getCompleteCalls() != 1 {
		t.Fatalf("expected 1 local complete call, got %d", localMock.getCompleteCalls())
	}
	if anthropicMock.getCompleteCalls() != 0 {
		t.Fatalf("expected 0 anthropic complete calls, got %d", anthropicMock.getCompleteCalls())
	}
}

func TestComplete_OverrideModelNotFound(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)
	resp := &provider.Response{Model: "claude-sonnet-4-6"}
	mp := &mockProvider{
		name:         "anthropic",
		completeResp: resp,
		models:       []provider.Model{{ID: "claude-sonnet-4-6"}},
	}
	_ = r.RegisterProvider(mp)

	req := &provider.Request{Model: "nonexistent-model"}
	got, err := r.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != resp {
		t.Fatal("expected fallthrough to default provider")
	}
	if mp.getCompleteCalls() != 1 {
		t.Fatalf("expected 1 complete call on default, got %d", mp.getCompleteCalls())
	}
}

func TestComplete_FallbackOnRetriableError(t *testing.T) {
	cfg := validConfig()
	cfg.Fallback = &RouteTarget{Provider: "local", Model: "qwen2.5-coder-7b"}
	r, _ := NewRouter(cfg, nil, nil)

	anthropicMock := &mockProvider{
		name: "anthropic",
		completeErr: &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: 502,
			Message:    "bad gateway",
			Retriable:  true,
		},
	}
	localResp := &provider.Response{Model: "qwen2.5-coder-7b"}
	localMock := &mockProvider{
		name:         "local",
		completeResp: localResp,
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	req := &provider.Request{}
	got, err := r.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != localResp {
		t.Fatal("expected response from fallback provider")
	}
	if anthropicMock.getCompleteCalls() != 1 {
		t.Fatalf("expected 1 anthropic call, got %d", anthropicMock.getCompleteCalls())
	}
	if localMock.getCompleteCalls() != 1 {
		t.Fatalf("expected 1 local call, got %d", localMock.getCompleteCalls())
	}
}

func TestComplete_AuthErrorNoFallback(t *testing.T) {
	cfg := validConfig()
	cfg.Fallback = &RouteTarget{Provider: "local", Model: "qwen2.5-coder-7b"}
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

	req := &provider.Request{}
	_, err := r.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !contains(err.Error(), "authentication failed") {
		t.Fatalf("expected auth error message, got: %s", err.Error())
	}
	if !contains(err.Error(), "Check your API key") {
		t.Fatalf("expected remediation message, got: %s", err.Error())
	}
	if anthropicMock.getCompleteCalls() != 1 {
		t.Fatalf("expected 1 anthropic call, got %d", anthropicMock.getCompleteCalls())
	}
	if localMock.getCompleteCalls() != 0 {
		t.Fatalf("expected 0 local calls (no fallback on auth), got %d", localMock.getCompleteCalls())
	}
}

func TestComplete_AuthError403NoFallback(t *testing.T) {
	cfg := validConfig()
	cfg.Fallback = &RouteTarget{Provider: "local", Model: "qwen2.5-coder-7b"}
	r, _ := NewRouter(cfg, nil, nil)

	anthropicMock := &mockProvider{
		name: "anthropic",
		completeErr: &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: 403,
			Message:    "forbidden",
			Retriable:  false,
		},
	}
	localMock := &mockProvider{
		name:         "local",
		completeResp: &provider.Response{Model: "qwen2.5-coder-7b"},
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	req := &provider.Request{}
	_, err := r.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !contains(err.Error(), "authentication failed") {
		t.Fatalf("expected auth error message, got: %s", err.Error())
	}
	if localMock.getCompleteCalls() != 0 {
		t.Fatalf("expected 0 local calls, got %d", localMock.getCompleteCalls())
	}
}

func TestComplete_NonRetriableErrorNoFallback(t *testing.T) {
	cfg := validConfig()
	cfg.Fallback = &RouteTarget{Provider: "local", Model: "qwen2.5-coder-7b"}
	r, _ := NewRouter(cfg, nil, nil)

	origErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 400,
		Message:    "bad request",
		Retriable:  false,
	}
	anthropicMock := &mockProvider{
		name:        "anthropic",
		completeErr: origErr,
	}
	localMock := &mockProvider{
		name:         "local",
		completeResp: &provider.Response{Model: "qwen2.5-coder-7b"},
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	req := &provider.Request{}
	_, err := r.Complete(context.Background(), req)
	if err != origErr {
		t.Fatalf("expected original error to be returned, got: %v", err)
	}
	if localMock.getCompleteCalls() != 0 {
		t.Fatalf("expected 0 local calls, got %d", localMock.getCompleteCalls())
	}
}

func TestComplete_FallbackAlsoFails(t *testing.T) {
	cfg := validConfig()
	cfg.Fallback = &RouteTarget{Provider: "local", Model: "qwen2.5-coder-7b"}
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
	fallbackErr := &provider.ProviderError{
		Provider:   "local",
		StatusCode: 500,
		Message:    "internal error",
		Retriable:  true,
	}
	localMock := &mockProvider{
		name:        "local",
		completeErr: fallbackErr,
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	req := &provider.Request{}
	_, err := r.Complete(context.Background(), req)
	if err != fallbackErr {
		t.Fatalf("expected fallback error, got: %v", err)
	}
	if anthropicMock.getCompleteCalls() != 1 {
		t.Fatalf("expected 1 anthropic call, got %d", anthropicMock.getCompleteCalls())
	}
	if localMock.getCompleteCalls() != 1 {
		t.Fatalf("expected 1 local call, got %d", localMock.getCompleteCalls())
	}
}

func TestComplete_NoFallbackConfigured(t *testing.T) {
	cfg := validConfig() // no fallback
	r, _ := NewRouter(cfg, nil, nil)

	retriableErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 429,
		Message:    "rate limited",
		Retriable:  true,
	}
	anthropicMock := &mockProvider{
		name:        "anthropic",
		completeErr: retriableErr,
	}
	_ = r.RegisterProvider(anthropicMock)

	req := &provider.Request{}
	_, err := r.Complete(context.Background(), req)
	if err != retriableErr {
		t.Fatalf("expected retriable error returned directly, got: %v", err)
	}
}

// --- Stream routing tests ---

func TestStream_DefaultRouting(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)

	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.TokenDelta{Text: "hello"}
	close(ch)

	mp := &mockProvider{
		name:     "anthropic",
		streamCh: ch,
	}
	_ = r.RegisterProvider(mp)

	req := &provider.Request{}
	got, err := r.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil stream channel")
	}
	if mp.getStreamCalls() != 1 {
		t.Fatalf("expected 1 stream call, got %d", mp.getStreamCalls())
	}

	// Drain the channel to verify events.
	event := <-got
	td, ok := event.(provider.TokenDelta)
	if !ok {
		t.Fatal("expected TokenDelta event")
	}
	if td.Text != "hello" {
		t.Fatalf("expected 'hello', got %s", td.Text)
	}
}

func TestStream_FallbackOnRetriableError(t *testing.T) {
	cfg := validConfig()
	cfg.Fallback = &RouteTarget{Provider: "local", Model: "qwen2.5-coder-7b"}
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

	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.TokenDelta{Text: "fallback"}
	close(ch)
	localMock := &mockProvider{
		name:     "local",
		streamCh: ch,
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	req := &provider.Request{}
	got, err := r.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil stream channel from fallback")
	}

	event := <-got
	td, ok := event.(provider.TokenDelta)
	if !ok {
		t.Fatal("expected TokenDelta event")
	}
	if td.Text != "fallback" {
		t.Fatalf("expected 'fallback', got %s", td.Text)
	}
}

// --- Health tracking tests ---

func TestHealthTracking_SuccessUpdatesHealth(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)
	mp := &mockProvider{
		name:         "anthropic",
		completeResp: &provider.Response{Model: "claude-sonnet-4-6"},
	}
	_ = r.RegisterProvider(mp)

	before := time.Now()
	_, _ = r.Complete(context.Background(), &provider.Request{})

	health := r.ProviderHealthMap()
	h := health["anthropic"]
	if !h.Healthy {
		t.Fatal("expected healthy after success")
	}
	if h.LastSuccessAt.Before(before) {
		t.Fatal("expected LastSuccessAt to be recent")
	}
}

func TestHealthTracking_FailureUpdatesHealth(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)
	testErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 400,
		Message:    "bad request",
		Retriable:  false,
	}
	mp := &mockProvider{
		name:        "anthropic",
		completeErr: testErr,
	}
	_ = r.RegisterProvider(mp)

	before := time.Now()
	_, _ = r.Complete(context.Background(), &provider.Request{})

	health := r.ProviderHealthMap()
	h := health["anthropic"]
	if h.Healthy {
		t.Fatal("expected unhealthy after failure")
	}
	if h.LastError != testErr {
		t.Fatal("expected LastError to be the test error")
	}
	if h.LastErrorAt.Before(before) {
		t.Fatal("expected LastErrorAt to be recent")
	}
}

func TestHealthTracking_FallbackUpdatesHealth(t *testing.T) {
	cfg := validConfig()
	cfg.Fallback = &RouteTarget{Provider: "local", Model: "qwen2.5-coder-7b"}
	r, _ := NewRouter(cfg, nil, nil)

	anthropicMock := &mockProvider{
		name: "anthropic",
		completeErr: &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: 502,
			Message:    "bad gateway",
			Retriable:  true,
		},
	}
	localMock := &mockProvider{
		name:         "local",
		completeResp: &provider.Response{Model: "qwen2.5-coder-7b"},
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	_, _ = r.Complete(context.Background(), &provider.Request{})

	health := r.ProviderHealthMap()
	if health["anthropic"].Healthy {
		t.Fatal("expected anthropic to be unhealthy")
	}
	if !health["local"].Healthy {
		t.Fatal("expected local to be healthy")
	}
}

// --- Models tests ---

func TestModels_AggregatesAllProviders(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)

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
		},
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	models, err := r.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}

	ids := make(map[string]bool)
	for _, m := range models {
		ids[m.ID] = true
	}
	for _, expected := range []string{"claude-sonnet-4-6", "claude-haiku-3.5", "qwen2.5-coder-7b"} {
		if !ids[expected] {
			t.Fatalf("missing expected model: %s", expected)
		}
	}
}

func TestModels_SkipsFailingProvider(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)

	anthropicMock := &mockProvider{
		name:      "anthropic",
		modelsErr: &provider.ProviderError{Provider: "anthropic", Message: "network error"},
	}
	localMock := &mockProvider{
		name:   "local",
		models: []provider.Model{{ID: "qwen2.5-coder-7b"}},
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	models, err := r.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ID != "qwen2.5-coder-7b" {
		t.Fatalf("expected qwen2.5-coder-7b, got %s", models[0].ID)
	}
}

// --- Name test ---

func TestName(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)
	if r.Name() != "router" {
		t.Fatalf("expected 'router', got %s", r.Name())
	}
}

// --- Sub-call tracking tests ---

// mockSubCallStore implements tracking.SubCallStore for testing.
type mockSubCallStore struct {
	mu    sync.Mutex
	calls []tracking.InsertSubCallParams
}

func (s *mockSubCallStore) InsertSubCall(_ context.Context, params tracking.InsertSubCallParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, params)
	return nil
}

func (s *mockSubCallStore) getCalls() []tracking.InsertSubCallParams {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]tracking.InsertSubCallParams{}, s.calls...)
}

func TestComplete_TracksSubCalls(t *testing.T) {
	store := &mockSubCallStore{}
	cfg := validConfig()
	r, _ := NewRouter(cfg, store, nil)

	mock := &mockProvider{
		name: "anthropic",
		completeResp: &provider.Response{
			Model: "claude-sonnet-4-6",
			Usage: provider.Usage{InputTokens: 100, OutputTokens: 50},
		},
	}
	_ = r.RegisterProvider(mock)

	req := &provider.Request{Purpose: "chat"}
	_, err := r.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := store.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 tracked call, got %d", len(calls))
	}
	if calls[0].Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", calls[0].Provider)
	}
	if calls[0].Purpose != "chat" {
		t.Errorf("expected purpose 'chat', got %q", calls[0].Purpose)
	}
	if calls[0].TokensIn != 100 {
		t.Errorf("expected 100 input tokens, got %d", calls[0].TokensIn)
	}
}

// --- Validate tests ---

func TestValidate_AnthropicAuthFailure(t *testing.T) {
	cfg := validConfig()
	r, _ := NewRouter(cfg, nil, nil)

	// Mock anthropic that fails Models() call (simulates auth failure)
	anthropicMock := &mockProvider{
		name:      "anthropic",
		modelsErr: &provider.ProviderError{Provider: "anthropic", StatusCode: 401, Message: "unauthorized"},
	}
	_ = r.RegisterProvider(anthropicMock)

	err := r.Validate(context.Background())
	// Only provider fails validation, so no providers left -> error
	if err == nil {
		t.Fatal("expected error when only provider fails validation")
	}
}

func TestValidate_UnreachableProviderUnregistered(t *testing.T) {
	cfg := RouterConfig{
		Default: RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
	r, _ := NewRouter(cfg, nil, nil)

	anthropicMock := &mockProvider{
		name:   "anthropic",
		models: []provider.Model{{ID: "claude-sonnet-4-6"}},
	}
	unreachableMock := &mockProvider{
		name:      "local",
		modelsErr: fmt.Errorf("connection refused"),
	}
	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(unreachableMock)

	err := r.Validate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Unreachable provider should have been unregistered
	health := r.ProviderHealthMap()
	if _, ok := health["local"]; ok {
		t.Error("expected unreachable provider to be unregistered")
	}
}

// --- Helper ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
