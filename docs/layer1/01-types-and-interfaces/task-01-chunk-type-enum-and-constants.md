# Task 01: ChunkType Enum and Pipeline Constants

**Epic:** 01 — Types and Interfaces
**Status:** ⬚ Not started
**Dependencies:** L0-E01 (project scaffolding)

---

## Description

Define the `ChunkType` string enum and all pipeline-level constants in `internal/rag/`. These are the foundational values that every other type and function in the RAG pipeline references. The `ChunkType` enum classifies parsed code elements by their syntactic role. The constants govern chunk size limits, embedding dimensions, batch sizes, and schema versioning.

## Acceptance Criteria

- [ ] File `internal/rag/types.go` created with package declaration `package rag`
- [ ] `ChunkType` defined as a named `string` type
- [ ] The following `ChunkType` constants defined:
  - `ChunkTypeFunction  ChunkType = "function"`
  - `ChunkTypeMethod    ChunkType = "method"`
  - `ChunkTypeType      ChunkType = "type"`
  - `ChunkTypeInterface ChunkType = "interface"`
  - `ChunkTypeClass     ChunkType = "class"`
  - `ChunkTypeSection   ChunkType = "section"` (for Markdown heading-delimited blocks)
  - `ChunkTypeFallback  ChunkType = "fallback"` (for 40-line sliding window chunks in unsupported languages)
- [ ] Constant `MaxBodyLength = 2000` (int; bodies exceeding this are truncated before storage)
- [ ] Constant `DefaultEmbeddingDims = 3584` (int; float32 vector dimensionality for nomic-embed-code)
- [ ] Constant `DefaultEmbedBatchSize = 32` (int; texts per embedding API request)
- [ ] Constant `SchemaVersion` defined as a `string` (initial value `"1"`; changing this triggers a full re-index by dropping and recreating the LanceDB table)
- [ ] Constant `QueryPrefix = "Represent this query for searching relevant code: "` (string; prepended to queries before embedding, per nomic-embed-code asymmetric retrieval recommendation)
- [ ] File compiles cleanly: `go build ./internal/rag/...`
