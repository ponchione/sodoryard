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

	"github.com/ponchione/sodoryard/internal/db"
	sid "github.com/ponchione/sodoryard/internal/id"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	"github.com/ponchione/sodoryard/internal/provider"
)

// Conversation is the application-level representation of a conversation row.
// It converts sql.NullString fields to Go-native pointer types.
type Conversation struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Title     *string   `json:"title,omitempty"`
	Model     *string   `json:"model,omitempty"`
	Provider  *string   `json:"provider,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

type ProjectMemoryStore interface {
	CreateConversation(ctx context.Context, args projectmemory.CreateConversationArgs) error
	DeleteConversation(ctx context.Context, args projectmemory.DeleteConversationArgs) error
	SetConversationTitle(ctx context.Context, args projectmemory.SetConversationTitleArgs) error
	SetRuntimeDefaults(ctx context.Context, args projectmemory.SetRuntimeDefaultsArgs) error
	AppendUserMessage(ctx context.Context, args projectmemory.AppendUserMessageArgs) error
	PersistIteration(ctx context.Context, args projectmemory.PersistIterationArgs) error
	CancelIteration(ctx context.Context, args projectmemory.CancelIterationArgs) error
	DiscardTurn(ctx context.Context, args projectmemory.DiscardTurnArgs) error
	ReadConversation(ctx context.Context, id string) (projectmemory.Conversation, bool, error)
	ListConversations(ctx context.Context, projectID string, limit, offset int) ([]projectmemory.Conversation, error)
	CountConversations(ctx context.Context, projectID string) (int64, error)
	ListMessages(ctx context.Context, conversationID string, includeCompressed bool) ([]projectmemory.Message, error)
	GetMessagePage(ctx context.Context, conversationID string, limit, offset int) ([]projectmemory.Message, error)
	NextTurnNumber(ctx context.Context, conversationID string) (int, error)
	SearchConversations(ctx context.Context, projectID string, query string, maxResults int) ([]projectmemory.ConversationSearchHit, error)
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
	memory  ProjectMemoryStore
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

func NewProjectMemoryManager(memory ProjectMemoryStore, seen *SeenFiles, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	if seen == nil {
		seen = NewSeenFiles()
	}
	return &Manager{
		HistoryManager: &HistoryManager{memory: memory, seen: seen, now: time.Now},
		logger:         logger,
		newID:          sid.New,
		memory:         memory,
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
	if m.memory != nil {
		if err := m.memory.CreateConversation(ctx, projectmemory.CreateConversationArgs{
			ID:          id,
			ProjectID:   projectID,
			Title:       stringPtrValue(options.Title),
			Model:       stringPtrValue(options.Model),
			Provider:    stringPtrValue(options.Provider),
			CreatedAtUS: uint64(now.UnixMicro()),
		}); err != nil {
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
	if m.memory != nil {
		row, found, err := m.memory.ReadConversation(ctx, conversationID)
		if err != nil {
			return nil, fmt.Errorf("conversation manager: get: %w", err)
		}
		if !found {
			return nil, fmt.Errorf("conversation manager: get %q: %w", conversationID, sql.ErrNoRows)
		}
		return memoryConversationToConversation(row), nil
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
	if m.memory != nil {
		rows, err := m.memory.ListConversations(ctx, projectID, limit, offset)
		if err != nil {
			return nil, fmt.Errorf("conversation manager: list: %w", err)
		}
		summaries := make([]ConversationSummary, 0, len(rows))
		for _, row := range rows {
			summaries = append(summaries, ConversationSummary{
				ID:        row.ID,
				Title:     stringPtrFromNonEmpty(row.Title),
				UpdatedAt: unixMicroTime(row.UpdatedAtUS),
			})
		}
		return summaries, nil
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
	if m.memory != nil {
		if err := m.memory.DeleteConversation(ctx, projectmemory.DeleteConversationArgs{ID: conversationID}); err != nil {
			return fmt.Errorf("conversation manager: delete %q: %w", conversationID, err)
		}
		return nil
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

	now := m.now().UTC()
	if m.memory != nil {
		if err := m.memory.SetConversationTitle(ctx, projectmemory.SetConversationTitleArgs{
			ID:          conversationID,
			Title:       title,
			UpdatedAtUS: uint64(now.UnixMicro()),
		}); err != nil {
			return fmt.Errorf("conversation manager: set title: %w", err)
		}
		return nil
	}

	timestamp := now.Format(time.RFC3339)
	if err := m.queries.SetConversationTitle(ctx, db.SetConversationTitleParams{
		Title:     sql.NullString{String: title, Valid: true},
		UpdatedAt: timestamp,
		ID:        conversationID,
	}); err != nil {
		return fmt.Errorf("conversation manager: set title: %w", err)
	}
	return nil
}

// SetRuntimeDefaults updates the conversation-scoped provider/model defaults
// and bumps updated_at.
func (m *Manager) SetRuntimeDefaults(ctx context.Context, conversationID string, provider, model *string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	now := m.now().UTC()
	if m.memory != nil {
		if err := m.memory.SetRuntimeDefaults(ctx, projectmemory.SetRuntimeDefaultsArgs{
			ID:          conversationID,
			Provider:    stringPtrValue(provider),
			Model:       stringPtrValue(model),
			UpdatedAtUS: uint64(now.UnixMicro()),
		}); err != nil {
			return fmt.Errorf("conversation manager: set runtime defaults: %w", err)
		}
		return nil
	}

	params := db.SetConversationRuntimeDefaultsParams{
		UpdatedAt: now.Format(time.RFC3339),
		ID:        conversationID,
	}
	if model != nil {
		params.Model = sql.NullString{String: *model, Valid: true}
	}
	if provider != nil {
		params.Provider = sql.NullString{String: *provider, Valid: true}
	}
	if err := m.queries.SetConversationRuntimeDefaults(ctx, params); err != nil {
		return fmt.Errorf("conversation manager: set runtime defaults: %w", err)
	}
	return nil
}

// Count returns the total number of conversations for a project.
func (m *Manager) Count(ctx context.Context, projectID string) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if m.memory != nil {
		return m.memory.CountConversations(ctx, projectID)
	}
	return m.queries.CountConversations(ctx, projectID)
}

// NextTurnNumber returns the next turn number for a conversation without
// loading message content.
func (m *Manager) NextTurnNumber(ctx context.Context, conversationID string) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if m.memory != nil {
		next, err := m.memory.NextTurnNumber(ctx, conversationID)
		if err != nil {
			return 0, fmt.Errorf("conversation manager: next turn number: %w", err)
		}
		return next, nil
	}
	next, err := m.queries.NextTurnNumber(ctx, conversationID)
	if err != nil {
		return 0, fmt.Errorf("conversation manager: next turn number: %w", err)
	}
	return int(next), nil
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
	if m.memory != nil {
		rows, err := m.memory.ListMessages(ctx, conversationID, true)
		if err != nil {
			return nil, fmt.Errorf("conversation manager: get messages: %w", err)
		}
		messages := make([]MessageView, 0, len(rows))
		for _, row := range rows {
			messages = append(messages, messageViewFromMemory(row))
		}
		return messages, nil
	}

	rows, err := m.queries.ListAllMessages(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("conversation manager: get messages: %w", err)
	}

	messages := make([]MessageView, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, messageViewFromValues(
			row.ID,
			row.Role,
			row.Content,
			row.ToolUseID,
			row.ToolName,
			row.TurnNumber,
			row.Iteration,
			row.Sequence,
			row.IsCompressed,
			row.IsSummary,
			row.CreatedAt,
		))
	}
	return messages, nil
}

// GetMessagePage returns a bounded page from the newest messages, preserving
// chronological order within the returned page.
func (m *Manager) GetMessagePage(ctx context.Context, conversationID string, limit, offset int) ([]MessageView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	if m.memory != nil {
		rows, err := m.memory.GetMessagePage(ctx, conversationID, limit, offset)
		if err != nil {
			return nil, fmt.Errorf("conversation manager: get message page: %w", err)
		}
		messages := make([]MessageView, 0, len(rows))
		for _, row := range rows {
			messages = append(messages, messageViewFromMemory(row))
		}
		return messages, nil
	}

	rows, err := m.queries.ListMessagePage(ctx, db.ListMessagePageParams{
		ConversationID: conversationID,
		Limit:          int64(limit),
		Offset:         int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("conversation manager: get message page: %w", err)
	}

	messages := make([]MessageView, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, messageViewFromValues(
			row.ID,
			row.Role,
			row.Content,
			row.ToolUseID,
			row.ToolName,
			row.TurnNumber,
			row.Iteration,
			row.Sequence,
			row.IsCompressed,
			row.IsSummary,
			row.CreatedAt,
		))
	}
	return messages, nil
}

func messageViewFromValues(
	id int64,
	role string,
	content sql.NullString,
	toolUseID sql.NullString,
	toolName sql.NullString,
	turnNumber int64,
	iteration int64,
	sequence float64,
	isCompressed int64,
	isSummary int64,
	createdAt string,
) MessageView {
	mv := MessageView{
		ID:           id,
		Role:         role,
		TurnNumber:   turnNumber,
		Iteration:    iteration,
		Sequence:     sequence,
		IsCompressed: isCompressed != 0,
		IsSummary:    isSummary != 0,
		CreatedAt:    createdAt,
	}
	if content.Valid {
		mv.Content = &content.String
	}
	if toolUseID.Valid {
		mv.ToolUseID = &toolUseID.String
	}
	if toolName.Valid {
		mv.ToolName = &toolName.String
	}
	return mv
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

// Search performs project-scoped full-text search across conversation messages using FTS5.
func (m *Manager) Search(ctx context.Context, projectID string, query string) ([]SearchResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if m.memory != nil {
		rows, err := m.memory.SearchConversations(ctx, projectID, query, 20)
		if err != nil {
			return nil, fmt.Errorf("conversation manager: search: %w", err)
		}
		results := make([]SearchResult, 0, len(rows))
		for _, row := range rows {
			results = append(results, SearchResult{
				ID:        row.ID,
				Title:     stringPtrFromNonEmpty(row.Title),
				UpdatedAt: unixMicroTime(row.UpdatedAtUS).UTC().Format(time.RFC3339),
				Snippet:   sanitizeSearchSnippet(row.Snippet),
			})
		}
		return results, nil
	}

	params := db.SearchConversationsParams{ProjectID: projectID, Content: query}
	rows, err := m.queries.SearchConversations(ctx, params)
	if err != nil && shouldRetryLiteralSearch(err, query) {
		params.Content = buildLiteralFTSQuery(query)
		rows, err = m.queries.SearchConversations(ctx, params)
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
	for _, terminator := range []string{`","type":"`, `"},{`, `"}]`, `"}`} {
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

func memoryConversationToConversation(row projectmemory.Conversation) *Conversation {
	return &Conversation{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		Title:     stringPtrFromNonEmpty(row.Title),
		Model:     stringPtrFromNonEmpty(row.Model),
		Provider:  stringPtrFromNonEmpty(row.Provider),
		CreatedAt: unixMicroTime(row.CreatedAtUS),
		UpdatedAt: unixMicroTime(row.UpdatedAtUS),
	}
}

func messageViewFromMemory(row projectmemory.Message) MessageView {
	return MessageView{
		ID:           int64(row.Sequence) + 1,
		Role:         row.Role,
		Content:      stringPtrFromNonEmpty(row.Content),
		ToolUseID:    stringPtrFromNonEmpty(row.ToolUseID),
		ToolName:     stringPtrFromNonEmpty(row.ToolName),
		TurnNumber:   int64(row.TurnNumber),
		Iteration:    int64(row.Iteration),
		Sequence:     float64(row.Sequence),
		IsCompressed: row.Compressed,
		IsSummary:    row.IsSummary,
		CreatedAt:    unixMicroTime(row.CreatedAtUS).UTC().Format(time.RFC3339),
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPtrFromNonEmpty(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func unixMicroTime(value uint64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.UnixMicro(int64(value)).UTC()
}
