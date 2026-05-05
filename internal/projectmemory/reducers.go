package projectmemory

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ponchione/shunter/schema"
	"github.com/ponchione/shunter/types"
)

type WriteDocumentArgs struct {
	Path         string `json:"path"`
	Content      string `json:"content"`
	Actor        string `json:"actor"`
	Kind         string `json:"kind"`
	Title        string `json:"title"`
	TagsJSON     string `json:"tags_json"`
	MetadataJSON string `json:"metadata_json"`
}

type PatchDocumentArgs struct {
	Path            string `json:"path"`
	Operation       string `json:"operation"`
	ExpectedOldHash string `json:"expected_old_hash"`
	NewContent      string `json:"new_content"`
	Actor           string `json:"actor"`
	Kind            string `json:"kind"`
	Title           string `json:"title"`
	TagsJSON        string `json:"tags_json"`
	MetadataJSON    string `json:"metadata_json"`
}

type DeleteDocumentArgs struct {
	Path            string `json:"path"`
	ExpectedOldHash string `json:"expected_old_hash"`
	Actor           string `json:"actor"`
}

type ImportDocumentsBatchArgs struct {
	Documents []WriteDocumentArgs `json:"documents"`
	Actor     string              `json:"actor"`
}

type MarkBrainIndexDirtyArgs struct {
	ProjectID string `json:"project_id"`
	Reason    string `json:"reason"`
}

type MarkBrainIndexCleanArgs struct {
	ProjectID       string `json:"project_id"`
	LastIndexedAtUS uint64 `json:"last_indexed_at_us"`
	MetadataJSON    string `json:"metadata_json"`
}

type MarkCodeIndexDirtyArgs struct {
	ProjectID string `json:"project_id"`
	Reason    string `json:"reason"`
}

type MarkCodeIndexCleanArgs struct {
	ProjectID         string             `json:"project_id"`
	LastIndexedCommit string             `json:"last_indexed_commit"`
	LastIndexedAtUS   uint64             `json:"last_indexed_at_us"`
	Files             []CodeFileIndexArg `json:"files"`
	DeletedPaths      []string           `json:"deleted_paths"`
	MetadataJSON      string             `json:"metadata_json"`
}

type CodeFileIndexArg struct {
	FilePath   string `json:"file_path"`
	FileHash   string `json:"file_hash"`
	ChunkCount uint32 `json:"chunk_count"`
}

type CreateConversationArgs struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Title        string `json:"title"`
	Model        string `json:"model"`
	Provider     string `json:"provider"`
	CreatedAtUS  uint64 `json:"created_at_us"`
	SettingsJSON string `json:"settings_json"`
}

type DeleteConversationArgs struct {
	ID string `json:"id"`
}

type SetConversationTitleArgs struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	UpdatedAtUS uint64 `json:"updated_at_us"`
}

type SetRuntimeDefaultsArgs struct {
	ID          string `json:"id"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	UpdatedAtUS uint64 `json:"updated_at_us"`
}

type AppendUserMessageArgs struct {
	ConversationID string `json:"conversation_id"`
	TurnNumber     uint32 `json:"turn_number"`
	Content        string `json:"content"`
	CreatedAtUS    uint64 `json:"created_at_us"`
}

type PersistIterationArgs struct {
	ConversationID string                    `json:"conversation_id"`
	TurnNumber     uint32                    `json:"turn_number"`
	Iteration      uint32                    `json:"iteration"`
	Messages       []PersistIterationMessage `json:"messages"`
	CreatedAtUS    uint64                    `json:"created_at_us"`
}

type PersistIterationMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	ToolUseID string `json:"tool_use_id"`
	ToolName  string `json:"tool_name"`
}

type CompressMessagesArgs struct {
	ConversationID      string                         `json:"conversation_id"`
	MessageIDs          []string                       `json:"message_ids"`
	SanitizedMessages   []CompressMessageContentUpdate `json:"sanitized_messages,omitempty"`
	SummaryContent      string                         `json:"summary_content,omitempty"`
	SummaryTurnNumber   uint32                         `json:"summary_turn_number,omitempty"`
	SummaryIteration    uint32                         `json:"summary_iteration,omitempty"`
	SummarySequence     uint64                         `json:"summary_sequence,omitempty"`
	SummaryOfJSON       string                         `json:"summary_of_json,omitempty"`
	SummaryMetadataJSON string                         `json:"summary_metadata_json,omitempty"`
	CreatedAtUS         uint64                         `json:"created_at_us"`
}

type CompressMessageContentUpdate struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type CancelIterationArgs struct {
	ConversationID string `json:"conversation_id"`
	TurnNumber     uint32 `json:"turn_number"`
	Iteration      uint32 `json:"iteration"`
}

type DiscardTurnArgs struct {
	ConversationID string `json:"conversation_id"`
	TurnNumber     uint32 `json:"turn_number"`
}

type RecordSubCallArgs struct {
	ID                  string `json:"id"`
	ConversationID      string `json:"conversation_id"`
	MessageID           string `json:"message_id"`
	TurnNumber          uint32 `json:"turn_number"`
	Iteration           uint32 `json:"iteration"`
	Provider            string `json:"provider"`
	Model               string `json:"model"`
	Purpose             string `json:"purpose"`
	Status              string `json:"status"`
	StartedAtUS         uint64 `json:"started_at_us"`
	CompletedAtUS       uint64 `json:"completed_at_us"`
	TokensIn            uint64 `json:"tokens_in"`
	TokensOut           uint64 `json:"tokens_out"`
	CacheReadTokens     uint64 `json:"cache_read_tokens"`
	CacheCreationTokens uint64 `json:"cache_creation_tokens"`
	LatencyMs           uint64 `json:"latency_ms"`
	Error               string `json:"error"`
	MetadataJSON        string `json:"metadata_json"`
}

type RecordToolExecutionArgs struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	TurnNumber     uint32 `json:"turn_number"`
	Iteration      uint32 `json:"iteration"`
	ToolUseID      string `json:"tool_use_id"`
	ToolName       string `json:"tool_name"`
	Status         string `json:"status"`
	StartedAtUS    uint64 `json:"started_at_us"`
	CompletedAtUS  uint64 `json:"completed_at_us"`
	DurationMs     uint64 `json:"duration_ms"`
	InputJSON      string `json:"input_json"`
	OutputSize     uint64 `json:"output_size"`
	NormalizedSize uint64 `json:"normalized_size"`
	Error          string `json:"error"`
	MetadataJSON   string `json:"metadata_json"`
}

type StoreContextReportArgs struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	TurnNumber     uint32 `json:"turn_number"`
	CreatedAtUS    uint64 `json:"created_at_us"`
	UpdatedAtUS    uint64 `json:"updated_at_us"`
	RequestJSON    string `json:"request_json"`
	ReportJSON     string `json:"report_json"`
	QualityJSON    string `json:"quality_json"`
}

type UpdateContextReportQualityArgs struct {
	ConversationID string `json:"conversation_id"`
	TurnNumber     uint32 `json:"turn_number"`
	UpdatedAtUS    uint64 `json:"updated_at_us"`
	QualityJSON    string `json:"quality_json"`
}

type reducerResult struct {
	Path        string `json:"path,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
	Revision    uint32 `json:"revision,omitempty"`
	OperationID string `json:"operation_id,omitempty"`
}

func writeDocumentReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args WriteDocumentArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	return upsertDocument(ctx, documentMutation{
		OperationType: "write_document",
		Path:          args.Path,
		Content:       args.Content,
		Actor:         args.Actor,
		Kind:          args.Kind,
		Title:         args.Title,
		TagsJSON:      args.TagsJSON,
		MetadataJSON:  args.MetadataJSON,
	})
}

func patchDocumentReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args PatchDocumentArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	path, err := normalizeDocumentPath(args.Path)
	if err != nil {
		return nil, err
	}
	_, current, found := findDocumentByPath(ctx.DB, path)
	if !found || current.Deleted {
		return nil, fmt.Errorf("Document not found: %s", path)
	}
	if strings.TrimSpace(args.ExpectedOldHash) == "" {
		return nil, fmt.Errorf("expected_old_hash is required")
	}
	if current.ContentHash != args.ExpectedOldHash {
		return nil, fmt.Errorf("patch conflict for %s: expected old hash %s, current hash %s", path, args.ExpectedOldHash, current.ContentHash)
	}
	return upsertDocument(ctx, documentMutation{
		OperationType: "patch_document",
		Path:          path,
		Content:       args.NewContent,
		Actor:         args.Actor,
		Kind:          firstNonEmpty(args.Kind, current.Kind),
		Title:         firstNonEmpty(args.Title, current.Title),
		TagsJSON:      firstNonEmpty(args.TagsJSON, current.TagsJSON),
		MetadataJSON:  firstNonEmpty(args.MetadataJSON, current.MetadataJSON),
	})
}

func deleteDocumentReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args DeleteDocumentArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	path, err := normalizeDocumentPath(args.Path)
	if err != nil {
		return nil, err
	}
	rowID, current, found := findDocumentByPath(ctx.DB, path)
	if !found || current.Deleted {
		return nil, fmt.Errorf("Document not found: %s", path)
	}
	if args.ExpectedOldHash != "" && current.ContentHash != args.ExpectedOldHash {
		return nil, fmt.Errorf("delete conflict for %s: expected old hash %s, current hash %s", path, args.ExpectedOldHash, current.ContentHash)
	}
	nowUS := reducerNowUS(ctx)
	beforeHash := current.ContentHash
	operationID := memoryOperationID("delete_document", path, args.Actor, nowUS, beforeHash, "")
	deleteChunksForPath(ctx.DB, path)
	current.Deleted = true
	current.UpdatedAtUS = nowUS
	current.ContentHash = ""
	current.ContentSize = 0
	current.ChunkCount = 0
	if _, err := ctx.DB.Update(uint32(tableDocuments), rowID, documentRow(current)); err != nil {
		return nil, err
	}
	revision := nextRevision(ctx.DB, path)
	if _, err := ctx.DB.Insert(uint32(tableDocumentRevisions), revisionRow(documentRevisionID(path, revision), path, revision, "", operationID, nowUS, "delete_document", args.Actor)); err != nil {
		return nil, err
	}
	if _, err := ctx.DB.Insert(uint32(tableMemoryOperations), operationRow(operationID, "delete_document", path, args.Actor, nowUS, beforeHash, "", mustJSON(args))); err != nil {
		return nil, err
	}
	if err := markBrainIndexDirty(ctx.DB, DefaultProjectID, "delete_document", nowUS); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{Path: path, Revision: revision, OperationID: operationID})
}

func importDocumentsBatchReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args ImportDocumentsBatchArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	for _, doc := range args.Documents {
		if strings.TrimSpace(doc.Actor) == "" {
			doc.Actor = args.Actor
		}
		if _, err := upsertDocument(ctx, documentMutation{
			OperationType: "import_documents_batch",
			Path:          doc.Path,
			Content:       doc.Content,
			Actor:         doc.Actor,
			Kind:          doc.Kind,
			Title:         doc.Title,
			TagsJSON:      doc.TagsJSON,
			MetadataJSON:  doc.MetadataJSON,
		}); err != nil {
			return nil, err
		}
	}
	return encodeReducerResult(reducerResult{})
}

func markBrainIndexDirtyReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args MarkBrainIndexDirtyArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	projectID := firstNonEmpty(args.ProjectID, DefaultProjectID)
	if err := markBrainIndexDirty(ctx.DB, projectID, args.Reason, reducerNowUS(ctx)); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{})
}

func markBrainIndexCleanReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args MarkBrainIndexCleanArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	projectID := firstNonEmpty(args.ProjectID, DefaultProjectID)
	metadataJSON := defaultString(args.MetadataJSON, emptyJSONObject)
	rowID, _, found := firstRow(ctx.DB.SeekIndex(uint32(tableBrainIndexState), uint32(indexBrainIndexStatePrimary), types.NewString(projectID)))
	row := brainIndexStateRow(projectID, args.LastIndexedAtUS, false, 0, "", metadataJSON)
	if found {
		if _, err := ctx.DB.Update(uint32(tableBrainIndexState), rowID, row); err != nil {
			return nil, err
		}
	} else if _, err := ctx.DB.Insert(uint32(tableBrainIndexState), row); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{})
}

func markCodeIndexDirtyReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args MarkCodeIndexDirtyArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	projectID := firstNonEmpty(args.ProjectID, DefaultProjectID)
	rowID, row, found := firstRow(ctx.DB.SeekIndex(uint32(tableCodeIndexState), uint32(indexCodeIndexStatePrimary), types.NewString(projectID)))
	lastCommit := ""
	lastIndexedAtUS := uint64(0)
	if found {
		state := decodeCodeIndexStateRow(row)
		lastCommit = state.LastIndexedCommit
		lastIndexedAtUS = state.LastIndexedAtUS
	}
	state := codeIndexStateRow(projectID, lastCommit, lastIndexedAtUS, true, args.Reason, emptyJSONObject)
	if found {
		if _, err := ctx.DB.Update(uint32(tableCodeIndexState), rowID, state); err != nil {
			return nil, err
		}
	} else if _, err := ctx.DB.Insert(uint32(tableCodeIndexState), state); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{})
}

func markCodeIndexCleanReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args MarkCodeIndexCleanArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	projectID := firstNonEmpty(args.ProjectID, DefaultProjectID)
	indexedAtUS := args.LastIndexedAtUS
	if indexedAtUS == 0 {
		indexedAtUS = reducerNowUS(ctx)
	}
	for _, deleted := range args.DeletedPaths {
		path := strings.TrimSpace(deleted)
		if path == "" {
			continue
		}
		if rowID, _, found := firstRow(ctx.DB.SeekIndex(uint32(tableCodeFileIndexState), uint32(indexCodeFileIndexStatePrimary), types.NewString(CodeFileIndexID(projectID, path)))); found {
			_ = ctx.DB.Delete(uint32(tableCodeFileIndexState), rowID)
		}
	}
	for _, file := range args.Files {
		path := strings.TrimSpace(file.FilePath)
		if path == "" {
			return nil, fmt.Errorf("code file index path is required")
		}
		fileID := CodeFileIndexID(projectID, path)
		row := codeFileIndexStateRow(CodeFileIndexState{
			FileID:          fileID,
			ProjectID:       projectID,
			FilePath:        path,
			FileHash:        file.FileHash,
			ChunkCount:      file.ChunkCount,
			LastIndexedAtUS: indexedAtUS,
		})
		if rowID, _, found := firstRow(ctx.DB.SeekIndex(uint32(tableCodeFileIndexState), uint32(indexCodeFileIndexStatePrimary), types.NewString(fileID))); found {
			if _, err := ctx.DB.Update(uint32(tableCodeFileIndexState), rowID, row); err != nil {
				return nil, err
			}
		} else if _, err := ctx.DB.Insert(uint32(tableCodeFileIndexState), row); err != nil {
			return nil, err
		}
	}
	stateRowID, _, found := firstRow(ctx.DB.SeekIndex(uint32(tableCodeIndexState), uint32(indexCodeIndexStatePrimary), types.NewString(projectID)))
	state := codeIndexStateRow(projectID, args.LastIndexedCommit, indexedAtUS, false, "", defaultString(args.MetadataJSON, emptyJSONObject))
	if found {
		if _, err := ctx.DB.Update(uint32(tableCodeIndexState), stateRowID, state); err != nil {
			return nil, err
		}
	} else if _, err := ctx.DB.Insert(uint32(tableCodeIndexState), state); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{})
}

func createConversationReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args CreateConversationArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(args.ID)
	if id == "" {
		return nil, fmt.Errorf("conversation id is required")
	}
	projectID := strings.TrimSpace(args.ProjectID)
	if projectID == "" {
		return nil, fmt.Errorf("project id is required")
	}
	if _, _, found := findConversationByID(ctx.DB, id); found {
		return nil, fmt.Errorf("conversation already exists: %s", id)
	}
	nowUS := args.CreatedAtUS
	if nowUS == 0 {
		nowUS = reducerNowUS(ctx)
	}
	row := conversationRow(Conversation{
		ID:           id,
		ProjectID:    projectID,
		Title:        args.Title,
		CreatedAtUS:  nowUS,
		UpdatedAtUS:  nowUS,
		Provider:     args.Provider,
		Model:        args.Model,
		SettingsJSON: defaultString(args.SettingsJSON, emptyJSONObject),
		Deleted:      false,
	})
	if _, err := ctx.DB.Insert(uint32(tableConversations), row); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{})
}

func deleteConversationReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args DeleteConversationArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	rowID, conversation, found := findConversationByID(ctx.DB, strings.TrimSpace(args.ID))
	if !found || conversation.Deleted {
		return nil, fmt.Errorf("conversation not found: %s", args.ID)
	}
	deleteMessagesForConversation(ctx.DB, conversation.ID)
	conversation.Deleted = true
	conversation.UpdatedAtUS = reducerNowUS(ctx)
	if _, err := ctx.DB.Update(uint32(tableConversations), rowID, conversationRow(conversation)); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{})
}

func setConversationTitleReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args SetConversationTitleArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	rowID, conversation, found := findConversationByID(ctx.DB, strings.TrimSpace(args.ID))
	if !found || conversation.Deleted {
		return nil, fmt.Errorf("conversation not found: %s", args.ID)
	}
	conversation.Title = args.Title
	conversation.UpdatedAtUS = nonZeroUS(args.UpdatedAtUS, reducerNowUS(ctx))
	if _, err := ctx.DB.Update(uint32(tableConversations), rowID, conversationRow(conversation)); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{})
}

func setRuntimeDefaultsReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args SetRuntimeDefaultsArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	rowID, conversation, found := findConversationByID(ctx.DB, strings.TrimSpace(args.ID))
	if !found || conversation.Deleted {
		return nil, fmt.Errorf("conversation not found: %s", args.ID)
	}
	conversation.Provider = args.Provider
	conversation.Model = args.Model
	conversation.UpdatedAtUS = nonZeroUS(args.UpdatedAtUS, reducerNowUS(ctx))
	if _, err := ctx.DB.Update(uint32(tableConversations), rowID, conversationRow(conversation)); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{})
}

func appendUserMessageReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args AppendUserMessageArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Content) == "" {
		return nil, fmt.Errorf("user message content is required")
	}
	message, err := insertConversationMessage(ctx.DB, args.ConversationID, Message{
		TurnNumber:  args.TurnNumber,
		Iteration:   1,
		Role:        "user",
		Content:     args.Content,
		CreatedAtUS: nonZeroUS(args.CreatedAtUS, reducerNowUS(ctx)),
		Visible:     true,
	})
	if err != nil {
		return nil, err
	}
	if err := touchConversation(ctx.DB, args.ConversationID, nonZeroUS(args.CreatedAtUS, reducerNowUS(ctx))); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{Revision: uint32(message.Sequence)})
}

func persistIterationReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args PersistIterationArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	if len(args.Messages) == 0 {
		return nil, fmt.Errorf("persist_iteration requires at least one message")
	}
	nowUS := nonZeroUS(args.CreatedAtUS, reducerNowUS(ctx))
	assistantMessageID := ""
	for _, msg := range args.Messages {
		role := strings.TrimSpace(msg.Role)
		if role != "assistant" && role != "tool" {
			return nil, fmt.Errorf("unsupported iteration message role: %s", msg.Role)
		}
		inserted, err := insertConversationMessage(ctx.DB, args.ConversationID, Message{
			TurnNumber:  args.TurnNumber,
			Iteration:   args.Iteration,
			Role:        role,
			Content:     msg.Content,
			ToolUseID:   msg.ToolUseID,
			ToolName:    msg.ToolName,
			CreatedAtUS: nowUS,
			Visible:     true,
		})
		if err != nil {
			return nil, err
		}
		if role == "assistant" {
			assistantMessageID = inserted.ID
		}
	}
	if assistantMessageID != "" {
		if err := linkIterationSubCallsToMessage(ctx.DB, args.ConversationID, args.TurnNumber, args.Iteration, assistantMessageID); err != nil {
			return nil, err
		}
	}
	if err := touchConversation(ctx.DB, args.ConversationID, nowUS); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{})
}

func compressMessagesReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args CompressMessagesArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	conversationID := strings.TrimSpace(args.ConversationID)
	if conversationID == "" {
		return nil, fmt.Errorf("compress_messages requires conversation_id")
	}
	_, conversation, found := findConversationByID(ctx.DB, conversationID)
	if !found || conversation.Deleted {
		return nil, fmt.Errorf("conversation not found: %s", conversationID)
	}

	compressSet := make(map[string]struct{}, len(args.MessageIDs))
	for _, id := range args.MessageIDs {
		if id = strings.TrimSpace(id); id != "" {
			compressSet[id] = struct{}{}
		}
	}
	sanitizeSet := make(map[string]string, len(args.SanitizedMessages))
	for _, update := range args.SanitizedMessages {
		id := strings.TrimSpace(update.ID)
		if id != "" {
			sanitizeSet[id] = update.Content
		}
	}
	if len(compressSet) == 0 && len(sanitizeSet) == 0 && strings.TrimSpace(args.SummaryContent) == "" {
		return nil, fmt.Errorf("compress_messages requires messages or summary content")
	}

	seenCompress := make(map[string]struct{}, len(compressSet))
	seenSanitize := make(map[string]struct{}, len(sanitizeSet))
	for _, row := range ctx.DB.SeekIndex(uint32(tableMessages), uint32(indexMessagesConversation), types.NewString(conversationID)) {
		message := decodeMessageRow(row)
		if _, ok := compressSet[message.ID]; ok {
			seenCompress[message.ID] = struct{}{}
		}
		if _, ok := sanitizeSet[message.ID]; ok {
			seenSanitize[message.ID] = struct{}{}
		}
	}
	for id := range compressSet {
		if _, ok := seenCompress[id]; !ok {
			return nil, fmt.Errorf("compress_messages message not found: %s", id)
		}
	}
	for id := range sanitizeSet {
		if _, ok := seenSanitize[id]; !ok {
			return nil, fmt.Errorf("compress_messages sanitized message not found: %s", id)
		}
	}

	nowUS := nonZeroUS(args.CreatedAtUS, reducerNowUS(ctx))
	for rowID, row := range ctx.DB.SeekIndex(uint32(tableMessages), uint32(indexMessagesConversation), types.NewString(conversationID)) {
		message := decodeMessageRow(row)
		changed := false
		if content, ok := sanitizeSet[message.ID]; ok {
			message.Content = content
			changed = true
		}
		if _, ok := compressSet[message.ID]; ok {
			message.Compressed = true
			message.Visible = true
			changed = true
		}
		if changed {
			if _, err := ctx.DB.Update(uint32(tableMessages), rowID, messageRow(message)); err != nil {
				return nil, err
			}
		}
	}

	if summaryContent := strings.TrimSpace(args.SummaryContent); summaryContent != "" {
		iteration := args.SummaryIteration
		if iteration == 0 {
			iteration = 1
		}
		summaryOfJSON := strings.TrimSpace(args.SummaryOfJSON)
		if summaryOfJSON == "" {
			encoded, err := json.Marshal(args.MessageIDs)
			if err != nil {
				return nil, fmt.Errorf("encode compression summary message ids: %w", err)
			}
			summaryOfJSON = string(encoded)
		}
		summary := Message{
			ID:             CompressionSummaryMessageID(conversationID, args.SummarySequence, nowUS),
			ConversationID: conversationID,
			TurnNumber:     args.SummaryTurnNumber,
			Iteration:      iteration,
			Sequence:       args.SummarySequence,
			Role:           "assistant",
			Content:        summaryContent,
			CreatedAtUS:    nowUS,
			Visible:        true,
			IsSummary:      true,
			SummaryOfJSON:  summaryOfJSON,
			MetadataJSON:   defaultString(args.SummaryMetadataJSON, emptyJSONObject),
		}
		if _, err := ctx.DB.Insert(uint32(tableMessages), messageRow(summary)); err != nil {
			return nil, err
		}
	}
	if err := touchConversation(ctx.DB, conversationID, nowUS); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{})
}

func cancelIterationReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args CancelIterationArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	for rowID, row := range ctx.DB.SeekIndex(uint32(tableMessages), uint32(indexMessagesConversation), types.NewString(args.ConversationID)) {
		message := decodeMessageRow(row)
		if message.TurnNumber == args.TurnNumber && message.Iteration == args.Iteration && message.Role != "user" {
			_ = ctx.DB.Delete(uint32(tableMessages), rowID)
		}
	}
	deleteIterationToolExecutions(ctx.DB, args.ConversationID, args.TurnNumber, args.Iteration)
	deleteIterationSubCalls(ctx.DB, args.ConversationID, args.TurnNumber, args.Iteration)
	return encodeReducerResult(reducerResult{})
}

func discardTurnReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args DiscardTurnArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	for rowID, row := range ctx.DB.SeekIndex(uint32(tableMessages), uint32(indexMessagesConversation), types.NewString(args.ConversationID)) {
		message := decodeMessageRow(row)
		if message.TurnNumber == args.TurnNumber {
			_ = ctx.DB.Delete(uint32(tableMessages), rowID)
		}
	}
	deleteTurnToolExecutions(ctx.DB, args.ConversationID, args.TurnNumber)
	deleteTurnSubCalls(ctx.DB, args.ConversationID, args.TurnNumber)
	deleteTurnContextReports(ctx.DB, args.ConversationID, args.TurnNumber)
	return encodeReducerResult(reducerResult{})
}

func recordSubCallReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args RecordSubCallArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	provider := strings.TrimSpace(args.Provider)
	if provider == "" {
		return nil, fmt.Errorf("sub-call provider is required")
	}
	model := strings.TrimSpace(args.Model)
	if model == "" {
		return nil, fmt.Errorf("sub-call model is required")
	}
	purpose := strings.TrimSpace(args.Purpose)
	if purpose == "" {
		return nil, fmt.Errorf("sub-call purpose is required")
	}
	completedAtUS := nonZeroUS(args.CompletedAtUS, reducerNowUS(ctx))
	startedAtUS := args.StartedAtUS
	if startedAtUS == 0 {
		latencyUS := args.LatencyMs * 1000
		if latencyUS > 0 && completedAtUS > latencyUS {
			startedAtUS = completedAtUS - latencyUS
		} else {
			startedAtUS = completedAtUS
		}
	}
	status := strings.TrimSpace(args.Status)
	if status == "" {
		if strings.TrimSpace(args.Error) != "" {
			status = "error"
		} else {
			status = "success"
		}
	}
	id := strings.TrimSpace(args.ID)
	if id == "" {
		id = SubCallID(args.ConversationID, args.MessageID, fmt.Sprint(args.TurnNumber), fmt.Sprint(args.Iteration), provider, model, purpose, fmt.Sprint(completedAtUS), fmt.Sprint(args.TokensIn), fmt.Sprint(args.TokensOut), args.Error)
	}
	if _, _, found := firstRow(ctx.DB.SeekIndex(uint32(tableSubCalls), uint32(indexSubCallsPrimary), types.NewString(id))); found {
		return nil, fmt.Errorf("sub-call already exists: %s", id)
	}
	if _, err := ctx.DB.Insert(uint32(tableSubCalls), subCallRow(SubCall{
		ID:                  id,
		ConversationID:      strings.TrimSpace(args.ConversationID),
		MessageID:           strings.TrimSpace(args.MessageID),
		TurnNumber:          args.TurnNumber,
		Iteration:           args.Iteration,
		Provider:            provider,
		Model:               model,
		Purpose:             purpose,
		Status:              status,
		StartedAtUS:         startedAtUS,
		CompletedAtUS:       completedAtUS,
		TokensIn:            args.TokensIn,
		TokensOut:           args.TokensOut,
		CacheReadTokens:     args.CacheReadTokens,
		CacheCreationTokens: args.CacheCreationTokens,
		LatencyMs:           args.LatencyMs,
		Error:               args.Error,
		MetadataJSON:        defaultString(args.MetadataJSON, emptyJSONObject),
	})); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{OperationID: id})
}

func recordToolExecutionReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args RecordToolExecutionArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	conversationID := strings.TrimSpace(args.ConversationID)
	if conversationID == "" {
		return nil, fmt.Errorf("tool execution conversation id is required")
	}
	if _, conversation, found := findConversationByID(ctx.DB, conversationID); !found || conversation.Deleted {
		return nil, fmt.Errorf("conversation not found: %s", conversationID)
	}
	toolUseID := strings.TrimSpace(args.ToolUseID)
	if toolUseID == "" {
		return nil, fmt.Errorf("tool execution tool_use_id is required")
	}
	toolName := strings.TrimSpace(args.ToolName)
	if toolName == "" {
		return nil, fmt.Errorf("tool execution tool_name is required")
	}
	completedAtUS := nonZeroUS(args.CompletedAtUS, reducerNowUS(ctx))
	startedAtUS := args.StartedAtUS
	if startedAtUS == 0 {
		latencyUS := args.DurationMs * 1000
		if latencyUS > 0 && completedAtUS > latencyUS {
			startedAtUS = completedAtUS - latencyUS
		} else {
			startedAtUS = completedAtUS
		}
	}
	status := strings.TrimSpace(args.Status)
	if status == "" {
		if strings.TrimSpace(args.Error) != "" {
			status = "error"
		} else {
			status = "success"
		}
	}
	id := strings.TrimSpace(args.ID)
	if id == "" {
		id = ToolExecutionID(conversationID, args.TurnNumber, args.Iteration, toolUseID, toolName)
	}
	if _, _, found := firstRow(ctx.DB.SeekIndex(uint32(tableToolExecutions), uint32(indexToolExecutionsPrimary), types.NewString(id))); found {
		return nil, fmt.Errorf("tool execution already exists: %s", id)
	}
	if _, err := ctx.DB.Insert(uint32(tableToolExecutions), toolExecutionRow(ToolExecution{
		ID:             id,
		ConversationID: conversationID,
		TurnNumber:     args.TurnNumber,
		Iteration:      args.Iteration,
		ToolUseID:      toolUseID,
		ToolName:       toolName,
		Status:         status,
		StartedAtUS:    startedAtUS,
		CompletedAtUS:  completedAtUS,
		DurationMs:     args.DurationMs,
		InputJSON:      args.InputJSON,
		OutputSize:     args.OutputSize,
		NormalizedSize: args.NormalizedSize,
		Error:          args.Error,
		MetadataJSON:   defaultString(args.MetadataJSON, emptyJSONObject),
	})); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{OperationID: id})
}

func storeContextReportReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args StoreContextReportArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	conversationID := strings.TrimSpace(args.ConversationID)
	if conversationID == "" {
		return nil, fmt.Errorf("context report conversation id is required")
	}
	if _, conversation, found := findConversationByID(ctx.DB, conversationID); !found || conversation.Deleted {
		return nil, fmt.Errorf("conversation not found: %s", conversationID)
	}
	if args.TurnNumber == 0 {
		return nil, fmt.Errorf("context report turn number is required")
	}
	expectedID := ContextReportID(conversationID, args.TurnNumber)
	id := strings.TrimSpace(args.ID)
	if id == "" {
		id = expectedID
	}
	if id != expectedID {
		return nil, fmt.Errorf("context report id must match conversation and turn")
	}
	if _, _, found := firstRow(ctx.DB.SeekIndex(uint32(tableContextReports), uint32(indexContextReportsPrimary), types.NewString(id))); found {
		return nil, fmt.Errorf("context report already exists: %s", id)
	}
	createdAtUS := nonZeroUS(args.CreatedAtUS, reducerNowUS(ctx))
	updatedAtUS := nonZeroUS(args.UpdatedAtUS, createdAtUS)
	if _, err := ctx.DB.Insert(uint32(tableContextReports), contextReportRow(ContextReport{
		ID:             id,
		ConversationID: conversationID,
		TurnNumber:     args.TurnNumber,
		CreatedAtUS:    createdAtUS,
		UpdatedAtUS:    updatedAtUS,
		RequestJSON:    defaultString(args.RequestJSON, emptyJSONObject),
		ReportJSON:     defaultString(args.ReportJSON, emptyJSONObject),
		QualityJSON:    defaultString(args.QualityJSON, emptyJSONObject),
	})); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{OperationID: id})
}

func updateContextReportQualityReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args UpdateContextReportQualityArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	conversationID := strings.TrimSpace(args.ConversationID)
	if conversationID == "" {
		return nil, fmt.Errorf("context report conversation id is required")
	}
	if args.TurnNumber == 0 {
		return nil, fmt.Errorf("context report turn number is required")
	}
	id := ContextReportID(conversationID, args.TurnNumber)
	rowID, row, found := firstRow(ctx.DB.SeekIndex(uint32(tableContextReports), uint32(indexContextReportsPrimary), types.NewString(id)))
	if !found {
		return nil, fmt.Errorf("context report not found: %s/%d", conversationID, args.TurnNumber)
	}
	report := decodeContextReportRow(row)
	report.UpdatedAtUS = nonZeroUS(args.UpdatedAtUS, reducerNowUS(ctx))
	report.QualityJSON = defaultString(args.QualityJSON, emptyJSONObject)
	if _, err := ctx.DB.Update(uint32(tableContextReports), rowID, contextReportRow(report)); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{OperationID: id})
}

type documentMutation struct {
	OperationType string
	Path          string
	Content       string
	Actor         string
	Kind          string
	Title         string
	TagsJSON      string
	MetadataJSON  string
}

func upsertDocument(ctx *schema.ReducerContext, mutation documentMutation) ([]byte, error) {
	path, err := normalizeDocumentPath(mutation.Path)
	if err != nil {
		return nil, err
	}
	nowUS := reducerNowUS(ctx)
	newHash := contentHash(mutation.Content)
	chunks := splitDocumentChunks(path, mutation.Content)
	rowID, current, found := findDocumentByPath(ctx.DB, path)
	beforeHash := ""
	createdAtUS := nowUS
	if found {
		beforeHash = current.ContentHash
		createdAtUS = current.CreatedAtUS
		deleteChunksForPath(ctx.DB, path)
	}
	for _, chunk := range chunks {
		if _, err := ctx.DB.Insert(uint32(tableDocumentChunks), chunkRow(chunk)); err != nil {
			return nil, err
		}
	}
	doc := Document{
		Path:         path,
		Kind:         firstNonEmpty(mutation.Kind, documentKindBrain),
		Title:        mutation.Title,
		ContentHash:  newHash,
		ContentSize:  uint64(len(mutation.Content)),
		ChunkCount:   uint32(len(chunks)),
		CreatedAtUS:  createdAtUS,
		UpdatedAtUS:  nowUS,
		Deleted:      false,
		TagsJSON:     defaultString(mutation.TagsJSON, emptyJSONArray),
		MetadataJSON: defaultString(mutation.MetadataJSON, emptyJSONObject),
	}
	if found {
		if _, err := ctx.DB.Update(uint32(tableDocuments), rowID, documentRow(doc)); err != nil {
			return nil, err
		}
	} else if _, err := ctx.DB.Insert(uint32(tableDocuments), documentRow(doc)); err != nil {
		return nil, err
	}
	revision := nextRevision(ctx.DB, path)
	operationID := memoryOperationID(mutation.OperationType, path, mutation.Actor, nowUS, beforeHash, newHash)
	if _, err := ctx.DB.Insert(uint32(tableDocumentRevisions), revisionRow(documentRevisionID(path, revision), path, revision, newHash, operationID, nowUS, mutation.OperationType, mutation.Actor)); err != nil {
		return nil, err
	}
	if _, err := ctx.DB.Insert(uint32(tableMemoryOperations), operationRow(operationID, mutation.OperationType, path, mutation.Actor, nowUS, beforeHash, newHash, mustJSON(mutation))); err != nil {
		return nil, err
	}
	if err := markBrainIndexDirty(ctx.DB, DefaultProjectID, mutation.OperationType, nowUS); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{Path: path, ContentHash: newHash, Revision: revision, OperationID: operationID})
}

func findDocumentByPath(db types.ReducerDB, path string) (types.RowID, Document, bool) {
	rowID, row, ok := firstRow(db.SeekIndex(uint32(tableDocuments), uint32(indexDocumentsPrimary), types.NewString(path)))
	if !ok {
		return 0, Document{}, false
	}
	return rowID, decodeDocumentRow(row), true
}

func deleteChunksForPath(db types.ReducerDB, path string) {
	for rowID := range db.SeekIndex(uint32(tableDocumentChunks), uint32(indexDocumentChunksPath), types.NewString(path)) {
		_ = db.Delete(uint32(tableDocumentChunks), rowID)
	}
}

func nextRevision(db types.ReducerDB, path string) uint32 {
	var maxRevision uint32
	for _, row := range db.SeekIndex(uint32(tableDocumentRevisions), uint32(indexDocumentRevisionsPath), types.NewString(path)) {
		revision := row[2].AsUint32()
		if revision > maxRevision {
			maxRevision = revision
		}
	}
	return maxRevision + 1
}

func markBrainIndexDirty(db types.ReducerDB, projectID string, reason string, nowUS uint64) error {
	rowID, row, found := firstRow(db.SeekIndex(uint32(tableBrainIndexState), uint32(indexBrainIndexStatePrimary), types.NewString(projectID)))
	lastIndexedAtUS := uint64(0)
	if found {
		lastIndexedAtUS = row[1].AsUint64()
	}
	state := brainIndexStateRow(projectID, lastIndexedAtUS, true, nowUS, reason, emptyJSONObject)
	if found {
		_, err := db.Update(uint32(tableBrainIndexState), rowID, state)
		return err
	}
	_, err := db.Insert(uint32(tableBrainIndexState), state)
	return err
}

func findConversationByID(db types.ReducerDB, id string) (types.RowID, Conversation, bool) {
	rowID, row, ok := firstRow(db.SeekIndex(uint32(tableConversations), uint32(indexConversationsPrimary), types.NewString(id)))
	if !ok {
		return 0, Conversation{}, false
	}
	return rowID, decodeConversationRow(row), true
}

func deleteMessagesForConversation(db types.ReducerDB, conversationID string) {
	for rowID := range db.SeekIndex(uint32(tableMessages), uint32(indexMessagesConversation), types.NewString(conversationID)) {
		_ = db.Delete(uint32(tableMessages), rowID)
	}
	deleteSubCallsForConversation(db, conversationID)
	deleteToolExecutionsForConversation(db, conversationID)
	deleteContextReportsForConversation(db, conversationID)
}

func insertConversationMessage(db types.ReducerDB, conversationID string, message Message) (Message, error) {
	if strings.TrimSpace(conversationID) == "" {
		return Message{}, fmt.Errorf("conversation id is required")
	}
	_, conversation, found := findConversationByID(db, conversationID)
	if !found || conversation.Deleted {
		return Message{}, fmt.Errorf("conversation not found: %s", conversationID)
	}
	sequence := nextMessageSequence(db, conversationID)
	message.ConversationID = conversationID
	message.Sequence = sequence
	message.ID = MessageID(conversationID, sequence)
	message.SummaryOfJSON = defaultString(message.SummaryOfJSON, emptyJSONArray)
	message.MetadataJSON = defaultString(message.MetadataJSON, emptyJSONObject)
	if _, err := db.Insert(uint32(tableMessages), messageRow(message)); err != nil {
		return Message{}, err
	}
	return message, nil
}

func nextMessageSequence(db types.ReducerDB, conversationID string) uint64 {
	var maxSequence uint64
	found := false
	for _, row := range db.SeekIndex(uint32(tableMessages), uint32(indexMessagesConversation), types.NewString(conversationID)) {
		sequence := row[4].AsUint64()
		if !found || sequence > maxSequence {
			maxSequence = sequence
			found = true
		}
	}
	if !found {
		return 0
	}
	return maxSequence + 1
}

func touchConversation(db types.ReducerDB, conversationID string, updatedAtUS uint64) error {
	rowID, conversation, found := findConversationByID(db, conversationID)
	if !found || conversation.Deleted {
		return fmt.Errorf("conversation not found: %s", conversationID)
	}
	conversation.UpdatedAtUS = updatedAtUS
	_, err := db.Update(uint32(tableConversations), rowID, conversationRow(conversation))
	return err
}

func linkIterationSubCallsToMessage(db types.ReducerDB, conversationID string, turnNumber uint32, iteration uint32, messageID string) error {
	for rowID, row := range db.SeekIndex(uint32(tableSubCalls), uint32(indexSubCallsConversation), types.NewString(conversationID)) {
		subCall := decodeSubCallRow(row)
		if subCall.TurnNumber != turnNumber || subCall.Iteration != iteration || subCall.Purpose != "chat" || subCall.MessageID != "" {
			continue
		}
		subCall.MessageID = messageID
		if _, err := db.Update(uint32(tableSubCalls), rowID, subCallRow(subCall)); err != nil {
			return err
		}
	}
	return nil
}

func deleteSubCallsForConversation(db types.ReducerDB, conversationID string) {
	for rowID := range db.SeekIndex(uint32(tableSubCalls), uint32(indexSubCallsConversation), types.NewString(conversationID)) {
		_ = db.Delete(uint32(tableSubCalls), rowID)
	}
}

func deleteToolExecutionsForConversation(db types.ReducerDB, conversationID string) {
	for rowID := range db.SeekIndex(uint32(tableToolExecutions), uint32(indexToolExecutionsConversation), types.NewString(conversationID)) {
		_ = db.Delete(uint32(tableToolExecutions), rowID)
	}
}

func deleteContextReportsForConversation(db types.ReducerDB, conversationID string) {
	for rowID := range db.SeekIndex(uint32(tableContextReports), uint32(indexContextReportsConversation), types.NewString(conversationID)) {
		_ = db.Delete(uint32(tableContextReports), rowID)
	}
}

func deleteIterationSubCalls(db types.ReducerDB, conversationID string, turnNumber uint32, iteration uint32) {
	for rowID, row := range db.SeekIndex(uint32(tableSubCalls), uint32(indexSubCallsConversation), types.NewString(conversationID)) {
		subCall := decodeSubCallRow(row)
		if subCall.TurnNumber == turnNumber && subCall.Iteration == iteration {
			_ = db.Delete(uint32(tableSubCalls), rowID)
		}
	}
}

func deleteIterationToolExecutions(db types.ReducerDB, conversationID string, turnNumber uint32, iteration uint32) {
	for rowID, row := range db.SeekIndex(uint32(tableToolExecutions), uint32(indexToolExecutionsConversation), types.NewString(conversationID)) {
		execution := decodeToolExecutionRow(row)
		if execution.TurnNumber == turnNumber && execution.Iteration == iteration {
			_ = db.Delete(uint32(tableToolExecutions), rowID)
		}
	}
}

func deleteTurnSubCalls(db types.ReducerDB, conversationID string, turnNumber uint32) {
	for rowID, row := range db.SeekIndex(uint32(tableSubCalls), uint32(indexSubCallsConversation), types.NewString(conversationID)) {
		subCall := decodeSubCallRow(row)
		if subCall.TurnNumber == turnNumber {
			_ = db.Delete(uint32(tableSubCalls), rowID)
		}
	}
}

func deleteTurnToolExecutions(db types.ReducerDB, conversationID string, turnNumber uint32) {
	for rowID, row := range db.SeekIndex(uint32(tableToolExecutions), uint32(indexToolExecutionsConversation), types.NewString(conversationID)) {
		execution := decodeToolExecutionRow(row)
		if execution.TurnNumber == turnNumber {
			_ = db.Delete(uint32(tableToolExecutions), rowID)
		}
	}
}

func deleteTurnContextReports(db types.ReducerDB, conversationID string, turnNumber uint32) {
	for rowID, row := range db.SeekIndex(uint32(tableContextReports), uint32(indexContextReportsConversation), types.NewString(conversationID)) {
		report := decodeContextReportRow(row)
		if report.TurnNumber == turnNumber {
			_ = db.Delete(uint32(tableContextReports), rowID)
		}
	}
}

func firstRow(seq func(func(types.RowID, types.ProductValue) bool)) (types.RowID, types.ProductValue, bool) {
	for rowID, row := range seq {
		return rowID, row, true
	}
	return 0, nil, false
}

func reducerNowUS(ctx *schema.ReducerContext) uint64 {
	if ctx == nil {
		return 0
	}
	return uint64(ctx.Caller.Timestamp.UTC().UnixMicro())
}

func decodeReducerArgs(raw []byte, out any) error {
	if len(raw) == 0 {
		return fmt.Errorf("reducer args are required")
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode reducer args: %w", err)
	}
	return nil
}

func encodeReducerResult(result reducerResult) ([]byte, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode reducer result: %w", err)
	}
	return data, nil
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return emptyJSONObject
	}
	return string(data)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func nonZeroUS(value uint64, fallback uint64) uint64 {
	if value != 0 {
		return value
	}
	return fallback
}
