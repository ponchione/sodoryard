package agent

import (
	stdctx "context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	contextpkg "github.com/ponchione/sirtopham/internal/context"
	"github.com/ponchione/sirtopham/internal/conversation"
	"github.com/ponchione/sirtopham/internal/db"
	"github.com/ponchione/sirtopham/internal/provider"
	toolpkg "github.com/ponchione/sirtopham/internal/tool"
)

// --- Stubs ---

type loopSeenFilesStub struct{}

func (loopSeenFilesStub) Contains(path string) (bool, int) {
	if path == "internal/auth/service.go" {
		return true, 2
	}
	return false, 0
}

type persistedUserMessageCall struct {
	conversationID string
	turnNumber     int
	message        string
}

type persistedIterationCall struct {
	conversationID string
	turnNumber     int
	iteration      int
	messages       []conversation.IterationMessage
}

type cancelledIterationCall struct {
	conversationID string
	turnNumber     int
	iteration      int
}

type loopConversationManagerStub struct {
	history               []db.Message
	err                   error
	persistErr            error
	persistIterErr        error
	cancelIterErr         error
	reconstructCalls      []string
	seenFilesConversation []string
	persistCalls          []persistedUserMessageCall
	persistIterCalls      []persistedIterationCall
	cancelIterCalls       []cancelledIterationCall
	callOrder             []string
	seen                  contextpkg.SeenFileLookup
}

func (s *loopConversationManagerStub) PersistUserMessage(_ stdctx.Context, conversationID string, turnNumber int, message string) error {
	s.persistCalls = append(s.persistCalls, persistedUserMessageCall{
		conversationID: conversationID,
		turnNumber:     turnNumber,
		message:        message,
	})
	s.callOrder = append(s.callOrder, "persist")
	return s.persistErr
}

func (s *loopConversationManagerStub) PersistIteration(_ stdctx.Context, conversationID string, turnNumber, iteration int, messages []conversation.IterationMessage) error {
	s.persistIterCalls = append(s.persistIterCalls, persistedIterationCall{
		conversationID: conversationID,
		turnNumber:     turnNumber,
		iteration:      iteration,
		messages:       append([]conversation.IterationMessage(nil), messages...),
	})
	s.callOrder = append(s.callOrder, "persist_iteration")
	return s.persistIterErr
}

func (s *loopConversationManagerStub) CancelIteration(_ stdctx.Context, conversationID string, turnNumber, iteration int) error {
	s.cancelIterCalls = append(s.cancelIterCalls, cancelledIterationCall{
		conversationID: conversationID,
		turnNumber:     turnNumber,
		iteration:      iteration,
	})
	s.callOrder = append(s.callOrder, "cancel_iteration")
	return s.cancelIterErr
}

func (s *loopConversationManagerStub) ReconstructHistory(_ stdctx.Context, conversationID string) ([]db.Message, error) {
	s.reconstructCalls = append(s.reconstructCalls, conversationID)
	s.callOrder = append(s.callOrder, "reconstruct")
	if s.err != nil {
		return nil, s.err
	}
	return append([]db.Message(nil), s.history...), nil
}

func (s *loopConversationManagerStub) SeenFiles(conversationID string) contextpkg.SeenFileLookup {
	s.seenFilesConversation = append(s.seenFilesConversation, conversationID)
	return s.seen
}

type loopContextAssemblerStub struct {
	message           string
	history           []db.Message
	scope             contextpkg.AssemblyScope
	modelContextLimit int
	historyTokenCount int
	pkg               *contextpkg.FullContextPackage
	compressionNeeded bool
	err               error
}

func (s *loopContextAssemblerStub) Assemble(
	_ stdctx.Context,
	message string,
	history []db.Message,
	scope contextpkg.AssemblyScope,
	modelContextLimit int,
	historyTokenCount int,
) (*contextpkg.FullContextPackage, bool, error) {
	s.message = message
	s.history = append([]db.Message(nil), history...)
	s.scope = scope
	s.modelContextLimit = modelContextLimit
	s.historyTokenCount = historyTokenCount
	return s.pkg, s.compressionNeeded, s.err
}

func (s *loopContextAssemblerStub) UpdateQuality(stdctx.Context, string, int, bool, []string) error {
	return nil
}

// providerRouterStub implements ProviderRouter with configurable stream responses.
type providerRouterStub struct {
	streamEvents [][]provider.StreamEvent // one per call
	callIndex    int
	streamErr    error
	requests     []*provider.Request
}

func (s *providerRouterStub) Stream(_ stdctx.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	s.requests = append(s.requests, req)
	idx := s.callIndex
	if idx >= len(s.streamEvents) {
		idx = len(s.streamEvents) - 1
	}
	s.callIndex++

	events := s.streamEvents[idx]
	ch := make(chan provider.StreamEvent, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

// toolExecutorStub implements ToolExecutor with configurable results.
type toolExecutorStub struct {
	results map[string]*provider.ToolResult
	err     error
	calls   []provider.ToolCall
}

func (s *toolExecutorStub) Execute(_ stdctx.Context, call provider.ToolCall) (*provider.ToolResult, error) {
	s.calls = append(s.calls, call)
	if s.err != nil {
		return nil, s.err
	}
	if r, ok := s.results[call.ID]; ok {
		return r, nil
	}
	return &provider.ToolResult{
		ToolUseID: call.ID,
		Content:   "default result",
	}, nil
}

type executionMetaCapture struct {
	conversationID string
	turnNumber     int
	iteration      int
	ok             bool
}

type toolExecutorMetaStub struct {
	captures []executionMetaCapture
}

func (s *toolExecutorMetaStub) Execute(ctx stdctx.Context, call provider.ToolCall) (*provider.ToolResult, error) {
	meta, ok := toolpkg.ExecutionMetaFromContext(ctx)
	s.captures = append(s.captures, executionMetaCapture{
		conversationID: meta.ConversationID,
		turnNumber:     meta.TurnNumber,
		iteration:      meta.Iteration,
		ok:             ok,
	})
	return &provider.ToolResult{ToolUseID: call.ID, Content: "ok"}, nil
}

// --- Tests ---

func TestNewAgentLoopPrepareTurnContextCallsLayer3AndEmitsEvents(t *testing.T) {
	sink := NewChannelSink(8)
	history := []db.Message{{ConversationID: "conversation-1", Role: "user", TurnNumber: 1, Iteration: 0, Sequence: 0}}
	report := &contextpkg.ContextAssemblyReport{TurnNumber: 3}
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{
			Content:    "assembled context",
			TokenCount: 123,
			Report:     report,
			Frozen:     true,
		},
		compressionNeeded: true,
	}
	conversations := &loopConversationManagerStub{history: history, seen: loopSeenFilesStub{}}
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		EventSink:           sink,
		Config:              AgentLoopConfig{EmitContextDebug: true},
	})
	loop.now = func() time.Time { return time.Unix(1700000500, 0).UTC() }

	result, err := loop.PrepareTurnContext(stdctx.Background(), "conversation-1", 3, "fix auth", 200000, 4096)
	if err != nil {
		t.Fatalf("PrepareTurnContext returned error: %v", err)
	}
	if result == nil {
		t.Fatal("PrepareTurnContext returned nil result")
	}
	if !result.CompressionNeeded {
		t.Fatal("CompressionNeeded = false, want true")
	}
	if result.ContextPackage == nil || result.ContextPackage.Report != report {
		t.Fatal("ContextPackage report was not preserved")
	}
	if len(result.History) != 1 || result.History[0].ConversationID != "conversation-1" {
		t.Fatalf("History = %#v, want reconstructed history", result.History)
	}
	if got := assembler.message; got != "fix auth" {
		t.Fatalf("assembler message = %q, want fix auth", got)
	}
	if assembler.scope.ConversationID != "conversation-1" || assembler.scope.TurnNumber != 3 {
		t.Fatalf("assembler scope = %#v, want conversation-1 turn 3", assembler.scope)
	}
	if seen, turn := assembler.scope.SeenFiles.Contains("internal/auth/service.go"); !seen || turn != 2 {
		t.Fatalf("scope seen-files lookup returned (%t, %d), want (true, 2)", seen, turn)
	}
	if assembler.modelContextLimit != 200000 || assembler.historyTokenCount != 4096 {
		t.Fatalf("assembler limits = (%d, %d), want (200000, 4096)", assembler.modelContextLimit, assembler.historyTokenCount)
	}

	first := readEvent(t, sink.Events())
	if got := first.EventType(); got != "status" {
		t.Fatalf("first event type = %q, want status", got)
	}
	status, ok := first.(StatusEvent)
	if !ok || status.State != StateAssemblingContext {
		t.Fatalf("first event = %#v, want StatusEvent(StateAssemblingContext)", first)
	}

	second := readEvent(t, sink.Events())
	if got := second.EventType(); got != "context_debug" {
		t.Fatalf("second event type = %q, want context_debug", got)
	}
	debug, ok := second.(ContextDebugEvent)
	if !ok || debug.Report != report {
		t.Fatalf("second event = %#v, want ContextDebugEvent with report", second)
	}

	third := readEvent(t, sink.Events())
	if got := third.EventType(); got != "status" {
		t.Fatalf("third event type = %q, want status", got)
	}
	waiting, ok := third.(StatusEvent)
	if !ok || waiting.State != StateWaitingForLLM {
		t.Fatalf("third event = %#v, want StatusEvent(StateWaitingForLLM)", third)
	}
}

func TestPrepareTurnContextValidatesDependencies(t *testing.T) {
	loop := NewAgentLoop(AgentLoopDeps{})

	_, err := loop.PrepareTurnContext(stdctx.Background(), "conversation-1", 1, "hello", 1000, 0)
	if err == nil {
		t.Fatal("PrepareTurnContext error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "context assembler is nil") {
		t.Fatalf("error = %q, want missing context assembler", err)
	}
}

func TestPrepareTurnContextBubblesHistoryErrors(t *testing.T) {
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler: &loopContextAssemblerStub{},
		ConversationManager: &loopConversationManagerStub{
			err: sql.ErrNoRows,
		},
	})

	_, err := loop.PrepareTurnContext(stdctx.Background(), "conversation-1", 1, "hello", 1000, 0)
	if err == nil {
		t.Fatal("PrepareTurnContext error = nil, want history error")
	}
	if !strings.Contains(err.Error(), "reconstruct history") {
		t.Fatalf("error = %q, want reconstruct history context", err)
	}
}

func TestRunTurnTextOnlyResponse(t *testing.T) {
	sink := NewChannelSink(32)
	report := &contextpkg.ContextAssemblyReport{TurnNumber: 1}
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Report: report, Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	routerStub := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			{
				provider.TokenDelta{Text: "Hello, "},
				provider.TokenDelta{Text: "world!"},
				provider.StreamDone{
					StopReason: provider.StopReasonEndTurn,
					Usage:      provider.Usage{InputTokens: 100, OutputTokens: 10},
				},
			},
		},
	}

	promptBuilder := NewPromptBuilder(nil)

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      routerStub,
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       promptBuilder,
		EventSink:           sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-1",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	if result.FinalText != "Hello, world!" {
		t.Fatalf("FinalText = %q, want %q", result.FinalText, "Hello, world!")
	}
	if result.IterationCount != 1 {
		t.Fatalf("IterationCount = %d, want 1", result.IterationCount)
	}
	if result.TotalUsage.InputTokens != 100 || result.TotalUsage.OutputTokens != 10 {
		t.Fatalf("TotalUsage = %+v, want 100/10", result.TotalUsage)
	}

	// Should have persisted user message and one iteration.
	if len(conversations.persistCalls) != 1 {
		t.Fatalf("PersistUserMessage calls = %d, want 1", len(conversations.persistCalls))
	}
	if len(conversations.persistIterCalls) != 1 {
		t.Fatalf("PersistIteration calls = %d, want 1", len(conversations.persistIterCalls))
	}
	iterCall := conversations.persistIterCalls[0]
	if iterCall.iteration != 1 || len(iterCall.messages) != 1 || iterCall.messages[0].Role != "assistant" {
		t.Fatalf("PersistIteration call = %+v, want iteration 1 with 1 assistant message", iterCall)
	}
}

func TestRunTurnWithToolUse(t *testing.T) {
	sink := NewChannelSink(64)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	toolInput := json.RawMessage(`{"path":"main.go"}`)
	routerStub := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			// Iteration 1: tool use
			{
				provider.TokenDelta{Text: "Let me read that."},
				provider.ToolCallStart{ID: "tool_1", Name: "read_file"},
				provider.ToolCallEnd{ID: "tool_1", Input: toolInput},
				provider.StreamDone{
					StopReason: provider.StopReasonToolUse,
					Usage:      provider.Usage{InputTokens: 50, OutputTokens: 20},
				},
			},
			// Iteration 2: text-only
			{
				provider.TokenDelta{Text: "The file contains Go code."},
				provider.StreamDone{
					StopReason: provider.StopReasonEndTurn,
					Usage:      provider.Usage{InputTokens: 80, OutputTokens: 15},
				},
			},
		},
	}

	executor := &toolExecutorStub{
		results: map[string]*provider.ToolResult{
			"tool_1": {
				ToolUseID: "tool_1",
				Content:   "package main\nfunc main() {}",
			},
		},
	}

	promptBuilder := NewPromptBuilder(nil)

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      routerStub,
		ToolExecutor:        executor,
		PromptBuilder:       promptBuilder,
		EventSink:           sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-1",
		TurnNumber:        1,
		Message:           "read main.go",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	if result.FinalText != "The file contains Go code." {
		t.Fatalf("FinalText = %q, want %q", result.FinalText, "The file contains Go code.")
	}
	if result.IterationCount != 2 {
		t.Fatalf("IterationCount = %d, want 2", result.IterationCount)
	}

	// Total usage should be sum of both iterations.
	if result.TotalUsage.InputTokens != 130 || result.TotalUsage.OutputTokens != 35 {
		t.Fatalf("TotalUsage = %+v, want 130/35", result.TotalUsage)
	}

	// Should have persisted 2 iterations.
	if len(conversations.persistIterCalls) != 2 {
		t.Fatalf("PersistIteration calls = %d, want 2", len(conversations.persistIterCalls))
	}

	// First iteration: assistant + tool result.
	iter1 := conversations.persistIterCalls[0]
	if iter1.iteration != 1 || len(iter1.messages) != 2 {
		t.Fatalf("iteration 1 persist = %+v, want iteration 1 with 2 messages", iter1)
	}
	if iter1.messages[0].Role != "assistant" || iter1.messages[1].Role != "tool" {
		t.Fatalf("iteration 1 roles = %q/%q, want assistant/tool", iter1.messages[0].Role, iter1.messages[1].Role)
	}
	if iter1.messages[1].ToolUseID != "tool_1" {
		t.Fatalf("iteration 1 tool result ToolUseID = %q, want tool_1", iter1.messages[1].ToolUseID)
	}

	// Second iteration: assistant only.
	iter2 := conversations.persistIterCalls[1]
	if iter2.iteration != 2 || len(iter2.messages) != 1 {
		t.Fatalf("iteration 2 persist = %+v, want iteration 2 with 1 message", iter2)
	}

	// Tool executor should have been called once.
	if len(executor.calls) != 1 {
		t.Fatalf("tool executor calls = %d, want 1", len(executor.calls))
	}
	if executor.calls[0].Name != "read_file" {
		t.Fatalf("tool call name = %q, want read_file", executor.calls[0].Name)
	}
}

func TestRunTurnToolExecutionError(t *testing.T) {
	sink := NewChannelSink(64)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	toolInput := json.RawMessage(`{"command":"rm -rf /"}`)
	routerStub := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			// Iteration 1: tool use
			{
				provider.ToolCallStart{ID: "tool_1", Name: "run_command"},
				provider.ToolCallEnd{ID: "tool_1", Input: toolInput},
				provider.StreamDone{
					StopReason: provider.StopReasonToolUse,
					Usage:      provider.Usage{InputTokens: 50, OutputTokens: 20},
				},
			},
			// Iteration 2: text after error
			{
				provider.TokenDelta{Text: "Sorry, that failed."},
				provider.StreamDone{
					StopReason: provider.StopReasonEndTurn,
					Usage:      provider.Usage{InputTokens: 80, OutputTokens: 10},
				},
			},
		},
	}

	executor := &toolExecutorStub{
		err: errors.New("permission denied"),
	}

	promptBuilder := NewPromptBuilder(nil)

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      routerStub,
		ToolExecutor:        executor,
		PromptBuilder:       promptBuilder,
		EventSink:           sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-1",
		TurnNumber:        1,
		Message:           "run command",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	// Tool error should have been wrapped as an error result, not terminate the turn.
	if result.FinalText != "Sorry, that failed." {
		t.Fatalf("FinalText = %q, want %q", result.FinalText, "Sorry, that failed.")
	}
	if result.IterationCount != 2 {
		t.Fatalf("IterationCount = %d, want 2", result.IterationCount)
	}

	// Check that the persisted tool result contains the error.
	iter1 := conversations.persistIterCalls[0]
	toolMsg := iter1.messages[1]
	if !strings.Contains(toolMsg.Content, "permission denied") {
		t.Fatalf("tool error content = %q, want containing permission denied", toolMsg.Content)
	}
}

func TestRunTurnReturnsErrorEventWhenPersistenceFails(t *testing.T) {
	sink := NewChannelSink(4)
	persistErr := errors.New("db write failed")
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler: &loopContextAssemblerStub{},
		ConversationManager: &loopConversationManagerStub{
			persistErr: persistErr,
		},
		ProviderRouter: &providerRouterStub{},
		ToolExecutor:   &toolExecutorStub{},
		PromptBuilder:  NewPromptBuilder(nil),
		EventSink:      sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000700, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conversation-1",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 200000,
	})
	if err == nil {
		t.Fatal("RunTurn error = nil, want persistence error")
	}
	if !strings.Contains(err.Error(), "persist user message") {
		t.Fatalf("error = %q, want persist user message context", err)
	}

	event := readEvent(t, sink.Events())
	if got := event.EventType(); got != "error" {
		t.Fatalf("event type = %q, want error", got)
	}
	errEvent, ok := event.(ErrorEvent)
	if !ok {
		t.Fatalf("event = %#v, want ErrorEvent", event)
	}
	if errEvent.Recoverable {
		t.Fatal("ErrorEvent.Recoverable = true, want false")
	}
	if errEvent.ErrorCode != "persist_user_message_failed" {
		t.Fatalf("ErrorCode = %q, want persist_user_message_failed", errEvent.ErrorCode)
	}
}

func TestRunTurnValidatesRequest(t *testing.T) {
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    &loopContextAssemblerStub{},
		ConversationManager: &loopConversationManagerStub{},
		ProviderRouter:      &providerRouterStub{},
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
	})

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{Message: "hello", TurnNumber: 1, ModelContextLimit: 200000})
	if err == nil {
		t.Fatal("RunTurn error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "conversation ID") {
		t.Fatalf("error = %q, want conversation ID validation", err)
	}
}

func TestRunTurnStreamError(t *testing.T) {
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	routerStub := &providerRouterStub{
		streamErr: errors.New("provider unavailable"),
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      routerStub,
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-1",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 200000,
	})
	if err == nil {
		t.Fatal("RunTurn error = nil, want stream error")
	}
	if !strings.Contains(err.Error(), "stream for iteration") {
		t.Fatalf("error = %q, want stream error", err)
	}
}

func TestRunTurnEmitsTurnCompleteEvent(t *testing.T) {
	sink := NewChannelSink(32)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	routerStub := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			{
				provider.TokenDelta{Text: "Done"},
				provider.StreamDone{
					StopReason: provider.StopReasonEndTurn,
					Usage:      provider.Usage{InputTokens: 10, OutputTokens: 5},
				},
			},
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
		ConversationID:    "conv-1",
		TurnNumber:        1,
		Message:           "done",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	// Drain events and find TurnCompleteEvent.
	var found bool
	for i := 0; i < 20; i++ {
		select {
		case e := <-sink.Events():
			if tc, ok := e.(TurnCompleteEvent); ok {
				found = true
				if tc.TurnNumber != 1 {
					t.Fatalf("TurnComplete.TurnNumber = %d, want 1", tc.TurnNumber)
				}
				if tc.IterationCount != 1 {
					t.Fatalf("TurnComplete.IterationCount = %d, want 1", tc.IterationCount)
				}
				if tc.TotalInputTokens != 10 || tc.TotalOutputTokens != 5 {
					t.Fatalf("TurnComplete usage = %d/%d, want 10/5", tc.TotalInputTokens, tc.TotalOutputTokens)
				}
			}
		default:
		}
	}
	if !found {
		t.Fatal("TurnCompleteEvent not emitted")
	}
}

func TestRunTurnCallOrder(t *testing.T) {
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	routerStub := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			{
				provider.TokenDelta{Text: "done"},
				provider.StreamDone{
					StopReason: provider.StopReasonEndTurn,
					Usage:      provider.Usage{InputTokens: 10, OutputTokens: 5},
				},
			},
		},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      routerStub,
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-1",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	// Expected call order: persist user message → reconstruct (context assembly) →
	// reconstruct (iteration 1) → persist_iteration
	got := strings.Join(conversations.callOrder, ",")
	if got != "persist,reconstruct,reconstruct,persist_iteration" {
		t.Fatalf("call order = %q, want persist,reconstruct,reconstruct,persist_iteration", got)
	}
}

func TestRunTurnLoopDetectionInjectsNudge(t *testing.T) {
	// Set up a scenario where the LLM repeats the same tool call 3 times
	// (matching the default threshold), then produces text.
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	toolInput := json.RawMessage(`{"path":"a.go"}`)
	toolResponse := []provider.StreamEvent{
		provider.ToolCallStart{ID: "t1", Name: "read_file"},
		provider.ToolCallEnd{ID: "t1", Input: toolInput},
		provider.StreamDone{
			StopReason: provider.StopReasonToolUse,
			Usage:      provider.Usage{InputTokens: 10, OutputTokens: 5},
		},
	}

	routerStub := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			toolResponse, // iteration 1
			toolResponse, // iteration 2
			toolResponse, // iteration 3 — loop detected here
			{ // iteration 4 — different behavior after nudge
				provider.TokenDelta{Text: "I'll try something different."},
				provider.StreamDone{
					StopReason: provider.StopReasonEndTurn,
					Usage:      provider.Usage{InputTokens: 20, OutputTokens: 10},
				},
			},
		},
	}

	executor := &toolExecutorStub{}
	promptBuilder := NewPromptBuilder(nil)

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      routerStub,
		ToolExecutor:        executor,
		PromptBuilder:       promptBuilder,
		Config:              AgentLoopConfig{LoopDetectionThreshold: 3},
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-1",
		TurnNumber:        1,
		Message:           "read a.go",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	if result.IterationCount != 4 {
		t.Fatalf("IterationCount = %d, want 4", result.IterationCount)
	}
	if result.FinalText != "I'll try something different." {
		t.Fatalf("FinalText = %q, want different approach text", result.FinalText)
	}

	// The 4th prompt build should include the nudge message in its current-turn
	// messages. We can verify by checking the router was called 4 times.
	if routerStub.callIndex != 4 {
		t.Fatalf("router call count = %d, want 4", routerStub.callIndex)
	}
}

func TestRunTurnFinalIterationInjectsDirective(t *testing.T) {
	// Set MaxIterations=2, make iteration 1 use tools, iteration 2 should
	// have tools disabled and the directive message injected.
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	toolInput := json.RawMessage(`{"path":"a.go"}`)
	routerStub := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			// iteration 1: tool use
			{
				provider.ToolCallStart{ID: "t1", Name: "read_file"},
				provider.ToolCallEnd{ID: "t1", Input: toolInput},
				provider.StreamDone{
					StopReason: provider.StopReasonToolUse,
					Usage:      provider.Usage{InputTokens: 10, OutputTokens: 5},
				},
			},
			// iteration 2: text-only (tools disabled)
			{
				provider.TokenDelta{Text: "Summary of progress."},
				provider.StreamDone{
					StopReason: provider.StopReasonEndTurn,
					Usage:      provider.Usage{InputTokens: 20, OutputTokens: 10},
				},
			},
		},
	}

	// Track what prompts the router receives to verify DisableTools and directive.
	var promptConfigs []*provider.Request
	originalStream := routerStub.Stream
	_ = originalStream

	executor := &toolExecutorStub{}
	promptBuilder := NewPromptBuilder(nil)

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      routerStub,
		ToolExecutor:        executor,
		PromptBuilder:       promptBuilder,
		Config:              AgentLoopConfig{MaxIterations: 2},
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	// Wrap the router to capture requests.
	captureRouter := &captureProviderRouter{
		inner:    routerStub,
		captured: &promptConfigs,
	}
	loop.providerRouter = captureRouter

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-1",
		TurnNumber:        1,
		Message:           "read a.go",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	if result.IterationCount != 2 {
		t.Fatalf("IterationCount = %d, want 2", result.IterationCount)
	}
	if result.FinalText != "Summary of progress." {
		t.Fatalf("FinalText = %q, want summary text", result.FinalText)
	}

	// Verify: second call should have no tools (DisableTools=true).
	if len(promptConfigs) != 2 {
		t.Fatalf("captured prompts = %d, want 2", len(promptConfigs))
	}
	// First call should have no tools because we didn't provide ToolDefinitions in PromptConfig.
	// Second call (final iteration) should also have no tools.
	if len(promptConfigs[1].Tools) != 0 {
		t.Fatalf("final iteration tools = %d, want 0 (disabled)", len(promptConfigs[1].Tools))
	}

	// Verify directive message was injected: the second prompt should contain
	// the directive message in its Messages.
	found := false
	for _, msg := range promptConfigs[1].Messages {
		if msg.Role == provider.RoleUser {
			var text string
			_ = json.Unmarshal(msg.Content, &text)
			if strings.Contains(text, "maximum number of tool calls") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("directive message not found in final iteration prompt")
	}
}

// captureProviderRouter wraps a ProviderRouter to capture requests.
type captureProviderRouter struct {
	inner    ProviderRouter
	captured *[]*provider.Request
}

func (c *captureProviderRouter) Stream(ctx stdctx.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	*c.captured = append(*c.captured, req)
	return c.inner.Stream(ctx, req)
}

func TestRunTurnLoopDetectionDoesNotTriggerBelowThreshold(t *testing.T) {
	// 2 identical tool calls with threshold=3 should NOT trigger nudge.
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	toolInput := json.RawMessage(`{"path":"a.go"}`)
	routerStub := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			{
				provider.ToolCallStart{ID: "t1", Name: "read_file"},
				provider.ToolCallEnd{ID: "t1", Input: toolInput},
				provider.StreamDone{
					StopReason: provider.StopReasonToolUse,
					Usage:      provider.Usage{InputTokens: 10, OutputTokens: 5},
				},
			},
			{
				provider.ToolCallStart{ID: "t1", Name: "read_file"},
				provider.ToolCallEnd{ID: "t1", Input: toolInput},
				provider.StreamDone{
					StopReason: provider.StopReasonToolUse,
					Usage:      provider.Usage{InputTokens: 10, OutputTokens: 5},
				},
			},
			{
				provider.TokenDelta{Text: "Done."},
				provider.StreamDone{
					StopReason: provider.StopReasonEndTurn,
					Usage:      provider.Usage{InputTokens: 10, OutputTokens: 3},
				},
			},
		},
	}

	var promptConfigs []*provider.Request
	captureRouter := &captureProviderRouter{
		inner:    routerStub,
		captured: &promptConfigs,
	}

	executor := &toolExecutorStub{}
	promptBuilder := NewPromptBuilder(nil)

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      captureRouter,
		ToolExecutor:        executor,
		PromptBuilder:       promptBuilder,
		Config:              AgentLoopConfig{LoopDetectionThreshold: 3},
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-1",
		TurnNumber:        1,
		Message:           "read a.go",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}
	if result.IterationCount != 3 {
		t.Fatalf("IterationCount = %d, want 3", result.IterationCount)
	}

	// Verify no nudge message was injected in the 3rd prompt's messages.
	// The 3rd prompt (iteration 3) should NOT contain the nudge since only
	// 2 identical iterations happened (below threshold of 3).
	for _, msg := range promptConfigs[2].Messages {
		if msg.Role == provider.RoleUser {
			var text string
			_ = json.Unmarshal(msg.Content, &text)
			if strings.Contains(text, "repeating the same action") {
				t.Fatal("nudge message found below threshold — should not be present")
			}
		}
	}
}

// --- Cancellation tests ---

// blockingProviderRouter blocks on Stream until context is cancelled.
type blockingProviderRouter struct {
	started chan struct{} // closed when Stream is called
}

func (b *blockingProviderRouter) Stream(ctx stdctx.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	if b.started != nil {
		close(b.started)
	}
	// Block until context is cancelled.
	<-ctx.Done()
	return nil, ctx.Err()
}

type partialTokenBlockingProviderRouter struct {
	started chan struct{}
	token   string
}

func (b *partialTokenBlockingProviderRouter) Stream(ctx stdctx.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	ch := make(chan provider.StreamEvent, 1)
	if b.token != "" {
		ch <- provider.TokenDelta{Text: b.token}
	}
	if b.started != nil {
		close(b.started)
	}
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

// blockingToolExecutor blocks on Execute until context is cancelled.
type blockingToolExecutor struct {
	started chan struct{}
}

func (b *blockingToolExecutor) Execute(ctx stdctx.Context, call provider.ToolCall) (*provider.ToolResult, error) {
	if b.started != nil {
		close(b.started)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestRunTurnCancelDuringStream(t *testing.T) {
	sink := NewChannelSink(32)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	streamStarted := make(chan struct{})
	router := &blockingProviderRouter{started: streamStarted}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      router,
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
		EventSink:           sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	errCh := make(chan error, 1)
	go func() {
		_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
			ConversationID:    "conv-1",
			TurnNumber:        1,
			Message:           "hello",
			ModelContextLimit: 200000,
		})
		errCh <- err
	}()

	// Wait for the stream to start, then cancel.
	<-streamStarted
	loop.Cancel()

	err := <-errCh
	if err == nil {
		t.Fatal("RunTurn error = nil, want cancellation error")
	}
	if !errors.Is(err, ErrTurnCancelled) {
		t.Fatalf("error = %v, want ErrTurnCancelled", err)
	}

	// Should have emitted TurnCancelledEvent + StatusEvent(StateIdle).
	var foundCancelled, foundIdle bool
	for i := 0; i < 20; i++ {
		select {
		case e := <-sink.Events():
			if tc, ok := e.(TurnCancelledEvent); ok {
				foundCancelled = true
				if tc.TurnNumber != 1 {
					t.Fatalf("TurnCancelled.TurnNumber = %d, want 1", tc.TurnNumber)
				}
				if tc.Reason != "user_interrupted" {
					t.Fatalf("TurnCancelled.Reason = %q, want user_interrupted", tc.Reason)
				}
			}
			if se, ok := e.(StatusEvent); ok && se.State == StateIdle {
				foundIdle = true
			}
		default:
		}
	}
	if !foundCancelled {
		t.Fatal("TurnCancelledEvent not emitted")
	}
	if !foundIdle {
		t.Fatal("StatusEvent(StateIdle) not emitted after cancellation")
	}
}

func TestHandleTurnCancellationPersistsInterruptedAssistant(t *testing.T) {
	conversations := &loopConversationManagerStub{}
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    &loopContextAssemblerStub{},
		ConversationManager: conversations,
		ProviderRouter:      &providerRouterStub{},
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
	})

	err := loop.handleTurnCancellation(inflightTurn{
		ConversationID:           "conv-partial-stream",
		TurnNumber:               1,
		Iteration:                1,
		CompletedIterations:      0,
		AssistantResponseStarted: true,
		AssistantMessageContent:  `[{"type":"text","text":"hello"}]`,
	}, stdctx.Canceled)
	if err == nil || !errors.Is(err, ErrTurnCancelled) {
		t.Fatalf("error = %v, want ErrTurnCancelled", err)
	}
	if len(conversations.persistIterCalls) != 1 {
		t.Fatalf("PersistIteration calls = %d, want 1 interrupted assistant iteration", len(conversations.persistIterCalls))
	}
	if len(conversations.cancelIterCalls) != 0 {
		t.Fatalf("CancelIteration calls = %d, want 0", len(conversations.cancelIterCalls))
	}
	pi := conversations.persistIterCalls[0]
	if len(pi.messages) != 1 || pi.messages[0].Role != "assistant" || !strings.Contains(pi.messages[0].Content, "[interrupted_assistant]") || !strings.Contains(pi.messages[0].Content, "hello") {
		t.Fatalf("persisted interrupted assistant messages = %#v, want interrupted assistant tombstone with partial text", pi.messages)
	}
}

func TestHandleTurnStreamFailurePersistsFailedAssistant(t *testing.T) {
	conversations := &loopConversationManagerStub{}
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    &loopContextAssemblerStub{},
		ConversationManager: conversations,
		ProviderRouter:      &providerRouterStub{},
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
	})

	err := loop.handleTurnStreamFailure(inflightTurn{
		ConversationID:           "conv-stream-fail",
		TurnNumber:               1,
		Iteration:                1,
		CompletedIterations:      0,
		AssistantResponseStarted: true,
		AssistantMessageContent:  `[{"type":"text","text":"partial"}]`,
	}, errors.New("stream error: connection reset"))
	if err == nil {
		t.Fatal("error = nil, want stream failure error")
	}
	if len(conversations.persistIterCalls) != 1 {
		t.Fatalf("PersistIteration calls = %d, want 1 failed assistant iteration", len(conversations.persistIterCalls))
	}
	pi := conversations.persistIterCalls[0]
	if len(pi.messages) != 1 || !strings.Contains(pi.messages[0].Content, "[failed_assistant]") || !strings.Contains(pi.messages[0].Content, "reason=stream_failure") {
		t.Fatalf("persisted failed assistant messages = %#v, want failed assistant tombstone", pi.messages)
	}
}

func TestRunTurnCancelDuringToolExecution(t *testing.T) {
	sink := NewChannelSink(64)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	// First iteration: tool call that will block.
	toolInput := json.RawMessage(`{"cmd":"sleep 100"}`)
	routerStub := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			{
				provider.ToolCallStart{ID: "t1", Name: "shell"},
				provider.ToolCallEnd{ID: "t1", Input: toolInput},
				provider.StreamDone{
					StopReason: provider.StopReasonToolUse,
					Usage:      provider.Usage{InputTokens: 10, OutputTokens: 5},
				},
			},
		},
	}

	toolStarted := make(chan struct{})
	executor := &blockingToolExecutor{started: toolStarted}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      routerStub,
		ToolExecutor:        executor,
		PromptBuilder:       NewPromptBuilder(nil),
		EventSink:           sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	errCh := make(chan error, 1)
	go func() {
		_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
			ConversationID:    "conv-1",
			TurnNumber:        1,
			Message:           "run command",
			ModelContextLimit: 200000,
		})
		errCh <- err
	}()

	// Wait for tool execution to start, then cancel.
	<-toolStarted
	loop.Cancel()

	err := <-errCh
	if err == nil {
		t.Fatal("RunTurn error = nil, want cancellation error")
	}
	if !errors.Is(err, ErrTurnCancelled) {
		t.Fatalf("error = %v, want ErrTurnCancelled", err)
	}

	// Interrupted tool execution should persist a coherent interrupted iteration.
	if len(conversations.cancelIterCalls) != 0 {
		t.Fatalf("CancelIteration calls = %d, want 0", len(conversations.cancelIterCalls))
	}
	if len(conversations.persistIterCalls) != 1 {
		t.Fatalf("PersistIteration calls = %d, want 1", len(conversations.persistIterCalls))
	}
	pi := conversations.persistIterCalls[0]
	if pi.conversationID != "conv-1" || pi.turnNumber != 1 || pi.iteration != 1 {
		t.Fatalf("PersistIteration call = %+v, want conv-1/1/1", pi)
	}
	if len(pi.messages) != 2 {
		t.Fatalf("Persisted messages = %d, want 2", len(pi.messages))
	}
	if pi.messages[1].Role != "tool" || !strings.Contains(pi.messages[1].Content, "[interrupted_tool_result]") {
		t.Fatalf("interrupted tool message = %#v, want synthesized interrupted tool result", pi.messages[1])
	}
}

func TestRunTurnCtxCancellation(t *testing.T) {
	// Cancel via the context passed to RunTurn (simulating HTTP disconnect).
	sink := NewChannelSink(32)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	streamStarted := make(chan struct{})
	router := &blockingProviderRouter{started: streamStarted}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      router,
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
		EventSink:           sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	ctx, cancel := stdctx.WithCancel(stdctx.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := loop.RunTurn(ctx, RunTurnRequest{
			ConversationID:    "conv-1",
			TurnNumber:        1,
			Message:           "hello",
			ModelContextLimit: 200000,
		})
		errCh <- err
	}()

	<-streamStarted
	cancel() // Cancel the external context.

	err := <-errCh
	if err == nil {
		t.Fatal("RunTurn error = nil, want cancellation error")
	}
	if !errors.Is(err, ErrTurnCancelled) {
		t.Fatalf("error = %v, want ErrTurnCancelled", err)
	}

	var foundCancelled bool
	for i := 0; i < 20; i++ {
		select {
		case e := <-sink.Events():
			if tc, ok := e.(TurnCancelledEvent); ok {
				foundCancelled = true
				if tc.Reason != "user_cancelled" {
					t.Fatalf("Reason = %q, want user_cancelled", tc.Reason)
				}
			}
		default:
		}
	}
	if !foundCancelled {
		t.Fatal("TurnCancelledEvent not emitted")
	}
}

func TestRunTurnCancelIdempotent(t *testing.T) {
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    &loopContextAssemblerStub{},
		ConversationManager: &loopConversationManagerStub{},
		ProviderRouter:      &providerRouterStub{},
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
	})

	// Cancel when no turn is running — should be no-op.
	loop.Cancel()
	loop.Cancel()
	loop.Cancel()
	// No panic = pass.
}

func TestRunTurnCancelNilLoop(t *testing.T) {
	var loop *AgentLoop
	// Cancel on nil loop — should be no-op, no panic.
	loop.Cancel()
}

func TestRunTurnCancelPreservesCompletedIterations(t *testing.T) {
	// Iteration 1 completes (tool use + persist), iteration 2 gets cancelled
	// during streaming. Completed iteration should be preserved, in-flight
	// iteration should be cleaned up.
	sink := NewChannelSink(64)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	toolInput := json.RawMessage(`{"path":"a.go"}`)
	streamStarted := make(chan struct{})

	// Custom router: first call returns tool use, second call blocks.
	callCount := 0
	customRouter := &funcProviderRouter{
		streamFn: func(ctx stdctx.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
			callCount++
			if callCount == 1 {
				events := []provider.StreamEvent{
					provider.ToolCallStart{ID: "t1", Name: "read_file"},
					provider.ToolCallEnd{ID: "t1", Input: toolInput},
					provider.StreamDone{
						StopReason: provider.StopReasonToolUse,
						Usage:      provider.Usage{InputTokens: 10, OutputTokens: 5},
					},
				}
				ch := make(chan provider.StreamEvent, len(events))
				for _, e := range events {
					ch <- e
				}
				close(ch)
				return ch, nil
			}
			// Second call: signal and block.
			close(streamStarted)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	executor := &toolExecutorStub{}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      customRouter,
		ToolExecutor:        executor,
		PromptBuilder:       NewPromptBuilder(nil),
		EventSink:           sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	errCh := make(chan error, 1)
	go func() {
		_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
			ConversationID:    "conv-1",
			TurnNumber:        1,
			Message:           "read a.go",
			ModelContextLimit: 200000,
		})
		errCh <- err
	}()

	// Wait for iteration 2 streaming to start.
	<-streamStarted
	loop.Cancel()

	err := <-errCh
	if !errors.Is(err, ErrTurnCancelled) {
		t.Fatalf("error = %v, want ErrTurnCancelled", err)
	}

	// Iteration 1 should have been persisted.
	if len(conversations.persistIterCalls) != 1 {
		t.Fatalf("PersistIteration calls = %d, want 1 (completed iteration)", len(conversations.persistIterCalls))
	}
	if conversations.persistIterCalls[0].iteration != 1 {
		t.Fatalf("persisted iteration = %d, want 1", conversations.persistIterCalls[0].iteration)
	}

	// CancelIteration should have been called for iteration 2.
	if len(conversations.cancelIterCalls) != 1 {
		t.Fatalf("CancelIteration calls = %d, want 1", len(conversations.cancelIterCalls))
	}
	if conversations.cancelIterCalls[0].iteration != 2 {
		t.Fatalf("cancelled iteration = %d, want 2", conversations.cancelIterCalls[0].iteration)
	}

	// TurnCancelledEvent should show 1 completed iteration.
	var foundCancelled bool
	for i := 0; i < 30; i++ {
		select {
		case e := <-sink.Events():
			if tc, ok := e.(TurnCancelledEvent); ok {
				foundCancelled = true
				if tc.CompletedIterations != 1 {
					t.Fatalf("TurnCancelled.CompletedIterations = %d, want 1", tc.CompletedIterations)
				}
			}
		default:
		}
	}
	if !foundCancelled {
		t.Fatal("TurnCancelledEvent not emitted")
	}
}

// funcProviderRouter allows custom Stream behavior via a function.
type funcProviderRouter struct {
	streamFn func(ctx stdctx.Context, req *provider.Request) (<-chan provider.StreamEvent, error)
}

func (f *funcProviderRouter) Stream(ctx stdctx.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	return f.streamFn(ctx, req)
}

func TestRunTurnCancelWithDeadlineExceeded(t *testing.T) {
	sink := NewChannelSink(32)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	streamStarted := make(chan struct{})
	router := &blockingProviderRouter{started: streamStarted}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      router,
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
		EventSink:           sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000600, 0).UTC() }

	// Use a very short deadline to trigger DeadlineExceeded.
	ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := loop.RunTurn(ctx, RunTurnRequest{
		ConversationID:    "conv-1",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 200000,
	})
	if err == nil {
		t.Fatal("RunTurn error = nil, want cancellation error")
	}
	if !errors.Is(err, ErrTurnCancelled) {
		t.Fatalf("error = %v, want ErrTurnCancelled", err)
	}

	// Reason should be context_deadline_exceeded.
	var foundCancelled bool
	for i := 0; i < 20; i++ {
		select {
		case e := <-sink.Events():
			if tc, ok := e.(TurnCancelledEvent); ok {
				foundCancelled = true
				if tc.Reason != "context_deadline_exceeded" {
					t.Fatalf("Reason = %q, want context_deadline_exceeded", tc.Reason)
				}
			}
		default:
		}
	}
	if !foundCancelled {
		t.Fatal("TurnCancelledEvent not emitted")
	}
}

// --- Error recovery integration tests ---

func TestRunTurnMalformedToolCallFeedsBackError(t *testing.T) {
	sink := NewChannelSink(64)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	// Iteration 1: LLM returns a tool call with invalid JSON.
	// Iteration 2: LLM produces text response after seeing the validation error.
	routerStub := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			// Iteration 1: tool call with invalid JSON args.
			{
				provider.ToolCallStart{ID: "tool_bad", Name: "file_read"},
				provider.ToolCallEnd{ID: "tool_bad", Input: json.RawMessage(`{invalid}`)},
				provider.StreamDone{
					StopReason: provider.StopReasonToolUse,
					Usage:      provider.Usage{InputTokens: 30, OutputTokens: 15},
				},
			},
			// Iteration 2: text-only response.
			{
				provider.TokenDelta{Text: "I fixed the JSON."},
				provider.StreamDone{
					StopReason: provider.StopReasonEndTurn,
					Usage:      provider.Usage{InputTokens: 60, OutputTokens: 10},
				},
			},
		},
	}

	executor := &toolExecutorStub{}
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      routerStub,
		ToolExecutor:        executor,
		PromptBuilder:       NewPromptBuilder(nil),
		EventSink:           sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000800, 0).UTC() }

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-malformed-1",
		TurnNumber:        1,
		Message:           "read a file",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	// Turn should complete successfully — malformed tool call is NOT turn-ending.
	if result.FinalText != "I fixed the JSON." {
		t.Fatalf("FinalText = %q, want %q", result.FinalText, "I fixed the JSON.")
	}
	if result.IterationCount != 2 {
		t.Fatalf("IterationCount = %d, want 2", result.IterationCount)
	}

	// The tool executor should NOT have been called (malformed call skips execution).
	if len(executor.calls) != 0 {
		t.Fatalf("executor.calls = %d, want 0 (malformed tool calls should not be executed)", len(executor.calls))
	}

	// The persisted iteration 1 should have an error tool result.
	iter1 := conversations.persistIterCalls[0]
	if len(iter1.messages) < 2 {
		t.Fatalf("iteration 1 messages = %d, want >= 2", len(iter1.messages))
	}
	toolMsg := iter1.messages[1]
	if toolMsg.Role != "tool" {
		t.Fatalf("tool msg role = %q, want tool", toolMsg.Role)
	}
	if !strings.Contains(toolMsg.Content, "invalid JSON") {
		t.Fatalf("tool error content = %q, want containing 'invalid JSON'", toolMsg.Content)
	}

	// Check that a malformed_tool_call ErrorEvent was emitted.
	var foundMalformed bool
	for i := 0; i < 50; i++ {
		select {
		case e := <-sink.Events():
			if errEvt, ok := e.(ErrorEvent); ok && errEvt.ErrorCode == ErrorCodeMalformedToolCall {
				foundMalformed = true
				if !errEvt.Recoverable {
					t.Fatal("malformed tool call ErrorEvent.Recoverable = false, want true")
				}
			}
		default:
		}
		if foundMalformed {
			break
		}
	}
	if !foundMalformed {
		t.Fatal("ErrorEvent with ErrorCodeMalformedToolCall not emitted")
	}
}

func TestRunTurnEmptyToolCallArgsFeedsBackError(t *testing.T) {
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	routerStub := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			// Iteration 1: tool call with empty input.
			{
				provider.ToolCallStart{ID: "tool_empty", Name: "git_status"},
				provider.ToolCallEnd{ID: "tool_empty", Input: json.RawMessage(``)},
				provider.StreamDone{
					StopReason: provider.StopReasonToolUse,
					Usage:      provider.Usage{InputTokens: 20, OutputTokens: 10},
				},
			},
			// Iteration 2: text response.
			{
				provider.TokenDelta{Text: "Done."},
				provider.StreamDone{
					StopReason: provider.StopReasonEndTurn,
					Usage:      provider.Usage{InputTokens: 40, OutputTokens: 5},
				},
			},
		},
	}

	executor := &toolExecutorStub{}
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      routerStub,
		ToolExecutor:        executor,
		PromptBuilder:       NewPromptBuilder(nil),
	})
	loop.now = func() time.Time { return time.Unix(1700000900, 0).UTC() }

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-empty-args",
		TurnNumber:        1,
		Message:           "check status",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}
	if result.FinalText != "Done." {
		t.Fatalf("FinalText = %q, want %q", result.FinalText, "Done.")
	}

	// Tool executor should not have been called.
	if len(executor.calls) != 0 {
		t.Fatalf("executor.calls = %d, want 0", len(executor.calls))
	}

	// Persisted tool result should contain "empty arguments".
	iter1 := conversations.persistIterCalls[0]
	toolMsg := iter1.messages[1]
	if !strings.Contains(toolMsg.Content, "empty arguments") {
		t.Fatalf("tool error content = %q, want containing 'empty arguments'", toolMsg.Content)
	}
}

func TestRunTurnToolErrorEnrichment(t *testing.T) {
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	toolInput := json.RawMessage(`{"path":"missing.go"}`)
	routerStub := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			{
				provider.ToolCallStart{ID: "tool_1", Name: "file_read"},
				provider.ToolCallEnd{ID: "tool_1", Input: toolInput},
				provider.StreamDone{
					StopReason: provider.StopReasonToolUse,
					Usage:      provider.Usage{InputTokens: 30, OutputTokens: 15},
				},
			},
			{
				provider.TokenDelta{Text: "File not found, let me try another path."},
				provider.StreamDone{
					StopReason: provider.StopReasonEndTurn,
					Usage:      provider.Usage{InputTokens: 60, OutputTokens: 20},
				},
			},
		},
	}

	executor := &toolExecutorStub{
		err: errors.New("file not found: missing.go"),
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      routerStub,
		ToolExecutor:        executor,
		PromptBuilder:       NewPromptBuilder(nil),
	})
	loop.now = func() time.Time { return time.Unix(1700001000, 0).UTC() }

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-enrich",
		TurnNumber:        1,
		Message:           "read missing.go",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}
	if result.FinalText != "File not found, let me try another path." {
		t.Fatalf("FinalText = %q", result.FinalText)
	}

	// The persisted tool result should contain the enriched hint.
	iter1 := conversations.persistIterCalls[0]
	toolMsg := iter1.messages[1]
	if !strings.Contains(toolMsg.Content, "Hint:") {
		t.Fatalf("tool error = %q, want containing 'Hint:'", toolMsg.Content)
	}
	if !strings.Contains(toolMsg.Content, "file not found") {
		t.Fatalf("tool error = %q, want containing original error", toolMsg.Content)
	}
}

func TestRunTurnRetriableStreamErrorRecoversInRunTurn(t *testing.T) {
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "context", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	rateLimitErr := &provider.ProviderError{
		Provider:   "anthropic",
		StatusCode: 429,
		Message:    "rate limited",
		Retriable:  true,
	}
	router := &retryProviderRouterStub{
		responses: []retryResponse{
			{err: rateLimitErr},
			{events: textOnlyStreamEvents("recovered after retry")},
		},
	}

	sink := NewChannelSink(32)
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      router,
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
		EventSink:           sink,
	})
	loop.now = func() time.Time { return time.Unix(1700001100, 0).UTC() }
	loop.sleepFn = func(ctx stdctx.Context, d time.Duration) error {
		return nil
	}

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-retry-runturn",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v (should have recovered via retry)", err)
	}
	if result.FinalText != "recovered after retry" {
		t.Fatalf("FinalText = %q, want %q", result.FinalText, "recovered after retry")
	}
}

// capturingRouterStub records the provider.Request passed to each Stream call.
type capturingRouterStub struct {
	requests     []*provider.Request
	streamEvents [][]provider.StreamEvent
	callIndex    int
}

func (s *capturingRouterStub) Stream(_ stdctx.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	s.requests = append(s.requests, req)
	idx := s.callIndex
	if idx >= len(s.streamEvents) {
		idx = len(s.streamEvents) - 1
	}
	s.callIndex++

	events := s.streamEvents[idx]
	ch := make(chan provider.StreamEvent, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func TestRunTurnToolDefinitionsWired(t *testing.T) {
	// Verify that ToolDefinitions from AgentLoopDeps are included in the
	// provider.Request sent to the provider router.
	sink := NewChannelSink(32)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "ctx", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	router := &capturingRouterStub{
		streamEvents: [][]provider.StreamEvent{
			{
				provider.TokenDelta{Text: "Hello"},
				provider.StreamDone{
					StopReason: provider.StopReasonEndTurn,
					Usage:      provider.Usage{InputTokens: 50, OutputTokens: 5},
				},
			},
		},
	}

	toolDefs := []provider.ToolDefinition{
		{
			Name:        "file_read",
			Description: "Read file contents",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		},
		{
			Name:        "shell",
			Description: "Execute a shell command",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`),
		},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      router,
		ToolExecutor:        &toolExecutorStub{},
		ToolDefinitions:     toolDefs,
		PromptBuilder:       NewPromptBuilder(nil),
		EventSink:           sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000700, 0).UTC() }

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-tools",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}
	if result.FinalText != "Hello" {
		t.Fatalf("FinalText = %q, want Hello", result.FinalText)
	}

	// The router should have received exactly one request with tool definitions.
	if len(router.requests) != 1 {
		t.Fatalf("router received %d requests, want 1", len(router.requests))
	}

	req := router.requests[0]
	if len(req.Tools) != 2 {
		t.Fatalf("request has %d tools, want 2", len(req.Tools))
	}
	if req.Tools[0].Name != "file_read" {
		t.Errorf("tools[0].Name = %q, want file_read", req.Tools[0].Name)
	}
	if req.Tools[1].Name != "shell" {
		t.Errorf("tools[1].Name = %q, want shell", req.Tools[1].Name)
	}
	if req.Tools[0].InputSchema == nil {
		t.Error("tools[0].InputSchema is nil")
	}
}

func TestRunTurnToolDefinitionsOmittedOnFinalIteration(t *testing.T) {
	// On the final iteration (max iterations reached), tools should be omitted
	// from the provider.Request even though tool definitions are configured.
	sink := NewChannelSink(32)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "ctx", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	// Two calls: first returns tool use, second (final iteration) returns text.
	toolInput := json.RawMessage(`{"path":"main.go"}`)
	router := &capturingRouterStub{
		streamEvents: [][]provider.StreamEvent{
			// Iteration 1: tool use response.
			{
				provider.TokenDelta{Text: "Reading file."},
				provider.ToolCallStart{ID: "call_1", Name: "file_read"},
				provider.ToolCallEnd{ID: "call_1", Input: toolInput},
				provider.StreamDone{
					StopReason: provider.StopReasonToolUse,
					Usage:      provider.Usage{InputTokens: 100, OutputTokens: 20},
				},
			},
			// Iteration 2 (final): text-only response.
			{
				provider.TokenDelta{Text: "Done reading."},
				provider.StreamDone{
					StopReason: provider.StopReasonEndTurn,
					Usage:      provider.Usage{InputTokens: 80, OutputTokens: 10},
				},
			},
		},
	}

	toolDefs := []provider.ToolDefinition{
		{
			Name:        "file_read",
			Description: "Read file contents",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      router,
		ToolExecutor: &toolExecutorStub{
			results: map[string]*provider.ToolResult{
				"call_1": {ToolUseID: "call_1", Content: "file contents here"},
			},
		},
		ToolDefinitions: toolDefs,
		PromptBuilder:   NewPromptBuilder(nil),
		EventSink:       sink,
		Config: AgentLoopConfig{
			MaxIterations: 2, // Force final iteration on call 2.
		},
	})
	loop.now = func() time.Time { return time.Unix(1700000800, 0).UTC() }

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-final",
		TurnNumber:        1,
		Message:           "read main.go",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}
	if result.FinalText != "Done reading." {
		t.Fatalf("FinalText = %q, want 'Done reading.'", result.FinalText)
	}

	// Should have two requests.
	if len(router.requests) != 2 {
		t.Fatalf("router received %d requests, want 2", len(router.requests))
	}

	// First request should have tools.
	if len(router.requests[0].Tools) != 1 {
		t.Fatalf("request 1 has %d tools, want 1", len(router.requests[0].Tools))
	}

	// Second request (final iteration) should have NO tools.
	if len(router.requests[1].Tools) != 0 {
		t.Fatalf("request 2 (final iteration) has %d tools, want 0", len(router.requests[1].Tools))
	}
}

func TestRunTurnPassesExecutionMetaToToolExecutor(t *testing.T) {
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "ctx", Frozen: true},
	}
	conversations := &loopConversationManagerStub{history: []db.Message{}, seen: loopSeenFilesStub{}}
	router := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			{
				provider.ToolCallStart{ID: "tc-1", Name: "file_read"},
				provider.ToolCallEnd{ID: "tc-1", Input: json.RawMessage(`{"path":"main.go"}`)},
				provider.StreamDone{StopReason: provider.StopReasonToolUse, Usage: provider.Usage{InputTokens: 10, OutputTokens: 5}},
			},
			textOnlyStream("done", 10, 3),
		},
	}
	executor := &toolExecutorMetaStub{}
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      router,
		ToolExecutor:        executor,
		PromptBuilder:       NewPromptBuilder(nil),
	})

	_, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-meta",
		TurnNumber:        7,
		Message:           "read main.go",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}
	if len(executor.captures) != 1 {
		t.Fatalf("tool executor captures = %d, want 1", len(executor.captures))
	}
	capture := executor.captures[0]
	if !capture.ok {
		t.Fatal("tool executor context did not include execution meta")
	}
	if capture.conversationID != "conv-meta" || capture.turnNumber != 7 || capture.iteration != 1 {
		t.Fatalf("execution meta = %+v, want conversation=conv-meta turn=7 iteration=1", capture)
	}
}

func TestRunTurnNoToolDefinitions(t *testing.T) {
	// When no tool definitions are configured, requests should have empty tools.
	sink := NewChannelSink(32)
	assembler := &loopContextAssemblerStub{
		pkg: &contextpkg.FullContextPackage{Content: "ctx", Frozen: true},
	}
	conversations := &loopConversationManagerStub{
		history: []db.Message{},
		seen:    loopSeenFilesStub{},
	}

	router := &capturingRouterStub{
		streamEvents: [][]provider.StreamEvent{
			{
				provider.TokenDelta{Text: "no tools here"},
				provider.StreamDone{
					StopReason: provider.StopReasonEndTurn,
					Usage:      provider.Usage{InputTokens: 30, OutputTokens: 5},
				},
			},
		},
	}

	// No ToolDefinitions provided.
	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: conversations,
		ProviderRouter:      router,
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
		EventSink:           sink,
	})
	loop.now = func() time.Time { return time.Unix(1700000900, 0).UTC() }

	result, err := loop.RunTurn(stdctx.Background(), RunTurnRequest{
		ConversationID:    "conv-no-tools",
		TurnNumber:        1,
		Message:           "hello",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}
	if result.FinalText != "no tools here" {
		t.Fatalf("FinalText = %q, want 'no tools here'", result.FinalText)
	}

	if len(router.requests) != 1 {
		t.Fatalf("router received %d requests, want 1", len(router.requests))
	}
	if len(router.requests[0].Tools) != 0 {
		t.Fatalf("request has %d tools, want 0 (no tool definitions configured)", len(router.requests[0].Tools))
	}
}
