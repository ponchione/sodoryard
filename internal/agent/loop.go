package agent

import (
	stdctx "context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ponchione/sodoryard/internal/config"
	contextpkg "github.com/ponchione/sodoryard/internal/context"
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/provider"
)

// ErrTurnCancelled is returned by RunTurn when the turn is cancelled via
// Cancel() or context cancellation.
var ErrTurnCancelled = errors.New("agent loop: turn cancelled")

// ErrMaxIterationsExceeded is returned when a turn exhausts its iteration
// budget without reaching a terminal assistant response.
var ErrMaxIterationsExceeded = errors.New("agent loop: exceeded max iterations")

const (
	defaultMaxIterations                 = 50
	defaultLoopDetectThreshold           = 3
	defaultMaxToolResultsPerMessageChars = 200_000

	// loopNudgeMessage is injected as a user message when the loop detector
	// identifies repeated identical tool calls across consecutive iterations.
	loopNudgeMessage = "You appear to be repeating the same action. Please try a different approach or explain what you're trying to accomplish."

	// loopDirectiveMessage is injected as a user message on the final
	// iteration to guide the LLM toward completion instead of exploration.
	loopDirectiveMessage = "You have reached the maximum number of exploratory tool calls for this turn. Use any remaining completion tool only to write required brain documents or receipts; otherwise provide a final text summary of your progress and any remaining work."
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

// BatchToolExecutor is an optional extension for executors that can dispatch
// a whole provider batch while preserving result order.
type BatchToolExecutor interface {
	ExecuteBatch(ctx stdctx.Context, calls []provider.ToolCall) ([]provider.ToolResult, error)
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
	StripHistoricalLineNumbers bool `json:"strip_historical_line_numbers,omitempty"`
	ElideDuplicateReads        bool `json:"elide_duplicate_reads,omitempty"`
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

// RunTurn executes a full turn by preparing per-turn state once, then
// delegating each LLM/tool roundtrip to iteration helpers until a final result
// or terminal error is produced.
func (l *AgentLoop) RunTurn(ctx stdctx.Context, req RunTurnRequest) (*TurnResult, error) {
	if ctx == nil {
		ctx = stdctx.Background()
	}

	prepared, err := l.prepareRunTurn(ctx, req)
	if err != nil {
		return nil, err
	}
	defer prepared.cleanup()

	return l.runTurnIterations(prepared.ctx, prepared.exec)
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
func (l *AgentLoop) tryPostResponseCompression(ctx stdctx.Context, conversationID string, promptTokens int, modelContextLimit int) bool {
	if l.compressionEngine == nil {
		return false
	}
	if !contextpkg.NeedsCompressionPostResponse(promptTokens, modelContextLimit, l.contextCfg) {
		return false
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
		return false
	}

	l.logger.Info("post-response compression completed",
		"conversation_id", conversationID,
		"compressed", result.Compressed,
		"compressed_messages", result.CompressedMessages,
	)
	return result.Compressed
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
	turnExec *turnExecution,
	iteration int,
	toolDefinitions []provider.ToolDefinition,
	disableTools bool,
) (*streamResult, error) {
	if l.compressionEngine == nil {
		return nil, nil
	}

	l.logger.Warn("emergency compression triggered by context overflow",
		"conversation_id", turnExec.req.ConversationID,
		"turn", turnExec.req.TurnNumber,
		"iteration", iteration,
	)
	l.emit(StatusEvent{State: StateCompressing, Time: l.now()})

	compResult, compErr := l.compressionEngine.Compress(ctx, turnExec.req.ConversationID, l.contextCfg)
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
		"conversation_id", turnExec.req.ConversationID,
		"compressed_messages", compResult.CompressedMessages,
	)

	// Rebuild prompt with compressed history.
	history, err := l.refreshTurnHistory(ctx, turnExec)
	if err != nil {
		return nil, fmt.Errorf("agent loop: reconstruct history after emergency compression in iteration %d: %w", iteration, err)
	}

	// Resolve per-turn overrides for emergency compression path.
	emerModel := l.cfg.ModelName
	if turnExec.req.Model != "" {
		emerModel = turnExec.req.Model
	}
	emerProvider := l.cfg.ProviderName
	if turnExec.req.Provider != "" {
		emerProvider = turnExec.req.Provider
	}

	promptReq, err := l.promptBuilder.BuildPrompt(l.buildPromptConfig(turnExec.turnCtx.ContextPackage, history, turnExec.currentTurnMessages, toolDefinitions, emerProvider, emerModel, turnExec.req.ModelContextLimit, disableTools, turnExec.req.ConversationID, turnExec.req.TurnNumber, iteration))
	if err != nil {
		return nil, fmt.Errorf("agent loop: rebuild prompt after emergency compression in iteration %d: %w", iteration, err)
	}

	l.emit(StatusEvent{State: StateWaitingForLLM, Time: l.now()})
	result, err := l.streamWithRetry(ctx, promptReq, iteration, turnExec.req.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("agent loop: stream after emergency compression in iteration %d: %w", iteration, err)
	}

	return result, nil
}

func (l *AgentLoop) buildPromptConfig(contextPackage *contextpkg.FullContextPackage, history []db.Message, currentTurnMessages []provider.Message, toolDefinitions []provider.ToolDefinition, providerName, modelName string, contextLimit int, disableTools bool, conversationID string, turnNumber, iteration int) PromptConfig {
	return PromptConfig{
		BasePrompt:                 l.cfg.BasePrompt,
		ContextPackage:             contextPackage,
		History:                    history,
		CurrentTurnMessages:        currentTurnMessages,
		ToolDefinitions:            toolDefinitions,
		ProviderName:               providerName,
		ModelName:                  modelName,
		ContextLimit:               contextLimit,
		DisableTools:               disableTools,
		Purpose:                    "chat",
		ConversationID:             conversationID,
		TurnNumber:                 turnNumber,
		Iteration:                  iteration,
		CompressHistoricalResults:  l.cfg.CompressHistoricalResults,
		StripHistoricalLineNumbers: l.cfg.StripHistoricalLineNumbers,
		ElideDuplicateReads:        l.cfg.ElideDuplicateReads,
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
		cfg.BasePrompt = "You are a helpful AI assistant. Use file tools for project-root files. Use brain_read and brain_search for project-brain notes like notes/...md. Brain notes live in Shunter project memory, so do not use file_read or search_text to inspect them. If the user asks about project brain notes, prefer brain_read/brain_search first and only use repo file/search tools for project-root code and files outside the brain. Do not double-check project-brain answers with search_text or file_read once a brain tool already found the relevant note or content."
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
