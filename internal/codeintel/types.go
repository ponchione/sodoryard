package codeintel

// ChunkType classifies a parsed code or document chunk by syntactic role.
type ChunkType string

const (
	// ChunkTypeFunction represents a top-level function declaration.
	ChunkTypeFunction ChunkType = "function"
	// ChunkTypeMethod represents a method declaration.
	ChunkTypeMethod ChunkType = "method"
	// ChunkTypeType represents a type declaration.
	ChunkTypeType ChunkType = "type"
	// ChunkTypeInterface represents an interface declaration.
	ChunkTypeInterface ChunkType = "interface"
	// ChunkTypeClass represents a class declaration.
	ChunkTypeClass ChunkType = "class"
	// ChunkTypeSection represents a markdown heading-delimited section.
	ChunkTypeSection ChunkType = "section"
	// ChunkTypeFallback represents a fallback sliding-window chunk.
	ChunkTypeFallback ChunkType = "fallback"
)

const (
	// MaxBodyLength is the maximum stored body length for a chunk.
	MaxBodyLength = 2000
	// DefaultEmbeddingDims is the default embedding dimensionality for nomic-embed-code.
	DefaultEmbeddingDims = 3584
	// DefaultEmbedBatchSize is the default number of texts per embedding request.
	DefaultEmbedBatchSize = 32
	// SchemaVersion is the current Layer 1 schema version.
	SchemaVersion = "1"
	// QueryPrefix is prepended to queries before embedding for asymmetric retrieval.
	QueryPrefix = "Represent this query for searching relevant code: "
)
