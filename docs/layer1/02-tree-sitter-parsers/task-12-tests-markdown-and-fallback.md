# Task 12: Unit Tests — Markdown Splitter and Fallback Chunker

**Epic:** 02 — Tree-sitter Parsers
**Status:** ⬚ Not started
**Dependencies:** Task 06, Task 07

---

## Description

Write unit tests for the Markdown section splitter and the fallback chunker. These are the two non-tree-sitter parsers. Tests verify section splitting on `##` headers and the sliding window behavior.

## Acceptance Criteria

### Markdown splitter tests

- [ ] Test file at `internal/rag/parser/markdown_parser_test.go`
- [ ] **Basic section split test:** a file with content `## Intro\nHello\n## Usage\nDo stuff` produces 2 chunks:
  - Chunk 1: `Name` = `"Intro"`, `ChunkType` = `Section`, `Body` starts with `## Intro`
  - Chunk 2: `Name` = `"Usage"`, `ChunkType` = `Section`, `Body` starts with `## Usage`
- [ ] **Preamble capture test:** a file starting with `# Title\nSome intro\n## Section 1\nContent` produces 2 chunks: the first has `Name` equal to the filename (without extension), the second has `Name` = `"Section 1"`
- [ ] **No headings test:** a file with `Just some text\nMore text` returns 1 chunk with `Name` equal to the filename
- [ ] **Nested headings test:** `## Parent\nSome text\n### Child\nMore text\n## Sibling` produces 2 chunks (`Parent` and `Sibling`). The `### Child` heading is included in the body of `Parent`
- [ ] **Empty file test:** returns an empty `[]RawChunk` with no error
- [ ] **Line numbers test:** for a file where `## First` is on line 1 and `## Second` is on line 5, the first chunk has `LineStart` = 1, and the second chunk has `LineStart` = 5

### Fallback chunker tests

- [ ] Test file at `internal/rag/parser/fallback_parser_test.go`
- [ ] **Short file test (20 lines):** produces exactly 1 chunk. `Name` = `"chunk_1"`, `ChunkType` = `Fallback`, `LineStart` = 1, `LineEnd` = 20
- [ ] **Exact 40 lines test:** produces exactly 1 chunk spanning all 40 lines
- [ ] **41 lines test:** produces exactly 2 chunks: chunk 1 covers lines 1-40, chunk 2 covers lines 21-41
- [ ] **80 lines test:** produces exactly 3 chunks: lines 1-40, lines 21-60, lines 41-80
- [ ] **100 lines test:** produces exactly 4 chunks: lines 1-40, lines 21-60, lines 41-80, lines 61-100
- [ ] **Signature is first line test:** each chunk's `Signature` equals the first line of that chunk window, trimmed
- [ ] **Empty file test:** returns an empty `[]RawChunk` with no error
- [ ] **Single line file test:** produces exactly 1 chunk with `LineStart` = 1, `LineEnd` = 1
