package agent

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ponchione/sirtopham/internal/config"
	contextpkg "github.com/ponchione/sirtopham/internal/context"
	"github.com/ponchione/sirtopham/internal/conversation"
	"github.com/ponchione/sirtopham/internal/db"
	"github.com/ponchione/sirtopham/internal/provider"
	toolpkg "github.com/ponchione/sirtopham/internal/tool"
)

// ErrTurnCancelled is returned by RunTurn when the turn is cancelled via
// Cancel() or context cancellation.
var ErrTurnCancelled = errors.New("agent loop: turn cancelled")

const (
	defaultMaxIterations                 = 50
	defaultLoopDetectThreshold           = 3
	defaultMaxToolResultsPerMessageChars = 200_000

	// loopNudgeMessage is injected as a user message when the loop detector
	// identifies repeated identical tool calls across consecutive iterations.
	loopNudgeMessage = "You appear to be repeating the same action. Please try a different approach or explain what you're trying to accomplish."

	// loopDirectiveMessage is injected as a user message on the final
	// iteration (when tools are disabled) to guide the LLM toward a summary.
	loopDirectiveMessage = "You have reached the maximum number of tool calls for this turn. Please provide a text summary of your progress and any remaining work."
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
	MaxIterations                 int                  `json:"max_iterations,omitempty"`
	LoopDetectionThreshold        int                  `json:"loop_detection_threshold,omitempty"`
	ExtendedThinking              bool                 `json:"extended_thinking,omitempty"`
	BasePrompt                    string               `json:"base_prompt,omitempty"`
	ProviderName                  string               `json:"provider_name,omitempty"`
	ModelName                     string               `json:"model_name,omitempty"`
	EmitContextDebug              bool                 `json:"emit_context_debug,omitempty"`
	ContextConfig                 config.ContextConfig `json:"context_config,omitempty"`
	MaxToolResultsPerMessageChars int                  `json:"max_tool_results_per_message_chars,omitempty"`
	ToolResultStoreRoot           string               `json:"tool_result_store_root,omitempty"`

	// Prompt-cache controls for providers that support explicit cache markers
	// (currently Anthropic only).
	CacheSystemPrompt        bool `json:"cache_system_prompt,omitempty"`
	CacheAssembledContext    bool `json:"cache_assembled_context,omitempty"`
	CacheConversationHistory bool `json:"cache_conversation_history,omitempty"`

	// Phase 2 history compression (spec 11).
	CompressHistoricalResults  bool `json:"compress_historical_results,omitempty"`
	HistorySummarizeAfterTurns int  `json:"history_summarize_after_turns,omitempty"`
}

// AgentLoopDeps carries the dependencies needed by the agent loop.
type AgentLoopDeps struct {
	ContextAssembler    ContextAssembler
	ConversationManager ConversationManager
	ProviderRouter      ProviderRouter
	ToolExecutor        ToolExecutor
	ToolDefinitions     []provider.ToolDefinition
	PromptBuilder       *PromptBuilder
	EventSink           EventSink
	CompressionEngine   CompressionEngine
	ToolResultStore     ToolResultStore
	TitleGenerator      TitleGenerator
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

	// Model overrides the default model for this turn. If empty, the router's
	// default model is used. Populated by the WebSocket model_override event.
	Model string `json:"model,omitempty"`

	// Provider overrides the default provider for this turn. If empty, the
	// router's default provider is used. Populated by the WebSocket
	// model_override event.
	Provider string `json:"provider,omitempty"`
}

// TurnStartResult holds the frozen per-turn context package plus the history
// used to build it.
type TurnStartResult struct {
	History           []db.Message                   `json:"history,omitempty"`
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
	toolDefinitions     []provider.ToolDefinition
	promptBuilder       *PromptBuilder
	events              *MultiSink
	compressionEngine   CompressionEngine
	toolResultStore     ToolResultStore
	toolOutputManager   *ToolOutputManager
	titleGenerator      TitleGenerator
	cfg                 AgentLoopConfig
	contextCfg          config.ContextConfig
	logger              *slog.Logger
	now                 func() time.Time
	sleepFn             func(ctx stdctx.Context, d time.Duration) error

	// cancelMu protects cancellation state for thread-safe Cancel() calls.
	cancelMu sync.Mutex
	// cancelFn cancels the in-flight turn's derived context. Nil when idle.
	cancelFn stdctx.CancelFunc
	// interruptRequested distinguishes loop.Cancel() from external context cancellation.
	interruptRequested bool
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
	loop := &AgentLoop{
		contextAssembler:    deps.ContextAssembler,
		conversationManager: deps.ConversationManager,
		providerRouter:      deps.ProviderRouter,
		toolExecutor:        deps.ToolExecutor,
		toolDefinitions:     deps.ToolDefinitions,
		promptBuilder:       deps.PromptBuilder,
		events:              events,
		compressionEngine:   deps.CompressionEngine,
		toolResultStore:     deps.ToolResultStore,
		titleGenerator:      deps.TitleGenerator,
		cfg:                 withDefaultConfig(deps.Config),
		contextCfg:          deps.Config.ContextConfig,
		logger:              logger,
		now:                 time.Now,
	}
	if loop.toolResultStore == nil {
		loop.toolResultStore = NewFileToolResultStore(loop.cfg.ToolResultStoreRoot)
	}
	loop.toolOutputManager = NewToolOutputManager(loop.toolResultStore)
	return loop
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

// Cancel triggers cancellation of the in-progress turn. It is safe to call
// from any goroutine and is idempotent — calling Cancel() multiple times or
// when no turn is running is a no-op.
func (l *AgentLoop) Cancel() {
	if l == nil {
		return
	}
	l.cancelMu.Lock()
	defer l.cancelMu.Unlock()
	if l.cancelFn != nil {
		l.interruptRequested = true
		l.cancelFn()
	}
}

// setCancel stores the cancel function for the in-flight turn.
func (l *AgentLoop) setCancel(fn stdctx.CancelFunc) {
	l.cancelMu.Lock()
	defer l.cancelMu.Unlock()
	l.cancelFn = fn
	l.interruptRequested = false
}

// clearCancel clears the stored cancel function (turn is done).
func (l *AgentLoop) clearCancel() {
	l.cancelMu.Lock()
	defer l.cancelMu.Unlock()
	l.cancelFn = nil
	l.interruptRequested = false
}

func (l *AgentLoop) cancellationReason(cause error) turnCleanupReason {
	if errors.Is(cause, stdctx.DeadlineExceeded) {
		return cleanupReasonDeadlineExceeded
	}

	l.cancelMu.Lock()
	interruptRequested := l.interruptRequested
	l.cancelMu.Unlock()
	if interruptRequested {
		return cleanupReasonInterrupt
	}
	return cleanupReasonCancel
}

// RunTurn executes a full turn: persist user message → context assembly →
// iteration loop until text-only response or max iterations reached.
//
// The method is blocking — it returns only when the turn is complete,
// cancelled, or fails. Events stream via EventSink throughout.
//
// Cancellation is supported via the ctx parameter and the Cancel() method.
// On cancellation, completed iterations are preserved. The in-flight
// iteration is then either discarded via CancelIteration (when no assistant/tool
// state was materialized) or persisted as an interrupted iteration tombstone
// (for example assistant partial text or assistant tool_use plus interrupted
// tool results). RunTurn returns ErrTurnCancelled (wrapping the underlying
// context error).
func (l *AgentLoop) RunTurn(ctx stdctx.Context, req RunTurnRequest) (*TurnResult, error) {
	if ctx == nil {
		ctx = stdctx.Background()
	}
	if err := l.validate(); err != nil {
		return nil, err
	}
	if l.providerRouter == nil {
		return nil, fmt.Errorf("agent loop: provider router is nil")
	}
	if l.toolExecutor == nil {
		return nil, fmt.Errorf("agent loop: tool executor is nil")
	}
	if err := validateRunTurnRequest(req); err != nil {
		return nil, err
	}

	// Derive a cancelable context so Cancel() can stop this turn.
	ctx, cancel := stdctx.WithCancel(ctx)
	defer cancel()
	l.setCancel(cancel)
	defer l.clearCancel()

	turnStart := l.now()

	// Step 1: Persist user message.
	if err := l.conversationManager.PersistUserMessage(ctx, req.ConversationID, req.TurnNumber, req.Message); err != nil {
		if isCancelled(ctx) {
			return nil, l.handleCancellation(req.ConversationID, req.TurnNumber, 0, 0, ctx.Err())
		}
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
		if isCancelled(ctx) {
			return nil, l.handleCancellation(req.ConversationID, req.TurnNumber, 0, 0, ctx.Err())
		}
		return nil, err
	}

	// Step 3: Iteration loop.
	//
	// Resolve per-turn model/provider overrides. If the request carries an
	// override (from the WebSocket model_override event), use it; otherwise
	// use the config defaults.
	effectiveModel := l.cfg.ModelName
	if req.Model != "" {
		effectiveModel = req.Model
	}
	effectiveProvider := l.cfg.ProviderName
	if req.Provider != "" {
		effectiveProvider = req.Provider
	}

	iteration := 1
	completedIterations := 0
	var totalUsage provider.Usage
	var currentTurnMessages []provider.Message
	var allToolCalls []completedToolCall
	detector := newLoopDetector(l.cfg.LoopDetectionThreshold)

	// Add the user message as the first current-turn message.
	currentTurnMessages = append(currentTurnMessages, provider.NewUserMessage(req.Message))

	for l.cfg.MaxIterations == 0 || iteration <= l.cfg.MaxIterations {
		l.logger.Info("starting iteration",
			"conversation_id", req.ConversationID,
			"turn", req.TurnNumber,
			"iteration", iteration,
		)

		// Check for cancellation before each iteration.
		if isCancelled(ctx) {
			return nil, l.handleIterationSetupCancellation(req.ConversationID, req.TurnNumber, iteration, completedIterations, ctx.Err())
		}

		// 3a: Reconstruct history (includes persisted messages from prior iterations).
		history, err := l.conversationManager.ReconstructHistory(ctx, req.ConversationID)
		if err != nil {
			if isCancelled(ctx) {
				return nil, l.handleIterationSetupCancellation(req.ConversationID, req.TurnNumber, iteration, completedIterations, ctx.Err())
			}
			return nil, fmt.Errorf("agent loop: reconstruct history for iteration %d: %w", iteration, err)
		}

		// Determine if this is the last allowed iteration — disable tools to force text.
		disableTools := l.cfg.MaxIterations > 0 && iteration >= l.cfg.MaxIterations

		// Inject directive on final iteration to guide the LLM toward a summary.
		if disableTools {
			currentTurnMessages = append(currentTurnMessages, provider.NewUserMessage(
				loopDirectiveMessage,
			))
			l.logger.Warn("final iteration reached, disabling tools",
				"conversation_id", req.ConversationID,
				"turn", req.TurnNumber,
				"iteration", iteration,
				"max_iterations", l.cfg.MaxIterations,
			)
		}

		// 3b: Build prompt.
		promptReq, err := l.promptBuilder.BuildPrompt(l.buildPromptConfig(turnCtx.ContextPackage, history, currentTurnMessages, effectiveProvider, effectiveModel, req.ModelContextLimit, disableTools, req.ConversationID, req.TurnNumber, iteration))
		if err != nil {
			return nil, fmt.Errorf("agent loop: build prompt for iteration %d: %w", iteration, err)
		}

		// 3b.1: Preflight compression check — rough estimate before the LLM call.
		if compressed := l.tryPreflightCompression(ctx, req.ConversationID, promptReq, req.ModelContextLimit); compressed {
			// Rebuild prompt with compressed history.
			history, err = l.conversationManager.ReconstructHistory(ctx, req.ConversationID)
			if err != nil {
				return nil, fmt.Errorf("agent loop: reconstruct history after compression in iteration %d: %w", iteration, err)
			}
			promptReq, err = l.promptBuilder.BuildPrompt(l.buildPromptConfig(turnCtx.ContextPackage, history, currentTurnMessages, effectiveProvider, effectiveModel, req.ModelContextLimit, disableTools, req.ConversationID, req.TurnNumber, iteration))
			if err != nil {
				return nil, fmt.Errorf("agent loop: rebuild prompt after compression in iteration %d: %w", iteration, err)
			}
		}

		// 3c: Stream LLM request with retry for retriable errors.
		l.emit(StatusEvent{State: StateWaitingForLLM, Time: l.now()})

		result, err := l.streamWithRetry(ctx, promptReq, iteration, req.ConversationID)
		if err != nil {
			if isCancelled(ctx) {
				if result != nil {
					assistantContentJSON := ""
					if len(result.ContentBlocks) > 0 {
						assistantContentJSON, _ = contentBlocksToJSON(sanitizeContentBlocks(result.ContentBlocks))
					}
					return nil, l.handleTurnCancellation(inflightTurn{
						ConversationID:           req.ConversationID,
						TurnNumber:               req.TurnNumber,
						Iteration:                iteration,
						CompletedIterations:      completedIterations,
						AssistantResponseStarted: result.TextContent != "" || len(result.ContentBlocks) > 0,
						AssistantMessageContent:  assistantContentJSON,
					}, ctx.Err())
				}
				return nil, l.handleIterationSetupCancellation(req.ConversationID, req.TurnNumber, iteration, completedIterations, ctx.Err())
			}

			// Emergency compression: if the error is context overflow and we
			// have a compression engine, compress and retry once.
			if l.isContextOverflowError(err) {
				if retryResult, retryErr := l.tryEmergencyCompression(ctx, req, turnCtx, currentTurnMessages, iteration, disableTools); retryResult != nil || retryErr != nil {
					if retryErr != nil {
						return nil, retryErr
					}
					result = retryResult
					err = nil
				}
			}
			if err != nil {
				if result != nil && (result.TextContent != "" || len(result.ContentBlocks) > 0) {
					assistantContentJSON := ""
					if len(result.ContentBlocks) > 0 {
						assistantContentJSON, _ = contentBlocksToJSON(sanitizeContentBlocks(result.ContentBlocks))
					}
					return nil, l.handleTurnStreamFailure(inflightTurn{
						ConversationID:           req.ConversationID,
						TurnNumber:               req.TurnNumber,
						Iteration:                iteration,
						CompletedIterations:      completedIterations,
						AssistantResponseStarted: true,
						AssistantMessageContent:  assistantContentJSON,
					}, err)
				}
				return nil, err
			}
		}

		totalUsage = totalUsage.Add(result.Usage)

		// 3c.1: Post-response compression check — exact token count from the
		// API response. If triggered, compresses before the next iteration.
		l.tryPostResponseCompression(ctx, req.ConversationID, result.Usage.InputTokens, req.ModelContextLimit)

		// Sanitize content blocks (replace invalid tool_use JSON inputs) before serialization.
		sanitizedBlocks := sanitizeContentBlocks(result.ContentBlocks)

		// Serialize assistant content blocks for persistence.
		assistantContentJSON, err := contentBlocksToJSON(sanitizedBlocks)
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
				if isCancelled(ctx) {
					return nil, l.handleTurnCancellation(inflightTurn{
						ConversationID:           req.ConversationID,
						TurnNumber:               req.TurnNumber,
						Iteration:                iteration,
						CompletedIterations:      completedIterations,
						AssistantResponseStarted: true,
						AssistantMessageContent:  assistantContentJSON,
					}, ctx.Err())
				}
				return nil, fmt.Errorf("agent loop: persist final iteration %d: %w", iteration, err)
			}

			// Post-turn quality metrics.
			l.updatePostTurnQuality(ctx, req.ConversationID, req.TurnNumber, allToolCalls)

			// Title generation for first turn in a new conversation.
			l.maybeGenerateTitle(req.ConversationID, req.TurnNumber)

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
			l.emit(StatusEvent{State: StateIdle, Time: l.now()})

			return &TurnResult{
				TurnStartResult: *turnCtx,
				FinalText:       result.TextContent,
				IterationCount:  iteration,
				TotalUsage:      totalUsage,
				Duration:        turnDuration,
			}, nil
		}

		// 3e: Tool dispatch — execute each tool call with error recovery.
		// Layer 1: tool errors → feed back to LLM as tool result.
		// Malformed tool calls → synthetic error result with correction guidance.
		l.emit(StatusEvent{State: StateExecutingTools, Time: l.now()})

		var toolResults []provider.ToolResult
		inflight := inflightTurn{
			ConversationID:           req.ConversationID,
			TurnNumber:               req.TurnNumber,
			Iteration:                iteration,
			CompletedIterations:      completedIterations,
			AssistantResponseStarted: true,
			AssistantMessageContent:  assistantContentJSON,
			ToolCalls:                make([]inflightToolCall, len(result.ToolCalls)),
		}
		for i, tc := range result.ToolCalls {
			inflight.ToolCalls[i] = inflightToolCall{ToolCallID: tc.ID, ToolName: tc.Name}
		}
		toolsCancelled := false
		for idx, tc := range result.ToolCalls {
			// Check for cancellation before each tool call.
			if isCancelled(ctx) {
				toolsCancelled = true
				break
			}

			// Track tool calls for post-turn quality analysis.
			allToolCalls = append(allToolCalls, completedToolCall{
				ToolName:  tc.Name,
				Arguments: tc.Input,
			})

			// Validate tool call JSON before execution.
			validation := validateToolCallJSON(tc)
			if !validation.Valid {
				// Malformed tool call — do not execute, feed error back to LLM.
				l.logger.Warn("malformed tool call",
					"conversation_id", req.ConversationID,
					"turn", req.TurnNumber,
					"iteration", iteration,
					"tool_name", tc.Name,
					"tool_call_id", tc.ID,
					"error", validation.ErrorMessage,
				)
				l.emit(ErrorEvent{
					ErrorCode:   ErrorCodeMalformedToolCall,
					Message:     validation.ErrorMessage,
					Recoverable: true,
					Time:        l.now(),
				})
				toolResults = append(toolResults, provider.ToolResult{
					ToolUseID: tc.ID,
					Content:   validation.ErrorMessage,
					IsError:   true,
				})
				l.emit(ToolCallEndEvent{
					ToolCallID: tc.ID,
					Result:     validation.ErrorMessage,
					Duration:   0,
					Success:    false,
					Time:       l.now(),
				})
				continue
			}

			l.emit(ToolCallStartEvent{
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				Arguments:  tc.Input,
				Time:       l.now(),
			})
			inflight.ToolCalls[idx].Started = true

			toolStart := l.now()
			execCtx := toolpkg.ContextWithExecutionMeta(ctx, toolpkg.ExecutionMeta{
				ConversationID: req.ConversationID,
				TurnNumber:     req.TurnNumber,
				Iteration:      iteration,
			})
			toolResult, toolErr := l.toolExecutor.Execute(execCtx, tc)
			toolDuration := l.now().Sub(toolStart)

			// Check if tool execution was cancelled.
			if isCancelled(ctx) {
				toolsCancelled = true
				break
			}

			if toolErr != nil {
				// Layer 1: tool execution failed — enrich the error message
				// and feed it back to the LLM as a tool result so it can
				// self-correct. Tool errors are NOT turn-ending.
				enrichedMsg := enrichToolError(tc.Name, toolErr)
				toolResult = &provider.ToolResult{
					ToolUseID: tc.ID,
					Content:   enrichedMsg,
					IsError:   true,
				}
				l.emit(ErrorEvent{
					ErrorCode:   ErrorCodeToolExecution,
					Message:     enrichedMsg,
					Recoverable: true,
					Time:        l.now(),
				})
			}

			toolResults = append(toolResults, *toolResult)
			inflight.ToolCalls[idx].Completed = true
			inflight.ToolCalls[idx].ResultStored = true
			inflight.ToolMessages = append(inflight.ToolMessages, conversation.IterationMessage{
				Role:      "tool",
				Content:   toolResult.Content,
				ToolUseID: tc.ID,
				ToolName:  tc.Name,
			})

			l.emit(ToolCallOutputEvent{
				ToolCallID: tc.ID,
				Output:     toolResult.Content,
				Time:       l.now(),
			})
			l.emit(ToolCallEndEvent{
				ToolCallID: tc.ID,
				Result:     toolResult.Content,
				Duration:   toolDuration,
				Success:    !toolResult.IsError,
				Time:       l.now(),
			})
		}

		if toolsCancelled {
			return nil, l.handleTurnCancellation(inflight, ctx.Err())
		}

		managedToolResults := l.toolOutputManager.ApplyAggregateBudget(ctx, toolResults, result.ToolCalls, l.cfg.MaxToolResultsPerMessageChars)
		toolResults = managedToolResults.Results
		budgetReport := managedToolResults.Report
		if budgetReport.ReplacedResults > 0 {
			l.logger.Debug("aggregate tool-result budget applied",
				"conversation_id", req.ConversationID,
				"turn", req.TurnNumber,
				"iteration", iteration,
				"replaced_results", budgetReport.ReplacedResults,
				"persisted_results", budgetReport.PersistedResults,
				"inline_shrunk_results", budgetReport.InlineShrunkResults,
				"chars_saved", budgetReport.CharsSaved,
				"original_chars", budgetReport.OriginalChars,
				"final_chars", budgetReport.FinalChars,
				"max_chars", budgetReport.MaxChars,
			)
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
			if isCancelled(ctx) {
				return nil, l.handleCancellation(req.ConversationID, req.TurnNumber, iteration, completedIterations, ctx.Err())
			}
			return nil, fmt.Errorf("agent loop: persist iteration %d: %w", iteration, err)
		}

		completedIterations = iteration

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

		// 3f: Loop detection — check if the LLM is stuck repeating the same tool calls.
		detector.record(result.ToolCalls)
		if detector.isLooping() {
			l.logger.Warn("loop detected — injecting nudge",
				"conversation_id", req.ConversationID,
				"turn", req.TurnNumber,
				"iteration", iteration,
				"threshold", l.cfg.LoopDetectionThreshold,
			)
			currentTurnMessages = append(currentTurnMessages, provider.NewUserMessage(
				loopNudgeMessage,
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

// handleCancellation performs post-cancellation cleanup for any in-flight
// assistant/tool state, emits TurnCancelledEvent and StatusEvent(StateIdle),
// and returns ErrTurnCancelled wrapping the underlying cause.
func (l *AgentLoop) handleCancellation(conversationID string, turnNumber, currentIteration, completedIterations int, cause error) error {
	return l.handleTurnCancellation(inflightTurn{
		ConversationID:           conversationID,
		TurnNumber:               turnNumber,
		Iteration:                currentIteration,
		CompletedIterations:      completedIterations,
		AssistantResponseStarted: currentIteration > 0,
	}, cause)
}

func (l *AgentLoop) handleIterationSetupCancellation(conversationID string, turnNumber, currentIteration, completedIterations int, cause error) error {
	return l.handleTurnCancellation(inflightTurn{
		ConversationID:      conversationID,
		TurnNumber:          turnNumber,
		Iteration:           currentIteration,
		CompletedIterations: completedIterations,
	}, cause)
}

func (l *AgentLoop) handleTurnCancellation(turn inflightTurn, cause error) error {
	cleanupReason := l.cancellationReason(cause)
	return l.handleTurnCleanup(turn, cleanupReason, cause)
}

func (l *AgentLoop) handleTurnStreamFailure(turn inflightTurn, cause error) error {
	return l.handleTurnCleanup(turn, cleanupReasonStreamFailure, cause)
}

func (l *AgentLoop) handleTurnCleanup(turn inflightTurn, cleanupReason turnCleanupReason, cause error) error {
	plan := buildCleanupPlan(turn, cleanupReason)
	reason := cleanupReasonEventValue(plan.Reason)

	l.logger.Warn("turn cancelled",
		"conversation_id", turn.ConversationID,
		"turn", turn.TurnNumber,
		"current_iteration", turn.Iteration,
		"completed_iterations", turn.CompletedIterations,
		"reason", reason,
		"cleanup_actions", len(plan.Actions),
	)

	if len(plan.Actions) > 0 {
		cleanupCtx := stdctx.Background()
		if err := l.applyCleanupPlan(cleanupCtx, turn, plan); err != nil {
			l.logger.Error("failed to apply cancellation cleanup plan",
				"conversation_id", turn.ConversationID,
				"turn", turn.TurnNumber,
				"iteration", turn.Iteration,
				"error", err,
			)
		}
	}

	l.emit(TurnCancelledEvent{
		TurnNumber:          turn.TurnNumber,
		CompletedIterations: turn.CompletedIterations,
		Reason:              reason,
		Time:                l.now(),
	})
	l.emit(StatusEvent{State: StateIdle, Time: l.now()})

	if cause != nil {
		return fmt.Errorf("%w: %v", ErrTurnCancelled, cause)
	}
	return ErrTurnCancelled
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

	if l.cfg.EmitContextDebug {
		var report *contextpkg.ContextAssemblyReport
		if pkg != nil {
			report = pkg.Report
		}
		l.emit(ContextDebugEvent{Report: report, Time: l.now()})
	}
	l.emit(StatusEvent{State: StateWaitingForLLM, Time: l.now()})

	return &TurnStartResult{
		History:           append([]db.Message(nil), history...),
		ContextPackage:    pkg,
		CompressionNeeded: compressionNeeded,
	}, nil
}

// tryPreflightCompression checks whether the rough char-count estimate of
// the built prompt exceeds the compression threshold. If so and a compression
// engine is available, it runs compression and returns true.
func (l *AgentLoop) tryPreflightCompression(ctx stdctx.Context, conversationID string, req *provider.Request, modelContextLimit int) bool {
	if l.compressionEngine == nil {
		return false
	}
	chars := estimateRequestChars(req)
	if !contextpkg.NeedsCompressionPreflight(chars, modelContextLimit, l.contextCfg) {
		return false
	}

	l.logger.Info("preflight compression triggered",
		"conversation_id", conversationID,
		"estimated_chars", chars,
		"model_context_limit", modelContextLimit,
	)
	l.emit(StatusEvent{State: StateCompressing, Time: l.now()})

	result, err := l.compressionEngine.Compress(ctx, conversationID, l.contextCfg)
	if err != nil {
		l.logger.Error("preflight compression failed",
			"conversation_id", conversationID,
			"error", err,
		)
		l.emit(ErrorEvent{
			ErrorCode:   "compression_failed",
			Message:     fmt.Sprintf("Preflight compression failed: %s", err),
			Recoverable: true,
			Time:        l.now(),
		})
		return false
	}

	l.logger.Info("preflight compression completed",
		"conversation_id", conversationID,
		"compressed", result.Compressed,
		"compressed_messages", result.CompressedMessages,
	)
	return result.Compressed
}

// tryPostResponseCompression checks whether the exact prompt token count from
// the API response exceeds the compression threshold. If so, compresses before
// the next iteration. This is fire-and-forget — compression failure here is
// logged but non-fatal.
func (l *AgentLoop) tryPostResponseCompression(ctx stdctx.Context, conversationID string, promptTokens int, modelContextLimit int) {
	if l.compressionEngine == nil {
		return
	}
	if !contextpkg.NeedsCompressionPostResponse(promptTokens, modelContextLimit, l.contextCfg) {
		return
	}

	l.logger.Info("post-response compression triggered",
		"conversation_id", conversationID,
		"prompt_tokens", promptTokens,
		"model_context_limit", modelContextLimit,
	)
	l.emit(StatusEvent{State: StateCompressing, Time: l.now()})

	result, err := l.compressionEngine.Compress(ctx, conversationID, l.contextCfg)
	if err != nil {
		l.logger.Error("post-response compression failed",
			"conversation_id", conversationID,
			"error", err,
		)
		l.emit(ErrorEvent{
			ErrorCode:   "compression_failed",
			Message:     fmt.Sprintf("Post-response compression failed: %s", err),
			Recoverable: true,
			Time:        l.now(),
		})
		return
	}

	l.logger.Info("post-response compression completed",
		"conversation_id", conversationID,
		"compressed", result.Compressed,
		"compressed_messages", result.CompressedMessages,
	)
}

// isContextOverflowError checks if an error from streamWithRetry is classified
// as context overflow (400 context_length_exceeded or similar).
func (l *AgentLoop) isContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	classification := classifyStreamError(err)
	return classification.Code == ErrorCodeContextOverflow
}

// tryEmergencyCompression runs compression after a context overflow error and
// retries the LLM call once. Returns the stream result if the retry succeeds,
// or the retry error if it fails, or (nil, nil) if no compression engine is
// available.
func (l *AgentLoop) tryEmergencyCompression(
	ctx stdctx.Context,
	req RunTurnRequest,
	turnCtx *TurnStartResult,
	currentTurnMessages []provider.Message,
	iteration int,
	disableTools bool,
) (*streamResult, error) {
	if l.compressionEngine == nil {
		return nil, nil
	}

	l.logger.Warn("emergency compression triggered by context overflow",
		"conversation_id", req.ConversationID,
		"turn", req.TurnNumber,
		"iteration", iteration,
	)
	l.emit(StatusEvent{State: StateCompressing, Time: l.now()})

	compResult, compErr := l.compressionEngine.Compress(ctx, req.ConversationID, l.contextCfg)
	if compErr != nil {
		l.emit(ErrorEvent{
			ErrorCode:   "compression_failed",
			Message:     fmt.Sprintf("Emergency compression failed: %s", compErr),
			Recoverable: false,
			Time:        l.now(),
		})
		return nil, fmt.Errorf("agent loop: emergency compression for iteration %d: %w", iteration, compErr)
	}

	if !compResult.Compressed {
		l.emit(ErrorEvent{
			ErrorCode:   ErrorCodeContextOverflow,
			Message:     "Emergency compression ran but could not reduce context. Context overflow is unrecoverable.",
			Recoverable: false,
			Time:        l.now(),
		})
		return nil, fmt.Errorf("agent loop: context overflow on iteration %d: compression did not reduce context", iteration)
	}

	l.logger.Info("emergency compression completed, retrying LLM call",
		"conversation_id", req.ConversationID,
		"compressed_messages", compResult.CompressedMessages,
	)

	// Rebuild prompt with compressed history.
	history, err := l.conversationManager.ReconstructHistory(ctx, req.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("agent loop: reconstruct history after emergency compression in iteration %d: %w", iteration, err)
	}

	// Resolve per-turn overrides for emergency compression path.
	emerModel := l.cfg.ModelName
	if req.Model != "" {
		emerModel = req.Model
	}
	emerProvider := l.cfg.ProviderName
	if req.Provider != "" {
		emerProvider = req.Provider
	}

	promptReq, err := l.promptBuilder.BuildPrompt(l.buildPromptConfig(turnCtx.ContextPackage, history, currentTurnMessages, emerProvider, emerModel, req.ModelContextLimit, disableTools, req.ConversationID, req.TurnNumber, iteration))
	if err != nil {
		return nil, fmt.Errorf("agent loop: rebuild prompt after emergency compression in iteration %d: %w", iteration, err)
	}

	l.emit(StatusEvent{State: StateWaitingForLLM, Time: l.now()})
	result, err := l.streamWithRetry(ctx, promptReq, iteration, req.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("agent loop: stream after emergency compression in iteration %d: %w", iteration, err)
	}

	return result, nil
}

func (l *AgentLoop) buildPromptConfig(contextPackage *contextpkg.FullContextPackage, history []db.Message, currentTurnMessages []provider.Message, providerName, modelName string, contextLimit int, disableTools bool, conversationID string, turnNumber, iteration int) PromptConfig {
	return PromptConfig{
		BasePrompt:                 l.cfg.BasePrompt,
		ContextPackage:             contextPackage,
		History:                    history,
		CurrentTurnMessages:        currentTurnMessages,
		ToolDefinitions:            l.toolDefinitions,
		ProviderName:               providerName,
		ModelName:                  modelName,
		ContextLimit:               contextLimit,
		DisableTools:               disableTools,
		Purpose:                    "chat",
		ConversationID:             conversationID,
		TurnNumber:                 turnNumber,
		Iteration:                  iteration,
		CompressHistoricalResults:  l.cfg.CompressHistoricalResults,
		HistorySummarizeAfterTurns: l.cfg.HistorySummarizeAfterTurns,
		ExtendedThinking:           l.cfg.ExtendedThinking,
		CacheSystemPrompt:          l.cfg.CacheSystemPrompt,
		CacheAssembledContext:      l.cfg.CacheAssembledContext,
		CacheConversationHistory:   l.cfg.CacheConversationHistory,
	}
}

// updatePostTurnQuality calls ContextAssembler.UpdateQuality with the tool
// call analysis from this turn. Errors are logged but non-fatal.
func (l *AgentLoop) updatePostTurnQuality(ctx stdctx.Context, conversationID string, turnNumber int, calls []completedToolCall) {
	if l.contextAssembler == nil {
		return
	}
	usedSearch, readFiles := analyzeToolCalls(calls)
	if err := l.contextAssembler.UpdateQuality(ctx, conversationID, turnNumber, usedSearch, readFiles); err != nil {
		l.logger.Error("failed to update post-turn quality metrics",
			"conversation_id", conversationID,
			"turn", turnNumber,
			"error", err,
		)
	}
}

// maybeGenerateTitle fires a non-blocking title generation goroutine for the
// first turn of a new conversation (turnNumber == 1).
func (l *AgentLoop) maybeGenerateTitle(conversationID string, turnNumber int) {
	if l.titleGenerator == nil || turnNumber != 1 {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				l.logger.Error("title generation panicked",
					"conversation_id", conversationID,
					"panic", r,
				)
			}
		}()
		ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 30*time.Second)
		defer cancel()
		l.titleGenerator.GenerateTitle(ctx, conversationID)
	}()
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
	if cfg.MaxIterations < 0 {
		cfg.MaxIterations = defaultMaxIterations
	}
	if cfg.LoopDetectionThreshold <= 0 {
		cfg.LoopDetectionThreshold = defaultLoopDetectThreshold
	}
	if cfg.BasePrompt == "" {
		cfg.BasePrompt = "You are a helpful AI assistant. Use file tools for project-root files. Use brain_read and brain_search for vault-relative brain notes like notes/...md or .brain/notes/...md; do not treat those as repo-root file_read or search_text paths."
	}
	if cfg.MaxToolResultsPerMessageChars <= 0 {
		cfg.MaxToolResultsPerMessageChars = defaultMaxToolResultsPerMessageChars
	}
	return cfg
}

// isCancelled checks if the context has been cancelled.
func isCancelled(ctx stdctx.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
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
