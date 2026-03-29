package codeintel

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
