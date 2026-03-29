# Task 10: Integration Test — Top-K Ordering with 50+ Chunks

**Epic:** 05 — LanceDB Store
**Status:** ⬚ Not started
**Dependencies:** Task 07

---

## Description

Write an integration test that inserts 50+ chunks with synthetic embeddings into a temporary LanceDB store and verifies that vector search returns correct top-K ordering by cosine similarity. This validates that LanceDB's ANN index returns results in the expected order at a non-trivial dataset size, and that the full insert-then-search pipeline works end-to-end.

## Acceptance Criteria

- [ ] Test is in a file tagged `//go:build integration` (or in a `_test.go` file that uses `testing.Short()` to skip in CI)
- [ ] Inserts exactly 60 chunks, each with a unique random embedding (3584-dimension float32 vectors)
- [ ] Chunks span at least 3 different languages (`"go"`, `"typescript"`, `"python"`) and at least 2 chunk types (`"function"`, `"type"`)
- [ ] Creates a known query vector by copying one of the inserted embeddings and adding small noise (epsilon perturbation, e.g., add 0.001 to each dimension). This ensures the source chunk is the nearest neighbor.
- [ ] Searches with `topK=10` and no filter. Verifies:
  - The source chunk (whose embedding was copied) appears as result #1
  - Result similarity scores are in strictly descending order (result[i].Score >= result[i+1].Score)
  - Exactly 10 results are returned
- [ ] Searches with `topK=5` and a language filter (e.g., `language="go"`). Verifies:
  - All returned chunks have `Language == "go"`
  - At most 5 results returned
  - Scores are in descending order
- [ ] Searches with `topK=5` and a file path prefix filter. Verifies:
  - All returned chunks have `FilePath` starting with the specified prefix
- [ ] Total test runtime is under 30 seconds (validates acceptable LanceDB performance at this scale)
- [ ] Test uses `t.TempDir()` for the LanceDB directory — no cleanup needed
