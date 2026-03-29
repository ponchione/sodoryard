# Task 04: Error Handling and Diagnostics

**Epic:** 04 — Embedding Client
**Status:** ⬚ Not started
**Dependencies:** Task 02, Task 03

---

## Description

Harden the embedding client with specific, actionable error messages for each failure mode. The embedding container runs in Docker and is the most common point of failure during indexing — errors must clearly indicate whether the problem is the container being down, a timeout, a bad response, or a dimension mismatch, so the developer can fix it without reading source code.

## Acceptance Criteria

- [ ] **Connection refused** (container not running): When the HTTP POST to the embedding container fails with a connection error (TCP dial failure), wrap the error with a user-facing message:
  `"embedding container not reachable at {baseURL} -- is the Docker container running?"`
  Detection: check for `*net.OpError` or `syscall.ECONNREFUSED` in the error chain using `errors.As`
- [ ] **Timeout**: When the HTTP request exceeds `timeout_seconds`, wrap the error with:
  `"embedding request timed out after {timeout}s -- the container may be overloaded or the batch may be too large"`
  Detection: check for `context.DeadlineExceeded` or `*url.Error` with `Timeout() == true`
- [ ] **Context cancellation**: When the caller cancels the context, return `ctx.Err()` directly (no wrapping needed — this is expected behavior, not an error to diagnose)
- [ ] **Non-200 HTTP status**: Return error with:
  `"embedding request failed with status {statusCode}: {first 200 bytes of response body}"`
- [ ] **Malformed response**: When JSON deserialization of the response fails, return error with:
  `"failed to parse embedding response: {underlying error}"`
- [ ] **Wrong number of embeddings**: When the response contains a different number of embeddings than the number of input texts, return error with:
  `"expected {expected} embeddings, got {actual}"`
- [ ] **Dimension mismatch**: When any returned embedding vector has length != `rag.DefaultEmbeddingDims` (3584), return error with:
  `"embedding dimension mismatch: expected {expected}, got {actual} for input at index {i}"`
- [ ] **JSON marshal failure**: If `json.Marshal` fails on the request struct, return error with `fmt.Errorf("failed to marshal embedding request: %w", err)`. Note: this is practically unreachable since the request struct contains only `string` and `[]string` fields, but the error path must not be silently ignored.
- [ ] **Response body read failure**: If `io.ReadAll(resp.Body)` fails, return error with `fmt.Errorf("failed to read embedding response: %w", err)`
- [ ] All error messages include enough context to diagnose without a debugger (URL, counts, dimensions)
- [ ] Define sentinel errors or typed errors where useful for programmatic checking by callers:
  ```go
  var ErrContainerUnreachable = errors.New("embedding container unreachable")
  var ErrDimensionMismatch = errors.New("embedding dimension mismatch")
  ```
  Wrap these with `fmt.Errorf("...: %w", ErrContainerUnreachable)` so callers can use `errors.Is`
