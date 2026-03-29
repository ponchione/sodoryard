# Task 01: Indexer Struct and Constructor

**Epic:** 07 — Indexing Pipeline
**Status:** ⬚ Not started
**Dependencies:** L1-E01 (types & interfaces), L1-E02 (tree-sitter parsers), L1-E03 (Go AST parser), L1-E04 (embedding client), L1-E05 (LanceDB store), L1-E06 (description generator), L0-E03 (config), L0-E04 (SQLite), L0-E06 (schema/sqlc)

---

## Description

Define the `Indexer` struct, its configuration options, and the constructor function. The Indexer orchestrates the three-pass pipeline and holds references to every dependency: parsers, embedder, store, describer, sqlc queries, and config. This task establishes the struct shape and wiring — subsequent tasks implement the pipeline logic.

## Package

`internal/rag/indexer/`

## Acceptance Criteria

- [ ] `IndexerConfig` struct with fields:
  - `ProjectID string` — UUIDv7 from the projects table
  - `ProjectRoot string` — absolute path to the project directory
  - `IncludeGlobs []string` — from `config.Index.Include`
  - `ExcludeGlobs []string` — from `config.Index.Exclude`
  - `MaxFileSizeBytes int64` — from `config.Index.MaxFileSizeBytes` (default 51200)
  - `Force bool` — true when `--force` flag is set
  - `OnProgress func(ProgressEvent)` — optional callback for progress reporting. If nil, progress events are silently discarded
- [ ] `IndexerConfig` includes an optional `OnProgress func(ProgressEvent)` callback field. If nil, progress events are silently discarded
- [ ] `Indexer` struct with fields:
  - `config IndexerConfig`
  - `goParser` — Go AST parser: `*goparser.GoParser` (implements `rag.Parser` and also provides `ParseWithRelationships` for relationship metadata extraction)
  - `tsParser` — tree-sitter parser dispatcher (L1-E02 `Parser` interface)
  - `embedder` — `rag.Embedder` interface (L1-E04)
  - `store` — `rag.Store` interface (L1-E05)
  - `describer` — `rag.Describer` interface (L1-E06)
  - `queries` — sqlc-generated `DB` interface providing: `GetFileState(ctx, projectID, filePath) (*IndexState, error)`, `UpsertFileState(ctx, params) error`, `DeleteFileState(ctx, projectID, filePath) error`, `DeleteFileStatesByProject(ctx, projectID) error`, `GetProjectCommit(ctx, projectID) (string, error)`, `UpsertProjectCommit(ctx, params) error`
  - `logger` — structured logger (L0-E02)
- [ ] Constructor function: `func NewIndexer(cfg IndexerConfig, goParser Parser, tsParser Parser, embedder rag.Embedder, store rag.Store, describer rag.Describer, queries DB, logger *slog.Logger) *Indexer`
- [ ] Constructor validates that required dependencies are non-nil, returns error if any are missing
- [ ] `Run(ctx context.Context) error` method signature defined (body is a stub that returns nil — implemented in Task 10)
- [ ] File compiles cleanly with `go build ./internal/rag/indexer/...`
