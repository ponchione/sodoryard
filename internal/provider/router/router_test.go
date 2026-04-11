package router

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
	"github.com/ponchione/sodoryard/internal/provider/tracking"
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
	lastReqPtr    *provider.Request // captures original pointer passed to provider
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Complete(_ context.Context, req *provider.Request) (*provider.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completeCalls++
	m.lastReqPtr = req
	// Capture a snapshot of the model for verification.
	m.lastReq = &provider.Request{Model: req.Model}
	return m.completeResp, m.completeErr
}

func (m *mockProvider) Stream(_ context.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.streamCalls++
	m.lastReqPtr = req
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

func validConfigWithFallback() RouterConfig {
	return RouterConfig{
		Default:  RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		Fallback: RouteTarget{Provider: "local", Model: "qwen2.5-coder-7b"},
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
	if mp.lastReqPtr == req {
		t.Fatal("expected router to pass a cloned request to provider")
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

func TestComplete_RetriableErrorReturnsDirectly(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)

	retriableErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 502,
		Message:    "bad gateway",
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
	if anthropicMock.getCompleteCalls() != 1 {
		t.Fatalf("expected 1 anthropic call, got %d", anthropicMock.getCompleteCalls())
	}
}

func TestComplete_AuthErrorWrapped(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)

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

	req := &provider.Request{}
	_, err := r.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !contains(err.Error(), "authentication failed") {
		t.Fatalf("expected auth error message, got: %s", err.Error())
	}
	if !contains(err.Error(), "ANTHROPIC_API_KEY") && !contains(err.Error(), "claude login") {
		t.Fatalf("expected remediation message, got: %s", err.Error())
	}
}

func TestComplete_AuthError403Wrapped(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)

	anthropicMock := &mockProvider{
		name: "anthropic",
		completeErr: &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: 403,
			Message:    "forbidden",
			Retriable:  false,
		},
	}
	_ = r.RegisterProvider(anthropicMock)

	req := &provider.Request{}
	_, err := r.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !contains(err.Error(), "authentication failed") {
		t.Fatalf("expected auth error message, got: %s", err.Error())
	}
}

func TestComplete_NonRetriableErrorReturnsDirectly(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)

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
	_ = r.RegisterProvider(anthropicMock)

	req := &provider.Request{}
	_, err := r.Complete(context.Background(), req)
	if err != origErr {
		t.Fatalf("expected original error to be returned, got: %v", err)
	}
}

func TestComplete_RateLimitErrorReturnsDirectly(t *testing.T) {
	cfg := validConfig()
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

func TestComplete_RetriableErrorFallsBackWhenConfigured(t *testing.T) {
	r, _ := NewRouter(validConfigWithFallback(), nil, nil)

	primaryErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 502,
		Message:    "bad gateway",
		Retriable:  true,
	}
	anthropicMock := &mockProvider{
		name:        "anthropic",
		completeErr: primaryErr,
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
		t.Fatal("expected fallback response from local")
	}
	if anthropicMock.getCompleteCalls() != 1 {
		t.Fatalf("expected 1 anthropic call, got %d", anthropicMock.getCompleteCalls())
	}
	if localMock.getCompleteCalls() != 1 {
		t.Fatalf("expected 1 local call, got %d", localMock.getCompleteCalls())
	}
	if localMock.lastReq.Model != "qwen2.5-coder-7b" {
		t.Fatalf("expected fallback model qwen2.5-coder-7b, got %q", localMock.lastReq.Model)
	}
	if req.Model != "" {
		t.Fatalf("expected caller request to remain unmodified, got %q", req.Model)
	}
}

func TestComplete_RetriableErrorWithMissingFallbackReturnsWrappedPrimaryError(t *testing.T) {
	r, _ := NewRouter(validConfigWithFallback(), nil, nil)

	primaryErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 503,
		Message:    "service unavailable",
		Retriable:  true,
	}
	anthropicMock := &mockProvider{
		name:        "anthropic",
		completeErr: primaryErr,
	}
	_ = r.RegisterProvider(anthropicMock)

	_, err := r.Complete(context.Background(), &provider.Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "fallback provider not available: local") {
		t.Fatalf("unexpected error: %v", err)
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
	if req.Model != "" {
		t.Fatalf("expected req.Model to remain empty, got %s", req.Model)
	}
	if mp.lastReqPtr == req {
		t.Fatal("expected router to pass a cloned request to provider")
	}
}

func TestStream_RetriableErrorReturnsDirectly(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)

	retriableErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 502,
		Message:    "bad gateway",
		Retriable:  true,
	}
	anthropicMock := &mockProvider{
		name:      "anthropic",
		streamErr: retriableErr,
	}
	_ = r.RegisterProvider(anthropicMock)

	req := &provider.Request{}
	_, err := r.Stream(context.Background(), req)
	if err != retriableErr {
		t.Fatalf("expected retriable error returned directly, got: %v", err)
	}
}

func TestStream_RetriableErrorFallsBackWhenConfigured(t *testing.T) {
	r, _ := NewRouter(validConfigWithFallback(), nil, nil)

	primaryErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 502,
		Message:    "bad gateway",
		Retriable:  true,
	}
	anthropicMock := &mockProvider{
		name:      "anthropic",
		streamErr: primaryErr,
	}

	localCh := make(chan provider.StreamEvent, 1)
	localCh <- provider.TokenDelta{Text: "fallback-response"}
	close(localCh)
	localMock := &mockProvider{
		name:     "local",
		streamCh: localCh,
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	req := &provider.Request{}
	got, err := r.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	event := <-got
	td, ok := event.(provider.TokenDelta)
	if !ok {
		t.Fatal("expected TokenDelta event")
	}
	if td.Text != "fallback-response" {
		t.Fatalf("expected fallback-response, got %q", td.Text)
	}
	if localMock.lastReq.Model != "qwen2.5-coder-7b" {
		t.Fatalf("expected fallback model qwen2.5-coder-7b, got %q", localMock.lastReq.Model)
	}
	if req.Model != "" {
		t.Fatalf("expected caller request to remain unmodified, got %q", req.Model)
	}
}

func TestStream_AuthErrorWrapped(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)

	anthropicMock := &mockProvider{
		name: "anthropic",
		streamErr: &provider.ProviderError{
			Provider:   "anthropic",
			StatusCode: 401,
			Message:    "invalid api key",
			Retriable:  false,
		},
	}
	_ = r.RegisterProvider(anthropicMock)

	req := &provider.Request{}
	_, err := r.Stream(context.Background(), req)
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !contains(err.Error(), "authentication failed") {
		t.Fatalf("expected auth error message, got: %s", err.Error())
	}
	if !contains(err.Error(), "ANTHROPIC_API_KEY") && !contains(err.Error(), "claude login") {
		t.Fatalf("expected remediation message, got: %s", err.Error())
	}
}

func TestStream_PerRequestOverride(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)

	anthropicCh := make(chan provider.StreamEvent, 1)
	anthropicCh <- provider.TokenDelta{Text: "anthropic-response"}
	close(anthropicCh)
	anthropicMock := &mockProvider{
		name:     "anthropic",
		streamCh: anthropicCh,
		models:   []provider.Model{{ID: "claude-sonnet-4-6"}},
	}

	localCh := make(chan provider.StreamEvent, 1)
	localCh <- provider.TokenDelta{Text: "local-response"}
	close(localCh)
	localMock := &mockProvider{
		name:     "local",
		streamCh: localCh,
		models:   []provider.Model{{ID: "qwen2.5-coder-7b"}},
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	req := &provider.Request{Model: "qwen2.5-coder-7b"}
	got, err := r.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil stream channel")
	}

	event := <-got
	td, ok := event.(provider.TokenDelta)
	if !ok {
		t.Fatal("expected TokenDelta event")
	}
	if td.Text != "local-response" {
		t.Fatalf("expected 'local-response', got %s", td.Text)
	}

	if localMock.getStreamCalls() != 1 {
		t.Fatalf("expected 1 local stream call, got %d", localMock.getStreamCalls())
	}
	if anthropicMock.getStreamCalls() != 0 {
		t.Fatalf("expected 0 anthropic stream calls, got %d", anthropicMock.getStreamCalls())
	}
}

// --- Health tracking tests ---

func TestProviderHealthMap_ReturnsDeepCopies(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)
	mp := &mockProvider{name: "anthropic"}
	_ = r.RegisterProvider(mp)

	testErr := fmt.Errorf("boom")
	r.markFailure("anthropic", testErr)

	health := r.ProviderHealthMap()
	copyHealth := health["anthropic"]
	if copyHealth == nil {
		t.Fatal("expected copied health entry")
	}
	copyHealth.Healthy = true
	copyHealth.LastError = nil
	copyHealth.LastErrorAt = time.Time{}
	copyHealth.LastSuccessAt = time.Time{}

	refreshed := r.ProviderHealthMap()
	if refreshed["anthropic"] == copyHealth {
		t.Fatal("expected ProviderHealthMap to return distinct provider health copies")
	}
	if refreshed["anthropic"].Healthy {
		t.Fatal("expected internal health to remain unhealthy")
	}
	if refreshed["anthropic"].LastError != testErr {
		t.Fatal("expected internal LastError to remain unchanged")
	}
	if refreshed["anthropic"].LastErrorAt.IsZero() {
		t.Fatal("expected internal LastErrorAt to remain set")
	}
}

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

func TestHealthTracking_RequestValidationFailureDoesNotPoisonHealth(t *testing.T) {
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
	if !h.Healthy {
		t.Fatal("expected provider to remain healthy after request validation failure")
	}
	if h.LastError != nil {
		t.Fatalf("expected LastError to remain empty, got %v", h.LastError)
	}
	if !h.LastErrorAt.IsZero() {
		t.Fatalf("expected LastErrorAt to remain zero, got %v", h.LastErrorAt)
	}
	if !h.LastSuccessAt.IsZero() && h.LastSuccessAt.Before(before) {
		t.Fatal("unexpected stale LastSuccessAt timestamp")
	}
}

func TestHealthTracking_AuthFailureUpdatesHealth(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)
	testErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 401,
		Message:    "invalid api key",
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
		t.Fatal("expected unhealthy after auth failure")
	}
	if h.LastError != testErr {
		t.Fatal("expected LastError to be the auth error")
	}
	if h.LastErrorAt.Before(before) {
		t.Fatal("expected LastErrorAt to be recent")
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

func TestValidate_DefaultProviderUnavailableHardStop(t *testing.T) {
	cfg := validConfig()
	r, _ := NewRouter(cfg, nil, nil)

	anthropicMock := &mockProvider{
		name:      "anthropic",
		modelsErr: fmt.Errorf("unauthorized"),
	}
	localMock := &mockProvider{
		name:   "local",
		models: []provider.Model{{ID: "qwen2.5-coder-7b"}},
	}

	_ = r.RegisterProvider(anthropicMock)
	_ = r.RegisterProvider(localMock)

	// With no fallback, default provider failure is a hard stop.
	err := r.Validate(context.Background())
	if err == nil {
		t.Fatal("expected hard error when default provider fails validation")
	}
	if !contains(err.Error(), "default provider") {
		t.Fatalf("expected error about default provider, got: %s", err.Error())
	}
}

// --- Pinger tests ---

// mockPingProvider is a mock that implements both provider.Provider and provider.Pinger.
type mockPingProvider struct {
	mockProvider
	pingErr    error
	pingCalls  int
	authStatus *provider.AuthStatus
	authErr    error
}

func (m *mockPingProvider) Ping(_ context.Context) error {
	m.pingCalls++
	return m.pingErr
}

func (m *mockPingProvider) AuthStatus(_ context.Context) (*provider.AuthStatus, error) {
	if m.authErr != nil {
		return nil, m.authErr
	}
	return m.authStatus, nil
}

func TestValidate_UsesPingWhenAvailable(t *testing.T) {
	cfg := validConfig()
	r, _ := NewRouter(cfg, nil, nil)

	// Register a provider that implements Pinger.
	pingMock := &mockPingProvider{
		mockProvider: mockProvider{
			name:   "anthropic",
			models: []provider.Model{{ID: "claude-sonnet-4-6"}},
		},
	}
	_ = r.RegisterProvider(pingMock)

	err := r.Validate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pingMock.pingCalls != 1 {
		t.Fatalf("expected 1 Ping() call, got %d", pingMock.pingCalls)
	}
}

func TestValidate_PingFailureUnregistersProvider(t *testing.T) {
	cfg := RouterConfig{
		Default: RouteTarget{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
	r, _ := NewRouter(cfg, nil, nil)

	// Anthropic with failing Ping — hard stop since it's the default.
	failingPing := &mockPingProvider{
		mockProvider: mockProvider{
			name:   "anthropic",
			models: []provider.Model{{ID: "claude-sonnet-4-6"}},
		},
		pingErr: fmt.Errorf("auth check failed"),
	}

	_ = r.RegisterProvider(failingPing)

	err := r.Validate(context.Background())
	if err == nil {
		t.Fatal("expected hard error when default provider fails Ping()")
	}
}

func TestValidate_DefaultProviderAuthFailureReturnsRemediation(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)
	failingPing := &mockPingProvider{
		mockProvider: mockProvider{
			name:   "anthropic",
			models: []provider.Model{{ID: "claude-sonnet-4-6"}},
		},
		pingErr: provider.NewAuthProviderError("anthropic", provider.AuthMissingCredentials, 401, "invalid api key", "Configure ANTHROPIC_API_KEY or run `claude login`.", nil),
	}
	_ = r.RegisterProvider(failingPing)

	err := r.Validate(context.Background())
	if err == nil {
		t.Fatal("expected hard error when default provider auth fails Ping()")
	}
	if !contains(err.Error(), "authentication failed for provider anthropic") {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	if !contains(err.Error(), "ANTHROPIC_API_KEY") && !contains(err.Error(), "claude login") {
		t.Fatalf("expected remediation guidance, got: %s", err.Error())
	}
}

func TestValidate_NonDefaultAuthFailureUnregistersProvider(t *testing.T) {
	cfg := validConfig()
	r, _ := NewRouter(cfg, nil, nil)

	defaultProvider := &mockProvider{name: "anthropic", models: []provider.Model{{ID: "claude-sonnet-4-6"}}}
	badSecondary := &mockPingProvider{
		mockProvider: mockProvider{name: "secondary", models: []provider.Model{{ID: "gpt-5.1-codex-mini"}}},
		pingErr:      provider.NewAuthProviderError("secondary", provider.AuthInvalidCredentials, 401, "secondary auth rejected", "Refresh the secondary provider credentials.", nil),
	}

	_ = r.RegisterProvider(defaultProvider)
	_ = r.RegisterProvider(badSecondary)

	if err := r.Validate(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	health := r.ProviderHealthMap()
	if _, ok := health["secondary"]; ok {
		t.Fatal("expected failing non-default provider to be unregistered")
	}
	if _, ok := health["anthropic"]; !ok {
		t.Fatal("expected default provider to remain registered")
	}
}

func TestAuthStatuses_ReturnsProviderStatusesAndErrors(t *testing.T) {
	r, _ := NewRouter(validConfig(), nil, nil)
	good := &mockPingProvider{
		mockProvider: mockProvider{name: "anthropic", models: []provider.Model{{ID: "claude-sonnet-4-6"}}},
		authStatus:   &provider.AuthStatus{Provider: "anthropic", Mode: "api_key", Source: "env:ANTHROPIC_API_KEY", HasAccessToken: true},
	}
	bad := &mockPingProvider{
		mockProvider: mockProvider{name: "codex", models: []provider.Model{{ID: "gpt-5.1-codex-mini"}}},
		authErr:      provider.NewAuthProviderError("codex", provider.AuthMissingCredentials, 0, "missing Codex auth", "Run `codex auth`.", nil),
	}
	_ = r.RegisterProvider(good)
	_ = r.RegisterProvider(bad)

	statuses, err := r.AuthStatuses(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if statuses["anthropic"] == nil || statuses["anthropic"].Mode != "api_key" {
		t.Fatalf("unexpected anthropic status: %+v", statuses["anthropic"])
	}
	if statuses["codex"] == nil || !contains(statuses["codex"].Detail, "missing Codex auth") {
		t.Fatalf("unexpected codex status: %+v", statuses["codex"])
	}
	if !contains(statuses["codex"].Remediation, "codex auth") {
		t.Fatalf("expected remediation in codex status, got %+v", statuses["codex"])
	}
}

func TestValidate_FallsBackToModelsWhenNoPinger(t *testing.T) {
	cfg := validConfig()
	r, _ := NewRouter(cfg, nil, nil)

	// Register a provider that does NOT implement Pinger (plain mockProvider).
	mp := &mockProvider{
		name:   "anthropic",
		models: []provider.Model{{ID: "claude-sonnet-4-6"}},
	}
	_ = r.RegisterProvider(mp)

	// Should still pass by using Models() check.
	err := r.Validate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
