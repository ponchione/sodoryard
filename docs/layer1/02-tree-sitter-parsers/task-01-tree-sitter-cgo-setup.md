# Task 01: Tree-sitter CGo Dependency Setup

**Epic:** 02 — Tree-sitter Parsers
**Status:** ⬚ Not started
**Dependencies:** L1-E01 (types & interfaces), L0-E01 (Makefile)

---

## Description

Add the tree-sitter Go bindings and language grammar dependencies to `go.mod`, verify that CGo compilation works with the tree-sitter C libraries, and update the Makefile if additional linker or compiler flags are needed. This is the foundation that all subsequent parser tasks build on. Each grammar must be pinned to a specific version so that node type names remain stable.

## Acceptance Criteria

- [ ] `go get github.com/tree-sitter/go-tree-sitter` added to `go.mod` at a pinned version
- [ ] Go grammar added: `github.com/tree-sitter/tree-sitter-go` at a pinned version
- [ ] TypeScript grammar added: `github.com/tree-sitter/tree-sitter-typescript` at a pinned version (provides both TypeScript and TSX grammars)
- [ ] Python grammar added: `github.com/tree-sitter/tree-sitter-python` at a pinned version
- [ ] A minimal smoke test file exists at `internal/rag/parser/treesitter_smoke_test.go` that:
  - Creates a tree-sitter parser instance for each language (Go, TypeScript, TSX, Python)
  - Parses a trivial source string (e.g., `package main` for Go, `def foo(): pass` for Python, `function foo() {}` for TypeScript)
  - Asserts the root node is not nil and has no errors
- [ ] `CGO_ENABLED=1 go test ./internal/rag/parser/...` passes with no linker errors
- [ ] If the Makefile needs additional CGo flags (e.g., `-ltree-sitter` or include paths), those are added to the `CGOFLAGS` or equivalent variable
- [ ] `go mod tidy` leaves no unused dependencies
