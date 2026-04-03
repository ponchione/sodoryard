package tracking_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/ponchione/sirtopham/internal/provider"
	"github.com/ponchione/sirtopham/internal/provider/tracking"
)

// mockProvider implements provider.Provider with controllable return values.
type mockProvider struct {
	name         string
	completeResp *provider.Response
	completeErr  error
	streamCh     <-chan provider.StreamEvent
	streamErr    error
	modelsList   []provider.Model
	authStatus   *provider.AuthStatus
	authErr      error
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Models(_ context.Context) ([]provider.Model, error) {
	return m.modelsList, nil
}
func (m *mockProvider) Complete(_ context.Context, _ *provider.Request) (*provider.Response, error) {
	return m.completeResp, m.completeErr
}
func (m *mockProvider) Stream(_ context.Context, _ *provider.Request) (<-chan provider.StreamEvent, error) {
	return m.streamCh, m.streamErr
}
func (m *mockProvider) AuthStatus(_ context.Context) (*provider.AuthStatus, error) {
	if m.authErr != nil {
		return nil, m.authErr
	}
	return m.authStatus, nil
}

// mockStore implements tracking.SubCallStore with call recording.
type mockStore struct {
	calls []tracking.InsertSubCallParams
	err   error
}

func (m *mockStore) InsertSubCall(_ context.Context, params tracking.InsertSubCallParams) error {
	m.calls = append(m.calls, params)
	return m.err
}

func TestTrackedProvider_DelegatesAuthStatus(t *testing.T) {
	mp := &mockProvider{
		name:       "codex",
		authStatus: &provider.AuthStatus{Provider: "codex", Mode: "oauth", Source: "sirtopham_store", HasAccessToken: true},
	}
	store := &mockStore{}
	tp := tracking.NewTrackedProvider(mp, store, slog.Default())

	reporter, ok := any(tp).(provider.AuthStatusReporter)
	if !ok {
		t.Fatal("expected tracked provider to implement AuthStatusReporter")
	}
	status, err := reporter.AuthStatus(context.Background())
	if err != nil {
		t.Fatalf("AuthStatus() error: %v", err)
	}
	if status == nil || status.Provider != "codex" || status.Source != "sirtopham_store" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestComplete_SuccessfulCall(t *testing.T) {
	resp := &provider.Response{
		Model: "claude-sonnet-4-20250514",
		Usage: provider.Usage{
			InputTokens:         1500,
			OutputTokens:        300,
			CacheReadTokens:     200,
			CacheCreationTokens: 50,
		},
		StopReason: provider.StopReasonEndTurn,
	}
	mp := &mockProvider{name: "anthropic", completeResp: resp}
	store := &mockStore{}
	logger := slog.Default()

	tp := tracking.NewTrackedProvider(mp, store, logger)

	req := &provider.Request{
		Purpose:        "chat",
		ConversationID: "conv-abc",
		TurnNumber:     3,
		Iteration:      1,
		Model:          "claude-sonnet-4-20250514",
	}

	gotResp, gotErr := tp.Complete(context.Background(), req)

	// Response and error pass through unchanged.
	if gotResp != resp {
		t.Error("expected exact same response pointer")
	}
	if gotErr != nil {
		t.Errorf("expected nil error, got %v", gotErr)
	}

	// Exactly one sub-call recorded.
	if len(store.calls) != 1 {
		t.Fatalf("expected 1 store call, got %d", len(store.calls))
	}

	p := store.calls[0]
	if p.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", p.Provider, "anthropic")
	}
	if p.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want %q", p.Model, "claude-sonnet-4-20250514")
	}
	if p.Purpose != "chat" {
		t.Errorf("Purpose = %q, want %q", p.Purpose, "chat")
	}
	if p.TokensIn != 1500 {
		t.Errorf("TokensIn = %d, want %d", p.TokensIn, 1500)
	}
	if p.TokensOut != 300 {
		t.Errorf("TokensOut = %d, want %d", p.TokensOut, 300)
	}
	if p.CacheReadTokens != 200 {
		t.Errorf("CacheReadTokens = %d, want %d", p.CacheReadTokens, 200)
	}
	if p.CacheCreationTokens != 50 {
		t.Errorf("CacheCreationTokens = %d, want %d", p.CacheCreationTokens, 50)
	}
	if p.Success != 1 {
		t.Errorf("Success = %d, want %d", p.Success, 1)
	}
	if p.ErrorMessage != nil {
		t.Errorf("ErrorMessage = %v, want nil", p.ErrorMessage)
	}
	if p.LatencyMs < 0 {
		t.Errorf("LatencyMs = %d, want >= 0", p.LatencyMs)
	}
	if p.ConversationID == nil || *p.ConversationID != "conv-abc" {
		t.Errorf("ConversationID = %v, want pointer to %q", p.ConversationID, "conv-abc")
	}
	if p.TurnNumber == nil || *p.TurnNumber != 3 {
		t.Errorf("TurnNumber = %v, want pointer to %d", p.TurnNumber, 3)
	}
	if p.Iteration == nil || *p.Iteration != 1 {
		t.Errorf("Iteration = %v, want pointer to %d", p.Iteration, 1)
	}
	if p.MessageID != nil {
		t.Errorf("MessageID = %v, want nil", p.MessageID)
	}

	// CreatedAt should be a valid RFC3339 timestamp.
	if _, err := time.Parse(time.RFC3339, p.CreatedAt); err != nil {
		t.Errorf("CreatedAt = %q, not valid RFC3339: %v", p.CreatedAt, err)
	}
}

func TestComplete_FailedCall(t *testing.T) {
	callErr := errors.New("connection timeout")
	mp := &mockProvider{name: "anthropic", completeErr: callErr}
	store := &mockStore{}
	logger := slog.Default()

	tp := tracking.NewTrackedProvider(mp, store, logger)

	req := &provider.Request{
		Purpose:        "chat",
		Model:          "claude-sonnet-4-20250514",
		ConversationID: "",
	}

	gotResp, gotErr := tp.Complete(context.Background(), req)

	if gotResp != nil {
		t.Errorf("expected nil response, got %v", gotResp)
	}
	if gotErr != callErr {
		t.Errorf("expected exact same error, got %v", gotErr)
	}

	if len(store.calls) != 1 {
		t.Fatalf("expected 1 store call, got %d", len(store.calls))
	}

	p := store.calls[0]
	if p.Success != 0 {
		t.Errorf("Success = %d, want %d", p.Success, 0)
	}
	if p.ErrorMessage == nil || *p.ErrorMessage != "connection timeout" {
		t.Errorf("ErrorMessage = %v, want pointer to %q", p.ErrorMessage, "connection timeout")
	}
	if p.TokensIn != 0 || p.TokensOut != 0 || p.CacheReadTokens != 0 || p.CacheCreationTokens != 0 {
		t.Error("expected all token counts to be 0")
	}
	if p.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want %q (fallback to request model)", p.Model, "claude-sonnet-4-20250514")
	}
	if p.ConversationID != nil {
		t.Errorf("ConversationID = %v, want nil (empty string in request)", p.ConversationID)
	}
	if p.LatencyMs < 0 {
		t.Errorf("LatencyMs = %d, want >= 0", p.LatencyMs)
	}
}

func TestComplete_FailedCallWithPartialResponse(t *testing.T) {
	partialResp := &provider.Response{
		Usage: provider.Usage{
			InputTokens: 500,
			OutputTokens: 0,
		},
	}
	callErr := errors.New("partial failure")
	mp := &mockProvider{name: "anthropic", completeResp: partialResp, completeErr: callErr}
	store := &mockStore{}
	logger := slog.Default()

	tp := tracking.NewTrackedProvider(mp, store, logger)

	req := &provider.Request{Purpose: "chat", Model: "claude-sonnet-4-20250514"}

	tp.Complete(context.Background(), req)

	if len(store.calls) != 1 {
		t.Fatalf("expected 1 store call, got %d", len(store.calls))
	}

	p := store.calls[0]
	if p.Success != 0 {
		t.Errorf("Success = %d, want %d", p.Success, 0)
	}
	if p.TokensIn != 500 {
		t.Errorf("TokensIn = %d, want %d (from partial response)", p.TokensIn, 500)
	}
	if p.ErrorMessage == nil || *p.ErrorMessage != "partial failure" {
		t.Errorf("ErrorMessage = %v, want pointer to %q", p.ErrorMessage, "partial failure")
	}
}

func TestComplete_StoreFailureDoesNotBlockInference(t *testing.T) {
	resp := &provider.Response{
		Model: "claude-sonnet-4-20250514",
		Usage: provider.Usage{InputTokens: 100, OutputTokens: 50},
	}
	mp := &mockProvider{name: "anthropic", completeResp: resp}
	store := &mockStore{err: errors.New("database is locked")}
	logger := slog.Default()

	tp := tracking.NewTrackedProvider(mp, store, logger)

	req := &provider.Request{Purpose: "chat", Model: "claude-sonnet-4-20250514"}

	gotResp, gotErr := tp.Complete(context.Background(), req)

	// Response passes through unchanged despite store failure.
	if gotResp != resp {
		t.Error("expected exact same response pointer despite store failure")
	}
	if gotErr != nil {
		t.Errorf("expected nil error despite store failure, got %v", gotErr)
	}
}

func TestComplete_OptionalFieldsNilForZeroValues(t *testing.T) {
	resp := &provider.Response{Model: "m", Usage: provider.Usage{}}
	mp := &mockProvider{name: "p", completeResp: resp}
	store := &mockStore{}
	logger := slog.Default()

	tp := tracking.NewTrackedProvider(mp, store, logger)

	req := &provider.Request{
		ConversationID: "",
		TurnNumber:     0,
		Iteration:      0,
	}

	tp.Complete(context.Background(), req)

	if len(store.calls) != 1 {
		t.Fatalf("expected 1 store call, got %d", len(store.calls))
	}

	p := store.calls[0]
	if p.ConversationID != nil {
		t.Errorf("ConversationID = %v, want nil", p.ConversationID)
	}
	if p.TurnNumber != nil {
		t.Errorf("TurnNumber = %v, want nil", p.TurnNumber)
	}
	if p.Iteration != nil {
		t.Errorf("Iteration = %v, want nil", p.Iteration)
	}
}

func TestStream_SuccessfulStream(t *testing.T) {
	ch := make(chan provider.StreamEvent, 4)
	ch <- provider.TokenDelta{Text: "Hello"}
	ch <- provider.TokenDelta{Text: " world"}
	ch <- provider.StreamUsage{Usage: provider.Usage{
		InputTokens: 1000, OutputTokens: 100, CacheReadTokens: 50, CacheCreationTokens: 0,
	}}
	ch <- provider.StreamDone{StopReason: provider.StopReasonEndTurn, Usage: provider.Usage{
		InputTokens: 1000, OutputTokens: 150, CacheReadTokens: 50, CacheCreationTokens: 0,
	}}
	close(ch)

	mp := &mockProvider{name: "anthropic", streamCh: ch}
	store := &mockStore{}
	logger := slog.Default()

	tp := tracking.NewTrackedProvider(mp, store, logger)

	req := &provider.Request{
		Purpose:        "compression",
		ConversationID: "conv-xyz",
		TurnNumber:     2,
		Iteration:      0,
		Model:          "claude-sonnet-4-20250514",
	}

	out, err := tp.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []provider.StreamEvent
	for e := range out {
		events = append(events, e)
	}

	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}

	// Verify event order and types.
	if _, ok := events[0].(provider.TokenDelta); !ok {
		t.Error("event[0] should be TokenDelta")
	}
	if _, ok := events[1].(provider.TokenDelta); !ok {
		t.Error("event[1] should be TokenDelta")
	}
	if _, ok := events[2].(provider.StreamUsage); !ok {
		t.Error("event[2] should be StreamUsage")
	}
	if _, ok := events[3].(provider.StreamDone); !ok {
		t.Error("event[3] should be StreamDone")
	}

	if len(store.calls) != 1 {
		t.Fatalf("expected 1 store call, got %d", len(store.calls))
	}

	p := store.calls[0]
	if p.Purpose != "compression" {
		t.Errorf("Purpose = %q, want %q", p.Purpose, "compression")
	}
	if p.TokensIn != 1000 {
		t.Errorf("TokensIn = %d, want %d (from StreamDone)", p.TokensIn, 1000)
	}
	if p.TokensOut != 150 {
		t.Errorf("TokensOut = %d, want %d (from StreamDone)", p.TokensOut, 150)
	}
	if p.CacheReadTokens != 50 {
		t.Errorf("CacheReadTokens = %d, want %d", p.CacheReadTokens, 50)
	}
	if p.CacheCreationTokens != 0 {
		t.Errorf("CacheCreationTokens = %d, want %d", p.CacheCreationTokens, 0)
	}
	if p.Success != 1 {
		t.Errorf("Success = %d, want %d", p.Success, 1)
	}
	if p.ErrorMessage != nil {
		t.Errorf("ErrorMessage = %v, want nil", p.ErrorMessage)
	}
	if p.LatencyMs < 0 {
		t.Errorf("LatencyMs = %d, want >= 0", p.LatencyMs)
	}
	if p.Iteration != nil {
		t.Errorf("Iteration = %v, want nil (zero value in request)", p.Iteration)
	}
	if p.ConversationID == nil || *p.ConversationID != "conv-xyz" {
		t.Errorf("ConversationID = %v, want pointer to %q", p.ConversationID, "conv-xyz")
	}
}

func TestStream_FatalError(t *testing.T) {
	ch := make(chan provider.StreamEvent, 2)
	ch <- provider.TokenDelta{Text: "partial"}
	ch <- provider.StreamError{Err: errors.New("rate limit"), Fatal: true, Message: "rate limit exceeded"}
	close(ch)

	mp := &mockProvider{name: "openai", streamCh: ch}
	store := &mockStore{}
	logger := slog.Default()

	tp := tracking.NewTrackedProvider(mp, store, logger)

	req := &provider.Request{Purpose: "chat", Model: "gpt-4o"}

	out, err := tp.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []provider.StreamEvent
	for e := range out {
		events = append(events, e)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	if len(store.calls) != 1 {
		t.Fatalf("expected 1 store call, got %d", len(store.calls))
	}

	p := store.calls[0]
	if p.Success != 0 {
		t.Errorf("Success = %d, want %d", p.Success, 0)
	}
	if p.ErrorMessage == nil || *p.ErrorMessage != "rate limit" {
		t.Errorf("ErrorMessage = %v, want pointer containing %q", p.ErrorMessage, "rate limit")
	}
	if p.TokensIn != 0 || p.TokensOut != 0 {
		t.Error("expected token counts to be 0 (no usage event received)")
	}
}

func TestStream_InnerStreamFails(t *testing.T) {
	streamErr := errors.New("connection refused")
	mp := &mockProvider{name: "anthropic", streamErr: streamErr}
	store := &mockStore{}
	logger := slog.Default()

	tp := tracking.NewTrackedProvider(mp, store, logger)

	req := &provider.Request{Purpose: "chat", Model: "claude-sonnet-4-20250514"}

	out, err := tp.Stream(context.Background(), req)

	if out != nil {
		t.Error("expected nil channel when inner Stream fails")
	}
	if err != streamErr {
		t.Errorf("expected exact same error, got %v", err)
	}

	if len(store.calls) != 1 {
		t.Fatalf("expected 1 store call, got %d", len(store.calls))
	}

	p := store.calls[0]
	if p.Success != 0 {
		t.Errorf("Success = %d, want %d", p.Success, 0)
	}
	if p.ErrorMessage == nil || *p.ErrorMessage != "connection refused" {
		t.Errorf("ErrorMessage = %v, want pointer to %q", p.ErrorMessage, "connection refused")
	}
	if p.TokensIn != 0 || p.TokensOut != 0 || p.CacheReadTokens != 0 || p.CacheCreationTokens != 0 {
		t.Error("expected all token counts to be 0")
	}
}

func TestStream_StoreFailureDuringCompletion(t *testing.T) {
	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.StreamDone{StopReason: provider.StopReasonEndTurn, Usage: provider.Usage{
		InputTokens: 100, OutputTokens: 50,
	}}
	close(ch)

	mp := &mockProvider{name: "anthropic", streamCh: ch}
	store := &mockStore{err: errors.New("disk full")}
	logger := slog.Default()

	tp := tracking.NewTrackedProvider(mp, store, logger)

	req := &provider.Request{Purpose: "chat", Model: "claude-sonnet-4-20250514"}

	out, err := tp.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []provider.StreamEvent
	for e := range out {
		events = append(events, e)
	}

	// All events received unchanged, channel closes normally.
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if _, ok := events[0].(provider.StreamDone); !ok {
		t.Error("expected StreamDone event")
	}
	// No panic, no hang — tracking failure is invisible to consumer.
}

func TestStream_NoUsageEventsRecordsZerosWithFailure(t *testing.T) {
	ch := make(chan provider.StreamEvent)
	close(ch) // Immediately closed, no events.

	mp := &mockProvider{name: "anthropic", streamCh: ch}
	store := &mockStore{}
	logger := slog.Default()

	tp := tracking.NewTrackedProvider(mp, store, logger)

	req := &provider.Request{Purpose: "chat", Model: "claude-sonnet-4-20250514"}

	out, err := tp.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Drain the channel.
	var events []provider.StreamEvent
	for e := range out {
		events = append(events, e)
	}

	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}

	if len(store.calls) != 1 {
		t.Fatalf("expected 1 store call, got %d", len(store.calls))
	}

	p := store.calls[0]
	if p.TokensIn != 0 || p.TokensOut != 0 || p.CacheReadTokens != 0 || p.CacheCreationTokens != 0 {
		t.Error("expected all token counts to be 0")
	}
	if p.Success != 0 {
		t.Errorf("Success = %d, want %d (no usage events = failure)", p.Success, 0)
	}
}

func TestStream_EventsPassThroughUnmodified(t *testing.T) {
	ch := make(chan provider.StreamEvent, 8)
	ch <- provider.TokenDelta{Text: "a"}
	ch <- provider.ThinkingDelta{Thinking: "b"}
	ch <- provider.ToolCallStart{ID: "tc_1", Name: "read"}
	ch <- provider.ToolCallDelta{ID: "tc_1", Delta: "{}"}
	ch <- provider.ToolCallEnd{ID: "tc_1", Input: json.RawMessage("{}")}
	ch <- provider.StreamUsage{Usage: provider.Usage{InputTokens: 10, OutputTokens: 5}}
	ch <- provider.StreamError{Err: errors.New("warn"), Fatal: false, Message: "warning"}
	ch <- provider.StreamDone{StopReason: provider.StopReasonEndTurn, Usage: provider.Usage{InputTokens: 10, OutputTokens: 5}}
	close(ch)

	mp := &mockProvider{name: "anthropic", streamCh: ch}
	store := &mockStore{}
	logger := slog.Default()

	tp := tracking.NewTrackedProvider(mp, store, logger)

	req := &provider.Request{Purpose: "chat", Model: "m"}

	out, err := tp.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []provider.StreamEvent
	for e := range out {
		events = append(events, e)
	}

	if len(events) != 8 {
		t.Fatalf("expected 8 events, got %d", len(events))
	}

	// Verify each event type and content.
	if e, ok := events[0].(provider.TokenDelta); !ok || e.Text != "a" {
		t.Errorf("event[0] = %v, want TokenDelta{Text: %q}", events[0], "a")
	}
	if e, ok := events[1].(provider.ThinkingDelta); !ok || e.Thinking != "b" {
		t.Errorf("event[1] = %v, want ThinkingDelta{Thinking: %q}", events[1], "b")
	}
	if e, ok := events[2].(provider.ToolCallStart); !ok || e.ID != "tc_1" || e.Name != "read" {
		t.Errorf("event[2] = %v, want ToolCallStart{ID: %q, Name: %q}", events[2], "tc_1", "read")
	}
	if e, ok := events[3].(provider.ToolCallDelta); !ok || e.ID != "tc_1" || e.Delta != "{}" {
		t.Errorf("event[3] = %v, want ToolCallDelta{ID: %q, Delta: %q}", events[3], "tc_1", "{}")
	}
	if e, ok := events[4].(provider.ToolCallEnd); !ok || e.ID != "tc_1" {
		t.Errorf("event[4] = %v, want ToolCallEnd{ID: %q}", events[4], "tc_1")
	}
	if _, ok := events[5].(provider.StreamUsage); !ok {
		t.Errorf("event[5] = %v, want StreamUsage", events[5])
	}
	if e, ok := events[6].(provider.StreamError); !ok || e.Fatal || e.Message != "warning" {
		t.Errorf("event[6] = %v, want StreamError{Fatal: false, Message: %q}", events[6], "warning")
	}
	if _, ok := events[7].(provider.StreamDone); !ok {
		t.Errorf("event[7] = %v, want StreamDone", events[7])
	}
}

func TestNameAndModelsDelegateToInner(t *testing.T) {
	mp := &mockProvider{
		name:       "anthropic",
		modelsList: []provider.Model{{ID: "claude-sonnet-4-20250514"}},
	}
	store := &mockStore{}
	logger := slog.Default()

	tp := tracking.NewTrackedProvider(mp, store, logger)

	if got := tp.Name(); got != "anthropic" {
		t.Errorf("Name() = %q, want %q", got, "anthropic")
	}

	models, err := tp.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error from Models: %v", err)
	}
	if len(models) != 1 || models[0].ID != "claude-sonnet-4-20250514" {
		t.Errorf("Models() = %v, want [Model{ID: %q}]", models, "claude-sonnet-4-20250514")
	}

	// No tracking for Name or Models.
	if len(store.calls) != 0 {
		t.Errorf("expected 0 store calls, got %d", len(store.calls))
	}
}
