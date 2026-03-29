# Task 02: Arrow Record Builder for Chunks Table

**Epic:** 05 — LanceDB Store
**Status:** ⬚ Not started
**Dependencies:** Task 01, L1-E01 (Chunk type definition)

---

## Description

Implement the Apache Arrow record builder that converts `[]codeintel.Chunk` into Arrow records suitable for LanceDB insertion. This is the most error-prone part of the LanceDB integration: column types must match exactly, especially float32 for embedding vectors and string for JSON-serialized relationship fields. The builder defines the canonical schema for the `chunks` table.

## Acceptance Criteria

- [ ] Internal function `buildArrowSchema() *arrow.Schema` that returns the Arrow schema with these columns in order:
  - `id` — `arrow.BinaryTypes.String` (deterministic chunk ID, sha256 hex)
  - `project_name` — `arrow.BinaryTypes.String`
  - `file_path` — `arrow.BinaryTypes.String`
  - `language` — `arrow.BinaryTypes.String` (e.g., "go", "typescript", "python")
  - `chunk_type` — `arrow.BinaryTypes.String` (e.g., "function", "method", "type")
  - `name` — `arrow.BinaryTypes.String` (symbol name)
  - `signature` — `arrow.BinaryTypes.String` (everything before the function body)
  - `body` — `arrow.BinaryTypes.String` (full body text, max 2000 chars)
  - `description` — `arrow.BinaryTypes.String` (LLM-generated semantic summary)
  - `line_start` — `arrow.PrimitiveTypes.Int32`
  - `line_end` — `arrow.PrimitiveTypes.Int32`
  - `content_hash` — `arrow.BinaryTypes.String` (sha256 hex of body)
  - `indexed_at` — `arrow.BinaryTypes.String` (RFC 3339 timestamp)
  - `calls` — `arrow.BinaryTypes.String` (JSON-serialized `[]string`)
  - `called_by` — `arrow.BinaryTypes.String` (JSON-serialized `[]string`)
  - `types_used` — `arrow.BinaryTypes.String` (JSON-serialized `[]string`)
  - `implements_ifaces` — `arrow.BinaryTypes.String` (JSON-serialized `[]string`)
  - `imports` — `arrow.BinaryTypes.String` (JSON-serialized `[]string`)
  - `vector` — `arrow.FixedSizeListOf(3584, arrow.PrimitiveTypes.Float32)` (embedding, 3584 dimensions)
- [ ] Internal function `buildArrowRecord(chunks []codeintel.Chunk, embeddings [][]float32) (arrow.Record, error)` that:
  - Validates `len(chunks) == len(embeddings)`
  - Validates each embedding has exactly 3584 dimensions; returns error with chunk ID if mismatched
  - Serializes relationship fields (`Calls`, `CalledBy`, `TypesUsed`, `ImplementsIfaces`, `Imports`) to JSON strings via `json.Marshal`
  - Serializes `nil` slices as `"[]"` (empty JSON array), not `"null"`
  - Builds and returns a valid Arrow record matching the schema
- [ ] The embedding dimension count (3584) is sourced from the `codeintel.DefaultEmbeddingDims` constant (L1-E01), not hardcoded in this file
- [ ] Arrow record builder does not leak memory — records are properly released after use
