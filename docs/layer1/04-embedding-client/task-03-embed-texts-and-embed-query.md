# Task 03: EmbedTexts and EmbedQuery Methods

**Epic:** 04 — Embedding Client
**Status:** ⬚ Not started
**Dependencies:** Task 02

---

## Description

Implement the two public methods that satisfy the `Embedder` interface from L1-E01: `EmbedTexts` for batch document embedding with automatic sub-batch splitting, and `EmbedQuery` for single query embedding with the nomic-embed-code retrieval prefix. These are the only methods external callers (indexer, searcher) use.

## Acceptance Criteria

- [ ] `EmbedTexts` method:
  ```go
  func (c *Client) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)
  ```
  - If `texts` is empty, returns `nil, nil` (no-op, no HTTP call)
  - If `len(texts) <= c.batchSize`, calls `sendBatch` once
  - If `len(texts) > c.batchSize`, splits into sub-batches of `c.batchSize` and calls `sendBatch` for each sub-batch sequentially
  - Batch splitting example: 100 texts with batchSize=32 produces 4 batches: [0:32], [32:64], [64:96], [96:100]
  - Concatenates all sub-batch results in input order
  - If any sub-batch fails, returns the error immediately (does not continue remaining batches)
  - Checks `ctx.Err()` between sub-batches and returns early if context is cancelled
- [ ] `EmbedQuery` method:
  ```go
  func (c *Client) EmbedQuery(ctx context.Context, query string) ([]float32, error)
  ```
  - Prepends `c.queryPrefix` to the query text: `c.queryPrefix + query`
  - Calls `sendBatch(ctx, []string{prefixedQuery})`
  - Returns the single embedding vector (index 0) from the result
  - If query is empty string, returns error (do not send empty query to container)
- [ ] `*Client` satisfies the `rag.Embedder` interface — verified by compile-time assertion:
  ```go
  var _ rag.Embedder = (*Client)(nil)
  ```
- [ ] Compiles cleanly: `go build ./internal/rag/embedder/...`
