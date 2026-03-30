package agent

import (
	stdctx "context"
	"database/sql"
	"strings"
	"testing"
	"time"

	contextpkg "github.com/ponchione/sirtopham/internal/context"
	"github.com/ponchione/sirtopham/internal/db"
)

type loopSeenFilesStub struct{}

func (loopSeenFilesStub) Contains(path string) (bool, int) {
	if path == "internal/auth/service.go" {
		return true, 2
	}
	return false, 0
}

type loopConversationManagerStub struct {
	history               []db.Message
	err                   error
	reconstructCalls      []string
	seenFilesConversation []string
	seen                  contextpkg.SeenFileLookup
}

func (s *loopConversationManagerStub) ReconstructHistory(_ stdctx.Context, conversationID string) ([]db.Message, error) {
	s.reconstructCalls = append(s.reconstructCalls, conversationID)
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
