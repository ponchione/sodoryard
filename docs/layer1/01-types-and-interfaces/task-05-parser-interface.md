# Task 05: Parser Interface

**Epic:** 01 — Types and Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 02

---

## Description

Define the `Parser` interface that all language-specific parsers (Go AST, tree-sitter TypeScript, tree-sitter Python, Markdown splitter, fallback sliding window) must implement. The interface accepts a file path and its content, and returns a slice of `RawChunk` values. This abstraction allows the indexing pipeline to dispatch to the correct parser based on file extension without knowing the parser implementation details.

## Acceptance Criteria

- [ ] `Parser` interface defined in `internal/codeintel/interfaces.go` with the following method:
  ```go
  type Parser interface {
      // Parse extracts top-level declarations from the given file content.
      // filePath is used for error messages and chunk metadata, not for reading
      // the file — content is passed directly.
      // Returns an empty slice (not nil) if no chunks are found.
      // Returns an error if parsing fails (e.g., tree-sitter parse error).
      Parse(filePath string, content []byte) ([]RawChunk, error)
  }
  ```
- [ ] The interface lives in `internal/codeintel/interfaces.go` alongside other interfaces
- [ ] The `content` parameter is `[]byte` (not `string`) to avoid unnecessary copies from file reads
- [ ] Doc comment on the interface explains that implementations exist for Go (AST-based), TypeScript/TSX (tree-sitter), Python (tree-sitter), Markdown (heading splitter), and a fallback (40-line sliding window with 20-line overlap)
- [ ] File compiles cleanly: `go build ./internal/codeintel/...`
