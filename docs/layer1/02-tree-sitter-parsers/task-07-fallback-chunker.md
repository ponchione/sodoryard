# Task 07: Fallback Chunker

**Epic:** 02 — Tree-sitter Parsers
**Status:** ⬚ Not started
**Dependencies:** L1-E01 (RawChunk type, ChunkType enum)

---

## Description

Implement the fallback chunker for unsupported file types. When a file's extension does not match any language-specific parser, the fallback chunker produces chunks using a fixed-size sliding window. This ensures that every file in the index produces at least one chunk, even if no semantic parsing is available.

## Acceptance Criteria

- [ ] `FallbackParser` struct in `internal/rag/parser/fallback_parser.go` with method `Parse(filePath string, content []byte) ([]RawChunk, error)`
- [ ] Window size: 40 lines per chunk
- [ ] Overlap: 20 lines (each window starts 20 lines after the previous window's start)
- [ ] Each chunk produces a `RawChunk` with:
  - `Name`: `"chunk_N"` where N is the 1-based chunk index (e.g., `chunk_1`, `chunk_2`)
  - `Signature`: the first line of the chunk, trimmed of whitespace
  - `Body`: the full text of the 40-line window (or fewer lines for the last chunk)
  - `ChunkType`: `Fallback`
  - `LineStart`: 1-based line number of the first line in the window
  - `LineEnd`: 1-based line number of the last line in the window
- [ ] Files with 40 or fewer lines produce exactly one chunk containing all lines
- [ ] Files with 41-60 lines produce exactly two chunks: lines 1-40 and lines 21-60
- [ ] Files with 61-80 lines produce exactly three chunks: lines 1-40, lines 21-60, and lines 41-80
- [ ] The last chunk may be shorter than 40 lines (it includes all remaining lines from its start position)
- [ ] No chunk is generated if its start position is beyond the end of the file
- [ ] Empty files (zero lines) return an empty `[]RawChunk` with no error
- [ ] The parser never returns an error (line splitting is infallible for non-empty input)
