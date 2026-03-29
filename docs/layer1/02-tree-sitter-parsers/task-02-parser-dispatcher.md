# Task 02: Parser Dispatcher

**Epic:** 02 — Tree-sitter Parsers
**Status:** ⬚ Not started
**Dependencies:** Task 01, L1-E01 (Parser interface, RawChunk type)

---

## Description

Implement the parser dispatcher that selects the correct parser based on file extension. The dispatcher is the single entry point for all file parsing in the RAG pipeline. It accepts a file path and file content, determines the language from the file extension, and delegates to the appropriate language-specific parser. It implements the `Parser` interface from L1-E01 which returns `[]RawChunk`.

## Acceptance Criteria

- [ ] `Dispatcher` struct in `internal/rag/parser/dispatcher.go` implements the `Parser` interface: `Parse(filePath string, content []byte) ([]RawChunk, error)`
- [ ] `Dispatcher` struct is unexported (or exported if needed by tests). Contains a `parsers map[string]rag.Parser` field mapping file extensions to parser instances, and a `fallback rag.Parser` field for unrecognized extensions. Populated in `NewDispatcher()`.
- [ ] `NewDispatcher() *Dispatcher` constructor initializes all language parsers and the extension-to-parser mapping
- [ ] Extension routing maps:
  - `.go` -> Go tree-sitter parser
  - `.ts` -> TypeScript tree-sitter parser
  - `.tsx` -> TSX tree-sitter parser
  - `.py` -> Python tree-sitter parser
  - `.md`, `.markdown` -> Markdown section splitter
  - All other extensions -> Fallback chunker
- [ ] Extension matching is case-insensitive (e.g., `.Go`, `.TS` route correctly)
- [ ] The dispatcher extracts the extension from the file path using `filepath.Ext()`, lowercases it, and looks up the parser
- [ ] The dispatcher returns the `[]RawChunk` from the selected parser without modification
- [ ] The dispatcher propagates errors from the underlying parser
- [ ] If a language parser returns an error, the dispatcher falls back to the fallback chunker for that file and returns the fallback result (no error propagation for parse failures)
- [ ] If the fallback chunker itself errors, that error is returned to the caller
