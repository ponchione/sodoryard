# Task 06: Integration Test

**Epic:** 04 — Embedding Client
**Status:** ⬚ Not started
**Dependencies:** Task 03

---

## Description

Write an integration test that embeds real text against a running nomic-embed-code Docker container. This test is guarded by a build tag so it only runs when the container is available. It validates the full end-to-end path: HTTP transport, real model inference, and response parsing with actual 3584-dimension vectors.

## Acceptance Criteria

- [ ] Test file: `internal/codeintel/embedder/integration_test.go`
- [ ] Guarded by build tag: `//go:build integration` at top of file
- [ ] Runs only when invoked explicitly: `go test -tags=integration ./internal/codeintel/embedder/...`
- [ ] Before running assertions, checks if the container is reachable at the configured URL (default `http://localhost:8081`). If not reachable, calls `t.Skip("embedding container not available at http://localhost:8081")` — does not fail the test
- [ ] **Test: embed single text**: Embeds the string `"func Add(a, b int) int\nAdds two integers and returns the sum"`, verifies the returned vector has exactly 3584 dimensions, verifies all values are finite floats (no NaN, no Inf)
- [ ] **Test: embed batch**: Embeds 3 distinct texts, verifies 3 distinct vectors returned, each with 3584 dimensions, verifies the vectors are not all identical (the model should produce different embeddings for different inputs)
- [ ] **Test: embed query**: Embeds a query `"authentication middleware"` via `EmbedQuery`, verifies the returned vector has 3584 dimensions
- [ ] **Test: query vs document difference**: Embeds the same text via `EmbedTexts` and via `EmbedQuery`, verifies the two resulting vectors are different (because `EmbedQuery` prepends the retrieval prefix, producing a different embedding)
- [ ] Test timeout: each test case has a 30-second context timeout to prevent hanging if the container is slow
- [ ] Environment variable `EMBEDDING_URL` overrides the default base URL for CI/non-standard setups
