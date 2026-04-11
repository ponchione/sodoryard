package agent

import (
	stdctx "context"
	"errors"
	"strings"
	"testing"
	"time"

	contextpkg "github.com/ponchione/sodoryard/internal/context"
	"github.com/ponchione/sodoryard/internal/provider"
)

// retryProviderRouterStub allows per-call error/success configuration.
type retryProviderRouterStub struct {
	// responses is a list of (events, error) pairs, one per call.
	responses []retryResponse
	callIndex int
}

type retryResponse struct {
	events []provider.StreamEvent
	err    error
}

type streamingRetryProviderRouterStub struct{}

func (s *streamingRetryProviderRouterStub) Stream(ctx stdctx.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.TokenDelta{Text: "partial"}
	go func() {
		defer close(ch)
		<-ctx.Done()
	}()
	return ch, nil
}

func (s *retryProviderRouterStub) Stream(_ stdctx.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	idx := s.callIndex
	if idx >= len(s.responses) {
		idx = len(s.responses) - 1
	}
	s.callIndex++

	resp := s.responses[idx]
	if resp.err != nil {
		return nil, resp.err
	}

	ch := make(chan provider.StreamEvent, len(resp.events))
	for _, e := range resp.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func newRetryTestLoop(router ProviderRouter, sink EventSink) *AgentLoop {
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler: &loopContextAssemblerStub{
			pkg: &contextpkg.FullContextPackage{Content: "ctx", Frozen: true},
		},
		ConversationManager: &loopConversationManagerStub{
			seen: loopSeenFilesStub{},
		},
		ProviderRouter: router,
		ToolExecutor:   &toolExecutorStub{},
		PromptBuilder:  NewPromptBuilder(nil),
		EventSink:      sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000700, 0).UTC() }
	// Use a no-op sleep for fast tests.
	loop.sleepFn = func(ctx stdctx.Context, d time.Duration) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	return loop
}

func textOnlyStreamEvents(text string) []provider.StreamEvent {
	return []provider.StreamEvent{
		provider.TokenDelta{Text: text},
		provider.StreamDone{
			StopReason: provider.StopReasonEndTurn,
			Usage:      provider.Usage{InputTokens: 10, OutputTokens: 5},
		},
	}
}

// --- streamWithRetry tests ---

func TestStreamWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	router := &retryProviderRouterStub{
		responses: []retryResponse{
			{events: textOnlyStreamEvents("hello")},
		},
	}
	loop := newRetryTestLoop(router, nil)

	result, err := loop.streamWithRetry(
		stdctx.Background(),
		&provider.Request{},
		1,
		"conv-retry-1",
	)
	if err != nil {
		t.Fatalf("streamWithRetry error: %v", err)
	}
	if result.TextContent != "hello" {
		t.Fatalf("TextContent = %q, want %q", result.TextContent, "hello")
	}
	if router.callIndex != 1 {
		t.Fatalf("callIndex = %d, want 1 (single attempt)", router.callIndex)
	}
}

func TestStreamWithRetry_RetriableErrorRecoversOnSecondAttempt(t *testing.T) {
	rateLimitErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 429,
		Message:    "rate limited",
		Retriable:  true,
	}
	router := &retryProviderRouterStub{
		responses: []retryResponse{
			{err: rateLimitErr},
			{events: textOnlyStreamEvents("recovered")},
		},
	}
	sink := NewChannelSink(16)
	loop := newRetryTestLoop(router, sink)

	result, err := loop.streamWithRetry(
		stdctx.Background(),
		&provider.Request{},
		1,
		"conv-retry-2",
	)
	if err != nil {
		t.Fatalf("streamWithRetry error: %v", err)
	}
	if result.TextContent != "recovered" {
		t.Fatalf("TextContent = %q, want %q", result.TextContent, "recovered")
	}
	if router.callIndex != 2 {
		t.Fatalf("callIndex = %d, want 2 (retry succeeded)", router.callIndex)
	}

	// Should have emitted a recoverable ErrorEvent for the first attempt.
	event := drainUntilType(t, sink.Events(), "error")
	errEvt, ok := event.(ErrorEvent)
	if !ok {
		t.Fatalf("expected ErrorEvent, got %T", event)
	}
	if !errEvt.Recoverable {
		t.Fatal("first retry ErrorEvent.Recoverable = false, want true")
	}
	if errEvt.ErrorCode != ErrorCodeRateLimit {
		t.Fatalf("ErrorCode = %q, want %q", errEvt.ErrorCode, ErrorCodeRateLimit)
	}
}

func TestStreamWithRetry_AllRetriesExhausted(t *testing.T) {
	serverErr := &provider.ProviderError{
		Provider:   "openai",
		StatusCode: 500,
		Message:    "internal error",
		Retriable:  true,
	}
	router := &retryProviderRouterStub{
		responses: []retryResponse{
			{err: serverErr},
			{err: serverErr},
			{err: serverErr},
		},
	}
	sink := NewChannelSink(32)
	loop := newRetryTestLoop(router, sink)

	_, err := loop.streamWithRetry(
		stdctx.Background(),
		&provider.Request{},
		1,
		"conv-retry-3",
	)
	if err == nil {
		t.Fatal("streamWithRetry error = nil, want exhausted retries error")
	}
	if !strings.Contains(err.Error(), "all 3 attempts exhausted") {
		t.Fatalf("error = %q, want containing 'all 3 attempts exhausted'", err)
	}
	if router.callIndex != 3 {
		t.Fatalf("callIndex = %d, want 3 (all attempts used)", router.callIndex)
	}

	// Should have emitted recoverable events for attempts 1 and 2, then a
	// non-recoverable event when all retries exhausted.
	events := drainAllErrorEvents(t, sink.Events())
	if len(events) < 3 {
		t.Fatalf("got %d error events, want at least 3", len(events))
	}
	// Last one should be non-recoverable.
	last := events[len(events)-1]
	if last.Recoverable {
		t.Fatal("final ErrorEvent.Recoverable = true, want false")
	}
}

func TestStreamWithRetry_AuthFailureNoRetry(t *testing.T) {
	authErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 401,
		Message:    "invalid api key",
		Retriable:  false,
	}
	router := &retryProviderRouterStub{
		responses: []retryResponse{
			{err: authErr},
		},
	}
	sink := NewChannelSink(16)
	loop := newRetryTestLoop(router, sink)

	_, err := loop.streamWithRetry(
		stdctx.Background(),
		&provider.Request{},
		1,
		"conv-retry-4",
	)
	if err == nil {
		t.Fatal("streamWithRetry error = nil, want auth failure error")
	}
	if router.callIndex != 1 {
		t.Fatalf("callIndex = %d, want 1 (no retry for auth failure)", router.callIndex)
	}

	// Should emit a non-recoverable ErrorEvent.
	event := drainUntilType(t, sink.Events(), "error")
	errEvt := event.(ErrorEvent)
	if errEvt.Recoverable {
		t.Fatal("ErrorEvent.Recoverable = true, want false for auth failure")
	}
	if errEvt.ErrorCode != ErrorCodeAuthFailure {
		t.Fatalf("ErrorCode = %q, want %q", errEvt.ErrorCode, ErrorCodeAuthFailure)
	}
}

func TestStreamWithRetry_ContextOverflowNoRetry(t *testing.T) {
	overflowErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 400,
		Message:    "context_length_exceeded",
		Retriable:  false,
	}
	router := &retryProviderRouterStub{
		responses: []retryResponse{
			{err: overflowErr},
		},
	}
	loop := newRetryTestLoop(router, nil)

	_, err := loop.streamWithRetry(
		stdctx.Background(),
		&provider.Request{},
		1,
		"conv-retry-5",
	)
	if err == nil {
		t.Fatal("streamWithRetry error = nil, want context overflow error")
	}
	if router.callIndex != 1 {
		t.Fatalf("callIndex = %d, want 1 (no retry for context overflow)", router.callIndex)
	}
}

func TestStreamWithRetry_CancellationDuringSleep(t *testing.T) {
	serverErr := &provider.ProviderError{
		Provider:   "openai",
		StatusCode: 502,
		Message:    "bad gateway",
		Retriable:  true,
	}
	router := &retryProviderRouterStub{
		responses: []retryResponse{
			{err: serverErr},
			{events: textOnlyStreamEvents("should not reach")},
		},
	}

	ctx, cancel := stdctx.WithCancel(stdctx.Background())
	loop := newRetryTestLoop(router, nil)

	// Override sleep to cancel the context during the sleep.
	loop.sleepFn = func(ctx stdctx.Context, d time.Duration) error {
		cancel()
		return ctx.Err()
	}

	_, err := loop.streamWithRetry(ctx, &provider.Request{}, 1, "conv-retry-6")
	if err == nil {
		t.Fatal("streamWithRetry error = nil, want cancellation error")
	}
	if !errors.Is(err, stdctx.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestStreamWithRetry_CancellationReturnsPartialResult(t *testing.T) {
	ctx, cancel := stdctx.WithCancel(stdctx.Background())
	sink := NewChannelSink(16)
	loop := newRetryTestLoop(&streamingRetryProviderRouterStub{}, sink)

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	result, err := loop.streamWithRetry(ctx, &provider.Request{}, 1, "conv-retry-partial")
	if err == nil {
		t.Fatal("streamWithRetry error = nil, want cancellation error")
	}
	if !errors.Is(err, stdctx.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if result == nil {
		t.Fatal("result = nil, want partial result")
	}
	if result.TextContent != "partial" {
		t.Fatalf("TextContent = %q, want partial", result.TextContent)
	}

	assertNoRetryErrorEvents(t, sink.Events(), "cancellation")
}

func TestStreamWithRetry_DeadlineExceededReturnsPartialResult(t *testing.T) {
	ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 10*time.Millisecond)
	defer cancel()

	sink := NewChannelSink(16)
	loop := newRetryTestLoop(&streamingRetryProviderRouterStub{}, sink)

	result, err := loop.streamWithRetry(ctx, &provider.Request{}, 1, "conv-retry-deadline")
	if err == nil {
		t.Fatal("streamWithRetry error = nil, want deadline exceeded error")
	}
	if !errors.Is(err, stdctx.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if result == nil {
		t.Fatal("result = nil, want partial result")
	}
	if result.TextContent != "partial" {
		t.Fatalf("TextContent = %q, want partial", result.TextContent)
	}

	assertNoRetryErrorEvents(t, sink.Events(), "deadline exceeded")
}

func TestStreamWithRetry_RetriableRecoveryOnThirdAttempt(t *testing.T) {
	serverErr := &provider.ProviderError{
		Provider:   "openai",
		StatusCode: 503,
		Message:    "service unavailable",
		Retriable:  true,
	}
	router := &retryProviderRouterStub{
		responses: []retryResponse{
			{err: serverErr},
			{err: serverErr},
			{events: textOnlyStreamEvents("finally")},
		},
	}
	loop := newRetryTestLoop(router, nil)

	result, err := loop.streamWithRetry(
		stdctx.Background(),
		&provider.Request{},
		1,
		"conv-retry-7",
	)
	if err != nil {
		t.Fatalf("streamWithRetry error: %v", err)
	}
	if result.TextContent != "finally" {
		t.Fatalf("TextContent = %q, want %q", result.TextContent, "finally")
	}
	if router.callIndex != 3 {
		t.Fatalf("callIndex = %d, want 3", router.callIndex)
	}
}

func TestStreamWithRetry_NonProviderErrorNoRetry(t *testing.T) {
	router := &retryProviderRouterStub{
		responses: []retryResponse{
			{err: errors.New("network timeout")},
		},
	}
	loop := newRetryTestLoop(router, nil)

	_, err := loop.streamWithRetry(
		stdctx.Background(),
		&provider.Request{},
		1,
		"conv-retry-8",
	)
	if err == nil {
		t.Fatal("streamWithRetry error = nil, want error")
	}
	// Non-ProviderError => not retriable => no retry.
	if router.callIndex != 1 {
		t.Fatalf("callIndex = %d, want 1 (no retry for non-provider error)", router.callIndex)
	}
}

func TestStreamWithRetry_RetryAfterRespected(t *testing.T) {
	// RetryAfter of 10s should override the default 1s backoff delay.
	rateLimitErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 429,
		Message:    "rate limited",
		Retriable:  true,
		RetryAfter: 10 * time.Second,
	}
	router := &retryProviderRouterStub{
		responses: []retryResponse{
			{err: rateLimitErr},
			{events: textOnlyStreamEvents("recovered")},
		},
	}
	loop := newRetryTestLoop(router, nil)

	// Track the sleep duration that was used.
	var sleepDuration time.Duration
	loop.sleepFn = func(ctx stdctx.Context, d time.Duration) error {
		sleepDuration = d
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	result, err := loop.streamWithRetry(
		stdctx.Background(),
		&provider.Request{},
		1,
		"conv-retry-retryafter",
	)
	if err != nil {
		t.Fatalf("streamWithRetry error: %v", err)
	}
	if result.TextContent != "recovered" {
		t.Fatalf("TextContent = %q, want %q", result.TextContent, "recovered")
	}
	// The sleep duration should be max(1s backoff, 10s RetryAfter) = 10s.
	if sleepDuration != 10*time.Second {
		t.Fatalf("sleep duration = %v, want 10s (RetryAfter should override backoff)", sleepDuration)
	}
}

func TestStreamWithRetry_RetryAfterSmallerThanBackoff(t *testing.T) {
	// RetryAfter of 500ms is smaller than the 1s default backoff, so backoff wins.
	rateLimitErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 429,
		Message:    "rate limited",
		Retriable:  true,
		RetryAfter: 500 * time.Millisecond,
	}
	router := &retryProviderRouterStub{
		responses: []retryResponse{
			{err: rateLimitErr},
			{events: textOnlyStreamEvents("recovered")},
		},
	}
	loop := newRetryTestLoop(router, nil)

	var sleepDuration time.Duration
	loop.sleepFn = func(ctx stdctx.Context, d time.Duration) error {
		sleepDuration = d
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	result, err := loop.streamWithRetry(
		stdctx.Background(),
		&provider.Request{},
		1,
		"conv-retry-retryafter-small",
	)
	if err != nil {
		t.Fatalf("streamWithRetry error: %v", err)
	}
	if result.TextContent != "recovered" {
		t.Fatalf("TextContent = %q, want %q", result.TextContent, "recovered")
	}
	// The sleep duration should be max(1s backoff, 500ms RetryAfter) = 1s.
	if sleepDuration != 1*time.Second {
		t.Fatalf("sleep duration = %v, want 1s (backoff should win over smaller RetryAfter)", sleepDuration)
	}
}

// --- helpers ---

func assertNoRetryErrorEvents(t *testing.T, ch <-chan Event, context string) {
	t.Helper()
	for {
		select {
		case event := <-ch:
			if errEvt, ok := event.(ErrorEvent); ok {
				t.Fatalf("unexpected ErrorEvent on %s: %+v", context, errEvt)
			}
		default:
			return
		}
	}
}

// drainUntilType reads events from the channel until it finds one with the
// given event type, or fails the test.
func drainUntilType(t *testing.T, ch <-chan Event, eventType string) Event {
	t.Helper()
	for i := 0; i < 50; i++ {
		select {
		case e := <-ch:
			if e.EventType() == eventType {
				return e
			}
		default:
			t.Fatalf("no %q event found in channel", eventType)
		}
	}
	t.Fatalf("no %q event found after 50 drains", eventType)
	return nil
}

// drainAllErrorEvents reads all currently available events from the channel
// and collects ErrorEvents.
func drainAllErrorEvents(t *testing.T, ch <-chan Event) []ErrorEvent {
	t.Helper()
	var result []ErrorEvent
	for {
		select {
		case e := <-ch:
			if errEvt, ok := e.(ErrorEvent); ok {
				result = append(result, errEvt)
			}
		default:
			return result
		}
	}
}
