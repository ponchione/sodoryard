package projectmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ponchione/shunter/types"
)

const (
	DefaultProjectID  = "default"
	maxDocumentChunk  = 32 * 1024
	emptyJSONArray    = "[]"
	emptyJSONObject   = "{}"
	documentKindBrain = "document"
)

type Document struct {
	Path         string
	Kind         string
	Title        string
	ContentHash  string
	ContentSize  uint64
	ChunkCount   uint32
	CreatedAtUS  uint64
	UpdatedAtUS  uint64
	Deleted      bool
	TagsJSON     string
	MetadataJSON string
}

type documentChunk struct {
	ChunkID    string
	Path       string
	ChunkIndex uint32
	Body       string
	BodyHash   string
}

type BrainIndexState struct {
	ProjectID       string
	LastIndexedAtUS uint64
	Dirty           bool
	DirtySinceUS    uint64
	DirtyReason     string
	MetadataJSON    string
}

type CodeIndexState struct {
	ProjectID         string
	LastIndexedCommit string
	LastIndexedAtUS   uint64
	Dirty             bool
	DirtyReason       string
	MetadataJSON      string
}

type CodeFileIndexState struct {
	FileID          string
	ProjectID       string
	FilePath        string
	FileHash        string
	ChunkCount      uint32
	LastIndexedAtUS uint64
}

type Conversation struct {
	ID           string
	ProjectID    string
	Title        string
	CreatedAtUS  uint64
	UpdatedAtUS  uint64
	Provider     string
	Model        string
	SettingsJSON string
	Deleted      bool
}

type Message struct {
	ID             string
	ConversationID string
	TurnNumber     uint32
	Iteration      uint32
	Sequence       uint64
	Role           string
	Content        string
	ToolUseID      string
	ToolName       string
	CreatedAtUS    uint64
	Visible        bool
	Compressed     bool
	IsSummary      bool
	SummaryOfJSON  string
	MetadataJSON   string
}

type SubCall struct {
	ID                  string
	ConversationID      string
	MessageID           string
	TurnNumber          uint32
	Iteration           uint32
	Provider            string
	Model               string
	Purpose             string
	Status              string
	StartedAtUS         uint64
	CompletedAtUS       uint64
	TokensIn            uint64
	TokensOut           uint64
	CacheReadTokens     uint64
	CacheCreationTokens uint64
	LatencyMs           uint64
	Error               string
	MetadataJSON        string
}

type ToolExecution struct {
	ID             string
	ConversationID string
	TurnNumber     uint32
	Iteration      uint32
	ToolUseID      string
	ToolName       string
	Status         string
	StartedAtUS    uint64
	CompletedAtUS  uint64
	DurationMs     uint64
	InputJSON      string
	OutputSize     uint64
	NormalizedSize uint64
	Error          string
	MetadataJSON   string
}

type ContextReport struct {
	ID             string
	ConversationID string
	TurnNumber     uint32
	CreatedAtUS    uint64
	UpdatedAtUS    uint64
	RequestJSON    string
	ReportJSON     string
	QualityJSON    string
}

func documentRow(doc Document) types.ProductValue {
	return types.ProductValue{
		types.NewString(doc.Path),
		types.NewString(doc.Kind),
		types.NewString(doc.Title),
		types.NewString(doc.ContentHash),
		types.NewUint64(doc.ContentSize),
		types.NewUint32(doc.ChunkCount),
		types.NewUint64(doc.CreatedAtUS),
		types.NewUint64(doc.UpdatedAtUS),
		types.NewBool(doc.Deleted),
		types.NewString(defaultString(doc.TagsJSON, emptyJSONArray)),
		types.NewString(defaultString(doc.MetadataJSON, emptyJSONObject)),
	}
}

func decodeDocumentRow(row types.ProductValue) Document {
	return Document{
		Path:         row[0].AsString(),
		Kind:         row[1].AsString(),
		Title:        row[2].AsString(),
		ContentHash:  row[3].AsString(),
		ContentSize:  row[4].AsUint64(),
		ChunkCount:   row[5].AsUint32(),
		CreatedAtUS:  row[6].AsUint64(),
		UpdatedAtUS:  row[7].AsUint64(),
		Deleted:      row[8].AsBool(),
		TagsJSON:     row[9].AsString(),
		MetadataJSON: row[10].AsString(),
	}
}

func chunkRow(chunk documentChunk) types.ProductValue {
	return types.ProductValue{
		types.NewString(chunk.ChunkID),
		types.NewString(chunk.Path),
		types.NewUint32(chunk.ChunkIndex),
		types.NewString(chunk.Body),
		types.NewString(chunk.BodyHash),
	}
}

func decodeChunkRow(row types.ProductValue) documentChunk {
	return documentChunk{
		ChunkID:    row[0].AsString(),
		Path:       row[1].AsString(),
		ChunkIndex: row[2].AsUint32(),
		Body:       row[3].AsString(),
		BodyHash:   row[4].AsString(),
	}
}

func revisionRow(revisionID string, path string, revision uint32, contentHash string, operationID string, createdAtUS uint64, summary string, actor string) types.ProductValue {
	return types.ProductValue{
		types.NewString(revisionID),
		types.NewString(path),
		types.NewUint32(revision),
		types.NewString(contentHash),
		types.NewString(operationID),
		types.NewUint64(createdAtUS),
		types.NewString(summary),
		types.NewString(actor),
	}
}

func operationRow(operationID string, operationType string, path string, actor string, createdAtUS uint64, beforeHash string, afterHash string, payloadJSON string) types.ProductValue {
	return types.ProductValue{
		types.NewString(operationID),
		types.NewString(operationType),
		types.NewString(path),
		types.NewString(actor),
		types.NewUint64(createdAtUS),
		types.NewString(beforeHash),
		types.NewString(afterHash),
		types.NewString(payloadJSON),
	}
}

func brainIndexStateRow(projectID string, lastIndexedAtUS uint64, dirty bool, dirtySinceUS uint64, dirtyReason string, metadataJSON string) types.ProductValue {
	return types.ProductValue{
		types.NewString(projectID),
		types.NewUint64(lastIndexedAtUS),
		types.NewBool(dirty),
		types.NewUint64(dirtySinceUS),
		types.NewString(dirtyReason),
		types.NewString(defaultString(metadataJSON, emptyJSONObject)),
	}
}

func decodeBrainIndexStateRow(row types.ProductValue) BrainIndexState {
	return BrainIndexState{
		ProjectID:       row[0].AsString(),
		LastIndexedAtUS: row[1].AsUint64(),
		Dirty:           row[2].AsBool(),
		DirtySinceUS:    row[3].AsUint64(),
		DirtyReason:     row[4].AsString(),
		MetadataJSON:    row[5].AsString(),
	}
}

func codeIndexStateRow(projectID string, lastIndexedCommit string, lastIndexedAtUS uint64, dirty bool, dirtyReason string, metadataJSON string) types.ProductValue {
	return types.ProductValue{
		types.NewString(projectID),
		types.NewString(lastIndexedCommit),
		types.NewUint64(lastIndexedAtUS),
		types.NewBool(dirty),
		types.NewString(dirtyReason),
		types.NewString(defaultString(metadataJSON, emptyJSONObject)),
	}
}

func decodeCodeIndexStateRow(row types.ProductValue) CodeIndexState {
	return CodeIndexState{
		ProjectID:         row[0].AsString(),
		LastIndexedCommit: row[1].AsString(),
		LastIndexedAtUS:   row[2].AsUint64(),
		Dirty:             row[3].AsBool(),
		DirtyReason:       row[4].AsString(),
		MetadataJSON:      row[5].AsString(),
	}
}

func codeFileIndexStateRow(state CodeFileIndexState) types.ProductValue {
	return types.ProductValue{
		types.NewString(state.FileID),
		types.NewString(state.ProjectID),
		types.NewString(state.FilePath),
		types.NewString(state.FileHash),
		types.NewUint32(state.ChunkCount),
		types.NewUint64(state.LastIndexedAtUS),
	}
}

func decodeCodeFileIndexStateRow(row types.ProductValue) CodeFileIndexState {
	return CodeFileIndexState{
		FileID:          row[0].AsString(),
		ProjectID:       row[1].AsString(),
		FilePath:        row[2].AsString(),
		FileHash:        row[3].AsString(),
		ChunkCount:      row[4].AsUint32(),
		LastIndexedAtUS: row[5].AsUint64(),
	}
}

func conversationRow(conversation Conversation) types.ProductValue {
	return types.ProductValue{
		types.NewString(conversation.ID),
		types.NewString(conversation.ProjectID),
		types.NewString(conversation.Title),
		types.NewUint64(conversation.CreatedAtUS),
		types.NewUint64(conversation.UpdatedAtUS),
		types.NewString(conversation.Provider),
		types.NewString(conversation.Model),
		types.NewString(defaultString(conversation.SettingsJSON, emptyJSONObject)),
		types.NewBool(conversation.Deleted),
	}
}

func decodeConversationRow(row types.ProductValue) Conversation {
	return Conversation{
		ID:           row[0].AsString(),
		ProjectID:    row[1].AsString(),
		Title:        row[2].AsString(),
		CreatedAtUS:  row[3].AsUint64(),
		UpdatedAtUS:  row[4].AsUint64(),
		Provider:     row[5].AsString(),
		Model:        row[6].AsString(),
		SettingsJSON: row[7].AsString(),
		Deleted:      row[8].AsBool(),
	}
}

func messageRow(message Message) types.ProductValue {
	return types.ProductValue{
		types.NewString(message.ID),
		types.NewString(message.ConversationID),
		types.NewUint32(message.TurnNumber),
		types.NewUint32(message.Iteration),
		types.NewUint64(message.Sequence),
		types.NewString(message.Role),
		types.NewString(message.Content),
		types.NewString(message.ToolUseID),
		types.NewString(message.ToolName),
		types.NewUint64(message.CreatedAtUS),
		types.NewBool(message.Visible),
		types.NewBool(message.Compressed),
		types.NewBool(message.IsSummary),
		types.NewString(defaultString(message.SummaryOfJSON, emptyJSONArray)),
		types.NewString(defaultString(message.MetadataJSON, emptyJSONObject)),
	}
}

func decodeMessageRow(row types.ProductValue) Message {
	return Message{
		ID:             row[0].AsString(),
		ConversationID: row[1].AsString(),
		TurnNumber:     row[2].AsUint32(),
		Iteration:      row[3].AsUint32(),
		Sequence:       row[4].AsUint64(),
		Role:           row[5].AsString(),
		Content:        row[6].AsString(),
		ToolUseID:      row[7].AsString(),
		ToolName:       row[8].AsString(),
		CreatedAtUS:    row[9].AsUint64(),
		Visible:        row[10].AsBool(),
		Compressed:     row[11].AsBool(),
		IsSummary:      row[12].AsBool(),
		SummaryOfJSON:  row[13].AsString(),
		MetadataJSON:   row[14].AsString(),
	}
}

func subCallRow(subCall SubCall) types.ProductValue {
	return types.ProductValue{
		types.NewString(subCall.ID),
		types.NewString(subCall.ConversationID),
		types.NewString(subCall.MessageID),
		types.NewUint32(subCall.TurnNumber),
		types.NewUint32(subCall.Iteration),
		types.NewString(subCall.Provider),
		types.NewString(subCall.Model),
		types.NewString(subCall.Purpose),
		types.NewString(subCall.Status),
		types.NewUint64(subCall.StartedAtUS),
		types.NewUint64(subCall.CompletedAtUS),
		types.NewUint64(subCall.TokensIn),
		types.NewUint64(subCall.TokensOut),
		types.NewUint64(subCall.CacheReadTokens),
		types.NewUint64(subCall.CacheCreationTokens),
		types.NewUint64(subCall.LatencyMs),
		types.NewString(subCall.Error),
		types.NewString(defaultString(subCall.MetadataJSON, emptyJSONObject)),
	}
}

func decodeSubCallRow(row types.ProductValue) SubCall {
	return SubCall{
		ID:                  row[0].AsString(),
		ConversationID:      row[1].AsString(),
		MessageID:           row[2].AsString(),
		TurnNumber:          row[3].AsUint32(),
		Iteration:           row[4].AsUint32(),
		Provider:            row[5].AsString(),
		Model:               row[6].AsString(),
		Purpose:             row[7].AsString(),
		Status:              row[8].AsString(),
		StartedAtUS:         row[9].AsUint64(),
		CompletedAtUS:       row[10].AsUint64(),
		TokensIn:            row[11].AsUint64(),
		TokensOut:           row[12].AsUint64(),
		CacheReadTokens:     row[13].AsUint64(),
		CacheCreationTokens: row[14].AsUint64(),
		LatencyMs:           row[15].AsUint64(),
		Error:               row[16].AsString(),
		MetadataJSON:        row[17].AsString(),
	}
}

func toolExecutionRow(execution ToolExecution) types.ProductValue {
	return types.ProductValue{
		types.NewString(execution.ID),
		types.NewString(execution.ConversationID),
		types.NewUint32(execution.TurnNumber),
		types.NewUint32(execution.Iteration),
		types.NewString(execution.ToolUseID),
		types.NewString(execution.ToolName),
		types.NewString(execution.Status),
		types.NewUint64(execution.StartedAtUS),
		types.NewUint64(execution.CompletedAtUS),
		types.NewUint64(execution.DurationMs),
		types.NewString(execution.InputJSON),
		types.NewUint64(execution.OutputSize),
		types.NewUint64(execution.NormalizedSize),
		types.NewString(execution.Error),
		types.NewString(defaultString(execution.MetadataJSON, emptyJSONObject)),
	}
}

func decodeToolExecutionRow(row types.ProductValue) ToolExecution {
	return ToolExecution{
		ID:             row[0].AsString(),
		ConversationID: row[1].AsString(),
		TurnNumber:     row[2].AsUint32(),
		Iteration:      row[3].AsUint32(),
		ToolUseID:      row[4].AsString(),
		ToolName:       row[5].AsString(),
		Status:         row[6].AsString(),
		StartedAtUS:    row[7].AsUint64(),
		CompletedAtUS:  row[8].AsUint64(),
		DurationMs:     row[9].AsUint64(),
		InputJSON:      row[10].AsString(),
		OutputSize:     row[11].AsUint64(),
		NormalizedSize: row[12].AsUint64(),
		Error:          row[13].AsString(),
		MetadataJSON:   row[14].AsString(),
	}
}

func contextReportRow(report ContextReport) types.ProductValue {
	return types.ProductValue{
		types.NewString(report.ID),
		types.NewString(report.ConversationID),
		types.NewUint32(report.TurnNumber),
		types.NewUint64(report.CreatedAtUS),
		types.NewUint64(report.UpdatedAtUS),
		types.NewString(defaultString(report.RequestJSON, emptyJSONObject)),
		types.NewString(defaultString(report.ReportJSON, emptyJSONObject)),
		types.NewString(defaultString(report.QualityJSON, emptyJSONObject)),
	}
}

func decodeContextReportRow(row types.ProductValue) ContextReport {
	return ContextReport{
		ID:             row[0].AsString(),
		ConversationID: row[1].AsString(),
		TurnNumber:     row[2].AsUint32(),
		CreatedAtUS:    row[3].AsUint64(),
		UpdatedAtUS:    row[4].AsUint64(),
		RequestJSON:    row[5].AsString(),
		ReportJSON:     row[6].AsString(),
		QualityJSON:    row[7].AsString(),
	}
}

func splitDocumentChunks(path string, content string) []documentChunk {
	if content == "" {
		return nil
	}
	chunks := make([]documentChunk, 0, (len(content)/maxDocumentChunk)+1)
	for start, index := 0, 0; start < len(content); index++ {
		end := start + maxDocumentChunk
		if end > len(content) {
			end = len(content)
		}
		body := content[start:end]
		chunks = append(chunks, documentChunk{
			ChunkID:    documentChunkID(path, uint32(index)),
			Path:       path,
			ChunkIndex: uint32(index),
			Body:       body,
			BodyHash:   contentHash(body),
		})
		start = end
	}
	return chunks
}

func joinDocumentChunks(chunks []documentChunk) string {
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].ChunkIndex < chunks[j].ChunkIndex
	})
	var b strings.Builder
	for _, chunk := range chunks {
		b.WriteString(chunk.Body)
	}
	return b.String()
}

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func documentChunkID(path string, index uint32) string {
	return fmt.Sprintf("%s:%08d", stableID(path), index)
}

func documentRevisionID(path string, revision uint32) string {
	return fmt.Sprintf("%s:%08d", stableID(path), revision)
}

func memoryOperationID(operationType string, path string, actor string, atUS uint64, beforeHash string, afterHash string) string {
	return stableID(strings.Join([]string{operationType, path, actor, fmt.Sprint(atUS), beforeHash, afterHash}, "\x00"))
}

func CodeFileIndexID(projectID string, filePath string) string {
	return stableID(strings.Join([]string{projectID, filePath}, "\x00"))
}

func MessageID(conversationID string, sequence uint64) string {
	return fmt.Sprintf("%s:%020d", stableID(conversationID), sequence)
}

func CompressionSummaryMessageID(conversationID string, sequence uint64, createdAtUS uint64) string {
	return fmt.Sprintf("%s:summary:%020d:%020d", stableID(conversationID), sequence, createdAtUS)
}

func SubCallID(parts ...string) string {
	return stableID(strings.Join(parts, "\x00"))
}

func ToolExecutionID(conversationID string, turnNumber uint32, iteration uint32, toolUseID string, toolName string) string {
	return stableID(strings.Join([]string{conversationID, fmt.Sprint(turnNumber), fmt.Sprint(iteration), toolUseID, toolName}, "\x00"))
}

func ContextReportID(conversationID string, turnNumber uint32) string {
	return stableID(strings.Join([]string{conversationID, fmt.Sprint(turnNumber)}, "\x00"))
}

func stableID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func normalizeDocumentPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(filepath.ToSlash(raw))
	if trimmed == "" {
		return "", fmt.Errorf("document path is required")
	}
	if strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("document path must be relative: %s", raw)
	}
	cleaned := filepath.ToSlash(filepath.Clean(trimmed))
	if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("document path escapes brain root: %s", raw)
	}
	return strings.TrimPrefix(cleaned, "./"), nil
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
