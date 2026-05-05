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
