# Task 06: Markdown Section Splitter

**Epic:** 02 — Tree-sitter Parsers
**Status:** ⬚ Not started
**Dependencies:** L1-E01 (RawChunk type, ChunkType enum)

---

## Description

Implement the Markdown section splitter. Unlike the tree-sitter language parsers, this parser uses simple line-based splitting on `##` headers rather than tree-sitter. Each section (from one `##` header to the next, or to end of file) becomes a `RawChunk`. This enables semantic search over documentation files alongside code.

## Acceptance Criteria

- [ ] `MarkdownParser` struct in `internal/rag/parser/markdown_parser.go` with method `Parse(filePath string, content []byte) ([]RawChunk, error)`
- [ ] The parser splits content on lines starting with `## ` (two hashes followed by a space)
- [ ] Each section produces a `RawChunk` with:
  - `Name`: the heading text after `## `, trimmed of whitespace (e.g., `## Installation Guide` -> `Installation Guide`)
  - `Signature`: the full heading line (e.g., `## Installation Guide`)
  - `Body`: the full text of the section, from the `##` heading line through to (but not including) the next `##` heading line or end of file, with trailing whitespace trimmed
  - `ChunkType`: `Section`
  - `LineStart`: 1-based line number of the `##` heading
  - `LineEnd`: 1-based line number of the last non-empty line of the section
- [ ] Content before the first `##` heading (e.g., a `#` title or introductory text) is captured as a section with `Name` set to the filename (without path or extension) and `Signature` set to the first line of the file
- [ ] If the file contains no `##` headings at all, the entire file is returned as a single `RawChunk` with `Name` set to the filename (without path or extension)
- [ ] `###` and deeper headings do NOT cause splits -- they are included in the body of their parent `##` section
- [ ] Empty files return an empty `[]RawChunk` with no error
- [ ] The parser never returns an error (Markdown splitting is infallible for non-empty input)
