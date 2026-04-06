package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/ponchione/sirtopham/internal/db"
	sid "github.com/ponchione/sirtopham/internal/id"
	"github.com/ponchione/sirtopham/internal/provider"
)

// Conversation is the application-level representation of a conversation row.
// It converts sql.NullString fields to Go-native pointer types.
type Conversation struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	Title     *string    `json:"title,omitempty"`
	Model     *string    `json:"model,omitempty"`
	Provider  *string    `json:"provider,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// ConversationSummary is a lightweight projection for list views.
type ConversationSummary struct {
	ID        string    `json:"id"`
	Title     *string   `json:"title,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateOptions carries optional fields for conversation creation.
type CreateOptions struct {
	Title    *string
	Model    *string
	Provider *string
}

// CreateOption is a functional option for Create.
type CreateOption func(*CreateOptions)

// WithTitle sets an initial title on the new conversation.
func WithTitle(title string) CreateOption {
	return func(o *CreateOptions) { o.Title = &title }
}

// WithModel sets the model on the new conversation.
func WithModel(model string) CreateOption {
	return func(o *CreateOptions) { o.Model = &model }
}

// WithProvider sets the provider on the new conversation.
func WithProvider(provider string) CreateOption {
	return func(o *CreateOptions) { o.Provider = &provider }
}

// Manager provides the full conversation lifecycle: CRUD operations plus
// the history management operations needed by the agent loop. It embeds
// HistoryManager to satisfy the agent.ConversationManager interface while
// adding conversation-level lifecycle methods.
//
// Manager lives in internal/conversation/ (not internal/agent/) so that the
// REST API layer can use it without importing the agent loop.
type Manager struct {
	*HistoryManager
	queries *db.Queries
	logger  *slog.Logger
	newID   func() string // injectable for testing
}

// NewManager constructs a Manager backed by the given database. The seen
// tracker is optional — nil creates a fresh session-scoped tracker.
func NewManager(database *sql.DB, seen *SeenFiles, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		HistoryManager: NewHistoryManager(database, seen),
		queries:        db.New(database),
		logger:         logger,
		newID:          sid.New,
	}
}

// Create inserts a new conversation with a UUIDv7 ID. Functional options
// allow setting an initial title, model, and provider.
func (m *Manager) Create(ctx context.Context, projectID string, opts ...CreateOption) (*Conversation, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	options := &CreateOptions{}
	for _, opt := range opts {
		opt(options)
	}

	id := m.newID()
	now := m.now().UTC()
	timestamp := now.Format(time.RFC3339)

	params := db.InsertConversationParams{
		ID:        id,
		ProjectID: projectID,
		CreatedAt: timestamp,
		UpdatedAt: timestamp,
	}
	if options.Title != nil {
		params.Title = sql.NullString{String: *options.Title, Valid: true}
	}
	if options.Model != nil {
		params.Model = sql.NullString{String: *options.Model, Valid: true}
	}
	if options.Provider != nil {
		params.Provider = sql.NullString{String: *options.Provider, Valid: true}
	}

	if err := m.queries.InsertConversation(ctx, params); err != nil {
		return nil, fmt.Errorf("conversation manager: create: %w", err)
	}

	return &Conversation{
		ID:        id,
		ProjectID: projectID,
		Title:     options.Title,
		Model:     options.Model,
		Provider:  options.Provider,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Get loads a single conversation by ID. Returns an error wrapping sql.ErrNoRows
// if not found.
func (m *Manager) Get(ctx context.Context, conversationID string) (*Conversation, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	row, err := m.queries.GetConversation(ctx, conversationID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("conversation manager: get %q: %w", conversationID, err)
		}
		return nil, fmt.Errorf("conversation manager: get: %w", err)
	}

	return dbConversationToConversation(row), nil
}

// List returns conversations for a project ordered by updated_at DESC.
func (m *Manager) List(ctx context.Context, projectID string, limit, offset int) ([]ConversationSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := m.queries.ListConversations(ctx, db.ListConversationsParams{
		ProjectID: projectID,
		Limit:     int64(limit),
		Offset:    int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("conversation manager: list: %w", err)
	}

	summaries := make([]ConversationSummary, 0, len(rows))
	for _, row := range rows {
		s := ConversationSummary{
			ID: row.ID,
		}
		if row.Title.Valid {
			t := row.Title.String
			s.Title = &t
		}
		if parsed, err := time.Parse(time.RFC3339, row.UpdatedAt); err == nil {
			s.UpdatedAt = parsed
		}
		summaries = append(summaries, s)
	}
	return summaries, nil
}

// Delete removes a conversation and all related records. SQLite foreign key
// CASCADE handles messages, tool_executions, and sub_calls.
func (m *Manager) Delete(ctx context.Context, conversationID string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if err := m.queries.DeleteConversation(ctx, conversationID); err != nil {
		return fmt.Errorf("conversation manager: delete %q: %w", conversationID, err)
	}
	return nil
}

// SetTitle updates the conversation's title and updated_at timestamp.
func (m *Manager) SetTitle(ctx context.Context, conversationID, title string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	timestamp := m.now().UTC().Format(time.RFC3339)
	if err := m.queries.SetConversationTitle(ctx, db.SetConversationTitleParams{
		Title:     sql.NullString{String: title, Valid: true},
		UpdatedAt: timestamp,
		ID:        conversationID,
	}); err != nil {
		return fmt.Errorf("conversation manager: set title: %w", err)
	}
	return nil
}

// Count returns the total number of conversations for a project.
func (m *Manager) Count(ctx context.Context, projectID string) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return m.queries.CountConversations(ctx, projectID)
}

// MessageView is a JSON-friendly representation of a message for the REST API.
// It includes compression flags for the frontend to render compressed messages
// differently (greyed out / collapsed).
type MessageView struct {
	ID           int64   `json:"id"`
	Role         string  `json:"role"`
	Content      *string `json:"content,omitempty"`
	ToolUseID    *string `json:"tool_use_id,omitempty"`
	ToolName     *string `json:"tool_name,omitempty"`
	TurnNumber   int64   `json:"turn_number"`
	Iteration    int64   `json:"iteration"`
	Sequence     float64 `json:"sequence"`
	IsCompressed bool    `json:"is_compressed"`
	IsSummary    bool    `json:"is_summary"`
	CreatedAt    string  `json:"created_at"`
}

// GetMessages returns all messages for a conversation including compressed ones.
func (m *Manager) GetMessages(ctx context.Context, conversationID string) ([]MessageView, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	rows, err := m.queries.ListAllMessages(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("conversation manager: get messages: %w", err)
	}

	messages := make([]MessageView, 0, len(rows))
	for _, row := range rows {
		mv := MessageView{
			ID:           row.ID,
			Role:         row.Role,
			TurnNumber:   row.TurnNumber,
			Iteration:    row.Iteration,
			Sequence:     row.Sequence,
			IsCompressed: row.IsCompressed != 0,
			IsSummary:    row.IsSummary != 0,
			CreatedAt:    row.CreatedAt,
		}
		if row.Content.Valid {
			mv.Content = &row.Content.String
		}
		if row.ToolUseID.Valid {
			mv.ToolUseID = &row.ToolUseID.String
		}
		if row.ToolName.Valid {
			mv.ToolName = &row.ToolName.String
		}
		messages = append(messages, mv)
	}
	return messages, nil
}

// SearchResult represents a conversation matching a search query.
type SearchResult struct {
	ID        string  `json:"id"`
	Title     *string `json:"title,omitempty"`
	UpdatedAt string  `json:"updated_at"`
	Snippet   string  `json:"snippet"`
}

var (
	assistantToolNamePattern = regexp.MustCompile(`"name":"([^"]+)"`)
)

// Search performs full-text search across conversation messages using FTS5.
func (m *Manager) Search(ctx context.Context, query string) ([]SearchResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	rows, err := m.queries.SearchConversations(ctx, query)
	if err != nil && shouldRetryLiteralSearch(err, query) {
		rows, err = m.queries.SearchConversations(ctx, buildLiteralFTSQuery(query))
	}
	if err != nil {
		return nil, fmt.Errorf("conversation manager: search: %w", err)
	}

	type scoredSearchResult struct {
		result SearchResult
		score  int
	}

	orderedIDs := make([]string, 0, len(rows))
	bestByConversationID := make(map[string]scoredSearchResult, len(rows))
	for _, row := range rows {
		sr := SearchResult{
			ID:        row.ID,
			UpdatedAt: row.UpdatedAt,
			Snippet:   sanitizeSearchSnippet(row.Snippet),
		}
		if row.Title.Valid {
			sr.Title = &row.Title.String
		}
		scored := scoredSearchResult{result: sr, score: searchSnippetQuality(row.Role, sr.Snippet)}
		current, exists := bestByConversationID[row.ID]
		if !exists {
			orderedIDs = append(orderedIDs, row.ID)
			bestByConversationID[row.ID] = scored
			continue
		}
		if scored.score > current.score {
			bestByConversationID[row.ID] = scored
		}
	}

	results := make([]SearchResult, 0, len(bestByConversationID))
	for _, id := range orderedIDs {
		results = append(results, bestByConversationID[id].result)
	}
	return results, nil
}

func searchSnippetQuality(role string, snippet string) int {
	trimmed := strings.TrimSpace(snippet)
	if trimmed == "" {
		return -100
	}

	score := 0
	switch role {
	case "assistant":
		score += 30
	case "user":
		score += 20
	case "tool":
		score += 10
	}

	if strings.HasPrefix(trimmed, "[assistant tool call:") {
		return score
	}
	if strings.HasPrefix(trimmed, "Found ") && strings.Contains(trimmed, "brain document") {
		return score + 1
	}
	if strings.HasPrefix(trimmed, "Wrote brain document:") || strings.HasPrefix(trimmed, "Brain document:") {
		return score + 1
	}
	if strings.HasPrefix(trimmed, "[") {
		return score + 2
	}
	if strings.Contains(trimmed, "Found ") && strings.Contains(trimmed, "brain document") {
		return score + 3
	}
	return score + 10
}

func shouldRetryLiteralSearch(err error, query string) bool {
	if err == nil {
		return false
	}
	if strings.TrimSpace(query) == "" {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no such column:") || strings.Contains(msg, "fts5: syntax error")
}

func buildLiteralFTSQuery(query string) string {
	parts := strings.Fields(query)
	if len(parts) == 0 {
		return `""`
	}
	for i, part := range parts {
		parts[i] = `"` + strings.ReplaceAll(part, `"`, `""`) + `"`
	}
	return strings.Join(parts, " ")
}

func sanitizeSearchSnippet(snippet string) string {
	trimmed := strings.TrimSpace(snippet)
	if trimmed == "" {
		return ""
	}
	unhighlighted := stripSearchSnippetHighlights(trimmed)
	if strings.Contains(unhighlighted, "[failed_assistant]") {
		return "[assistant stream failure tombstone]"
	}
	if strings.Contains(unhighlighted, "[interrupted_assistant]") {
		return "[assistant interrupted tombstone]"
	}
	if strings.Contains(unhighlighted, "[interrupted_tool_result]") {
		return "[interrupted tool result]"
	}

	if text, ok := sanitizeAssistantSnippetHeuristically(unhighlighted); ok {
		return stripSearchSnippetHighlights(text)
	}
	if !strings.HasPrefix(trimmed, "[") {
		return unhighlighted
	}

	blocks, err := provider.ContentBlocksFromRaw(json.RawMessage(unhighlighted))
	if err != nil {
		return unhighlighted
	}
	texts := make([]string, 0, len(blocks))
	toolNames := make([]string, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if strings.Contains(block.Text, "[failed_assistant]") {
				return "[assistant stream failure tombstone]"
			}
			if strings.Contains(block.Text, "[interrupted_assistant]") {
				return "[assistant interrupted tombstone]"
			}
			if text := strings.TrimSpace(stripSearchSnippetHighlights(block.Text)); text != "" {
				texts = append(texts, text)
			}
		case "tool_use":
			if name := strings.TrimSpace(stripSearchSnippetHighlights(block.Name)); name != "" {
				toolNames = append(toolNames, name)
			}
		}
	}
	if len(texts) > 0 {
		return strings.Join(texts, " ")
	}
	if len(toolNames) > 0 {
		return "[assistant tool call: " + toolNames[0] + "]"
	}
	return unhighlighted
}

func stripSearchSnippetHighlights(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "<b>", ""), "</b>", "")
}

func sanitizeAssistantSnippetHeuristically(trimmed string) (string, bool) {
	if text, ok := extractAssistantTextSnippet(trimmed); ok {
		return text, true
	}
	if strings.Contains(trimmed, `"type":"tool_use"`) {
		if matches := assistantToolNamePattern.FindStringSubmatch(trimmed); len(matches) == 2 {
			return "[assistant tool call: " + matches[1] + "]", true
		}
	}
	return "", false
}

func extractAssistantTextSnippet(trimmed string) (string, bool) {
	marker := `"text":"`
	idx := strings.Index(trimmed, marker)
	if idx < 0 {
		return "", false
	}
	text := trimmed[idx+len(marker):]
	for _, terminator := range []string{`","type":"`, `"},{`, `"}]`, `"}` } {
		if end := strings.Index(text, terminator); end >= 0 {
			text = text[:end]
			break
		}
	}
	text = strings.ReplaceAll(text, `\"`, `"`)
	text = strings.ReplaceAll(text, `\n`, " ")
	text = strings.Join(strings.Fields(text), " ")
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	return text, true
}

// dbConversationToConversation converts a sqlc-generated db.Conversation to
// the application-level Conversation type.
func dbConversationToConversation(row db.Conversation) *Conversation {
	c := &Conversation{
		ID:        row.ID,
		ProjectID: row.ProjectID,
	}
	if row.Title.Valid {
		t := row.Title.String
		c.Title = &t
	}
	if row.Model.Valid {
		m := row.Model.String
		c.Model = &m
	}
	if row.Provider.Valid {
		p := row.Provider.String
		c.Provider = &p
	}
	if parsed, err := time.Parse(time.RFC3339, row.CreatedAt); err == nil {
		c.CreatedAt = parsed
	}
	if parsed, err := time.Parse(time.RFC3339, row.UpdatedAt); err == nil {
		c.UpdatedAt = parsed
	}
	return c
}
