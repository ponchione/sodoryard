package conversation

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/ponchione/sirtopham/internal/provider"
)

const titleSystemPrompt = `Generate a short, descriptive title (5-8 words) for a conversation based on its opening exchange. Prefer the actual subject matter or result over tool mechanics, file-access wording, or failure phrasing. Return only the title text, no quotes or formatting.`

const titleMaxTokens = 50

// TitleProvider is the narrow interface needed by title generation — a
// non-streaming LLM call. The provider.Router satisfies this via its
// Complete method, or any single-provider can be used directly.
type TitleProvider interface {
	Complete(ctx context.Context, req *provider.Request) (*provider.Response, error)
}

// TitleGen generates conversation titles via a lightweight LLM call.
// It satisfies the agent.TitleGenerator interface:
//
//	GenerateTitle(ctx context.Context, conversationID string)
//
// The generator is fire-and-forget: errors are logged but never propagated.
type TitleGen struct {
	manager  *Manager
	provider TitleProvider
	logger   *slog.Logger
	model    string
}

// NewTitleGen constructs a title generator. The model parameter selects which
// model to use for title generation (typically a fast, cheap model).
func NewTitleGen(manager *Manager, provider TitleProvider, model string, logger *slog.Logger) *TitleGen {
	if logger == nil {
		logger = slog.Default()
	}
	return &TitleGen{
		manager:  manager,
		provider: provider,
		logger:   logger,
		model:    model,
	}
}

// GenerateTitle implements agent.TitleGenerator. It makes a non-streaming LLM
// call to generate a title from the first user message in the conversation,
// then persists it via SetTitle. Errors are logged, never propagated.
func (g *TitleGen) GenerateTitle(ctx context.Context, conversationID string) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Reconstruct history to find the first user message.
	messages, err := g.manager.ReconstructHistory(ctx, conversationID)
	if err != nil {
		g.logger.Warn("title generation: failed to reconstruct history",
			"conversation_id", conversationID,
			"error", err,
		)
		return
	}

	// Find the first user message and first assistant text.
	var firstMessage string
	assistantFallback := ""
	for _, msg := range messages {
		if firstMessage == "" && msg.Role == "user" && msg.Content.Valid {
			firstMessage = msg.Content.String
		}
		if assistantFallback == "" && msg.Role == "assistant" && msg.Content.Valid {
			assistantFallback = assistantTitleCandidate(msg.Content.String)
		}
		if firstMessage != "" && assistantFallback != "" {
			break
		}
	}
	if firstMessage == "" {
		g.logger.Warn("title generation: no user message found",
			"conversation_id", conversationID,
		)
		return
	}

	promptInput := buildTitlePromptInput(firstMessage, assistantFallback)

	// Make a lightweight LLM call.
	req := &provider.Request{
		SystemBlocks: []provider.SystemBlock{
			{Text: titleSystemPrompt},
		},
		Messages: []provider.Message{
			provider.NewUserMessage(promptInput),
		},
		Model:          g.model,
		MaxTokens:      titleMaxTokens,
		Purpose:        "title_generation",
		ConversationID: conversationID,
	}

	resp, err := g.provider.Complete(ctx, req)
	if err != nil {
		g.logger.Warn("title generation: LLM call failed",
			"conversation_id", conversationID,
			"error", err,
		)
		return
	}

	title := cleanTitle(extractText(resp))
	if looksLikeMisleadingAccessTitle(title) && assistantFallback != "" {
		title = assistantFallback
	}
	if title == "" {
		g.logger.Warn("title generation: empty title returned",
			"conversation_id", conversationID,
		)
		return
	}

	if err := g.manager.SetTitle(ctx, conversationID, title); err != nil {
		g.logger.Warn("title generation: failed to persist title",
			"conversation_id", conversationID,
			"title", title,
			"error", err,
		)
		return
	}

	g.logger.Info("title generated",
		"conversation_id", conversationID,
		"title", title,
	)
}

// cleanTitle trims whitespace and surrounding quotes from a generated title.
func cleanTitle(raw string) string {
	title := strings.TrimSpace(raw)
	// Strip surrounding quotes (models often wrap titles in them).
	for _, q := range []string{`"`, `'`, "`"} {
		title = strings.TrimPrefix(title, q)
		title = strings.TrimSuffix(title, q)
	}
	title = strings.TrimSpace(title)
	if looksLikeTranscriptTombstone(title) {
		return ""
	}

	// Truncate overly long titles.
	if len(title) > 100 {
		title = title[:100]
		if lastSpace := strings.LastIndex(title, " "); lastSpace > 50 {
			title = title[:lastSpace]
		}
	}
	if looksLikeTranscriptTombstone(title) {
		return ""
	}
	return title
}

func looksLikeTranscriptTombstone(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	return strings.Contains(trimmed, "[interrupted_assistant]") ||
		strings.Contains(trimmed, "[failed_assistant]") ||
		strings.Contains(trimmed, "[interrupted_tool_result]")
}

func buildTitlePromptInput(firstUserMessage, firstAssistantText string) string {
	var sb strings.Builder
	sb.WriteString("First user message:\n")
	sb.WriteString(strings.TrimSpace(firstUserMessage))
	if firstAssistantText != "" {
		sb.WriteString("\n\nFirst assistant reply:\n")
		sb.WriteString(firstAssistantText)
	}
	return sb.String()
}

func assistantTitleCandidate(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	var blocks []provider.ContentBlock
	if err := json.Unmarshal([]byte(trimmed), &blocks); err != nil {
		return ""
	}
	for _, block := range blocks {
		if block.Type != "text" {
			continue
		}
		candidate := cleanTitle(firstMeaningfulLine(block.Text))
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func firstMeaningfulLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimLeft(trimmed, "#*- ")
		trimmed = strings.Trim(trimmed, "`\"'")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func looksLikeMisleadingAccessTitle(title string) bool {
	normalized := strings.ToLower(strings.TrimSpace(title))
	if normalized == "" {
		return false
	}
	for _, prefix := range []string{
		"unable to access",
		"cannot access",
		"can't access",
		"failed to access",
		"error accessing",
		"file not found",
		"missing file",
		"permission denied",
	} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

// extractText concatenates all text content blocks from a response.
func extractText(resp *provider.Response) string {
	if resp == nil {
		return ""
	}
	var sb strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	return sb.String()
}
