# L1-E07 — Indexing Pipeline

**Layer:** 1 — Code Intelligence
**Epic:** 07
**Status:** ⬜ Not Started
**Dependencies:** L1-E02 (tree-sitter parsers), L1-E03 (Go AST parser), L1-E04 (embedding client), L1-E05 (LanceDB store), L1-E06 (description generator), L0-E03 (config), L0-E04 (sqlite), L0-E06 (schema/sqlc)

---

## Description

Implement the three-pass indexing pipeline that orchestrates the full code intelligence workflow: (1) walk the project directory and parse files into chunks, (2) build the reverse call graph from forward call references, (3) generate descriptions, embed, and store in LanceDB. This is the heaviest epic in Layer 1 — it composes every other component into a working pipeline and adds change detection, incremental indexing via git diff, and the `index_state` SQLite table integration.

The indexer is the entry point for `sirtopham init` (full index), `sirtopham index` (manual re-index), and the conversation-start auto-reindex trigger. Ports from topham's `internal/rag/indexer.go` with the net-new addition of git-aware incremental indexing.

---

## Package

`internal/index/` — three-pass indexing pipeline orchestrator.

---

## Definition of Done

### Pass 1 — Walk + Parse

- [ ] Walks the project directory filtered by include/exclude globs from config (`index.include`, `index.exclude`)
- [ ] Respects `max_file_size_bytes` — skips files exceeding the limit
- [ ] For each file, computes content hash (`sha256(body)`)
- [ ] **Change detection:** compares content hash against `index_state` table (L0-E06). Skips files whose hash hasn't changed since last index
- [ ] Selects parser per file: Go AST parser for `.go` files, tree-sitter for `.ts/.tsx/.py/.md`, fallback for others
- [ ] Produces `Chunk` objects with forward call references (from Go AST parser) and basic metadata (from tree-sitter)
- [ ] Deterministic chunk IDs generated per [[L1-E01-types-and-interfaces]]

### Pass 2 — Reverse Call Graph

- [ ] Builds `pkgIndex`: maps `"dir.FuncName"` → chunk references for O(1) lookup
- [ ] Builds `suffixToDir`: enables package suffix matching for resolving import paths to directory paths
- [ ] For each chunk's `Calls` list, finds the target chunk via `pkgIndex` and adds the caller to the target's `CalledBy` list
- [ ] Bidirectional call graph is complete after this pass — every chunk has both `Calls` and `CalledBy` populated

### Pass 3 — Describe + Embed + Store

- [ ] Groups chunks by file for description generation (one LLM call per file)
- [ ] Sends file content + relationship context to the description generator ([[L1-E06-description-generator]])
- [ ] Builds embedding text per chunk: `signature + "\n" + description`
- [ ] Chunks without descriptions (LLM failure) use signature-only embedding text
- [ ] Batch embeds via the embedding client ([[L1-E04-embedding-client]])
- [ ] Upserts chunks with embeddings into LanceDB ([[L1-E05-lancedb-store]])
- [ ] Updates `index_state` table with new file hash, chunk count, and timestamp

### Change Detection & Incremental Indexing

- [ ] **File hash comparison:** stored in `index_state` SQLite table (L0-E06). Files with unchanged hashes are skipped entirely in Pass 1
- [ ] **Git-aware incremental:** `git diff --name-only <last-indexed-commit>..HEAD` identifies changed files since the last index. Only changed files are re-parsed and re-embedded. The `last_indexed_commit` is read from and written to the `projects` table
- [ ] **Schema versioning:** `SchemaVersion` constant from [[L1-E01-types-and-interfaces]] compared against stored version. Mismatch triggers full re-index (drop and recreate LanceDB table)
- [ ] **Force flag:** `--force` on `sirtopham index` triggers full re-index regardless of hashes or git state

### Indexing Triggers

- [ ] **Full index:** invoked by `sirtopham init` or first run against a project
- [ ] **Incremental index:** invoked at conversation start (auto-reindex) or manually via `sirtopham index`
- [ ] **Manual re-index:** `sirtopham index --force` for full rebuild

### Orchestration

- [ ] Progress reporting: emits progress events (files parsed, chunks generated, files described, chunks embedded) for CLI output and future web UI integration
- [ ] Structured logging via L0-E02 for each pass with timing
- [ ] Error handling: individual file failures do not stop the pipeline. Log the error, skip the file, continue
- [ ] Clean shutdown on context cancellation (e.g., Ctrl+C during long indexing run)

### Testing

- [ ] Integration test: create a temp directory with Go, TypeScript, Python, and Markdown files. Run full three-pass pipeline. Verify chunks exist in LanceDB with embeddings. Verify `index_state` rows exist
- [ ] Integration test: modify one file, run incremental index. Verify only the changed file is re-processed
- [ ] Integration test: change `SchemaVersion`, verify full re-index is triggered
- [ ] Unit tests for file walking with include/exclude glob filtering
- [ ] Unit tests for reverse call graph construction

---

## Architecture References

- [[04-code-intelligence-and-rag]] — "Component: Indexing Pipeline" (three-pass architecture, change detection, indexing triggers)
- [[08-data-model]] — `index_state` table schema, `projects.last_indexed_commit` column
- [[02-tech-stack-decisions]] — "Embeddings: nomic-embed-code via Docker", "Local LLM Inference: Docker Container"
- topham source: `internal/rag/indexer.go`

---

## Notes

- This is the most complex epic in Layer 1 because it orchestrates every other component. It should be the last epic implemented, after all dependencies are individually tested.
- The git-diff incremental indexing is net-new for sirtopham (topham uses file hash comparison only). The implementation shells out to `git diff --name-only` — no go-git library. This is consistent with [[01-project-vision-and-principles]] which specifies shell git execution.
- Pass 3 is the slowest pass because it makes LLM calls (description generation) and embedding calls (HTTP to Docker container). These are I/O-bound and could benefit from concurrency — describe multiple files in parallel, embed in parallel batches. topham runs these sequentially; sirtopham can optimize later if indexing speed is a problem.
- The `index_state` table and sqlc queries are already created by L0-E06. This epic writes the Go code that uses those generated query functions.
- Task-08 (index state persistence) and Task-09 (schema version check) do not have dedicated unit test tasks. They are tested indirectly via the integration tests in Task-14, which verify SQLite state after full and incremental pipeline runs.
- File walking should use `filepath.WalkDir` with the include/exclude globs from config. The glob matching should use `doublestar` or a similar library that supports `**` patterns.
