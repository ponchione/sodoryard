# Task 02: Client Struct and HTTP Transport

**Epic:** 04 — Embedding Client
**Status:** ⬚ Not started
**Dependencies:** Task 01, L1-E01 (Embedder interface, DefaultEmbeddingDims constant)

---

## Description

Define the `Client` struct in `internal/rag/embedder/` that holds the HTTP client and configuration. Implement the constructor and the core `sendBatch` method that POSTs a batch of texts to the embedding container and returns parsed vectors. This is the internal HTTP transport layer that the public `EmbedTexts` and `EmbedQuery` methods will call.

## Acceptance Criteria

- [ ] Package: `internal/rag/embedder/`
- [ ] `Client` struct with fields:
  - `httpClient *http.Client` (configured with timeout from `config.Embedding.TimeoutSeconds`)
  - `baseURL string` (from `config.Embedding.BaseURL`)
  - `model string` (from `config.Embedding.Model`)
  - `batchSize int` (from `config.Embedding.BatchSize`)
  - `queryPrefix string` (from `config.Embedding.QueryPrefix`)
- [ ] Constructor function:
  ```go
  func New(cfg config.Embedding) *Client
  ```
  Sets `httpClient.Timeout` to `time.Duration(cfg.TimeoutSeconds) * time.Second`. Stores all config fields.
- [ ] OpenAI-compatible request struct defined:
  ```go
  type embeddingRequest struct {
      Input []string `json:"input"`
      Model string   `json:"model"`
  }
  ```
- [ ] OpenAI-compatible response structs defined:
  ```go
  type embeddingResponse struct {
      Data []embeddingData `json:"data"`
  }
  type embeddingData struct {
      Embedding []float32 `json:"embedding"`
      Index     int       `json:"index"`
  }
  ```
- [ ] Core HTTP method:
  ```go
  func (c *Client) sendBatch(ctx context.Context, texts []string) ([][]float32, error)
  ```
  - Serializes `embeddingRequest` with `c.model` and the provided texts as JSON
  - POSTs to `{c.baseURL}/v1/embeddings` with `Content-Type: application/json`
  - Passes `ctx` to `http.NewRequestWithContext` for cancellation support
  - Reads and deserializes the response body into `embeddingResponse`
  - Validates: response contains exactly `len(texts)` embeddings
  - Validates: each embedding vector has length == `rag.DefaultEmbeddingDims` (3584)
  - Returns the embedding vectors ordered by input index (sorts by `embeddingData.Index`)
  - On HTTP status != 200: returns error with status code and response body snippet (first 200 bytes)
- [ ] Error wrapping uses `fmt.Errorf` with `%w` for all errors so callers can use `errors.Is`/`errors.As`
- [ ] Compiles cleanly: `go build ./internal/rag/embedder/...`
