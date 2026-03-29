# Task 09: Unit Tests — Go Tree-sitter Parser

**Epic:** 02 — Tree-sitter Parsers
**Status:** ⬚ Not started
**Dependencies:** Task 03

---

## Description

Write comprehensive unit tests for the Go tree-sitter parser. Tests use inline Go source strings parsed by the `GoParser` and assert the exact `RawChunk` fields returned. These tests validate the extraction of function declarations, method declarations, and type declarations (structs and interfaces).

## Acceptance Criteria

- [ ] Test file at `internal/codeintel/treesitter/go_parser_test.go`
- [ ] **Function extraction test:** parsing the source `func Foo(x int, y string) error { return nil }` produces a `RawChunk` with:
  - `Name` = `"Foo"`
  - `Signature` = `"func Foo(x int, y string) error"`
  - `ChunkType` = `Function`
  - `Body` contains the full function text
  - `LineStart` = 1, `LineEnd` = 1 (single-line function)
- [ ] **Multi-line function test:** parsing a function spanning lines 3-10 produces `LineStart` = 3, `LineEnd` = 10
- [ ] **Method extraction test:** parsing `func (s *Server) HandleRequest(w http.ResponseWriter, r *http.Request) { }` produces:
  - `Name` = `"HandleRequest"`
  - `Signature` = `"func (s *Server) HandleRequest(w http.ResponseWriter, r *http.Request)"`
  - `ChunkType` = `Method`
- [ ] **Struct type extraction test:** parsing `type Config struct { Host string; Port int }` produces:
  - `Name` = `"Config"`
  - `ChunkType` = `Type`
- [ ] **Interface type extraction test:** parsing `type Reader interface { Read(p []byte) (n int, err error) }` produces:
  - `Name` = `"Reader"`
  - `ChunkType` = `Interface`
- [ ] **Multiple declarations test:** a source with 2 functions, 1 method, and 1 type returns exactly 4 `RawChunk` entries in source order
- [ ] **No declarations test:** `package main\n\nimport "fmt"\n` returns an empty `[]RawChunk` (no error)
- [ ] **Function with no return type test:** `func main() { }` produces `Signature` = `"func main()"`
- [ ] **Multiple return values test:** `func Split(s string) (string, string, error) { return "", "", nil }` produces `Signature` = `"func Split(s string) (string, string, error)"`
