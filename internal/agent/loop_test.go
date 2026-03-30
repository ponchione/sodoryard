package agent

import (
	stdctx "context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sirtopham/internal/conversation"
	contextpkg "github.com/ponchione/sirtopham/internal/context"
	"github.com/ponchione/sirtopham/internal/db"
	"github.com/ponchione/sirtopham/internal/provider"
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
}

func (s *providerRouterStub) Stream(_ stdctx.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	if s.streamErr != nil {
		return nil, s.streamErr
	}
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
	if !strings.Contains(err.Error(), "stream request") {
		t.Fatalf("error = %q, want stream request error", err)
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


