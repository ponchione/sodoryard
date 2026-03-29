# Task 05: Pass 1 — Walk + Parse Orchestration

**Epic:** 07 — Indexing Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, Task 03, Task 04, L1-E01 (types), L1-E02 (tree-sitter parsers), L1-E03 (Go AST parser)

---

## Description

Implement the first pass of the indexing pipeline: walk the project directory, apply change detection filters, select the appropriate parser per file, and produce `Chunk` objects with forward call references and deterministic IDs. This pass composes the file walker (Task 02), change detection (Task 03), git-aware filtering (Task 04), and the parsers (L1-E02, L1-E03) into a single orchestrated flow.

## Function Signature

```go
// pass1WalkAndParse walks the project, filters to changed files, parses each,
// and returns the full chunk list plus the list of deleted file paths.
func (idx *Indexer) pass1WalkAndParse(ctx context.Context) ([]codeintel.Chunk, []string, error)
```

Returns: all chunks from changed files, list of deleted file relative paths, error.

## Acceptance Criteria

- [ ] Calls `walkFiles` (Task 02) to discover all eligible files
- [ ] If not a force re-index, calls `gitChangedFiles` (Task 04) to get git-diff file list. If git returns a non-nil list, pre-filters the walker output to only those files
- [ ] Calls `filterChangedFiles` (Task 03) on the (possibly pre-filtered) file list to apply hash-based change detection. Collects the `deleted` file list from this step
- [ ] For force re-index (`IndexerConfig.Force`), skips both git and hash filtering — all walked files proceed to parsing
- [ ] **Parser selection per file:**
  - `.go` files: use the Go AST parser (L1-E03). If the Go AST parser returns an error for a specific file, fall back to the tree-sitter Go parser (L1-E02)
  - `.ts`, `.tsx` files: use the tree-sitter TypeScript/TSX parser
  - `.py` files: use the tree-sitter Python parser
  - `.md` files: use the tree-sitter Markdown section splitter
  - All other extensions matching include globs: use the fallback chunker (40-line sliding windows with 20-line overlap)
- [ ] For each file, reads file content from disk
- [ ] Passes file content to the selected parser, receives `[]RawChunk`
- [ ] Converts each `RawChunk` to a `codeintel.Chunk`:
  - `ID` = `hex(sha256(filePath + chunkType + name + lineStart))` (matching the `ChunkID(filePath, chunkType, name, lineStart)` function from L1-E01 task-04)
  - `ProjectName` = from config
  - `FilePath` = relative path
  - `Language` = derived from file extension: `.go` → `go`, `.ts` → `typescript`, `.tsx` → `typescript`, `.py` → `python`, `.md`/`.markdown` → `markdown`, all others → `unknown`
  - `ChunkType` = from `RawChunk.ChunkType`
  - `Name`, `Signature`, `Body` = from `RawChunk`
  - `LineStart`, `LineEnd` = from `RawChunk`
  - `ContentHash` = `hex(sha256(body))`
  - `IndexedAt` = current timestamp (ISO8601)
  - `Calls` = from Go AST parser relationship metadata (empty for non-Go files)
  - `CalledBy` = empty (populated in Pass 2)
  - `TypesUsed`, `ImplementsIfaces`, `Imports` = from Go AST parser metadata (empty for non-Go)
- [ ] **Error handling per file:** if parsing fails for a single file, log the error (`"failed to parse file" path=<relpath> err=<error>`) and skip that file. Do not abort the entire pass
- [ ] Returns the accumulated `[]codeintel.Chunk` from all successfully parsed changed files
- [ ] Emits progress: `"pass 1 complete" files_walked=<N> files_changed=<N> files_skipped=<N> chunks_produced=<N> files_deleted=<N>`
- [ ] Context cancellation checked between files

## Work Breakdown

**Part A (~2-3h):** File reading, parser selection by extension (`.go` → Go AST parser, `.ts`/`.tsx` → TypeScript tree-sitter, `.py` → Python tree-sitter, `.md`/`.markdown` → Markdown splitter, others → fallback), and RawChunk collection with per-file error handling.

**Part B (~2h):** RawChunk-to-Chunk conversion (14 fields: ID generation, content hash, metadata population from Go AST relationships), progress event emission, and context cancellation.

This task should be worked in two sessions to stay within the 4-hour budget.
