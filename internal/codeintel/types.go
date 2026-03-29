package codeintel

import "time"

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
	// ChunkTypeEnum represents an enum declaration.
	ChunkTypeEnum ChunkType = "enum"
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

// FuncRef identifies a function or method in the call graph.
type FuncRef struct {
	Name    string `json:"name"`
	Package string `json:"package"`
}

// RawChunk is the direct output of a parser before enrichment and storage.
type RawChunk struct {
	Name      string
	Signature string
	Body      string
	ChunkType ChunkType
	LineStart int
	LineEnd   int

	// Relationship metadata populated by AST-aware parsers.
	Calls      []FuncRef
	TypesUsed  []string
	Implements []string
	Imports    []string
}

// Chunk is the fully enriched record stored in the semantic index.
type Chunk struct {
	ID          string
	ProjectName string

	FilePath  string
	Language  string
	ChunkType ChunkType

	Name        string
	Signature   string
	Body        string
	Description string

	LineStart   int
	LineEnd     int
	ContentHash string
	IndexedAt   time.Time

	Calls            []FuncRef
	CalledBy         []FuncRef
	TypesUsed        []string
	ImplementsIfaces []string
	Imports          []string

	Embedding []float32
}

// SearchResult wraps a retrieved chunk with ranking metadata.
type SearchResult struct {
	Chunk     Chunk
	Score     float64
	MatchedBy string
	HitCount  int
	FromHop   bool
}

// Filter constrains vector-search results by metadata.
type Filter struct {
	Language       string
	ChunkType      ChunkType
	FilePathPrefix string
}

// Description is a semantic name-summary pair returned by the describer.
type Description struct {
	Name        string
	Description string
}

// SearchOptions configures semantic search execution and hop expansion.
type SearchOptions struct {
	TopK               int
	Filter             Filter
	MaxResults         int
	EnableHopExpansion bool
	HopBudgetFraction  float64
	HopDepth           int
}

// GraphNode identifies a symbol returned by structural graph traversal.
type GraphNode struct {
	Symbol    string
	FilePath  string
	Kind      string
	Depth     int
	LineStart int
	LineEnd   int
}

// BlastRadiusResult groups the structural graph results around a target symbol.
type BlastRadiusResult struct {
	Upstream   []GraphNode
	Downstream []GraphNode
	Interfaces []GraphNode
}

// GraphQuery configures a blast-radius query against the structural graph.
type GraphQuery struct {
	Symbol       string
	MaxDepth     int
	MaxNodes     int
	IncludeKinds []string
	ExcludeKinds []string
}
