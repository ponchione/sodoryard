package agent

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/config"
	contextpkg "github.com/ponchione/sodoryard/internal/context"
	"github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/provider"
)

// --- Compression stubs ---

type compressionEngineStub struct {
	mu            sync.Mutex
	calls         []string // conversationIDs passed to Compress
	result        *contextpkg.CompressionResult
	err           error
	callCount     int
	resultsByCall []*contextpkg.CompressionResult // per-call results if set
	errsByCall    []error                         // per-call errors if set
}

func (s *compressionEngineStub) Compress(_ stdctx.Context, conversationID string, _ config.ContextConfig) (*contextpkg.CompressionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, conversationID)
	idx := s.callCount
	s.callCount++

	if idx < len(s.errsByCall) && s.errsByCall[idx] != nil {
		return nil, s.errsByCall[idx]
	}
	if idx < len(s.resultsByCall) {
		return s.resultsByCall[idx], nil
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

type titleGeneratorStub struct {
	mu    sync.Mutex
	calls []string // conversationIDs
}

func (s *titleGeneratorStub) GenerateTitle(_ stdctx.Context, conversationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, conversationID)
}

type toolResultStoreStub struct {
	mu        sync.Mutex
	refs      map[string]string
	bodies    map[string]string
	callOrder []string
	err       error
}

func (s *toolResultStoreStub) PersistToolResult(_ stdctx.Context, toolUseID, toolName, content string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return "", s.err
	}
	if s.refs == nil {
		s.refs = map[string]string{}
	}
	if s.bodies == nil {
		s.bodies = map[string]string{}
	}
	s.callOrder = append(s.callOrder, toolUseID)
	ref := s.refs[toolUseID]
	if ref == "" {
		ref = fmt.Sprintf("/tmp/persisted/%s-%s.txt", toolName, toolUseID)
		s.refs[toolUseID] = ref
	}
	s.bodies[toolUseID] = content
	return ref, nil
}

type updateQualityCapture struct {
	mu    sync.Mutex
	calls []updateQualityCall
	err   error
}

type updateQualityCall struct {
	conversationID string
	turnNumber     int
	usedSearchTool bool
	readFiles      []string
}

type capturingContextAssemblerStub struct {
	pkg               *contextpkg.FullContextPackage
	compressionNeeded bool
	err               error
	qualityCapture    *updateQualityCapture
}

func (s *capturingContextAssemblerStub) Assemble(
	_ stdctx.Context,
	_ string,
	_ []db.Message,
	_ contextpkg.AssemblyScope,
	_ int,
	_ int,
) (*contextpkg.FullContextPackage, bool, error) {
	return s.pkg, s.compressionNeeded, s.err
}

func (s *capturingContextAssemblerStub) UpdateQuality(_ stdctx.Context, conversationID string, turnNumber int, usedSearchTool bool, readFiles []string) error {
	if s.qualityCapture != nil {
		s.qualityCapture.mu.Lock()
		defer s.qualityCapture.mu.Unlock()
		s.qualityCapture.calls = append(s.qualityCapture.calls, updateQualityCall{
			conversationID: conversationID,
			turnNumber:     turnNumber,
			usedSearchTool: usedSearchTool,
			readFiles:      append([]string(nil), readFiles...),
		})
		return s.qualityCapture.err
	}
	return nil
}

// --- Helper ---

func textOnlyStream(text string, inputTokens, outputTokens int) []provider.StreamEvent {
	return []provider.StreamEvent{
		provider.TokenDelta{Text: text},
		provider.StreamDone{
			StopReason: provider.StopReasonEndTurn,
			Usage:      provider.Usage{InputTokens: inputTokens, OutputTokens: outputTokens},
		},
	}
}

func toolUseStream(toolID, toolName, args, text string, inputTokens, outputTokens int) []provider.StreamEvent {
	return []provider.StreamEvent{
		provider.TokenDelta{Text: text},
		provider.ToolCallStart{ID: toolID, Name: toolName},
		provider.ToolCallEnd{ID: toolID, Input: json.RawMessage(args)},
		provider.StreamDone{
			StopReason: provider.StopReasonToolUse,
			Usage:      provider.Usage{InputTokens: inputTokens, OutputTokens: outputTokens},
		},
	}
}

func drainEvents(sink *ChannelSink, max int) []Event {
	var events []Event
	for i := 0; i < max; i++ {
		select {
		case e := <-sink.Events():
			events = append(events, e)
		default:
			return events
		}
	}
	return events
}

func findEvent[T Event](events []Event) (T, bool) {
	var zero T
	for _, e := range events {
		if t, ok := e.(T); ok {
			return t, true
		}
	}
	return zero, false
}

func countEventType[T Event](events []Event) int {
	count := 0
	for _, e := range events {
		if _, ok := e.(T); ok {
			count++
		}
	}
	return count
}

func mustJSONStringContent(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal JSON string content: %v", err)
	}
	return s
}

// --- Tests ---

func TestRunTurnAggregateToolResultBudgetShrinksLargestFreshResult(t *testing.T) {
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	router := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			{
				provider.ToolCallStart{ID: "tc-1", Name: "search_text"},
				provider.ToolCallEnd{ID: "tc-1", Input: json.RawMessage(`{"query":"auth"}`)},
				provider.ToolCallStart{ID: "tc-2", Name: "file_read"},
				provider.ToolCallEnd{ID: "tc-2", Input: json.RawMessage(`{"path":"main.go"}`)},
				provider.StreamDone{StopReason: provider.StopReasonToolUse, Usage: provider.Usage{InputTokens: 10, OutputTokens: 5}},
			},
			textOnlyStream("done", 12, 4),
		},
	}

	largeSearch := strings.Repeat("S", 220)
	mediumRead := strings.Repeat("F", 180)
	toolExec := &toolExecutorStub{
		results: map[string]*provider.ToolResult{
			"tc-1": {ToolUseID: "tc-1", Content: largeSearch},
			"tc-2": {ToolUseID: "tc-2", Content: mediumRead},
		},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      router,
		ToolExecutor:        toolExec,
		PromptBuilder:       NewPromptBuilder(nil),
		Config: AgentLoopConfig{
			MaxToolResultsPerMessageChars: 300,
		},
	})

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-aggregate-budget",
		TurnNumber:        1,
		Message:           "inspect auth",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	if len(router.requests) != 2 {
		t.Fatalf("router request count = %d, want 2", len(router.requests))
	}

	secondReq := router.requests[1]
	var toolContents []string
	for _, msg := range secondReq.Messages {
		if msg.Role == provider.RoleTool {
			toolContents = append(toolContents, mustJSONStringContent(t, msg.Content))
		}
	}
	if len(toolContents) != 2 {
		t.Fatalf("tool message count in second request = %d, want 2", len(toolContents))
	}

	if got := len(toolContents[0]) + len(toolContents[1]); got > 300 {
		t.Fatalf("aggregate fresh tool-result chars in second request = %d, want <= 300; contents=%q / %q", got, toolContents[0], toolContents[1])
	}
	if len(toolContents[0]) >= len(largeSearch) {
		t.Fatalf("search_text result was not shrunk: len=%d original=%d", len(toolContents[0]), len(largeSearch))
	}
	if len(toolContents[1]) != len(mediumRead) {
		t.Fatalf("file_read result should be preserved when non-file_read shrinking is sufficient: len=%d want=%d", len(toolContents[1]), len(mediumRead))
	}
}

func TestRunTurnAggregateToolResultBudgetPersistsOversizedNonFileReadResult(t *testing.T) {
	conversations := &loopConversationManagerStub{history: []db.Message{}, seen: loopSeenFilesStub{}}
	assembler := &loopContextAssemblerStub{pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true}}
	router := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			{
				provider.ToolCallStart{ID: "tc-1", Name: "search_text"},
				provider.ToolCallEnd{ID: "tc-1", Input: json.RawMessage(`{"query":"auth"}`)},
				provider.StreamDone{StopReason: provider.StopReasonToolUse, Usage: provider.Usage{InputTokens: 10, OutputTokens: 5}},
			},
			textOnlyStream("done", 10, 3),
		},
	}
	fullOutput := strings.Repeat("SEARCH-RESULT-LINE\n", 40)
	toolExec := &toolExecutorStub{results: map[string]*provider.ToolResult{
		"tc-1": {ToolUseID: "tc-1", Content: fullOutput},
	}}
	store := &toolResultStoreStub{}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      router,
		ToolExecutor:        toolExec,
		PromptBuilder:       NewPromptBuilder(nil),
		ToolResultStore:     store,
		Config:              AgentLoopConfig{MaxToolResultsPerMessageChars: 120},
	})

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-persisted-tool-result",
		TurnNumber:        1,
		Message:           "inspect auth",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}
	if got := store.bodies["tc-1"]; got != fullOutput {
		t.Fatalf("persisted body = %q, want original full output", got)
	}
	secondReq := router.requests[1]
	var toolContent string
	for _, msg := range secondReq.Messages {
		if msg.Role == provider.RoleTool {
			toolContent = mustJSONStringContent(t, msg.Content)
			break
		}
	}
	if toolContent == "" {
		t.Fatal("no tool content found in second request")
	}
	for _, want := range []string{
		"[persisted_tool_result]",
		"path=/tmp/persisted/search_text-tc-1.txt",
		"tool=search_text",
		"tool_use_id=tc-1",
		"preview=",
	} {
		if !strings.Contains(toolContent, want) {
			t.Fatalf("tool content = %q, want %q", toolContent, want)
		}
	}
	if len(toolContent) >= len(fullOutput) {
		t.Fatalf("tool content len = %d, want less than original len %d", len(toolContent), len(fullOutput))
	}
}

func TestRunTurnEmitsStateIdleOnCompletion(t *testing.T) {
	sink := NewChannelSink(64)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				textOnlyStream("done", 10, 5),
			},
		},
		ToolExecutor:  &toolExecutorStub{},
		PromptBuilder: NewPromptBuilder(nil),
		EventSink:     sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-idle",
		TurnNumber:        1,
		Message:           "test",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	events := drainEvents(sink, 64)

	// Last event should be StatusEvent(StateIdle).
	var lastStatus StatusEvent
	found := false
	for i := len(events) - 1; i >= 0; i-- {
		if se, ok := events[i].(StatusEvent); ok {
			lastStatus = se
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no StatusEvent found")
	}
	if lastStatus.State != StateIdle {
		t.Fatalf("last StatusEvent.State = %q, want %q", lastStatus.State, StateIdle)
	}
}

func TestRunTurnPreflightCompressionTriggered(t *testing.T) {
	sink := NewChannelSink(64)
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}

	compression := &compressionEngineStub{
		result: &contextpkg.CompressionResult{
			Compressed:         true,
			CompressedMessages: 5,
		},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				textOnlyStream("done", 50, 10),
			},
		},
		ToolExecutor:      &toolExecutorStub{},
		PromptBuilder:     NewPromptBuilder(nil),
		EventSink:         sink,
		CompressionEngine: compression,
		Config: AgentLoopConfig{
			// Set a very low compression threshold so preflight triggers.
			ContextConfig: config.ContextConfig{
				CompressionThreshold: 0.001, // 0.1% — any content triggers it
			},
		},
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-preflight",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 100, // Small limit so the threshold is exceeded
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	// Compression should have been called.
	compression.mu.Lock()
	calls := append([]string(nil), compression.calls...)
	compression.mu.Unlock()

	if len(calls) == 0 {
		t.Fatal("CompressionEngine.Compress was not called")
	}
	if calls[0] != "conv-preflight" {
		t.Fatalf("Compress called with %q, want %q", calls[0], "conv-preflight")
	}

	// Should have emitted StateCompressing.
	events := drainEvents(sink, 64)
	compressingCount := 0
	for _, e := range events {
		if se, ok := e.(StatusEvent); ok && se.State == StateCompressing {
			compressingCount++
		}
	}
	if compressingCount == 0 {
		t.Fatal("StatusEvent(StateCompressing) not emitted for preflight compression")
	}
}

func TestRunTurnPreflightCompressionNotTriggeredWithoutEngine(t *testing.T) {
	// Verify that without a compression engine, no compression happens even
	// with a tiny context limit.
	sink := NewChannelSink(64)
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				textOnlyStream("done", 10, 5),
			},
		},
		ToolExecutor:  &toolExecutorStub{},
		PromptBuilder: NewPromptBuilder(nil),
		EventSink:     sink,
		// No CompressionEngine set.
		Config: AgentLoopConfig{
			ContextConfig: config.ContextConfig{
				CompressionThreshold: 0.001,
			},
		},
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-no-engine",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 100,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	// No StateCompressing should appear.
	events := drainEvents(sink, 64)
	for _, e := range events {
		if se, ok := e.(StatusEvent); ok && se.State == StateCompressing {
			t.Fatal("StatusEvent(StateCompressing) emitted without CompressionEngine")
		}
	}
}

func TestRunTurnPostResponseCompressionTriggered(t *testing.T) {
	sink := NewChannelSink(64)
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}

	compression := &compressionEngineStub{
		result: &contextpkg.CompressionResult{
			Compressed:         true,
			CompressedMessages: 3,
		},
	}

	// Respond with high input tokens to trigger post-response compression.
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				// Return very high input token count.
				{
					provider.TokenDelta{Text: "done"},
					provider.StreamDone{
						StopReason: provider.StopReasonEndTurn,
						Usage:      provider.Usage{InputTokens: 90000, OutputTokens: 100},
					},
				},
			},
		},
		ToolExecutor:      &toolExecutorStub{},
		PromptBuilder:     NewPromptBuilder(nil),
		EventSink:         sink,
		CompressionEngine: compression,
		Config: AgentLoopConfig{
			ContextConfig: config.ContextConfig{
				CompressionThreshold: 0.5, // 50% of model context
			},
		},
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-postresponse",
		TurnNumber:        1,
		Message:           "test",
		ModelContextLimit: 100000, // 50% = 50000, response has 90000 > threshold
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	compression.mu.Lock()
	callCount := len(compression.calls)
	compression.mu.Unlock()

	// At least one call to Compress (post-response). May also have preflight if
	// the rough estimate was high enough.
	if callCount == 0 {
		t.Fatal("CompressionEngine.Compress was not called for post-response compression")
	}
}

func TestRunTurnEmergencyCompressionOnContextOverflow(t *testing.T) {
	sink := NewChannelSink(64)
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}

	compression := &compressionEngineStub{
		result: &contextpkg.CompressionResult{
			Compressed:         true,
			CompressedMessages: 8,
		},
	}

	// First call: context overflow error. Second call (after compression): success.
	callCount := 0
	overflowErr := &provider.ProviderError{
		StatusCode: 400,
		Message:    "context_length_exceeded",
		Provider:   "anthropic",
	}

	routerStub := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			// Retry after emergency compression returns success.
			textOnlyStream("recovered", 50, 10),
		},
	}

	// Override the router to return overflow on first call.
	originalRouter := routerStub
	wrappedRouter := &overflowRouterStub{
		inner:    originalRouter,
		failCall: 0,
		err:      overflowErr,
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      wrappedRouter,
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
		EventSink:           sink,
		CompressionEngine:   compression,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }
	loop.sleepFn = func(_ stdctx.Context, _ time.Duration) error { return nil }
	_ = callCount

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-emergency",
		TurnNumber:        1,
		Message:           "test",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	if result.FinalText != "recovered" {
		t.Fatalf("FinalText = %q, want %q", result.FinalText, "recovered")
	}

	compression.mu.Lock()
	compCalls := len(compression.calls)
	compression.mu.Unlock()

	if compCalls == 0 {
		t.Fatal("CompressionEngine.Compress was not called for emergency compression")
	}
}

// overflowRouterStub returns an error on a specific call index and delegates
// to an inner router for all other calls.
type overflowRouterStub struct {
	inner     ProviderRouter
	failCall  int
	err       error
	mu        sync.Mutex
	callIndex int
}

func (s *overflowRouterStub) Stream(ctx stdctx.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	s.mu.Lock()
	idx := s.callIndex
	s.callIndex++
	s.mu.Unlock()

	if idx == s.failCall {
		return nil, s.err
	}
	return s.inner.Stream(ctx, req)
}

func TestRunTurnUpdateQualityCalledOnCompletion(t *testing.T) {
	sink := NewChannelSink(64)
	qualityCapture := &updateQualityCapture{}
	assembler := &capturingContextAssemblerStub{
		pkg:            &contextpkg.FullContextPackage{Content: "context", Frozen: true},
		qualityCapture: qualityCapture,
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	// Two iterations: tool call (file_read + search_semantic) then text.
	routerStub := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			// Iteration 1: two tool calls.
			{
				provider.ToolCallStart{ID: "tc-1", Name: "file_read"},
				provider.ToolCallEnd{ID: "tc-1", Input: json.RawMessage(`{"path":"internal/auth/service.go"}`)},
				provider.ToolCallStart{ID: "tc-2", Name: "search_semantic"},
				provider.ToolCallEnd{ID: "tc-2", Input: json.RawMessage(`{"query":"auth middleware"}`)},
				provider.StreamDone{
					StopReason: provider.StopReasonToolUse,
					Usage:      provider.Usage{InputTokens: 100, OutputTokens: 20},
				},
			},
			// Iteration 2: text response.
			textOnlyStream("done", 120, 30),
		},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      routerStub,
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
		EventSink:           sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-quality",
		TurnNumber:        2,
		Message:           "analyze auth",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	qualityCapture.mu.Lock()
	calls := append([]updateQualityCall(nil), qualityCapture.calls...)
	qualityCapture.mu.Unlock()

	if len(calls) != 1 {
		t.Fatalf("UpdateQuality calls = %d, want 1", len(calls))
	}
	c := calls[0]
	if c.conversationID != "conv-quality" {
		t.Fatalf("conversationID = %q, want %q", c.conversationID, "conv-quality")
	}
	if c.turnNumber != 2 {
		t.Fatalf("turnNumber = %d, want 2", c.turnNumber)
	}
	if !c.usedSearchTool {
		t.Fatal("usedSearchTool = false, want true (search_semantic was called)")
	}
	if len(c.readFiles) != 1 || c.readFiles[0] != "internal/auth/service.go" {
		t.Fatalf("readFiles = %v, want [internal/auth/service.go]", c.readFiles)
	}
}

func TestRunTurnTitleGenerationOnFirstTurn(t *testing.T) {
	sink := NewChannelSink(64)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}
	titleGen := &titleGeneratorStub{}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				textOnlyStream("hello", 10, 5),
			},
		},
		ToolExecutor:   &toolExecutorStub{},
		PromptBuilder:  NewPromptBuilder(nil),
		EventSink:      sink,
		TitleGenerator: titleGen,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	// Turn 1: title generation should fire.
	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-title",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	// Give the goroutine a moment to run.
	time.Sleep(50 * time.Millisecond)

	titleGen.mu.Lock()
	calls := append([]string(nil), titleGen.calls...)
	titleGen.mu.Unlock()

	if len(calls) != 1 || calls[0] != "conv-title" {
		t.Fatalf("TitleGenerator calls = %v, want [conv-title]", calls)
	}
}

func TestRunTurnTitleGenerationNotFiredOnSubsequentTurns(t *testing.T) {
	sink := NewChannelSink(64)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}
	titleGen := &titleGeneratorStub{}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				textOnlyStream("hello", 10, 5),
			},
		},
		ToolExecutor:   &toolExecutorStub{},
		PromptBuilder:  NewPromptBuilder(nil),
		EventSink:      sink,
		TitleGenerator: titleGen,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	// Turn 3: no title generation.
	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-title-2",
		TurnNumber:        3,
		Message:           "hello again",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	titleGen.mu.Lock()
	calls := append([]string(nil), titleGen.calls...)
	titleGen.mu.Unlock()

	if len(calls) != 0 {
		t.Fatalf("TitleGenerator should not fire on turn 3, got calls: %v", calls)
	}
}

func TestRunTurnTitleGenerationNotFiredWithoutGenerator(t *testing.T) {
	sink := NewChannelSink(64)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				textOnlyStream("hello", 10, 5),
			},
		},
		ToolExecutor:  &toolExecutorStub{},
		PromptBuilder: NewPromptBuilder(nil),
		EventSink:     sink,
		// No TitleGenerator
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-no-title",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}
	// Just verify it doesn't panic — no title generator means no goroutine.
}

func TestRunTurnPreflightCompressionFailureContinuesGracefully(t *testing.T) {
	sink := NewChannelSink(64)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}
	compression := &compressionEngineStub{
		err: fmt.Errorf("compression database locked"),
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				textOnlyStream("still works", 10, 5),
			},
		},
		ToolExecutor:      &toolExecutorStub{},
		PromptBuilder:     NewPromptBuilder(nil),
		EventSink:         sink,
		CompressionEngine: compression,
		Config: AgentLoopConfig{
			ContextConfig: config.ContextConfig{
				CompressionThreshold: 0.001,
			},
		},
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-comp-fail",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 100,
	})
	if err != nil {
		t.Fatalf("RunTurn should succeed despite compression failure, got: %v", err)
	}
	if result.FinalText != "still works" {
		t.Fatalf("FinalText = %q, want %q", result.FinalText, "still works")
	}

	// Should have emitted an error event for the compression failure.
	events := drainEvents(sink, 64)
	foundCompError := false
	for _, e := range events {
		if ee, ok := e.(ErrorEvent); ok && ee.ErrorCode == "compression_failed" {
			foundCompError = true
		}
	}
	if !foundCompError {
		t.Fatal("no ErrorEvent with code 'compression_failed' emitted")
	}
}

func TestRunTurnUpdateQualityNotCalledWithoutToolCalls(t *testing.T) {
	sink := NewChannelSink(64)
	qualityCapture := &updateQualityCapture{}
	assembler := &capturingContextAssemblerStub{
		pkg:            &contextpkg.FullContextPackage{Content: "context", Frozen: true},
		qualityCapture: qualityCapture,
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter: &providerRouterStub{
			streamEvents: [][]provider.StreamEvent{
				textOnlyStream("just text", 10, 5),
			},
		},
		ToolExecutor:  &toolExecutorStub{},
		PromptBuilder: NewPromptBuilder(nil),
		EventSink:     sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-no-tools",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	qualityCapture.mu.Lock()
	calls := qualityCapture.calls
	qualityCapture.mu.Unlock()

	if len(calls) != 1 {
		t.Fatalf("UpdateQuality calls = %d, want 1", len(calls))
	}
	// Should report no search tool used and no files read.
	if calls[0].usedSearchTool {
		t.Fatal("usedSearchTool should be false for text-only turn")
	}
	if len(calls[0].readFiles) != 0 {
		t.Fatalf("readFiles should be empty, got %v", calls[0].readFiles)
	}
}
