# Task 07: Embedder and Describer Interfaces

**Epic:** 01 — Types and Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 03

---

## Description

Define the `Embedder` and `Describer` interfaces. The `Embedder` wraps the nomic-embed-code embedding model (running in a Docker container on port 8081, accessed via `/v1/embeddings`). It has two methods: batch embedding for indexing and single-query embedding with the retrieval prefix. The `Describer` wraps the local LLM that generates semantic descriptions for code chunks. It accepts file content with relationship context and returns name-description pairs.

## Acceptance Criteria

- [ ] `Embedder` interface defined in `internal/rag/interfaces.go` with the following methods:
  ```go
  type Embedder interface {
      // EmbedTexts embeds a batch of texts for indexing.
      // Each text is typically "signature\ndescription" for a chunk.
      // Automatically splits into sub-batches of DefaultEmbedBatchSize (32).
      // Returns one embedding vector per input text, in the same order.
      // Each vector has DefaultEmbeddingDims (3584) float32 elements.
      EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)

      // EmbedQuery embeds a single query for retrieval.
      // Automatically prepends QueryPrefix ("Represent this query for
      // searching relevant code: ") before embedding.
      // Returns a single embedding vector.
      EmbedQuery(ctx context.Context, query string) ([]float32, error)
  }
  ```
- [ ] `Describer` interface defined in `internal/rag/interfaces.go` with the following methods:
  ```go
  type Describer interface {
      // DescribeFile sends file content and relationship context to a local LLM
      // and returns a description for each function/type in the file.
      //
      // fileContent is the source code, truncated to 6000 characters by the caller
      // before passing to the Describer.
      //
      // relationshipContext is a formatted string of call graph relationships
      // (calls, called_by, types_used, implements) for functions in this file,
      // appended to help the LLM understand each function's role in the codebase.
      //
      // Returns one Description per function/type found in the file.
      // If the LLM call fails or returns invalid JSON, returns an empty slice
      // and a nil error — the file is still indexed, just without descriptions.
      // Only returns a non-nil error for unrecoverable failures (e.g., context cancelled).
      DescribeFile(ctx context.Context, fileContent string, relationshipContext string) ([]Description, error)
  }
  ```
- [ ] `DescribeFile` graceful degradation behavior is documented in the interface doc comment: LLM failures produce empty descriptions, not errors (the indexing pipeline continues)
- [ ] File compiles cleanly: `go build ./internal/rag/...`
