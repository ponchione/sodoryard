# Task 05: Unit Tests

**Epic:** 04 — Embedding Client
**Status:** ⬚ Not started
**Dependencies:** Task 03, Task 04

---

## Description

Write comprehensive unit tests for the embedding client using `net/http/httptest` to simulate the embedding container. Tests cover the happy path for both methods, batch splitting logic, the query prefix, and every error path. No Docker container or real embedding model is needed — all tests use a mock HTTP server.

## Acceptance Criteria

- [ ] All tests in `internal/rag/embedder/embedder_test.go`
- [ ] All tests pass via `go test ./internal/rag/embedder/...`
- [ ] **Happy path — EmbedTexts single batch**: Send 5 texts, mock returns 5 vectors of 3584 float32s, verify all 5 vectors returned with correct values
- [ ] **Happy path — EmbedTexts batch splitting**: Send 70 texts with batchSize=32, verify the mock receives exactly 3 HTTP requests (batches of 32, 32, 6), verify all 70 vectors returned in correct input order
- [ ] **Happy path — EmbedQuery**: Send a query string, verify the mock receives a request where `input[0]` starts with the query prefix (`"Represent this query for searching relevant code: "`), verify a single 3584-dimension vector returned
- [ ] **Empty input — EmbedTexts**: Pass empty slice, verify no HTTP request made and nil returned
- [ ] **Empty query — EmbedQuery**: Pass empty string, verify error returned without making HTTP request
- [ ] **Connection refused**: Configure client with a URL that nothing listens on (e.g., `http://127.0.0.1:1` or a closed httptest server), verify error message contains "embedding container not reachable" and `errors.Is(err, ErrContainerUnreachable)` returns true
- [ ] **HTTP 500 error**: Mock returns status 500 with body "internal server error", verify error contains the status code and body snippet
- [ ] **Timeout**: Configure mock server to sleep 5 seconds, create client with 100ms timeout. Call EmbedTexts. Verify error message contains `embedding request timed out` and includes the timeout duration.
- [ ] **Malformed JSON response**: Mock returns status 200 with invalid JSON body, verify error contains "failed to parse embedding response"
- [ ] **Wrong embedding count**: Mock returns 3 embeddings for 5 input texts, verify error contains "expected 5 embeddings, got 3"
- [ ] **Dimension mismatch**: Mock returns vectors of length 768 instead of 3584, verify error contains "embedding dimension mismatch: expected 3584, got 768" and `errors.Is(err, ErrDimensionMismatch)` returns true
- [ ] **Context cancellation**: Create a pre-cancelled context, call EmbedTexts, verify `context.Canceled` error returned
- [ ] **Batch splitting stops on error**: Send 70 texts, mock fails on second batch, verify error returned and third batch was never sent (use a request counter in the mock)
- [ ] **Request body format**: Verify the mock receives a JSON body matching `{"input": [...], "model": "nomic-embed-code"}` — check model field and input array
- [ ] Mock helper function that generates a valid response with N vectors of 3584 float32s (all zeros or sequential values) for reuse across tests

## Sizing Note

Estimated ~3-4 hours. 14 test cases with mock HTTP server setup. If implementation exceeds expectations, the task can be split: happy-path tests (cases 1-6) in one session, error-path tests (cases 7-14) in another.
