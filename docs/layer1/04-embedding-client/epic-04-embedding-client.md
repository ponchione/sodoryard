# L1-E04 — Embedding Client

**Layer:** 1 — Code Intelligence
**Epic:** 04
**Status:** ⬜ Not Started
**Dependencies:** L1-E01 (types & interfaces), L0-E03 (config)

---

## Description

Implement the HTTP client for the nomic-embed-code embedding model running in a Docker container. The client sends batches of text to the `/v1/embeddings` API endpoint and receives back float32 vectors of 3584 dimensions. It handles both document embedding (for indexing — `signature + description` text) and query embedding (with the recommended retrieval prefix for asymmetric search).

This is a thin HTTP client with batch support, not a complex component. Ports from topham's `internal/rag/embedder.go`.

---

## Package

`internal/rag/embedder/` — embedding HTTP client.

---

## Definition of Done

- [ ] Implements the `Embedder` interface from [[L1-E01-types-and-interfaces]]
- [ ] `EmbedTexts(ctx, texts []string) ([][]float32, error)` — batch embeds document texts. Automatically splits into sub-batches of `DefaultEmbedBatchSize` (32) if input exceeds batch size
- [ ] `EmbedQuery(ctx, query string) ([]float32, error)` — embeds a single query with the nomic-embed-code retrieval prefix: `"Represent this query for searching relevant code: "` prepended to the query text
- [ ] HTTP POST to configurable base URL (default `http://localhost:8081`) at `/v1/embeddings` endpoint
- [ ] Request format matches the OpenAI embeddings API: `{"input": [...], "model": "nomic-embed-code"}`
- [ ] Response parsing: extracts `float32` vectors from the response JSON, validates dimensionality matches `DefaultEmbeddingDims` (3584)
- [ ] Configurable via `internal/config/`: embedding container URL, model name, batch size, timeout
- [ ] Error handling: connection refused (container not running), timeout, malformed response, dimension mismatch
- [ ] Context cancellation support on HTTP requests
- [ ] Unit tests with a mock HTTP server: batch splitting, query prefix prepending, error cases
- [ ] Integration test (guarded by build tag or env var): embed a real text against a running nomic-embed-code container, verify vector dimensions

---

## Architecture References

- [[04-code-intelligence-and-rag]] — "Component: Embedding Pipeline" (model, what gets embedded, query embedding, dimensions, batching)
- [[02-tech-stack-decisions]] — "Embeddings: nomic-embed-code via Docker" (port 8081, Docker container)
- topham source: `internal/rag/embedder.go`

---

## Notes

- The embedding container must be running for indexing and search to work. The client should return a clear error ("embedding container not reachable at http://localhost:8081 — is the Docker container running?") rather than a generic connection error.
- Batch splitting is important for large indexing runs. A file with 100 chunks produces 100 embedding texts that must be split into 4 batches of 32 (last batch has 4). The client handles this transparently.
- The query prefix is specific to nomic-embed-code's asymmetric retrieval mode. If the embedding model is ever swapped, this prefix may need to change. It's read from config or defined as a constant — not hardcoded in call sites.
