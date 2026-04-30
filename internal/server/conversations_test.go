package server_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/server"
)

// mockConversationService is a test double implementing server.ConversationService.
type mockConversationService struct {
	conversations []*conversation.Conversation
	summaries     []conversation.ConversationSummary
	messages      []conversation.MessageView
	searchResults []conversation.SearchResult

	// Capture last call params for assertions.
	lastListProjectID string
	lastListLimit     int
	lastListOffset    int
	lastCreateOpts    []conversation.CreateOption
	lastGetID         string
	lastDeleteID      string
	lastSearchQuery   string
	lastSetID         string
	lastSetProvider   *string
	lastSetModel      *string
	lastMessagesID    string
	lastMessageLimit  int
	lastMessageOffset int

	getErr    error
	listErr   error
	createErr error
	deleteErr error
	msgErr    error
	searchErr error

	nextTurnNumber int
	nextTurnErr    error
	lastNextTurnID string
	getMessagesN   int
}

func (m *mockConversationService) Create(_ context.Context, projectID string, opts ...conversation.CreateOption) (*conversation.Conversation, error) {
	m.lastCreateOpts = opts
	if m.createErr != nil {
		return nil, m.createErr
	}
	createOpts := &conversation.CreateOptions{}
	for _, opt := range opts {
		opt(createOpts)
	}
	c := &conversation.Conversation{
		ID:        "conv-new-123",
		ProjectID: projectID,
		Title:     createOpts.Title,
		Model:     createOpts.Model,
		Provider:  createOpts.Provider,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.conversations = append(m.conversations, c)
	return c, nil
}

func (m *mockConversationService) Get(_ context.Context, id string) (*conversation.Conversation, error) {
	m.lastGetID = id
	if m.getErr != nil {
		return nil, m.getErr
	}
	for _, c := range m.conversations {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, fmt.Errorf("conversation manager: get %q: %w", id, sql.ErrNoRows)
}

func (m *mockConversationService) List(_ context.Context, projectID string, limit, offset int) ([]conversation.ConversationSummary, error) {
	m.lastListProjectID = projectID
	m.lastListLimit = limit
	m.lastListOffset = offset
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.summaries, nil
}

func (m *mockConversationService) Delete(_ context.Context, id string) error {
	m.lastDeleteID = id
	return m.deleteErr
}

func (m *mockConversationService) SetRuntimeDefaults(_ context.Context, conversationID string, provider, model *string) error {
	m.lastSetID = conversationID
	m.lastSetProvider = provider
	m.lastSetModel = model
	for _, c := range m.conversations {
		if c.ID != conversationID {
			continue
		}
		c.Provider = provider
		c.Model = model
		break
	}
	return nil
}

func (m *mockConversationService) NextTurnNumber(_ context.Context, conversationID string) (int, error) {
	m.lastNextTurnID = conversationID
	if m.nextTurnErr != nil {
		return 0, m.nextTurnErr
	}
	if m.nextTurnNumber > 0 {
		return m.nextTurnNumber, nil
	}
	return 1, nil
}

func (m *mockConversationService) GetMessages(_ context.Context, conversationID string) ([]conversation.MessageView, error) {
	m.getMessagesN++
	if m.msgErr != nil {
		return nil, m.msgErr
	}
	return m.messages, nil
}

func (m *mockConversationService) GetMessagePage(_ context.Context, conversationID string, limit, offset int) ([]conversation.MessageView, error) {
	m.lastMessagesID = conversationID
	m.lastMessageLimit = limit
	m.lastMessageOffset = offset
	if m.msgErr != nil {
		return nil, m.msgErr
	}
	return m.messages, nil
}

func (m *mockConversationService) Search(_ context.Context, query string) ([]conversation.SearchResult, error) {
	m.lastSearchQuery = query
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return m.searchResults, nil
}

func setupConversationTests(t *testing.T, mock *mockConversationService) string {
	t.Helper()
	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConversationHandler(srv, mock, "test-project", newTestLogger())

	_, base := startServer(t, srv)
	return base
}

func TestListConversations(t *testing.T) {
	title := "Test Chat"
	mock := &mockConversationService{
		summaries: []conversation.ConversationSummary{
			{ID: "conv-1", Title: &title, UpdatedAt: time.Now()},
			{ID: "conv-2", UpdatedAt: time.Now()},
		},
	}
	base := setupConversationTests(t, mock)

	resp, err := http.Get(base + "/api/conversations")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body []map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body) != 2 {
		t.Fatalf("expected 2 conversations, got %d", len(body))
	}

	// Verify default limit/offset were applied.
	if mock.lastListLimit != 50 {
		t.Fatalf("expected default limit 50, got %d", mock.lastListLimit)
	}
	if mock.lastListProjectID != "test-project" {
		t.Fatalf("expected project ID test-project, got %q", mock.lastListProjectID)
	}
}

func TestListConversationsWithPagination(t *testing.T) {
	mock := &mockConversationService{}
	base := setupConversationTests(t, mock)

	resp, err := http.Get(base + "/api/conversations?limit=10&offset=20")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if mock.lastListLimit != 10 {
		t.Fatalf("expected limit 10, got %d", mock.lastListLimit)
	}
	if mock.lastListOffset != 20 {
		t.Fatalf("expected offset 20, got %d", mock.lastListOffset)
	}
}

func TestCreateConversation(t *testing.T) {
	mock := &mockConversationService{}
	base := setupConversationTests(t, mock)

	body := `{"title":"New Chat","model":"claude-3","provider":"anthropic"}`
	resp, err := http.Post(base+"/api/conversations", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["id"] != "conv-new-123" {
		t.Fatalf("expected id conv-new-123, got %v", result["id"])
	}
}

func TestCreateConversationEmptyBody(t *testing.T) {
	mock := &mockConversationService{}
	base := setupConversationTests(t, mock)

	resp, err := http.Post(base+"/api/conversations", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for empty body, got %d", resp.StatusCode)
	}
}

func TestGetConversation(t *testing.T) {
	title := "Found It"
	mock := &mockConversationService{
		conversations: []*conversation.Conversation{
			{ID: "conv-abc", ProjectID: "p1", Title: &title, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
	base := setupConversationTests(t, mock)

	resp, err := http.Get(base + "/api/conversations/conv-abc")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["id"] != "conv-abc" {
		t.Fatalf("expected id conv-abc, got %v", result["id"])
	}
}

func TestGetConversationNotFound(t *testing.T) {
	mock := &mockConversationService{}
	base := setupConversationTests(t, mock)

	resp, err := http.Get(base + "/api/conversations/nonexistent")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, body)
	}
}

func TestDeleteConversation(t *testing.T) {
	mock := &mockConversationService{}
	base := setupConversationTests(t, mock)

	req, _ := http.NewRequest(http.MethodDelete, base+"/api/conversations/conv-del", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	if mock.lastDeleteID != "conv-del" {
		t.Fatalf("expected delete ID conv-del, got %q", mock.lastDeleteID)
	}
}

func TestGetMessages(t *testing.T) {
	content := "hello world"
	mock := &mockConversationService{
		messages: []conversation.MessageView{
			{ID: 1, Role: "user", Content: &content, TurnNumber: 1, Sequence: 0},
			{ID: 2, Role: "assistant", Content: &content, TurnNumber: 1, Sequence: 1, IsCompressed: true},
		},
	}
	base := setupConversationTests(t, mock)

	resp, err := http.Get(base + "/api/conversations/conv-1/messages")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var msgs []map[string]any
	json.NewDecoder(resp.Body).Decode(&msgs)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if mock.lastMessagesID != "conv-1" {
		t.Fatalf("message page conversation = %q, want conv-1", mock.lastMessagesID)
	}
	if mock.lastMessageLimit != 200 {
		t.Fatalf("default message limit = %d, want 200", mock.lastMessageLimit)
	}
	if mock.lastMessageOffset != 0 {
		t.Fatalf("default message offset = %d, want 0", mock.lastMessageOffset)
	}

	// Second message should be compressed.
	if msgs[1]["is_compressed"] != true {
		t.Fatalf("expected is_compressed=true on second message")
	}
}

func TestGetMessagesUsesBoundedPagination(t *testing.T) {
	mock := &mockConversationService{}
	base := setupConversationTests(t, mock)

	resp, err := http.Get(base + "/api/conversations/conv-1/messages?limit=25&offset=50")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if mock.lastMessageLimit != 25 {
		t.Fatalf("message limit = %d, want 25", mock.lastMessageLimit)
	}
	if mock.lastMessageOffset != 50 {
		t.Fatalf("message offset = %d, want 50", mock.lastMessageOffset)
	}
}

func TestGetMessagesCapsPagination(t *testing.T) {
	mock := &mockConversationService{}
	base := setupConversationTests(t, mock)

	resp, err := http.Get(base + "/api/conversations/conv-1/messages?limit=999&offset=-20")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if mock.lastMessageLimit != 500 {
		t.Fatalf("message limit = %d, want 500", mock.lastMessageLimit)
	}
	if mock.lastMessageOffset != 0 {
		t.Fatalf("message offset = %d, want 0", mock.lastMessageOffset)
	}
}

func TestSearchConversations(t *testing.T) {
	title := "Chat about Go"
	mock := &mockConversationService{
		searchResults: []conversation.SearchResult{
			{ID: "conv-1", Title: &title, UpdatedAt: "2026-03-31T12:00:00Z", Snippet: "...about <b>Go</b>..."},
		},
	}
	base := setupConversationTests(t, mock)

	resp, err := http.Get(base + "/api/conversations/search?q=Go")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var results []map[string]any
	json.NewDecoder(resp.Body).Decode(&results)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if mock.lastSearchQuery != "Go" {
		t.Fatalf("expected search query 'Go', got %q", mock.lastSearchQuery)
	}
}

func TestSearchConversationsMissingQuery(t *testing.T) {
	mock := &mockConversationService{}
	base := setupConversationTests(t, mock)

	resp, err := http.Get(base + "/api/conversations/search")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing query, got %d", resp.StatusCode)
	}
}

func TestCreateConversationInvalidJSON(t *testing.T) {
	mock := &mockConversationService{}
	base := setupConversationTests(t, mock)

	resp, err := http.Post(base+"/api/conversations", "application/json", strings.NewReader("{invalid"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
