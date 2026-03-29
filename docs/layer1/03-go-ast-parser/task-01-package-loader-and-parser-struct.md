# Task 01: Package Loader and Parser Struct

**Epic:** 03 — Go AST Parser
**Status:** ⬚ Not started
**Dependencies:** L1-E01 (types & interfaces must be defined)

---

## Description

Define the `GoParser` struct in `internal/codeintel/goparser/` and implement its constructor. The constructor accepts a project root path, calls `go/packages.Load()` with the required load modes, and stores the loaded package data for reuse across all subsequent per-file parse calls. This is the expensive initialization step (seconds) that makes per-file parsing cheap.

## Acceptance Criteria

- [ ] Package `internal/codeintel/goparser/` exists with file `goparser.go`
- [ ] `GoParser` struct defined with at least the following fields:
  - `pkgs []*packages.Package` — loaded packages from `go/packages.Load()`
  - `fset *token.FileSet` — shared file set for position resolution
  - `pkgsByFile map[string]*packages.Package` — index mapping absolute file paths to their containing package (for fast per-file lookup)
  - `ifaceMap` — cached mapping of interface types discovered during loading (populated lazily or eagerly; see Task 05)
- [ ] Constructor function: `NewGoParser(projectRoot string) (*GoParser, error)`
- [ ] Constructor calls `packages.Load()` with config:
  - `Mode`: `packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps`
  - `Dir`: set to `projectRoot`
  - Pattern: `"./..."`  (loads all packages under the project root)
- [ ] Constructor builds the `pkgsByFile` index by iterating all loaded packages and mapping each `pkg.GoFiles` entry to its package
- [ ] Constructor returns a descriptive error if `go/packages.Load()` returns nil packages or if all packages have errors (individual package errors are logged but do not fail the whole load)
- [ ] Package errors (e.g., a single package with syntax errors) are collected and logged via `log/slog` but do not prevent the parser from being created — partial loads are acceptable
- [ ] `GoParser` compiles cleanly: `go build ./internal/codeintel/goparser/...`
