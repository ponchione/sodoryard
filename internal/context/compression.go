package context

import (
	stdctx "context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/config"
	dbpkg "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	"github.com/ponchione/sodoryard/internal/provider"
)

const (
	defaultCompressionHeadPreserve = 3
	defaultCompressionTailPreserve = 4
	defaultCompressionModel        = "local"
	defaultCompressionMaxTokens    = 1024
	compressionSummaryPrefix       = "[CONTEXT COMPACTION]"
)

// CompressionResult reports what changed after a persisted-history compression pass.
type CompressionResult struct {
	Compressed          bool `json:"compressed,omitempty"`
	SummaryInserted     bool `json:"summary_inserted,omitempty"`
	FallbackUsed        bool `json:"fallback_used,omitempty"`
	CacheInvalidated    bool `json:"cache_invalidated,omitempty"`
	CompressedTurnStart int  `json:"compressed_turn_start,omitempty"`
	CompressedTurnEnd   int  `json:"compressed_turn_end,omitempty"`
	CompressedMessages  int  `json:"compressed_messages,omitempty"`
}

// CompressionEngine compresses persisted SQLite conversation history using the
// Layer 3 head-tail preservation algorithm.
type CompressionEngine struct {
	db               *sql.DB
	queries          *dbpkg.Queries
	provider         provider.Provider
	now              func() time.Time
	summaryMaxTokens int
}

type compressionPlan struct {
	head            []dbpkg.Message
	middle          []dbpkg.Message
	tail            []dbpkg.Message
	leftBoundary    float64
	rightBoundary   float64
	summarySequence float64
	turnStart       int
	turnEnd         int
}

type projectMemoryCompressionPlan struct {
	head            []projectmemory.Message
	middle          []projectmemory.Message
	tail            []projectmemory.Message
	summarySequence uint64
	turnStart       int
	turnEnd         int
}

// NewCompressionEngine constructs the concrete Slice 6 compression engine.
func NewCompressionEngine(database *sql.DB, p provider.Provider) *CompressionEngine {
	engine := &CompressionEngine{
		db:               database,
		provider:         p,
		now:              time.Now,
		summaryMaxTokens: defaultCompressionMaxTokens,
	}
	if database != nil {
		engine.queries = dbpkg.New(database)
	}
	return engine
}

// NeedsCompressionPreflight applies the rough char-count trigger before an LLM call.
func NeedsCompressionPreflight(totalChars int, modelContextLimit int, cfg config.ContextConfig) bool {
	return exceedsCompressionThreshold(approximateTokensFromChars(totalChars), modelContextLimit, cfg)
}

// NeedsCompressionPostResponse applies the exact prompt-token trigger after an LLM call.
func NeedsCompressionPostResponse(promptTokens int, modelContextLimit int, cfg config.ContextConfig) bool {
	return exceedsCompressionThreshold(promptTokens, modelContextLimit, cfg)
}

// NeedsCompressionAfterProviderError detects overflow-style provider errors that
// should trigger immediate compression and one retry.
func NeedsCompressionAfterProviderError(statusCode int, err error) bool {
	if statusCode == 413 {
		return true
	}
	if statusCode != 400 || err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "context_length_exceeded")
}

// Compress rewrites persisted history for one conversation and returns whether
// prompt-history cache state must be invalidated afterward.
func (e *CompressionEngine) Compress(ctx stdctx.Context, conversationID string, cfg config.ContextConfig) (*CompressionResult, error) {
	if ctx == nil {
		ctx = stdctx.Background()
	}
	if e == nil || e.db == nil || e.queries == nil {
		return nil, errors.New("compression engine: database is nil")
	}
	if strings.TrimSpace(conversationID) == "" {
		return nil, errors.New("compression engine: conversation ID is empty")
	}

	messages, err := e.queries.ListMessagesForCompression(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("compression engine: list messages: %w", err)
	}
	plan, shouldCompress, err := buildCompressionPlan(messages, cfg)
	if err != nil {
		return nil, fmt.Errorf("compression engine: plan compression: %w", err)
	}
	if !shouldCompress {
		return &CompressionResult{}, nil
	}

	summaryText, summaryInserted, fallbackUsed, err := e.prepareSummary(ctx, conversationID, plan.middle, cfg)
	if err != nil {
		return nil, err
	}

	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("compression engine: begin transaction: %w", err)
	}
	defer tx.Rollback()
	qtx := e.queries.WithTx(tx)

	if err := qtx.MarkMessagesCompressedBetweenSequences(ctx, dbpkg.MarkMessagesCompressedBetweenSequencesParams{
		ConversationID: conversationID,
		Sequence:       plan.leftBoundary,
		Sequence_2:     plan.rightBoundary,
	}); err != nil {
		return nil, fmt.Errorf("compression engine: mark compressed messages: %w", err)
	}

	survivingToolResults := survivingToolResultIDs(plan.head, plan.tail)
	for _, msg := range survivingAssistantMessages(plan.head, plan.tail) {
		if !msg.Content.Valid || strings.TrimSpace(msg.Content.String) == "" {
			continue
		}
		content, changed, err := sanitizeAssistantMessageContent(msg.Content.String, survivingToolResults)
		if err != nil {
			return nil, fmt.Errorf("compression engine: sanitize assistant message %d: %w", msg.ID, err)
		}
		if !changed {
			continue
		}
		if err := qtx.UpdateMessageContent(ctx, dbpkg.UpdateMessageContentParams{
			Content: sql.NullString{String: content, Valid: true},
			ID:      msg.ID,
		}); err != nil {
			return nil, fmt.Errorf("compression engine: update assistant message %d: %w", msg.ID, err)
		}
	}

	// Second pass: compress orphaned tool-result messages whose originating
	// assistant tool_use block was compressed in the middle.
	survivingToolUses := survivingAssistantToolUseIDs(plan.head, plan.tail)
	for _, msg := range append(append([]dbpkg.Message(nil), plan.head...), plan.tail...) {
		if msg.Role != "tool" || !msg.ToolUseID.Valid || strings.TrimSpace(msg.ToolUseID.String) == "" {
			continue
		}
		if _, ok := survivingToolUses[msg.ToolUseID.String]; ok {
			continue
		}
		if err := qtx.MarkMessageCompressedByID(ctx, msg.ID); err != nil {
			return nil, fmt.Errorf("compression engine: compress orphaned tool result %d: %w", msg.ID, err)
		}
	}

	if summaryInserted {
		if err := qtx.InsertCompressionSummary(ctx, dbpkg.InsertCompressionSummaryParams{
			ConversationID: conversationID,
			Content:        sql.NullString{String: summaryText, Valid: true},
			TurnNumber:     int64(plan.turnEnd),
			Iteration:      1,
			Sequence:       plan.summarySequence,
			CompressedTurnStart: sql.NullInt64{
				Int64: int64(plan.turnStart),
				Valid: true,
			},
			CompressedTurnEnd: sql.NullInt64{
				Int64: int64(plan.turnEnd),
				Valid: true,
			},
			CreatedAt: e.now().UTC().Format(time.RFC3339),
		}); err != nil {
			return nil, fmt.Errorf("compression engine: insert summary message: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("compression engine: commit transaction: %w", err)
	}

	return &CompressionResult{
		Compressed:          true,
		SummaryInserted:     summaryInserted,
		FallbackUsed:        fallbackUsed,
		CacheInvalidated:    true,
		CompressedTurnStart: plan.turnStart,
		CompressedTurnEnd:   plan.turnEnd,
		CompressedMessages:  len(plan.middle),
	}, nil
}

type ProjectMemoryCompressionStore interface {
	ListMessages(ctx stdctx.Context, conversationID string, includeCompressed bool) ([]projectmemory.Message, error)
	CompressMessages(ctx stdctx.Context, args projectmemory.CompressMessagesArgs) error
}

type ProjectMemoryCompressionEngine struct {
	store            ProjectMemoryCompressionStore
	provider         provider.Provider
	now              func() time.Time
	summaryMaxTokens int
}

func NewProjectMemoryCompressionEngine(store ProjectMemoryCompressionStore, p provider.Provider) *ProjectMemoryCompressionEngine {
	return &ProjectMemoryCompressionEngine{
		store:            store,
		provider:         p,
		now:              time.Now,
		summaryMaxTokens: defaultCompressionMaxTokens,
	}
}

func (e *ProjectMemoryCompressionEngine) Compress(ctx stdctx.Context, conversationID string, cfg config.ContextConfig) (*CompressionResult, error) {
	if ctx == nil {
		ctx = stdctx.Background()
	}
	if e == nil || e.store == nil {
		return nil, errors.New("compression engine: project memory store is nil")
	}
	if strings.TrimSpace(conversationID) == "" {
		return nil, errors.New("compression engine: conversation ID is empty")
	}

	messages, err := e.store.ListMessages(ctx, conversationID, true)
	if err != nil {
		return nil, fmt.Errorf("compression engine: list project memory messages: %w", err)
	}
	plan, shouldCompress, err := buildProjectMemoryCompressionPlan(messages, cfg)
	if err != nil {
		return nil, fmt.Errorf("compression engine: plan project memory compression: %w", err)
	}
	if !shouldCompress {
		return &CompressionResult{}, nil
	}

	summaryText, summaryInserted, fallbackUsed, err := e.prepareSummary(ctx, conversationID, projectMemoryMessagesToDB(plan.middle), cfg)
	if err != nil {
		return nil, err
	}

	compressIDs := make([]string, 0, len(plan.middle))
	for _, msg := range plan.middle {
		compressIDs = append(compressIDs, msg.ID)
	}
	survivingToolResults := projectMemorySurvivingToolResultIDs(plan.head, plan.tail)
	sanitized := make([]projectmemory.CompressMessageContentUpdate, 0)
	for _, msg := range projectMemorySurvivingAssistantMessages(plan.head, plan.tail) {
		contentRaw := strings.TrimSpace(msg.Content)
		if contentRaw == "" || !strings.HasPrefix(contentRaw, "[") {
			continue
		}
		content, changed, err := sanitizeAssistantMessageContent(msg.Content, survivingToolResults)
		if err != nil {
			return nil, fmt.Errorf("compression engine: sanitize assistant message %s: %w", msg.ID, err)
		}
		if changed {
			sanitized = append(sanitized, projectmemory.CompressMessageContentUpdate{ID: msg.ID, Content: content})
		}
	}

	survivingToolUses := projectMemorySurvivingAssistantToolUseIDs(plan.head, plan.tail)
	for _, msg := range append(append([]projectmemory.Message(nil), plan.head...), plan.tail...) {
		if msg.Role != "tool" || strings.TrimSpace(msg.ToolUseID) == "" {
			continue
		}
		if _, ok := survivingToolUses[msg.ToolUseID]; ok {
			continue
		}
		compressIDs = append(compressIDs, msg.ID)
	}

	nowUS := uint64(e.now().UTC().UnixMicro())
	args := projectmemory.CompressMessagesArgs{
		ConversationID:    conversationID,
		MessageIDs:        compressIDs,
		SanitizedMessages: sanitized,
		CreatedAtUS:       nowUS,
	}
	if summaryInserted {
		summaryOf, err := json.Marshal(compressIDs)
		if err != nil {
			return nil, fmt.Errorf("compression engine: encode project memory summary ids: %w", err)
		}
		metadata, err := json.Marshal(struct {
			CompressedTurnStart int `json:"compressed_turn_start"`
			CompressedTurnEnd   int `json:"compressed_turn_end"`
			CompressedMessages  int `json:"compressed_messages"`
		}{
			CompressedTurnStart: plan.turnStart,
			CompressedTurnEnd:   plan.turnEnd,
			CompressedMessages:  len(plan.middle),
		})
		if err != nil {
			return nil, fmt.Errorf("compression engine: encode project memory summary metadata: %w", err)
		}
		args.SummaryContent = summaryText
		args.SummaryTurnNumber = uint32(plan.turnEnd)
		args.SummaryIteration = 1
		args.SummarySequence = plan.summarySequence
		args.SummaryOfJSON = string(summaryOf)
		args.SummaryMetadataJSON = string(metadata)
	}
	if err := e.store.CompressMessages(ctx, args); err != nil {
		return nil, fmt.Errorf("compression engine: compress project memory messages: %w", err)
	}

	return &CompressionResult{
		Compressed:          true,
		SummaryInserted:     summaryInserted,
		FallbackUsed:        fallbackUsed,
		CacheInvalidated:    true,
		CompressedTurnStart: plan.turnStart,
		CompressedTurnEnd:   plan.turnEnd,
		CompressedMessages:  len(plan.middle),
	}, nil
}

func (e *ProjectMemoryCompressionEngine) prepareSummary(ctx stdctx.Context, conversationID string, middle []dbpkg.Message, cfg config.ContextConfig) (string, bool, bool, error) {
	summary, err := e.generateSummary(ctx, conversationID, middle, cfg)
	if err == nil {
		return prefixedCompressionSummary(summary), true, false, nil
	}
	if ctx.Err() != nil {
		return "", false, false, fmt.Errorf("compression engine: summarize middle turns: %w", ctx.Err())
	}
	slog.Warn(
		"context compression summarization failed",
		"provider", compressionProviderName(e.provider),
		"model", compressionModel(cfg),
		"error", err,
		"messages", len(middle),
	)
	return "", false, true, nil
}

func (e *ProjectMemoryCompressionEngine) generateSummary(ctx stdctx.Context, conversationID string, middle []dbpkg.Message, cfg config.ContextConfig) (string, error) {
	if e.provider == nil {
		return "", errors.New("compression provider is nil")
	}
	prompt := buildCompressionPrompt(middle)
	response, err := e.provider.Complete(ctx, &provider.Request{
		Messages:       []provider.Message{provider.NewUserMessage(prompt)},
		Model:          compressionModel(cfg),
		MaxTokens:      e.summaryMaxTokens,
		Purpose:        "compression",
		ConversationID: conversationID,
		TurnNumber:     lastTurnNumber(middle),
	})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(extractCompressionSummaryText(response))
	if text == "" {
		return "", errors.New("compression summary response contained no text")
	}
	return text, nil
}

func buildCompressionPlan(messages []dbpkg.Message, cfg config.ContextConfig) (*compressionPlan, bool, error) {
	active := make([]dbpkg.Message, 0, len(messages))
	occupied := make(map[float64]struct{}, len(messages))
	for _, msg := range messages {
		occupied[msg.Sequence] = struct{}{}
		if msg.IsCompressed == 0 {
			active = append(active, msg)
		}
	}
	if len(active) == 0 {
		return nil, false, nil
	}

	headCount := compressionHeadPreserve(cfg)
	tailCount := compressionTailPreserve(cfg)
	if headCount+tailCount >= len(active) {
		return nil, false, nil
	}

	head := append([]dbpkg.Message(nil), active[:headCount]...)
	tail := append([]dbpkg.Message(nil), active[len(active)-tailCount:]...)
	middle := append([]dbpkg.Message(nil), active[headCount:len(active)-tailCount]...)
	if len(middle) == 0 {
		return nil, false, nil
	}

	leftBoundary := head[len(head)-1].Sequence
	rightBoundary := tail[0].Sequence
	summarySequence, err := chooseCompressionSequence(leftBoundary, rightBoundary, occupied)
	if err != nil {
		return nil, false, err
	}
	turnStart, turnEnd := compressionTurnRange(middle)

	return &compressionPlan{
		head:            head,
		middle:          middle,
		tail:            tail,
		leftBoundary:    leftBoundary,
		rightBoundary:   rightBoundary,
		summarySequence: summarySequence,
		turnStart:       turnStart,
		turnEnd:         turnEnd,
	}, true, nil
}

func buildProjectMemoryCompressionPlan(messages []projectmemory.Message, cfg config.ContextConfig) (*projectMemoryCompressionPlan, bool, error) {
	active := make([]projectmemory.Message, 0, len(messages))
	for _, msg := range messages {
		if !msg.Visible || msg.Compressed {
			continue
		}
		active = append(active, msg)
	}
	if len(active) == 0 {
		return nil, false, nil
	}

	headCount := compressionHeadPreserve(cfg)
	tailCount := compressionTailPreserve(cfg)
	if headCount+tailCount >= len(active) {
		return nil, false, nil
	}

	head := append([]projectmemory.Message(nil), active[:headCount]...)
	tail := append([]projectmemory.Message(nil), active[len(active)-tailCount:]...)
	middle := append([]projectmemory.Message(nil), active[headCount:len(active)-tailCount]...)
	if len(middle) == 0 {
		return nil, false, nil
	}
	turnStart, turnEnd := projectMemoryCompressionTurnRange(middle)
	return &projectMemoryCompressionPlan{
		head:            head,
		middle:          middle,
		tail:            tail,
		summarySequence: middle[0].Sequence,
		turnStart:       turnStart,
		turnEnd:         turnEnd,
	}, true, nil
}

func chooseCompressionSequence(left float64, right float64, occupied map[float64]struct{}) (float64, error) {
	if right <= left {
		return 0, fmt.Errorf("invalid compression sequence bounds: %v >= %v", left, right)
	}
	candidate := (left + right) / 2
	for i := 0; i < 1024; i++ {
		if _, exists := occupied[candidate]; !exists {
			return candidate, nil
		}
		candidate = (left + candidate) / 2
		if candidate <= left || candidate >= right {
			break
		}
	}
	return 0, fmt.Errorf("unable to choose unique compression summary sequence between %v and %v", left, right)
}

func compressionTurnRange(messages []dbpkg.Message) (int, int) {
	if len(messages) == 0 {
		return 0, 0
	}
	start, end := messageTurnBounds(messages[0])
	for _, msg := range messages[1:] {
		msgStart, msgEnd := messageTurnBounds(msg)
		if msgStart < start {
			start = msgStart
		}
		if msgEnd > end {
			end = msgEnd
		}
	}
	return start, end
}

func projectMemoryCompressionTurnRange(messages []projectmemory.Message) (int, int) {
	if len(messages) == 0 {
		return 0, 0
	}
	start, end := projectMemoryMessageTurnBounds(messages[0])
	for _, msg := range messages[1:] {
		msgStart, msgEnd := projectMemoryMessageTurnBounds(msg)
		if msgStart < start {
			start = msgStart
		}
		if msgEnd > end {
			end = msgEnd
		}
	}
	return start, end
}

func messageTurnBounds(msg dbpkg.Message) (int, int) {
	start := int(msg.TurnNumber)
	end := int(msg.TurnNumber)
	if msg.IsSummary != 0 {
		if msg.CompressedTurnStart.Valid {
			start = int(msg.CompressedTurnStart.Int64)
		}
		if msg.CompressedTurnEnd.Valid {
			end = int(msg.CompressedTurnEnd.Int64)
		}
	}
	return start, end
}

func projectMemoryMessageTurnBounds(msg projectmemory.Message) (int, int) {
	start := int(msg.TurnNumber)
	end := int(msg.TurnNumber)
	if msg.IsSummary {
		var metadata struct {
			CompressedTurnStart int `json:"compressed_turn_start"`
			CompressedTurnEnd   int `json:"compressed_turn_end"`
		}
		if err := json.Unmarshal([]byte(msg.MetadataJSON), &metadata); err == nil {
			if metadata.CompressedTurnStart > 0 {
				start = metadata.CompressedTurnStart
			}
			if metadata.CompressedTurnEnd > 0 {
				end = metadata.CompressedTurnEnd
			}
		}
	}
	return start, end
}

func (e *CompressionEngine) prepareSummary(ctx stdctx.Context, conversationID string, middle []dbpkg.Message, cfg config.ContextConfig) (string, bool, bool, error) {
	summary, err := e.generateSummary(ctx, conversationID, middle, cfg)
	if err == nil {
		return prefixedCompressionSummary(summary), true, false, nil
	}
	if ctx.Err() != nil {
		return "", false, false, fmt.Errorf("compression engine: summarize middle turns: %w", ctx.Err())
	}
	slog.Warn(
		"context compression summarization failed",
		"provider", compressionProviderName(e.provider),
		"model", compressionModel(cfg),
		"error", err,
		"messages", len(middle),
	)
	return "", false, true, nil
}

func (e *CompressionEngine) generateSummary(ctx stdctx.Context, conversationID string, middle []dbpkg.Message, cfg config.ContextConfig) (string, error) {
	if e.provider == nil {
		return "", errors.New("compression provider is nil")
	}
	prompt := buildCompressionPrompt(middle)
	response, err := e.provider.Complete(ctx, &provider.Request{
		Messages:       []provider.Message{provider.NewUserMessage(prompt)},
		Model:          compressionModel(cfg),
		MaxTokens:      e.summaryMaxTokens,
		Purpose:        "compression",
		ConversationID: conversationID,
		TurnNumber:     lastTurnNumber(middle),
	})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(extractCompressionSummaryText(response))
	if text == "" {
		return "", errors.New("compression summary response contained no text")
	}
	return text, nil
}

func buildCompressionPrompt(messages []dbpkg.Message) string {
	var builder strings.Builder
	builder.WriteString("Summarize the following conversation turns into concise bullet points.\n")
	builder.WriteString("Preserve: file paths mentioned, key decisions made, work completed, and errors with their resolutions.\n")
	builder.WriteString("Do not include code blocks.\n\n")
	for _, msg := range messages {
		builder.WriteString(renderCompressionMessage(msg))
		builder.WriteString("\n")
	}
	return builder.String()
}

func renderCompressionMessage(msg dbpkg.Message) string {
	content := ""
	if msg.Content.Valid {
		content = msg.Content.String
	}
	prefix := fmt.Sprintf("turn=%d iteration=%d role=%s", msg.TurnNumber, msg.Iteration, msg.Role)
	switch msg.Role {
	case "assistant":
		blocks, err := provider.ContentBlocksFromRaw(json.RawMessage(content))
		if err != nil {
			return prefix + ": " + strings.TrimSpace(content)
		}
		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			switch block.Type {
			case "text":
				if text := summarizeAssistantTextForCompression(block.Text); text != "" {
					parts = append(parts, text)
				}
			case "tool_use":
				input := strings.TrimSpace(string(block.Input))
				if input == "" {
					parts = append(parts, fmt.Sprintf("tool_use %s id=%s", block.Name, block.ID))
				} else {
					parts = append(parts, fmt.Sprintf("tool_use %s id=%s input=%s", block.Name, block.ID, input))
				}
			}
		}
		if len(parts) == 0 {
			return prefix + ": [assistant content omitted]"
		}
		return prefix + ": " + strings.Join(parts, " | ")
	case "tool":
		name := msg.ToolName.String
		if !msg.ToolName.Valid || strings.TrimSpace(name) == "" {
			name = "tool"
		}
		return fmt.Sprintf("%s (%s): %s", prefix, name, strings.TrimSpace(content))
	default:
		return prefix + ": " + strings.TrimSpace(content)
	}
}

func summarizeAssistantTextForCompression(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.Contains(text, "[failed_assistant]") {
		return "[assistant stream failure tombstone]"
	}
	if strings.Contains(text, "[interrupted_assistant]") {
		return "[assistant interrupted tombstone]"
	}
	return text
}

func extractCompressionSummaryText(response *provider.Response) string {
	return provider.TextContent(response)
}

func prefixedCompressionSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	summary = strings.TrimPrefix(summary, compressionSummaryPrefix)
	summary = strings.TrimLeft(summary, "\n ")
	return compressionSummaryPrefix + "\n" + summary
}

func survivingToolResultIDs(groups ...[]dbpkg.Message) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, group := range groups {
		for _, msg := range group {
			if msg.Role != "tool" || !msg.ToolUseID.Valid || strings.TrimSpace(msg.ToolUseID.String) == "" {
				continue
			}
			ids[msg.ToolUseID.String] = struct{}{}
		}
	}
	return ids
}

func projectMemorySurvivingToolResultIDs(groups ...[]projectmemory.Message) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, group := range groups {
		for _, msg := range group {
			if msg.Role != "tool" || strings.TrimSpace(msg.ToolUseID) == "" {
				continue
			}
			ids[msg.ToolUseID] = struct{}{}
		}
	}
	return ids
}

func survivingAssistantToolUseIDs(groups ...[]dbpkg.Message) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, group := range groups {
		for _, msg := range group {
			if msg.Role != "assistant" || !msg.Content.Valid || strings.TrimSpace(msg.Content.String) == "" {
				continue
			}
			blocks, err := provider.ContentBlocksFromRaw(json.RawMessage(msg.Content.String))
			if err != nil {
				continue
			}
			for _, block := range blocks {
				if block.Type == "tool_use" && block.ID != "" {
					ids[block.ID] = struct{}{}
				}
			}
		}
	}
	return ids
}

func projectMemorySurvivingAssistantToolUseIDs(groups ...[]projectmemory.Message) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, group := range groups {
		for _, msg := range group {
			if msg.Role != "assistant" || strings.TrimSpace(msg.Content) == "" {
				continue
			}
			blocks, err := provider.ContentBlocksFromRaw(json.RawMessage(msg.Content))
			if err != nil {
				continue
			}
			for _, block := range blocks {
				if block.Type == "tool_use" && block.ID != "" {
					ids[block.ID] = struct{}{}
				}
			}
		}
	}
	return ids
}

func survivingAssistantMessages(groups ...[]dbpkg.Message) []dbpkg.Message {
	assistants := make([]dbpkg.Message, 0)
	for _, group := range groups {
		for _, msg := range group {
			if msg.Role == "assistant" {
				assistants = append(assistants, msg)
			}
		}
	}
	return assistants
}

func projectMemorySurvivingAssistantMessages(groups ...[]projectmemory.Message) []projectmemory.Message {
	assistants := make([]projectmemory.Message, 0)
	for _, group := range groups {
		for _, msg := range group {
			if msg.Role == "assistant" {
				assistants = append(assistants, msg)
			}
		}
	}
	return assistants
}

func sanitizeAssistantMessageContent(raw string, survivingToolResults map[string]struct{}) (string, bool, error) {
	blocks, err := provider.ContentBlocksFromRaw(json.RawMessage(raw))
	if err != nil {
		return "", false, err
	}
	filtered := make([]provider.ContentBlock, 0, len(blocks))
	changed := false
	for _, block := range blocks {
		if block.Type != "tool_use" {
			filtered = append(filtered, block)
			continue
		}
		if _, ok := survivingToolResults[block.ID]; ok {
			filtered = append(filtered, block)
			continue
		}
		changed = true
	}
	if !changed {
		return raw, false, nil
	}
	if len(filtered) == 0 {
		filtered = []provider.ContentBlock{provider.NewTextBlock("[Tool call removed after context compaction]")}
	}
	encoded, err := json.Marshal(filtered)
	if err != nil {
		return "", false, fmt.Errorf("marshal sanitized assistant content: %w", err)
	}
	return string(encoded), true, nil
}

func projectMemoryMessagesToDB(messages []projectmemory.Message) []dbpkg.Message {
	out := make([]dbpkg.Message, 0, len(messages))
	for _, msg := range messages {
		dbMsg := dbpkg.Message{
			ConversationID: msg.ConversationID,
			Role:           msg.Role,
			Content:        sql.NullString{String: msg.Content, Valid: msg.Content != ""},
			ToolUseID:      sql.NullString{String: msg.ToolUseID, Valid: msg.ToolUseID != ""},
			ToolName:       sql.NullString{String: msg.ToolName, Valid: msg.ToolName != ""},
			TurnNumber:     int64(msg.TurnNumber),
			Iteration:      int64(msg.Iteration),
			Sequence:       float64(msg.Sequence),
		}
		if msg.Compressed {
			dbMsg.IsCompressed = 1
		}
		if msg.IsSummary {
			dbMsg.IsSummary = 1
			start, end := projectMemoryMessageTurnBounds(msg)
			dbMsg.CompressedTurnStart = sql.NullInt64{Int64: int64(start), Valid: start > 0}
			dbMsg.CompressedTurnEnd = sql.NullInt64{Int64: int64(end), Valid: end > 0}
		}
		out = append(out, dbMsg)
	}
	return out
}

func lastTurnNumber(messages []dbpkg.Message) int {
	if len(messages) == 0 {
		return 0
	}
	last := 0
	for _, msg := range messages {
		if int(msg.TurnNumber) > last {
			last = int(msg.TurnNumber)
		}
		if msg.IsSummary != 0 && msg.CompressedTurnEnd.Valid && int(msg.CompressedTurnEnd.Int64) > last {
			last = int(msg.CompressedTurnEnd.Int64)
		}
	}
	return last
}

func exceedsCompressionThreshold(tokenCount int, modelContextLimit int, cfg config.ContextConfig) bool {
	if tokenCount <= 0 || modelContextLimit <= 0 {
		return false
	}
	return float64(tokenCount) > float64(modelContextLimit)*compressionThreshold(cfg)
}

// approximateTokensFromChars estimates token count from a character count using
// the common heuristic of ~4 characters per token. The +3 provides ceiling-division
// rounding. Used for preflight compression checks where exact counts are unavailable.
func approximateTokensFromChars(totalChars int) int {
	if totalChars <= 0 {
		return 0
	}
	return (totalChars + 3) / 4
}

func compressionThreshold(cfg config.ContextConfig) float64 {
	if cfg.CompressionThreshold > 0 {
		return cfg.CompressionThreshold
	}
	return 0.5
}

func compressionHeadPreserve(cfg config.ContextConfig) int {
	if cfg.CompressionHeadPreserve > 0 {
		return cfg.CompressionHeadPreserve
	}
	return defaultCompressionHeadPreserve
}

func compressionTailPreserve(cfg config.ContextConfig) int {
	if cfg.CompressionTailPreserve > 0 {
		return cfg.CompressionTailPreserve
	}
	return defaultCompressionTailPreserve
}

func compressionModel(cfg config.ContextConfig) string {
	if strings.TrimSpace(cfg.CompressionModel) != "" {
		return cfg.CompressionModel
	}
	return defaultCompressionModel
}

func compressionProviderName(p provider.Provider) string {
	if p == nil {
		return "none"
	}
	name := strings.TrimSpace(p.Name())
	if name == "" {
		return "unknown"
	}
	return name
}
