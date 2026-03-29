package codeintel

import "context"

// Parser extracts top-level declarations from source or document content.
//
// Implementations exist for Go (AST-based), TypeScript/TSX (tree-sitter),
// Python (tree-sitter), Markdown (heading splitter), and a fallback sliding
// window parser with 40-line windows and 20-line overlap.
type Parser interface {
	// Parse extracts top-level declarations from the given file content.
	// filePath is used for error messages and chunk metadata, not for reading
	// the file — content is passed directly.
	// Returns an empty slice (not nil) if no chunks are found.
	// Returns an error if parsing fails.
	Parse(filePath string, content []byte) ([]RawChunk, error)
}

// Store persists chunks, performs vector search, and supports metadata lookups.
type Store interface {
	// Upsert inserts or updates chunks in the vector store.
	// Implementation strategy is delete-by-ID then insert because LanceDB has
	// no native upsert support. Callers provide native Go slices on Chunk and
	// the store handles any serialization required by the backing schema.
	Upsert(ctx context.Context, chunks []Chunk) error

	// VectorSearch performs cosine similarity search against stored embeddings.
	// The zero-value Filter applies no metadata constraints.
	VectorSearch(ctx context.Context, queryEmbedding []float32, topK int, filter Filter) ([]SearchResult, error)

	// GetByFilePath returns all chunks stored for a given file path.
	GetByFilePath(ctx context.Context, filePath string) ([]Chunk, error)

	// GetByName returns all chunks matching a symbol name.
	GetByName(ctx context.Context, name string) ([]Chunk, error)

	// DeleteByFilePath removes all chunks associated with a file path.
	DeleteByFilePath(ctx context.Context, filePath string) error

	// Close releases store-held resources.
	Close() error
}

// Embedder produces vector embeddings for chunks and queries.
type Embedder interface {
	// EmbedTexts embeds a batch of indexing texts.
	// Each text is typically "signature\ndescription" for a chunk.
	// Implementations may split requests into DefaultEmbedBatchSize batches and
	// must return one embedding vector per input text in the same order.
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)

	// EmbedQuery embeds a single retrieval query.
	// Implementations prepend QueryPrefix before embedding and return one vector.
	EmbedQuery(ctx context.Context, query string) ([]float32, error)
}

// Describer generates semantic descriptions for code entities in a file.
type Describer interface {
	// DescribeFile sends file content plus relationship context to a local LLM.
	// The caller truncates fileContent before invocation. Implementations return
	// one Description per function/type found in the file. If the LLM fails or
	// returns invalid JSON, DescribeFile returns an empty slice and nil error so
	// indexing can continue. Only unrecoverable failures such as context
	// cancellation should produce a non-nil error.
	DescribeFile(ctx context.Context, fileContent string, relationshipContext string) ([]Description, error)
}

// Searcher executes one or more semantic queries against the code index.
type Searcher interface {
	// Search embeds each query, runs vector search with the provided options,
	// deduplicates by chunk ID, re-ranks by hit count with best-score tie
	// breaking, optionally expands one-hop callers/callees according to
	// HopBudgetFraction, and returns up to MaxResults results. If no results are
	// found, Search returns an empty slice and nil error.
	Search(ctx context.Context, queries []string, opts SearchOptions) ([]SearchResult, error)
}

// GraphStore serves structural blast-radius queries from the graph index.
type GraphStore interface {
	// BlastRadius returns upstream callers, downstream callees, and interface
	// relationships for the queried symbol. MaxDepth controls traversal depth
	// and MaxNodes caps the total results. If the symbol is not found, the
	// returned BlastRadiusResult contains empty (not nil) slices.
	BlastRadius(ctx context.Context, query GraphQuery) (*BlastRadiusResult, error)

	// Close releases resources held by the graph store.
	Close() error
}
