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

type CancelIterationArgs struct {
	ConversationID string `json:"conversation_id"`
	TurnNumber     uint32 `json:"turn_number"`
	Iteration      uint32 `json:"iteration"`
}

type DiscardTurnArgs struct {
	ConversationID string `json:"conversation_id"`
	TurnNumber     uint32 `json:"turn_number"`
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
	sequence, err := insertConversationMessage(ctx.DB, args.ConversationID, Message{
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
	return encodeReducerResult(reducerResult{Revision: uint32(sequence)})
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
	for _, msg := range args.Messages {
		role := strings.TrimSpace(msg.Role)
		if role != "assistant" && role != "tool" {
			return nil, fmt.Errorf("unsupported iteration message role: %s", msg.Role)
		}
		if _, err := insertConversationMessage(ctx.DB, args.ConversationID, Message{
			TurnNumber:  args.TurnNumber,
			Iteration:   args.Iteration,
			Role:        role,
			Content:     msg.Content,
			ToolUseID:   msg.ToolUseID,
			ToolName:    msg.ToolName,
			CreatedAtUS: nowUS,
			Visible:     true,
		}); err != nil {
			return nil, err
		}
	}
	if err := touchConversation(ctx.DB, args.ConversationID, nowUS); err != nil {
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
	return encodeReducerResult(reducerResult{})
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
}

func insertConversationMessage(db types.ReducerDB, conversationID string, message Message) (uint64, error) {
	if strings.TrimSpace(conversationID) == "" {
		return 0, fmt.Errorf("conversation id is required")
	}
	_, conversation, found := findConversationByID(db, conversationID)
	if !found || conversation.Deleted {
		return 0, fmt.Errorf("conversation not found: %s", conversationID)
	}
	sequence := nextMessageSequence(db, conversationID)
	message.ConversationID = conversationID
	message.Sequence = sequence
	message.ID = MessageID(conversationID, sequence)
	message.SummaryOfJSON = defaultString(message.SummaryOfJSON, emptyJSONArray)
	message.MetadataJSON = defaultString(message.MetadataJSON, emptyJSONObject)
	if _, err := db.Insert(uint32(tableMessages), messageRow(message)); err != nil {
		return 0, err
	}
	return sequence, nil
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
