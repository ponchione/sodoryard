package vectorstore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/memory"

	"github.com/lancedb/lancedb-go/pkg/contracts"
	"github.com/lancedb/lancedb-go/pkg/lancedb"

	"github.com/ponchione/sirtopham/internal/codeintel"
)

const tableName = "chunks"

// Store wraps LanceDB for storing and retrieving code chunks with embeddings.
type Store struct {
	conn  contracts.IConnection
	table contracts.ITable
	pool  memory.Allocator
}

// NewStore connects to (or creates) a LanceDB database at the given directory.
func NewStore(ctx context.Context, dataDir string) (*Store, error) {
	conn, err := lancedb.Connect(ctx, dataDir, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to lancedb at %s: %w", dataDir, err)
	}

	s := &Store{
		conn: conn,
		pool: memory.NewGoAllocator(),
	}

	table, err := s.openOrCreateTable(ctx)
	if err != nil {
		conn.Close()
		return nil, err
	}
	s.table = table

	return s, nil
}

func (s *Store) openOrCreateTable(ctx context.Context) (contracts.ITable, error) {
	names, err := s.conn.TableNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}

	if slices.Contains(names, tableName) {
		return s.conn.OpenTable(ctx, tableName)
	}

	schema, err := buildSchema()
	if err != nil {
		return nil, fmt.Errorf("build schema: %w", err)
	}

	table, err := s.conn.CreateTable(ctx, tableName, schema)
	if err != nil {
		return nil, fmt.Errorf("create table %s: %w", tableName, err)
	}
	return table, nil
}

func buildSchema() (contracts.ISchema, error) {
	return lancedb.NewSchemaBuilder().
		AddStringField("id", false).
		AddStringField("project_name", false).
		AddStringField("file_path", false).
		AddStringField("language", false).
		AddStringField("chunk_type", false).
		AddStringField("name", false).
		AddStringField("signature", true).
		AddStringField("body", true).
		AddStringField("description", true).
		AddInt32Field("line_start", false).
		AddInt32Field("line_end", false).
		AddStringField("content_hash", false).
		AddTimestampField("indexed_at", arrow.Microsecond, false).
		AddStringField("calls", true).
		AddStringField("called_by", true).
		AddStringField("types_used", true).
		AddStringField("implements_ifaces", true).
		AddStringField("imports", true).
		AddVectorField("embedding", codeintel.DefaultEmbeddingDims, contracts.VectorDataTypeFloat32, false).
		Build()
}

// Upsert inserts or updates chunks in the vector store.
// Existing chunks with the same ID are deleted first (upsert via delete+insert).
func (s *Store) Upsert(ctx context.Context, chunks []codeintel.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}

	for _, c := range chunks {
		filter := fmt.Sprintf("id = '%s'", escapeLanceFilter(c.ID))
		if err := s.table.Delete(ctx, filter); err != nil {
			slog.Warn("failed to delete chunk before upsert", "id", c.ID, "error", err)
		}
	}

	record, err := s.chunksToRecord(chunks)
	if err != nil {
		return fmt.Errorf("build arrow record: %w", err)
	}
	defer record.Release()

	if err := s.table.AddRecords(ctx, []arrow.Record{record}, nil); err != nil {
		return fmt.Errorf("add records: %w", err)
	}

	return nil
}

// VectorSearch performs cosine similarity search against stored embeddings.
func (s *Store) VectorSearch(ctx context.Context, queryEmbedding []float32, topK int, filter codeintel.Filter) ([]codeintel.SearchResult, error) {
	filterStr := buildFilterString(filter)

	var rows []map[string]any
	var err error

	if filterStr != "" {
		rows, err = s.table.VectorSearchWithFilter(ctx, "embedding", queryEmbedding, topK, filterStr)
	} else {
		rows, err = s.table.VectorSearch(ctx, "embedding", queryEmbedding, topK)
	}
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	results := make([]codeintel.SearchResult, 0, len(rows))
	for _, row := range rows {
		chunk, score := rowToChunk(row)
		results = append(results, codeintel.SearchResult{
			Chunk: chunk,
			Score: score,
		})
	}

	return results, nil
}

// GetByFilePath returns all chunks stored for a given file path.
func (s *Store) GetByFilePath(ctx context.Context, filePath string) ([]codeintel.Chunk, error) {
	filter := fmt.Sprintf("file_path = '%s'", escapeLanceFilter(filePath))
	rows, err := s.table.SelectWithFilter(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("select chunks for %s: %w", filePath, err)
	}

	chunks := make([]codeintel.Chunk, 0, len(rows))
	for _, row := range rows {
		chunk, _ := rowToChunk(row)
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// GetByName returns all chunks matching a symbol name.
func (s *Store) GetByName(ctx context.Context, name string) ([]codeintel.Chunk, error) {
	filter := fmt.Sprintf("name = '%s'", escapeLanceFilter(name))
	rows, err := s.table.SelectWithFilter(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("select chunks by name %s: %w", name, err)
	}

	chunks := make([]codeintel.Chunk, 0, len(rows))
	for _, row := range rows {
		chunk, _ := rowToChunk(row)
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// DeleteByFilePath removes all chunks associated with a file path.
func (s *Store) DeleteByFilePath(ctx context.Context, filePath string) error {
	filter := fmt.Sprintf("file_path = '%s'", escapeLanceFilter(filePath))
	if err := s.table.Delete(ctx, filter); err != nil {
		return fmt.Errorf("delete chunks for %s: %w", filePath, err)
	}
	return nil
}

// Close releases store-held resources.
func (s *Store) Close() error {
	if s.table != nil {
		if err := s.table.Close(); err != nil {
			return err
		}
	}
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// DropAndRecreateTable drops the chunks table and recreates it.
func (s *Store) DropAndRecreateTable(ctx context.Context) error {
	if s.table != nil {
		_ = s.table.Close()
		s.table = nil
	}

	if err := s.conn.DropTable(ctx, tableName); err != nil {
		return fmt.Errorf("drop table %s: %w", tableName, err)
	}

	table, err := s.openOrCreateTable(ctx)
	if err != nil {
		return fmt.Errorf("recreate table: %w", err)
	}
	s.table = table
	return nil
}

// chunksToRecord builds an Arrow record from chunks.
// Embeddings are taken from each chunk's Embedding field.
func (s *Store) chunksToRecord(chunks []codeintel.Chunk) (arrow.Record, error) {
	schema, err := buildSchema()
	if err != nil {
		return nil, err
	}

	arrowSchema := schema.ToArrowSchema()
	builder := array.NewRecordBuilder(s.pool, arrowSchema)
	defer builder.Release()

	fID := builder.Field(0).(*array.StringBuilder)
	fProjectName := builder.Field(1).(*array.StringBuilder)
	fFilePath := builder.Field(2).(*array.StringBuilder)
	fLanguage := builder.Field(3).(*array.StringBuilder)
	fChunkType := builder.Field(4).(*array.StringBuilder)
	fName := builder.Field(5).(*array.StringBuilder)
	fSignature := builder.Field(6).(*array.StringBuilder)
	fBody := builder.Field(7).(*array.StringBuilder)
	fDescription := builder.Field(8).(*array.StringBuilder)
	fLineStart := builder.Field(9).(*array.Int32Builder)
	fLineEnd := builder.Field(10).(*array.Int32Builder)
	fContentHash := builder.Field(11).(*array.StringBuilder)
	fIndexedAt := builder.Field(12).(*array.TimestampBuilder)
	fCalls := builder.Field(13).(*array.StringBuilder)
	fCalledBy := builder.Field(14).(*array.StringBuilder)
	fTypesUsed := builder.Field(15).(*array.StringBuilder)
	fImplements := builder.Field(16).(*array.StringBuilder)
	fImports := builder.Field(17).(*array.StringBuilder)
	fEmbedding := builder.Field(18).(*array.FixedSizeListBuilder)
	fEmbValues := fEmbedding.ValueBuilder().(*array.Float32Builder)

	for _, c := range chunks {
		if len(c.Embedding) != codeintel.DefaultEmbeddingDims {
			return nil, fmt.Errorf("chunk %q embedding dimension mismatch: got %d, want %d",
				c.ID, len(c.Embedding), codeintel.DefaultEmbeddingDims)
		}
		fID.Append(c.ID)
		fProjectName.Append(c.ProjectName)
		fFilePath.Append(c.FilePath)
		fLanguage.Append(c.Language)
		fChunkType.Append(string(c.ChunkType))
		fName.Append(c.Name)
		fSignature.Append(c.Signature)
		fBody.Append(c.Body)
		fDescription.Append(c.Description)
		fLineStart.Append(int32(c.LineStart))
		fLineEnd.Append(int32(c.LineEnd))
		fContentHash.Append(c.ContentHash)
		fIndexedAt.Append(arrow.Timestamp(c.IndexedAt.UnixMicro()))
		fCalls.Append(marshalJSON(c.Calls))
		fCalledBy.Append(marshalJSON(c.CalledBy))
		fTypesUsed.Append(marshalJSON(c.TypesUsed))
		fImplements.Append(marshalJSON(c.ImplementsIfaces))
		fImports.Append(marshalJSON(c.Imports))

		fEmbedding.Append(true)
		for _, v := range c.Embedding {
			fEmbValues.Append(v)
		}
	}

	return builder.NewRecord(), nil
}

func buildFilterString(filter codeintel.Filter) string {
	var parts []string
	if filter.Language != "" {
		parts = append(parts, fmt.Sprintf("language = '%s'", escapeLanceFilter(filter.Language)))
	}
	if filter.ChunkType != "" {
		parts = append(parts, fmt.Sprintf("chunk_type = '%s'", escapeLanceFilter(string(filter.ChunkType))))
	}
	if filter.FilePathPrefix != "" {
		parts = append(parts, fmt.Sprintf("file_path LIKE '%s%%'", escapeLanceFilter(filter.FilePathPrefix)))
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, " AND ")
}

func rowToChunk(row map[string]any) (codeintel.Chunk, float64) {
	c := codeintel.Chunk{
		ID:          mapStr(row, "id"),
		ProjectName: mapStr(row, "project_name"),
		FilePath:    mapStr(row, "file_path"),
		Language:    mapStr(row, "language"),
		ChunkType:   codeintel.ChunkType(mapStr(row, "chunk_type")),
		Name:        mapStr(row, "name"),
		Signature:   mapStr(row, "signature"),
		Body:        mapStr(row, "body"),
		Description: mapStr(row, "description"),
		LineStart:   mapInt(row, "line_start"),
		LineEnd:     mapInt(row, "line_end"),
		ContentHash: mapStr(row, "content_hash"),
	}

	if v, ok := row["indexed_at"]; ok {
		switch t := v.(type) {
		case time.Time:
			c.IndexedAt = t
		case arrow.Timestamp:
			c.IndexedAt = time.UnixMicro(int64(t))
		case int64:
			c.IndexedAt = time.UnixMicro(t)
		}
	}

	c.Calls = unmarshalFuncRefs(mapStr(row, "calls"))
	c.CalledBy = unmarshalFuncRefs(mapStr(row, "called_by"))
	c.TypesUsed = unmarshalStrings(mapStr(row, "types_used"))
	c.ImplementsIfaces = unmarshalStrings(mapStr(row, "implements_ifaces"))
	c.Imports = unmarshalStrings(mapStr(row, "imports"))

	var score float64
	if v, ok := row["_distance"]; ok {
		switch d := v.(type) {
		case float32:
			score = 1.0 - float64(d)
		case float64:
			score = 1.0 - d
		}
	}

	return c, score
}

func mapStr(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func mapInt(m map[string]any, key string) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int32:
			return int(n)
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}

func escapeLanceFilter(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func marshalJSON(v any) string {
	if v == nil {
		return "[]"
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func unmarshalFuncRefs(s string) []codeintel.FuncRef {
	if s == "" || s == "[]" {
		return nil
	}
	var refs []codeintel.FuncRef
	if err := json.Unmarshal([]byte(s), &refs); err != nil {
		return nil
	}
	return refs
}

func unmarshalStrings(s string) []string {
	if s == "" || s == "[]" {
		return nil
	}
	var strs []string
	if err := json.Unmarshal([]byte(s), &strs); err != nil {
		return nil
	}
	return strs
}
