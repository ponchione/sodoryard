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
