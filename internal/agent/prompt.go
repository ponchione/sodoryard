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
func (b *PromptBuilder) buildMessages(config PromptConfig, wantCache bool) []provider.Message {
	historyLen := len(config.History)
	currentLen := len(config.CurrentTurnMessages)
	messages := make([]provider.Message, 0, historyLen+currentLen)

	// Block 3: history prefix.
	for _, dbMsg := range config.History {
		messages = append(messages, dbMessageToProviderMessage(dbMsg))
	}

	// Place cache marker on the last history message (block 3 breakpoint).
	// NOTE: provider.Message does not currently carry a CacheControl field.
	// The cache marker for Block 3 is left as a design placeholder. When the
	// Anthropic request translator consumes this Request, it should mark the
	// last history message for caching. For now we record the intent — the
	// actual per-message CacheControl field will be added to provider.Message
	// when the Anthropic request translator needs it. This avoids polluting
	// the universal Message type for providers that don't use it.
	_ = wantCache && historyLen > 0 // intent: mark messages[historyLen-1]

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
