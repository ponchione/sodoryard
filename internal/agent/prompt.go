package agent

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	contextpkg "github.com/ponchione/sirtopham/internal/context"
	"github.com/ponchione/sirtopham/internal/db"
	"github.com/ponchione/sirtopham/internal/provider"
	anthropicpkg "github.com/ponchione/sirtopham/internal/provider/anthropic"
	"github.com/ponchione/sirtopham/internal/tool"
)

const (
	// defaultMaxTokens is the output token budget passed to the provider.
	defaultMaxTokens = 16384

	// providerAnthropic is the provider name that gets explicit cache markers.
	providerAnthropic = "anthropic"
)

// PromptConfig carries all inputs for a single BuildPrompt call.
type PromptConfig struct {
	// BasePrompt is the thin system prompt (agent personality, guidelines).
	BasePrompt string

	// ContextPackage is the frozen Layer 3 output. May be nil if context
	// assembly produced nothing.
	ContextPackage *contextpkg.FullContextPackage

	// History is the reconstructed message array from the conversation manager,
	// ordered by sequence. These are completed turns only — not the current turn.
	History []db.Message

	// CurrentTurnMessages are the messages from the in-progress turn: the user
	// message plus any in-progress tool results from earlier iterations.
	CurrentTurnMessages []provider.Message

	// ToolDefinitions is the list of tools available to the model.
	ToolDefinitions []provider.ToolDefinition

	// ProviderName selects cache marker behavior: "anthropic" gets explicit
	// markers, all others get none.
	ProviderName string

	// ModelName is the model identifier passed through to the provider request.
	ModelName string

	// ContextLimit is the model's context window size in tokens.
	ContextLimit int

	// MaxTokens overrides the default output token budget. Zero means use the
	// default.
	MaxTokens int

	// DisableTools forces a text-only response by omitting tools from the
	// request. Used on the final iteration to prevent further tool calls.
	DisableTools bool

	// Purpose classifies this call: "chat", "compression", "title_generation".
	Purpose string

	// ConversationID is passed through for tracking/persistence metadata.
	ConversationID string

	// TurnNumber is passed through for tracking/persistence metadata.
	TurnNumber int

	// Iteration is passed through for tracking/persistence metadata.
	Iteration int

	// CompressHistoricalResults enables Phase 2 history compression.
	// When true, historical tool results from prior turns are compressed
	// to reduce token usage: line-number stripping, JSON re-minification,
	// duplicate result elision, and stale result summarization.
	CompressHistoricalResults bool

	// HistorySummarizeAfterTurns controls stale result summarization.
	// Tool results older than this many turns are replaced with a one-line
	// summary. Set to 0 to disable summarization (other transforms still apply).
	HistorySummarizeAfterTurns int

	// ExtendedThinking enables extended thinking for providers that support it
	// (currently Anthropic only).
	ExtendedThinking bool
}

// PromptBuilder constructs provider.Request objects from PromptConfig inputs.
// It is stateless — each BuildPrompt call produces a fresh request with no
// internal mutation.
type PromptBuilder struct {
	logger *slog.Logger
}

// NewPromptBuilder creates a PromptBuilder. The logger is optional.
func NewPromptBuilder(logger *slog.Logger) *PromptBuilder {
	if logger == nil {
		logger = slog.Default()
	}
	return &PromptBuilder{logger: logger}
}

// BuildPrompt assembles a provider.Request from the given config.
//
// The request layout follows the three-block prompt cache strategy:
//
//	Block 1 (system): base system prompt — identical across all sessions
//	Block 2 (system): assembled context  — frozen within a turn, changes per turn
//	Block 3 (history): conversation history prefix — grows monotonically
//	Fresh (uncached): current turn messages
//
// Cache markers (cache_control: ephemeral) are placed on the last element of
// each block for Anthropic; omitted for all other providers.
func (b *PromptBuilder) BuildPrompt(config PromptConfig) (*provider.Request, error) {
	if strings.TrimSpace(config.BasePrompt) == "" {
		return nil, fmt.Errorf("prompt builder: base prompt is required")
	}

	wantCache := strings.EqualFold(config.ProviderName, providerAnthropic)
	if config.ProviderName != "" && !wantCache {
		b.logger.Debug("prompt builder: no cache markers for provider", "provider", config.ProviderName)
	}

	// --- System blocks (Block 1 + optional Block 2) ---
	systemBlocks := b.buildSystemBlocks(config, wantCache)

	// --- Conversation messages (Block 3 + fresh) ---
	messages := b.buildMessages(config, wantCache)

	// --- Tools ---
	var tools []provider.ToolDefinition
	if !config.DisableTools {
		tools = config.ToolDefinitions
	}

	// --- MaxTokens ---
	maxTokens := config.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	req := &provider.Request{
		SystemBlocks:   systemBlocks,
		Messages:       messages,
		Tools:          tools,
		Model:          config.ModelName,
		MaxTokens:      maxTokens,
		Purpose:        config.Purpose,
		ConversationID: config.ConversationID,
		TurnNumber:     config.TurnNumber,
		Iteration:      config.Iteration,
	}

	// Wire extended thinking options for Anthropic provider.
	if config.ExtendedThinking && strings.EqualFold(config.ProviderName, providerAnthropic) {
		req.ProviderOptions = anthropicpkg.NewAnthropicOptions(true, 0)
	}

	return req, nil
}

// buildSystemBlocks constructs the system-level content blocks.
//
// Block 1: base prompt text. Cache marker on this block if no context follows.
// Block 2: assembled context text (if non-empty). Cache marker on this block.
func (b *PromptBuilder) buildSystemBlocks(config PromptConfig, wantCache bool) []provider.SystemBlock {
	hasContext := config.ContextPackage != nil && strings.TrimSpace(config.ContextPackage.Content) != ""

	blocks := make([]provider.SystemBlock, 0, 2)

	// Block 1: base system prompt.
	baseBlock := provider.SystemBlock{Text: config.BasePrompt}
	if wantCache && !hasContext {
		// No block 2 follows, so block 1 gets the first cache breakpoint.
		baseBlock.CacheControl = ephemeralCacheControl()
	}
	blocks = append(blocks, baseBlock)

	// Block 2: assembled context (optional).
	if hasContext {
		contextBlock := provider.SystemBlock{Text: config.ContextPackage.Content}
		if wantCache {
			contextBlock.CacheControl = ephemeralCacheControl()
		}
		blocks = append(blocks, contextBlock)
	}

	return blocks
}

// buildMessages constructs the conversation message array.
//
// History messages (Block 3) come first, with a cache marker on the last
// history message for Anthropic. Current turn messages follow with no markers.
//
// When CompressHistoricalResults is enabled, Phase 2 history compression is
// applied to tool results from prior turns before conversion to provider
// messages. This reduces token usage by stripping line numbers, minifying
// JSON, eliding duplicate file reads, and summarizing stale results.
func (b *PromptBuilder) buildMessages(config PromptConfig, wantCache bool) []provider.Message {
	history := config.History

	// Phase 2: Apply history compression if enabled.
	if config.CompressHistoricalResults && len(history) > 0 {
		history = b.compressHistory(history, config)
	}

	historyLen := len(history)
	currentLen := len(config.CurrentTurnMessages)
	messages := make([]provider.Message, 0, historyLen+currentLen)

	// Block 3: history prefix.
	for _, dbMsg := range history {
		messages = append(messages, dbMessageToProviderMessage(dbMsg))
	}

	// Place cache marker on the last history message (block 3 breakpoint).
	// For Anthropic, this enables prompt caching of the conversation prefix
	// so that only current-turn messages are reprocessed on each LLM call.
	if wantCache && historyLen > 0 {
		messages[historyLen-1].CacheControl = ephemeralCacheControl()
	}

	// Fresh content: current turn messages (no cache markers).
	messages = append(messages, config.CurrentTurnMessages...)

	return messages
}

// dbMessageToProviderMessage converts a db.Message (sqlc-generated) to a
// provider.Message suitable for the LLM request.
func dbMessageToProviderMessage(msg db.Message) provider.Message {
	pm := provider.Message{
		Role: provider.Role(msg.Role),
	}

	switch msg.Role {
	case "assistant":
		// Assistant content is already a JSON array of content blocks.
		if msg.Content.Valid {
			pm.Content = json.RawMessage(msg.Content.String)
		}
	case "user":
		// User content is plain text — wrap in a JSON string.
		if msg.Content.Valid {
			raw, _ := json.Marshal(msg.Content.String)
			pm.Content = raw
		}
	case "tool":
		// Tool content is plain text — wrap in a JSON string.
		if msg.Content.Valid {
			raw, _ := json.Marshal(msg.Content.String)
			pm.Content = raw
		}
		if msg.ToolUseID.Valid {
			pm.ToolUseID = msg.ToolUseID.String
		}
		if msg.ToolName.Valid {
			pm.ToolName = msg.ToolName.String
		}
	}

	return pm
}

// compressHistory applies Phase 2 history compression to db.Messages,
// converting to tool.HistoryMessage for the compressor and patching the
// content back into the original db.Message slice.
func (b *PromptBuilder) compressHistory(messages []db.Message, config PromptConfig) []db.Message {
	// Convert to tool.HistoryMessage for the compressor.
	histMsgs := make([]tool.HistoryMessage, len(messages))
	for i, msg := range messages {
		histMsgs[i] = tool.HistoryMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolName:   msg.ToolName,
			ToolUseID:  msg.ToolUseID,
			TurnNumber: msg.TurnNumber,
		}
	}

	compressor := &tool.HistoryCompressor{
		CurrentTurn:         int64(config.TurnNumber),
		SummarizeAfterTurns: config.HistorySummarizeAfterTurns,
	}

	compressed := compressor.CompressHistory(histMsgs)

	// Copy the original messages and patch in compressed content.
	result := make([]db.Message, len(messages))
	copy(result, messages)
	for i, hm := range compressed {
		result[i].Content = hm.Content
	}

	return result
}

// ephemeralCacheControl returns the Anthropic cache control marker.
func ephemeralCacheControl() *provider.CacheControl {
	return &provider.CacheControl{Type: "ephemeral"}
}

// nullStr is a convenience for creating sql.NullString values in tests and
// internal usage.
func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
