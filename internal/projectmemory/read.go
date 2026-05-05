package projectmemory

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ponchione/shunter"
	"github.com/ponchione/shunter/types"
)

func (r *Runtime) ReadDocument(ctx context.Context, path string) (Document, string, error) {
	normalized, err := normalizeDocumentPath(path)
	if err != nil {
		return Document{}, "", err
	}
	var doc Document
	var content string
	var found bool
	err = r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableDocuments, indexDocumentsPrimary, types.NewString(normalized)) {
			doc = decodeDocumentRow(row)
			found = true
			break
		}
		if !found || doc.Deleted {
			return nil
		}
		var chunks []documentChunk
		for _, row := range view.SeekIndex(tableDocumentChunks, indexDocumentChunksPath, types.NewString(normalized)) {
			chunks = append(chunks, decodeChunkRow(row))
		}
		content = joinDocumentChunks(chunks)
		return nil
	})
	if err != nil {
		return Document{}, "", err
	}
	if !found || doc.Deleted {
		return Document{}, "", fmt.Errorf("Document not found: %s", normalized)
	}
	return doc, content, nil
}

func (r *Runtime) ListDocuments(ctx context.Context, directory string) ([]string, error) {
	prefix := strings.Trim(filepathSlash(directory), "/")
	if prefix != "" {
		prefix += "/"
	}
	var paths []string
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.TableScan(tableDocuments) {
			doc := decodeDocumentRow(row)
			if doc.Deleted {
				continue
			}
			if prefix != "" && !strings.HasPrefix(doc.Path, prefix) {
				continue
			}
			paths = append(paths, doc.Path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func (r *Runtime) SearchDocuments(ctx context.Context, query string, maxResults int) ([]SearchHit, error) {
	if maxResults <= 0 {
		maxResults = 10
	}
	normalizedQuery := normalizeKeyword(query)
	if normalizedQuery == "" {
		return nil, nil
	}
	var hits []SearchHit
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.TableScan(tableDocuments) {
			doc := decodeDocumentRow(row)
			if doc.Deleted {
				continue
			}
			var chunks []documentChunk
			for _, chunkRow := range view.SeekIndex(tableDocumentChunks, indexDocumentChunksPath, types.NewString(doc.Path)) {
				chunks = append(chunks, decodeChunkRow(chunkRow))
			}
			content := joinDocumentChunks(chunks)
			score := keywordScore(normalizedQuery, doc, content)
			if score == 0 {
				continue
			}
			hits = append(hits, SearchHit{
				Path:    doc.Path,
				Snippet: snippetAround(content, normalizedQuery),
				Score:   score,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Score > hits[j].Score
	})
	if len(hits) > maxResults {
		hits = hits[:maxResults]
	}
	return hits, nil
}

func (r *Runtime) ReadBrainIndexState(ctx context.Context) (BrainIndexState, bool, error) {
	var state BrainIndexState
	var found bool
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableBrainIndexState, indexBrainIndexStatePrimary, types.NewString(DefaultProjectID)) {
			state = decodeBrainIndexStateRow(row)
			found = true
			break
		}
		return nil
	})
	if err != nil {
		return BrainIndexState{}, false, err
	}
	return state, found, nil
}

func (r *Runtime) ReadCodeIndexState(ctx context.Context) (CodeIndexState, bool, error) {
	var state CodeIndexState
	var found bool
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableCodeIndexState, indexCodeIndexStatePrimary, types.NewString(DefaultProjectID)) {
			state = decodeCodeIndexStateRow(row)
			found = true
			break
		}
		return nil
	})
	if err != nil {
		return CodeIndexState{}, false, err
	}
	return state, found, nil
}

func (r *Runtime) ListCodeFileIndexStates(ctx context.Context) ([]CodeFileIndexState, error) {
	var states []CodeFileIndexState
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableCodeFileIndexState, indexCodeFileIndexStateProject, types.NewString(DefaultProjectID)) {
			states = append(states, decodeCodeFileIndexStateRow(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].FilePath < states[j].FilePath
	})
	return states, nil
}

func (r *Runtime) ReadConversation(ctx context.Context, id string) (Conversation, bool, error) {
	var conversation Conversation
	var found bool
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableConversations, indexConversationsPrimary, types.NewString(strings.TrimSpace(id))) {
			conversation = decodeConversationRow(row)
			found = true
			break
		}
		return nil
	})
	if err != nil {
		return Conversation{}, false, err
	}
	if !found || conversation.Deleted {
		return Conversation{}, false, nil
	}
	return conversation, true, nil
}

func (r *Runtime) ListConversations(ctx context.Context, projectID string, limit, offset int) ([]Conversation, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var conversations []Conversation
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableConversations, indexConversationsProject, types.NewString(projectID)) {
			conversation := decodeConversationRow(row)
			if conversation.Deleted {
				continue
			}
			conversations = append(conversations, conversation)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(conversations, func(i, j int) bool {
		if conversations[i].UpdatedAtUS == conversations[j].UpdatedAtUS {
			return conversations[i].ID < conversations[j].ID
		}
		return conversations[i].UpdatedAtUS > conversations[j].UpdatedAtUS
	})
	if offset >= len(conversations) {
		return nil, nil
	}
	end := offset + limit
	if end > len(conversations) {
		end = len(conversations)
	}
	return conversations[offset:end], nil
}

func (r *Runtime) CountConversations(ctx context.Context, projectID string) (int64, error) {
	var count int64
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableConversations, indexConversationsProject, types.NewString(projectID)) {
			if !decodeConversationRow(row).Deleted {
				count++
			}
		}
		return nil
	})
	return count, err
}

func (r *Runtime) ListMessages(ctx context.Context, conversationID string, includeCompressed bool) ([]Message, error) {
	var messages []Message
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, row := range view.SeekIndex(tableMessages, indexMessagesConversation, types.NewString(conversationID)) {
			message := decodeMessageRow(row)
			if !message.Visible {
				continue
			}
			if message.Compressed && !includeCompressed {
				continue
			}
			messages = append(messages, message)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortMessages(messages)
	return messages, nil
}

func (r *Runtime) GetMessagePage(ctx context.Context, conversationID string, limit, offset int) ([]Message, error) {
	if limit <= 0 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	messages, err := r.ListMessages(ctx, conversationID, true)
	if err != nil {
		return nil, err
	}
	if offset >= len(messages) {
		return nil, nil
	}
	start := len(messages) - offset - limit
	if start < 0 {
		start = 0
	}
	end := len(messages) - offset
	if end < 0 {
		end = 0
	}
	return messages[start:end], nil
}

func (r *Runtime) NextTurnNumber(ctx context.Context, conversationID string) (int, error) {
	messages, err := r.ListMessages(ctx, conversationID, true)
	if err != nil {
		return 0, err
	}
	maxTurn := uint32(0)
	for _, message := range messages {
		if message.TurnNumber > maxTurn {
			maxTurn = message.TurnNumber
		}
	}
	return int(maxTurn) + 1, nil
}

func (r *Runtime) SearchConversations(ctx context.Context, projectID string, query string, maxResults int) ([]ConversationSearchHit, error) {
	if maxResults <= 0 {
		maxResults = 20
	}
	normalizedQuery := normalizeKeyword(query)
	if normalizedQuery == "" {
		return nil, nil
	}
	conversations, err := r.ListConversations(ctx, projectID, int(^uint(0)>>1), 0)
	if err != nil {
		return nil, err
	}
	type scoredHit struct {
		hit   ConversationSearchHit
		score int
		order int
	}
	best := make(map[string]scoredHit)
	order := 0
	err = r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		for _, conversation := range conversations {
			titleMatch := strings.Contains(normalizeKeyword(conversation.Title), normalizedQuery)
			for _, row := range view.SeekIndex(tableMessages, indexMessagesConversation, types.NewString(conversation.ID)) {
				message := decodeMessageRow(row)
				if !message.Visible {
					continue
				}
				contentMatch := strings.Contains(normalizeKeyword(message.Content), normalizedQuery)
				if !titleMatch && !contentMatch {
					continue
				}
				score := conversationSearchScore(message.Role, titleMatch, contentMatch)
				hit := ConversationSearchHit{
					ID:          conversation.ID,
					Title:       conversation.Title,
					UpdatedAtUS: conversation.UpdatedAtUS,
					Role:        message.Role,
					Snippet:     snippetAround(message.Content, normalizedQuery),
				}
				if hit.Snippet == "" && titleMatch {
					hit.Snippet = conversation.Title
				}
				current, exists := best[conversation.ID]
				if !exists || score > current.score {
					best[conversation.ID] = scoredHit{hit: hit, score: score, order: order}
				}
				order++
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	hits := make([]scoredHit, 0, len(best))
	for _, hit := range best {
		hits = append(hits, hit)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score == hits[j].score {
			return hits[i].order < hits[j].order
		}
		return hits[i].score > hits[j].score
	})
	if len(hits) > maxResults {
		hits = hits[:maxResults]
	}
	out := make([]ConversationSearchHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, hit.hit)
	}
	return out, nil
}

func (r *Runtime) ListSubCalls(ctx context.Context, conversationID string) ([]SubCall, error) {
	var subCalls []SubCall
	err := r.rt.Read(ctx, func(view shunter.LocalReadView) error {
		if strings.TrimSpace(conversationID) == "" {
			for _, row := range view.TableScan(tableSubCalls) {
				subCalls = append(subCalls, decodeSubCallRow(row))
			}
			return nil
		}
		for _, row := range view.SeekIndex(tableSubCalls, indexSubCallsConversation, types.NewString(conversationID)) {
			subCalls = append(subCalls, decodeSubCallRow(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortSubCalls(subCalls)
	return subCalls, nil
}

func (r *Runtime) ListTurnSubCalls(ctx context.Context, conversationID string, turnNumber uint32) ([]SubCall, error) {
	subCalls, err := r.ListSubCalls(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	out := make([]SubCall, 0, len(subCalls))
	for _, subCall := range subCalls {
		if subCall.TurnNumber == turnNumber {
			out = append(out, subCall)
		}
	}
	return out, nil
}

type SearchHit struct {
	Path    string
	Snippet string
	Score   float64
}

type ConversationSearchHit struct {
	ID          string
	Title       string
	UpdatedAtUS uint64
	Role        string
	Snippet     string
}

func sortMessages(messages []Message) {
	sort.Slice(messages, func(i, j int) bool {
		if messages[i].Sequence == messages[j].Sequence {
			return messages[i].ID < messages[j].ID
		}
		return messages[i].Sequence < messages[j].Sequence
	})
}

func sortSubCalls(subCalls []SubCall) {
	sort.Slice(subCalls, func(i, j int) bool {
		if subCalls[i].CompletedAtUS == subCalls[j].CompletedAtUS {
			return subCalls[i].ID < subCalls[j].ID
		}
		return subCalls[i].CompletedAtUS < subCalls[j].CompletedAtUS
	})
}

func conversationSearchScore(role string, titleMatch bool, contentMatch bool) int {
	score := 0
	if titleMatch {
		score += 40
	}
	if contentMatch {
		score += 10
	}
	switch role {
	case "assistant":
		score += 30
	case "user":
		score += 20
	case "tool":
		score += 10
	}
	return score
}

func filepathSlash(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
}

func normalizeKeyword(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func keywordScore(query string, doc Document, content string) float64 {
	score := 0.0
	if strings.Contains(normalizeKeyword(doc.Path), query) {
		score += 2
	}
	if strings.Contains(normalizeKeyword(doc.Title), query) {
		score += 2
	}
	if strings.Contains(normalizeKeyword(doc.TagsJSON), query) {
		score += 1
	}
	score += float64(strings.Count(normalizeKeyword(content), query))
	return score
}

func snippetAround(content string, normalizedQuery string) string {
	if content == "" {
		return ""
	}
	normalizedContent := normalizeKeyword(content)
	idx := strings.Index(normalizedContent, normalizedQuery)
	if idx < 0 {
		runes := []rune(content)
		if len(runes) > 160 {
			return string(runes[:160]) + "..."
		}
		return content
	}
	start := idx - 80
	if start < 0 {
		start = 0
	}
	end := idx + len(normalizedQuery) + 80
	if end > len(content) {
		end = len(content)
	}
	return strings.TrimSpace(content[start:end])
}
