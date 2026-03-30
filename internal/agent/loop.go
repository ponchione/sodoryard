package agent

import (
	stdctx "context"
	"fmt"
	"log/slog"
	"time"

	contextpkg "github.com/ponchione/sirtopham/internal/context"
	"github.com/ponchione/sirtopham/internal/db"
)

const (
	defaultMaxIterations       = 50
	defaultLoopDetectThreshold = 3
)

// ContextAssembler is the narrow Layer 3 boundary the agent loop needs at turn
// start and after turn completion.
type ContextAssembler interface {
	Assemble(
		ctx stdctx.Context,
		message string,
		history []db.Message,
		scope contextpkg.AssemblyScope,
		modelContextLimit int,
		historyTokenCount int,
	) (*contextpkg.FullContextPackage, bool, error)
	UpdateQuality(ctx stdctx.Context, conversationID string, turnNumber int, usedSearchTool bool, readFiles []string) error
}

// ConversationManager is the narrow Layer 5 conversation boundary needed for
// turn-start context assembly wiring.
type ConversationManager interface {
	ReconstructHistory(ctx stdctx.Context, conversationID string) ([]db.Message, error)
	SeenFiles(conversationID string) contextpkg.SeenFileLookup
}

// AgentLoopConfig carries the initial state-machine knobs for the future full
// RunTurn implementation.
type AgentLoopConfig struct {
	MaxIterations          int  `json:"max_iterations,omitempty"`
	LoopDetectionThreshold int  `json:"loop_detection_threshold,omitempty"`
	ExtendedThinking       bool `json:"extended_thinking,omitempty"`
}

// AgentLoopDeps carries the dependencies currently needed by the turn-start
// seam. Additional fields can be added as the full loop lands.
type AgentLoopDeps struct {
	ContextAssembler    ContextAssembler
	ConversationManager ConversationManager
	EventSink           EventSink
	Config              AgentLoopConfig
	Logger              *slog.Logger
}

// TurnStartResult holds the frozen per-turn context package plus the history
// used to build it.
type TurnStartResult struct {
	History           []db.Message                   `json:"history,omitempty"`
	ContextPackage    *contextpkg.FullContextPackage `json:"context_package,omitempty"`
	CompressionNeeded bool                           `json:"compression_needed,omitempty"`
}

// AgentLoop is the early Layer 5 orchestration shell. For now it owns the
// turn-start seam that reconstructs history and invokes Layer 3 assembly.
type AgentLoop struct {
	contextAssembler    ContextAssembler
	conversationManager ConversationManager
	events              *MultiSink
	cfg                 AgentLoopConfig
	logger              *slog.Logger
	now                 func() time.Time
}

// NewAgentLoop constructs the minimal agent-loop shell and applies default
// config values for future turn execution.
func NewAgentLoop(deps AgentLoopDeps) *AgentLoop {
	events := NewMultiSink()
	if deps.EventSink != nil {
		events.Add(deps.EventSink)
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &AgentLoop{
		contextAssembler:    deps.ContextAssembler,
		conversationManager: deps.ConversationManager,
		events:              events,
		cfg:                 withDefaultConfig(deps.Config),
		logger:              logger,
		now:                 time.Now,
	}
}

// Subscribe adds another event sink to the loop's internal fan-out sink.
func (l *AgentLoop) Subscribe(sink EventSink) {
	if l == nil || l.events == nil {
		return
	}
	l.events.Add(sink)
}

// Unsubscribe removes an event sink from the loop's internal fan-out sink.
func (l *AgentLoop) Unsubscribe(sink EventSink) {
	if l == nil || l.events == nil {
		return
	}
	l.events.Remove(sink)
}

// Close closes the loop's internal event fan-out sink.
func (l *AgentLoop) Close() {
	if l == nil || l.events == nil {
		return
	}
	l.events.Close()
}

// PrepareTurnContext reconstructs persisted history, builds the Layer 3
// AssemblyScope, invokes ContextAssembler.Assemble, and emits the early events
// the future RunTurn loop will rely on.
func (l *AgentLoop) PrepareTurnContext(
	ctx stdctx.Context,
	conversationID string,
	turnNumber int,
	message string,
	modelContextLimit int,
	historyTokenCount int,
) (*TurnStartResult, error) {
	if ctx == nil {
		ctx = stdctx.Background()
	}
	if err := l.validate(); err != nil {
		return nil, err
	}

	l.emit(StatusEvent{State: StateAssemblingContext, Time: l.now()})
	history, err := l.conversationManager.ReconstructHistory(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("agent loop: reconstruct history: %w", err)
	}

	scope := contextpkg.AssemblyScope{
		ConversationID: conversationID,
		TurnNumber:     turnNumber,
		SeenFiles:      l.conversationManager.SeenFiles(conversationID),
	}
	pkg, compressionNeeded, err := l.contextAssembler.Assemble(ctx, message, history, scope, modelContextLimit, historyTokenCount)
	if err != nil {
		return nil, fmt.Errorf("agent loop: assemble turn context: %w", err)
	}

	var report *contextpkg.ContextAssemblyReport
	if pkg != nil {
		report = pkg.Report
	}
	l.emit(ContextDebugEvent{Report: report, Time: l.now()})
	l.emit(StatusEvent{State: StateWaitingForLLM, Time: l.now()})

	return &TurnStartResult{
		History:           append([]db.Message(nil), history...),
		ContextPackage:    pkg,
		CompressionNeeded: compressionNeeded,
	}, nil
}

func (l *AgentLoop) validate() error {
	if l == nil {
		return fmt.Errorf("agent loop: loop is nil")
	}
	if l.contextAssembler == nil {
		return fmt.Errorf("agent loop: context assembler is nil")
	}
	if l.conversationManager == nil {
		return fmt.Errorf("agent loop: conversation manager is nil")
	}
	if l.events == nil {
		return fmt.Errorf("agent loop: event sink is unavailable")
	}
	if l.now == nil {
		return fmt.Errorf("agent loop: clock is unavailable")
	}
	return nil
}

func (l *AgentLoop) emit(event Event) {
	if l == nil || l.events == nil || event == nil {
		return
	}
	l.events.Emit(event)
}

func withDefaultConfig(cfg AgentLoopConfig) AgentLoopConfig {
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = defaultMaxIterations
	}
	if cfg.LoopDetectionThreshold <= 0 {
		cfg.LoopDetectionThreshold = defaultLoopDetectThreshold
	}
	if !cfg.ExtendedThinking {
		cfg.ExtendedThinking = true
	}
	return cfg
}
