# Task 02: Retrieval Result Types

**Epic:** 01 — Context Assembly Types & Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Define the structs that represent results from each of the retrieval paths: semantic code search (`RAGHit`), project brain search (`BrainHit`), structural graph queries (`GraphHit`), and explicit file reads (`FileResult`). Also define the `RetrievalResults` aggregate struct that collects slices of all hit types plus convention text and git context. These types are consumed by the budget manager and serializer.

## Acceptance Criteria

- [ ] `RAGHit` struct defined with fields: chunk ID, file path, name, signature, description, body, similarity score (`float64`), language, chunk type, line start, line end
- [ ] `BrainHit` struct defined with fields: document path, title, snippet, match score (`float64`), match mode (keyword/semantic/graph), tags (`[]string`)
- [ ] `GraphHit` struct defined with fields: symbol name, file path, relationship type (caller/callee/implements), depth (`int`)
- [ ] `FileResult` struct defined with fields: file path, content (`string`), token count (`int`), truncated flag (`bool`)
- [ ] `RetrievalResults` aggregate struct defined containing: `RAGHits []RAGHit`, `BrainHits []BrainHit`, `GraphHits []GraphHit`, `FileResults []FileResult`, `ConventionText string`, `GitContext string`
- [ ] All structs have GoDoc comments explaining their source retrieval path
- [ ] Package compiles with no errors
