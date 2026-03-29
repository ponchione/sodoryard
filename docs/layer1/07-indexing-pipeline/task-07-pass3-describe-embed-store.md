# Task 07: Pass 3 — Describe + Embed + Store

**Epic:** 07 — Indexing Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 06, L1-E04 (embedding client), L1-E05 (LanceDB store), L1-E06 (description generator)

---

## Description

Implement the third and final pass of the indexing pipeline: generate descriptions via the local LLM, build embedding text, batch-embed via the embedding container, and upsert chunks with embeddings into LanceDB. This is the slowest pass because it makes LLM calls (one per file) and embedding HTTP calls (batched). Each step has explicit fallback behavior for failures.

## Function Signature

```go
// pass3DescribeEmbedStore generates descriptions, embeds, and stores all chunks.
func (idx *Indexer) pass3DescribeEmbedStore(ctx context.Context, chunks []codeintel.Chunk) error
```

## Acceptance Criteria

### Description Generation

- [ ] Groups chunks by `FilePath` — produces a `map[string][]codeintel.Chunk` where each key is a relative file path and the value is all chunks from that file
- [ ] For each file group, reads the file content from disk and truncates to 6000 characters before passing to the describer (the caller is responsible for truncation, not the describer)
- [ ] Calls `describer.DescribeFile(ctx, fileContent, fileChunks)` which returns `map[string]string` (name to description)
- [ ] Applies descriptions to chunks: for each chunk in the file, sets `chunk.Description = descriptions[chunk.Name]` if present
- [ ] **Description failure handling:** if the describer returns an empty map (LLM failure), the chunks proceed without descriptions. Log: `"description generation failed for file, proceeding without descriptions" path=<relpath>`
- [ ] **Per-file error handling:** if describing a single file fails, log the error and continue to the next file. Never abort Pass 3 for a single file's failure

### Embedding Text Construction

- [ ] For each chunk, builds the embedding text:
  - If `chunk.Description` is non-empty: `chunk.Signature + "\n" + chunk.Description`
  - If `chunk.Description` is empty: `chunk.Signature` (signature-only fallback)
- [ ] Collects all embedding texts into a single `[]string` for batch embedding

### Batch Embedding

- [ ] Calls `embedder.EmbedTexts(ctx, embeddingTexts)` which handles sub-batching internally (batches of `DefaultEmbedBatchSize` = 32)
- [ ] Receives `[][]float32` — one vector per chunk, in the same order as the input texts
- [ ] Attaches each embedding vector to the corresponding chunk (the chunk struct or a parallel data structure for the store upsert)
- [ ] **Embedding failure handling:** if the embedder returns an error, the entire Pass 3 fails with a wrapped error. Embedding is not optional — chunks without embeddings cannot be stored in LanceDB

### Store Upsert

- [ ] Calls `store.Upsert(ctx, chunks, embeddings)` with the full batch for the file (the Store interface's Upsert method from L1-E05 accepts a slice of chunks and their corresponding embeddings)
- [ ] For incremental indexing: before upserting, calls `store.DeleteByFilePath(ctx, filePath)` for each changed file to remove stale chunks, then inserts the new chunks. This handles cases where a file's chunk boundaries changed (functions added/removed/renamed)
- [ ] **Store failure handling:** if upsert fails, return the error — store failures are not recoverable within the pipeline

### Deleted File Cleanup

- [ ] For each deleted file path (from Pass 1): calls `store.DeleteByFilePath(ctx, relPath)` to remove all chunks for that file from LanceDB

### Progress and Logging

- [ ] Emits progress at each sub-stage:
  - `"describing files" total_files=<N>`
  - `"file described" path=<relpath> descriptions=<N>` (per file)
  - `"embedding chunks" total_chunks=<N>`
  - `"storing chunks" total_chunks=<N>`
- [ ] Emits summary: `"pass 3 complete" files_described=<N> chunks_embedded=<N> chunks_stored=<N> files_cleaned=<N>`
- [ ] Context cancellation checked between files (description stage) and before embedding/store stages

## Work Breakdown

**Part A (~2-3h):** Description generation (file content truncation, relationship context formatting, DescribeFile call, graceful failure handling) and embedding text construction (signature + description concatenation).

**Part B (~2h):** Batch embedding via EmbedTexts, store.Upsert call, deleted file cleanup via store.DeleteByFilePath, and progress event emission.

This task should be worked in two sessions to stay within the 4-hour budget.
