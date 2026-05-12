package operator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/conversation"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/provider"
)

const rawChatMaxTokens = 16384

func (s *Service) SendChatMessage(ctx context.Context, req ChatTurnRequest) (ChatTurnResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return ChatTurnResult{}, fmt.Errorf("chat message is required")
	}
	cfg, err := s.config()
	if err != nil {
		return ChatTurnResult{}, err
	}
	if s == nil || s.rt == nil || s.rt.ProviderRouter == nil || s.rt.ConversationManager == nil {
		return ChatTurnResult{}, fmt.Errorf("chat runtime is unavailable")
	}
	providerName := cfg.Routing.Default.Provider
	modelName := cfg.Routing.Default.Model
	convID := strings.TrimSpace(req.ConversationID)
	createdConversation := false
	if convID == "" {
		conv, err := s.rt.ConversationManager.Create(ctx, cfg.ProjectRoot,
			conversation.WithTitle(chatTitle(message)),
			conversation.WithProvider(providerName),
			conversation.WithModel(modelName),
		)
		if err != nil {
			return ChatTurnResult{}, fmt.Errorf("create chat conversation: %w", err)
		}
		convID = conv.ID
		createdConversation = true
	} else {
		if _, err := s.rt.ConversationManager.Get(ctx, convID); err != nil {
			return ChatTurnResult{}, fmt.Errorf("load chat conversation: %w", err)
		}
		if err := s.rt.ConversationManager.SetRuntimeDefaults(ctx, convID, &providerName, &modelName); err != nil {
			return ChatTurnResult{}, fmt.Errorf("update chat conversation runtime defaults: %w", err)
		}
	}

	turnNumber, err := s.rt.ConversationManager.NextTurnNumber(ctx, convID)
	if err != nil {
		cause := fmt.Errorf("compute chat turn number: %w", err)
		if createdConversation {
			return ChatTurnResult{}, s.cleanupFailedRawChatTurn(ctx, convID, 0, true, cause)
		}
		return ChatTurnResult{}, cause
	}
	if err := s.rt.ConversationManager.PersistUserMessage(ctx, convID, turnNumber, message); err != nil {
		cause := fmt.Errorf("persist chat user message: %w", err)
		if createdConversation {
			return ChatTurnResult{}, s.cleanupFailedRawChatTurn(ctx, convID, turnNumber, true, cause)
		}
		return ChatTurnResult{}, cause
	}
	history, err := s.rt.ConversationManager.ReconstructHistory(ctx, convID)
	if err != nil {
		return ChatTurnResult{}, s.cleanupFailedRawChatTurn(ctx, convID, turnNumber, createdConversation, fmt.Errorf("load chat history: %w", err))
	}
	resp, err := s.rt.ProviderRouter.Complete(ctx, &provider.Request{
		Messages:        chatProviderMessages(history),
		Purpose:         "chat",
		ConversationID:  convID,
		TurnNumber:      turnNumber,
		Iteration:       1,
		MaxTokens:       rawChatMaxTokens,
		SystemBlocks:    nil,
		Tools:           nil,
		ProviderOptions: nil,
	})
	if err != nil {
		return ChatTurnResult{}, s.cleanupFailedRawChatTurn(ctx, convID, turnNumber, createdConversation, fmt.Errorf("run raw chat completion: %w", err))
	}
	if resp == nil {
		return ChatTurnResult{}, s.cleanupFailedRawChatTurn(ctx, convID, turnNumber, createdConversation, fmt.Errorf("run raw chat completion: provider returned nil response"))
	}
	assistantContent, err := json.Marshal(resp.Content)
	if err != nil {
		return ChatTurnResult{}, s.cleanupFailedRawChatTurn(ctx, convID, turnNumber, createdConversation, fmt.Errorf("serialize chat assistant message: %w", err))
	}
	if len(resp.Content) == 0 {
		assistantContent, _ = json.Marshal([]provider.ContentBlock{provider.NewTextBlock("")})
	}
	if err := s.rt.ConversationManager.PersistIteration(ctx, convID, turnNumber, 1, []conversation.IterationMessage{{
		Role:    "assistant",
		Content: string(assistantContent),
	}}); err != nil {
		return ChatTurnResult{}, s.cleanupFailedRawChatTurn(ctx, convID, turnNumber, createdConversation, fmt.Errorf("persist chat assistant message: %w", err))
	}
	history, err = s.rt.ConversationManager.ReconstructHistory(ctx, convID)
	if err != nil {
		return ChatTurnResult{}, fmt.Errorf("reload chat history: %w", err)
	}
	model := modelName
	if strings.TrimSpace(resp.Model) != "" {
		model = resp.Model
	}
	return ChatTurnResult{
		ConversationID: convID,
		Provider:       providerName,
		Model:          model,
		Messages:       chatDisplayMessages(history),
		InputTokens:    resp.Usage.InputTokens,
		OutputTokens:   resp.Usage.OutputTokens,
		StopReason:     string(resp.StopReason),
	}, nil
}

func (s *Service) cleanupFailedRawChatTurn(ctx context.Context, conversationID string, turnNumber int, deleteConversation bool, cause error) error {
	cleanupCtx := context.WithoutCancel(ctx)
	cleanupCtx, cancel := context.WithTimeout(cleanupCtx, 5*time.Second)
	defer cancel()

	var cleanupErr error
	if deleteConversation {
		cleanupErr = s.rt.ConversationManager.Delete(cleanupCtx, conversationID)
	} else {
		cleanupErr = s.rt.ConversationManager.DiscardTurn(cleanupCtx, conversationID, turnNumber)
	}
	if cleanupErr != nil {
		return errors.Join(cause, cleanupErr)
	}
	return cause
}

func chatProviderMessages(messages []appdb.Message) []provider.Message {
	out := make([]provider.Message, 0, len(messages))
	for _, msg := range messages {
		pm := provider.Message{Role: provider.Role(msg.Role)}
		switch msg.Role {
		case "user":
			if msg.Content.Valid {
				pm.Content = provider.NewUserMessage(msg.Content.String).Content
			}
		case "assistant":
			if msg.Content.Valid {
				pm.Content = json.RawMessage(msg.Content.String)
			}
		case "tool":
			if msg.Content.Valid {
				pm.Content = provider.NewToolResultMessage("", "", msg.Content.String).Content
			}
			if msg.ToolUseID.Valid {
				pm.ToolUseID = msg.ToolUseID.String
			}
			if msg.ToolName.Valid {
				pm.ToolName = msg.ToolName.String
			}
		default:
			continue
		}
		out = append(out, pm)
	}
	return out
}

func chatDisplayMessages(messages []appdb.Message) []ChatMessage {
	out := make([]ChatMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		out = append(out, ChatMessage{
			Role:      msg.Role,
			Content:   chatDisplayContent(msg.Role, msg.Content),
			CreatedAt: msg.CreatedAt,
		})
	}
	return out
}

func chatDisplayContent(role string, content sql.NullString) string {
	if !content.Valid {
		return ""
	}
	if role != "assistant" {
		return content.String
	}
	blocks, err := provider.ContentBlocksFromRaw(json.RawMessage(content.String))
	if err != nil {
		return content.String
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, strings.TrimSpace(block.Text))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n")
}

func chatTitle(message string) string {
	title := trimOneLineForChat(message, 64)
	if title == "" {
		return "Raw chat"
	}
	return "Chat: " + title
}

func trimOneLineForChat(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 1 {
		return value[:limit]
	}
	return strings.TrimSpace(value[:limit-1]) + "..."
}
