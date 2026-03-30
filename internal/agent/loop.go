package agent

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ponchione/sirtopham/internal/conversation"
	contextpkg "github.com/ponchione/sirtopham/internal/context"
	"github.com/ponchione/sirtopham/internal/db"
	"github.com/ponchione/sirtopham/internal/provider"
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
// turn-start persistence, per-iteration persistence, cancellation, and
// context-assembly wiring.
type ConversationManager interface {
	PersistUserMessage(ctx stdctx.Context, conversationID string, turnNumber int, message string) error
	PersistIteration(ctx stdctx.Context, conversationID string, turnNumber, iteration int, messages []conversation.IterationMessage) error
	CancelIteration(ctx stdctx.Context, conversationID string, turnNumber, iteration int) error
	ReconstructHistory(ctx stdctx.Context, conversationID string) ([]db.Message, error)
	SeenFiles(conversationID string) contextpkg.SeenFileLookup
}

// ProviderRouter is the narrow interface the agent loop needs for LLM calls.
// The provider.Router satisfies this directly.
type ProviderRouter interface {
	Stream(ctx stdctx.Context, req *provider.Request) (<-chan provider.StreamEvent, error)
}

// ToolExecutor dispatches tool calls and returns results. Layer 4 will provide
// a concrete implementation; the agent loop depends only on this interface.
type ToolExecutor interface {
	Execute(ctx stdctx.Context, call provider.ToolCall) (*provider.ToolResult, error)
}

// AgentLoopConfig carries the initial state-machine knobs for the future full
// RunTurn implementation.
type AgentLoopConfig struct {
	MaxIterations          int    `json:"max_iterations,omitempty"`
	LoopDetectionThreshold int    `json:"loop_detection_threshold,omitempty"`
	ExtendedThinking       bool   `json:"extended_thinking,omitempty"`
	BasePrompt             string `json:"base_prompt,omitempty"`
	ProviderName           string `json:"provider_name,omitempty"`
	ModelName              string `json:"model_name,omitempty"`
}

// AgentLoopDeps carries the dependencies needed by the agent loop.
type AgentLoopDeps struct {
	ContextAssembler    ContextAssembler
	ConversationManager ConversationManager
	ProviderRouter      ProviderRouter
	ToolExecutor        ToolExecutor
	PromptBuilder       *PromptBuilder
	EventSink           EventSink
	Config              AgentLoopConfig
	Logger              *slog.Logger
}

// RunTurnRequest is the request shape for RunTurn.
type RunTurnRequest struct {
	ConversationID    string `json:"conversation_id"`
	TurnNumber        int    `json:"turn_number"`
	Message           string `json:"message"`
	ModelContextLimit int    `json:"model_context_limit"`
	HistoryTokenCount int    `json:"history_token_count,omitempty"`
}

// TurnStartResult holds the frozen per-turn context package plus the history
// used to build it.
type TurnStartResult struct {
	History           []db.Message                  `json:"history,omitempty"`
	ContextPackage    *contextpkg.FullContextPackage `json:"context_package,omitempty"`
	CompressionNeeded bool                           `json:"compression_needed,omitempty"`
}

// AgentLoop is the Layer 5 orchestration engine. It owns the turn state machine
// that sequences: persist user message → context assembly → iteration loop
// (prompt build → LLM stream → tool dispatch → persist → repeat).
type AgentLoop struct {
	contextAssembler    ContextAssembler
	conversationManager ConversationManager
	providerRouter      ProviderRouter
	toolExecutor        ToolExecutor
	promptBuilder       *PromptBuilder
	events              *MultiSink
	cfg                 AgentLoopConfig
	logger              *slog.Logger
	now                 func() time.Time
}

// NewAgentLoop constructs the agent loop and applies default config values.
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
		providerRouter:      deps.ProviderRouter,
		toolExecutor:        deps.ToolExecutor,
		promptBuilder:       deps.PromptBuilder,
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

// RunTurn executes a full turn: persist user message → context assembly →
// iteration loop until text-only response or max iterations reached.
//
// The method is blocking — it returns only when the turn is complete,
// cancelled, or fails. Events stream via EventSink throughout.
func (l *AgentLoop) RunTurn(ctx stdctx.Context, req RunTurnRequest) (*TurnResult, error) {
	if ctx == nil {
		ctx = stdctx.Background()
	}
	if err := l.validate(); err != nil {
		return nil, err
	}
	if err := validateRunTurnRequest(req); err != nil {
		return nil, err
	}

	turnStart := l.now()

	// Step 1: Persist user message.
	if err := l.conversationManager.PersistUserMessage(ctx, req.ConversationID, req.TurnNumber, req.Message); err != nil {
		wrapped := fmt.Errorf("agent loop: persist user message: %w", err)
		l.emit(ErrorEvent{
			ErrorCode:   "persist_user_message_failed",
			Message:     wrapped.Error(),
			Recoverable: false,
			Time:        l.now(),
		})
		return nil, wrapped
	}

	// Step 2: Context assembly (frozen for the turn).
	turnCtx, err := l.PrepareTurnContext(
		ctx,
		req.ConversationID,
		req.TurnNumber,
		req.Message,
		req.ModelContextLimit,
		req.HistoryTokenCount,
	)
	if err != nil {
		return nil, err
	}

	// Step 3: Iteration loop.
	iteration := 1
	var totalUsage provider.Usage
	var currentTurnMessages []provider.Message

	// Add the user message as the first current-turn message.
	currentTurnMessages = append(currentTurnMessages, provider.NewUserMessage(req.Message))

	for iteration <= l.cfg.MaxIterations {
		l.logger.Info("starting iteration",
			"conversation_id", req.ConversationID,
			"turn", req.TurnNumber,
			"iteration", iteration,
		)

		// 3a: Reconstruct history (includes persisted messages from prior iterations).
		history, err := l.conversationManager.ReconstructHistory(ctx, req.ConversationID)
		if err != nil {
			return nil, fmt.Errorf("agent loop: reconstruct history for iteration %d: %w", iteration, err)
		}

		// Determine if this is the last allowed iteration — disable tools to force text.
		disableTools := iteration >= l.cfg.MaxIterations

		// 3b: Build prompt.
		promptReq, err := l.promptBuilder.BuildPrompt(PromptConfig{
			BasePrompt:          l.cfg.BasePrompt,
			ContextPackage:      turnCtx.ContextPackage,
			History:             history,
			CurrentTurnMessages: currentTurnMessages,
			ProviderName:        l.cfg.ProviderName,
			ModelName:           l.cfg.ModelName,
			ContextLimit:        req.ModelContextLimit,
			DisableTools:        disableTools,
			Purpose:             "chat",
			ConversationID:      req.ConversationID,
			TurnNumber:          req.TurnNumber,
			Iteration:           iteration,
		})
		if err != nil {
			return nil, fmt.Errorf("agent loop: build prompt for iteration %d: %w", iteration, err)
		}

		// 3c: Stream LLM request.
		l.emit(StatusEvent{State: StateWaitingForLLM, Time: l.now()})

		streamCh, err := l.providerRouter.Stream(ctx, promptReq)
		if err != nil {
			return nil, fmt.Errorf("agent loop: stream request for iteration %d: %w", iteration, err)
		}

		result, err := consumeStream(ctx, streamCh, l.emit, func() string {
			return l.now().UTC().Format(time.RFC3339)
		})
		if err != nil {
			return nil, fmt.Errorf("agent loop: consume stream for iteration %d: %w", iteration, err)
		}

		totalUsage = totalUsage.Add(result.Usage)

		// Serialize assistant content blocks for persistence.
		assistantContentJSON, err := contentBlocksToJSON(result.ContentBlocks)
		if err != nil {
			return nil, fmt.Errorf("agent loop: serialize assistant content for iteration %d: %w", iteration, err)
		}

		// 3d: Check if text-only response (turn complete).
		if !result.HasToolUse() {
			// Persist the assistant message as a single-message iteration.
			persistMessages := []conversation.IterationMessage{
				{
					Role:    "assistant",
					Content: assistantContentJSON,
				},
			}
			if err := l.conversationManager.PersistIteration(ctx, req.ConversationID, req.TurnNumber, iteration, persistMessages); err != nil {
				return nil, fmt.Errorf("agent loop: persist final iteration %d: %w", iteration, err)
			}

			// Turn complete.
			turnDuration := l.now().Sub(turnStart)
			l.emit(TurnCompleteEvent{
				TurnNumber:        req.TurnNumber,
				IterationCount:    iteration,
				TotalInputTokens:  totalUsage.InputTokens,
				TotalOutputTokens: totalUsage.OutputTokens,
				Duration:          turnDuration,
				Time:              l.now(),
			})

			return &TurnResult{
				TurnStartResult:   *turnCtx,
				FinalText:         result.TextContent,
				IterationCount:    iteration,
				TotalUsage:        totalUsage,
				Duration:          turnDuration,
			}, nil
		}

		// 3e: Tool dispatch — execute each tool call.
		l.emit(StatusEvent{State: StateExecutingTools, Time: l.now()})

		var toolResults []provider.ToolResult
		for _, tc := range result.ToolCalls {
			l.emit(ToolCallStartEvent{
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				Arguments:  tc.Input,
				Time:       l.now(),
			})

			toolStart := l.now()
			toolResult, toolErr := l.toolExecutor.Execute(ctx, tc)
			toolDuration := l.now().Sub(toolStart)

			if toolErr != nil {
				// Tool execution failed — create an error result.
				toolResult = &provider.ToolResult{
					ToolUseID: tc.ID,
					Content:   fmt.Sprintf("Error: %s", toolErr.Error()),
					IsError:   true,
				}
			}

			toolResults = append(toolResults, *toolResult)

			l.emit(ToolCallEndEvent{
				ToolCallID: tc.ID,
				Result:     toolResult.Content,
				Duration:   toolDuration,
				Success:    !toolResult.IsError,
				Time:       l.now(),
			})
		}

		// Build the iteration messages for persistence: assistant + tool results.
		persistMessages := []conversation.IterationMessage{
			{
				Role:    "assistant",
				Content: assistantContentJSON,
			},
		}
		for _, tr := range toolResults {
			persistMessages = append(persistMessages, conversation.IterationMessage{
				Role:      "tool",
				Content:   tr.Content,
				ToolUseID: tr.ToolUseID,
				ToolName:  toolNameFromResults(result.ToolCalls, tr.ToolUseID),
			})
		}

		if err := l.conversationManager.PersistIteration(ctx, req.ConversationID, req.TurnNumber, iteration, persistMessages); err != nil {
			return nil, fmt.Errorf("agent loop: persist iteration %d: %w", iteration, err)
		}

		// Build tool result messages for the next iteration's current-turn messages.
		// We need to build the assistant message as a provider.Message plus tool result messages.
		assistantMsg := provider.Message{
			Role:    provider.RoleAssistant,
			Content: json.RawMessage(assistantContentJSON),
		}
		currentTurnMessages = append(currentTurnMessages, assistantMsg)

		for _, tr := range toolResults {
			currentTurnMessages = append(currentTurnMessages, provider.NewToolResultMessage(
				tr.ToolUseID,
				toolNameFromResults(result.ToolCalls, tr.ToolUseID),
				tr.Content,
			))
		}

		iteration++
	}

	// Should not reach here — loop exits via text-only response or the final
	// iteration with disabled tools. If we do, it means max iterations was hit
	// but the model still produced tool calls on the last allowed iteration
	// (which shouldn't happen since tools are disabled).
	return nil, fmt.Errorf("agent loop: exceeded max iterations (%d)", l.cfg.MaxIterations)
}

// TurnResult holds the complete output of a turn.
type TurnResult struct {
	TurnStartResult

	// FinalText is the text content from the final (text-only) assistant response.
	FinalText string `json:"final_text,omitempty"`

	// IterationCount is the number of LLM roundtrips in this turn.
	IterationCount int `json:"iteration_count"`

	// TotalUsage is the aggregate token usage across all iterations.
	TotalUsage provider.Usage `json:"total_usage"`

	// Duration is the wall-clock time for the entire turn.
	Duration time.Duration `json:"duration,omitempty"`
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

func validateRunTurnRequest(req RunTurnRequest) error {
	if strings.TrimSpace(req.ConversationID) == "" {
		return fmt.Errorf("agent loop: conversation ID is required")
	}
	if req.TurnNumber <= 0 {
		return fmt.Errorf("agent loop: turn number must be positive")
	}
	if strings.TrimSpace(req.Message) == "" {
		return fmt.Errorf("agent loop: message is required")
	}
	if req.ModelContextLimit <= 0 {
		return fmt.Errorf("agent loop: model context limit must be positive")
	}
	return nil
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
	if cfg.BasePrompt == "" {
		cfg.BasePrompt = "You are a helpful AI assistant."
	}
	return cfg
}

// toolNameFromResults looks up the tool name for a tool_use ID from the tool calls list.
func toolNameFromResults(toolCalls []provider.ToolCall, toolUseID string) string {
	for _, tc := range toolCalls {
		if tc.ID == toolUseID {
			return tc.Name
		}
	}
	return ""
}
