# L1-E01 — Types & Interfaces

**Layer:** 1 — Code Intelligence
**Epic:** 01
**Status:** ⬜ Not Started
**Dependencies:** L0-E01 (scaffolding)

---

## Description

Define the shared types and interfaces that every other Layer 1 epic implements or consumes. This includes the core data types (`Chunk`, `RawChunk`, `SearchResult`, `Filter`, `ChunkType`), the key abstractions (`Parser`, `Store`, `Embedder`, `Describer`, `Searcher`, `GraphStore`), and the constants that govern the pipeline (`MaxBodyLength`, `DefaultEmbeddingDims`, `DefaultEmbedBatchSize`, `SchemaVersion`).

This is the foundation contract for the entire code intelligence layer. Every other epic in Layer 1 depends on these types being stable and well-defined. The types port almost directly from topham's `internal/rag/types.go`, adapted for sirtopham's package structure.

---

## Package

`internal/codeintel/` — shared Layer 1 types, interfaces, constants, and hash helpers consumed across the code-intelligence pipeline.

---

## Definition of Done

- [ ] `Chunk` struct defined with all fields: identity (ID, ProjectName), location (FilePath, Language, ChunkType), content (Name, Signature, Body, Description), metadata (LineStart, LineEnd, ContentHash, IndexedAt), relationships (Calls, CalledBy, TypesUsed, ImplementsIfaces, Imports)
- [ ] `RawChunk` struct defined with parser output fields: Name, Signature, Body, ChunkType, LineStart, LineEnd
- [ ] `ChunkType` enum defined: Function, Method, Type, Interface, Class, Section, Fallback
- [ ] `SearchResult` struct defined: Chunk reference, similarity score, match metadata
- [ ] `Filter` struct defined: Language, ChunkType, FilePathPrefix filters for vector search
- [ ] `Parser` interface defined: accepts file path and content, returns `[]RawChunk`
- [ ] `Store` interface defined: Upsert, VectorSearch, GetByFilePath, GetByName, DeleteByFilePath, Close
- [ ] `Embedder` interface defined: EmbedTexts (batch), EmbedQuery (single with prefix)
- [ ] `Describer` interface defined: DescribeFile (accepts file content + relationship context, returns `[]Description`)
- [ ] `Searcher` interface defined: Search (accepts queries + filters + config, returns ranked `[]SearchResult`)
- [ ] `GraphStore` interface defined: blast radius query (accepts symbol, returns upstream/downstream/interface results)
- [ ] Constants defined: `MaxBodyLength = 2000`, `DefaultEmbeddingDims = 3584`, `DefaultEmbedBatchSize = 32`, `SchemaVersion`
- [ ] Deterministic chunk ID generation function: `sha256(filePath + chunkType + name + lineStart)`
- [ ] Content hash function: `sha256(body)`
- [ ] All types compile cleanly with `go build ./internal/codeintel/...`
- [ ] Unit tests for chunk ID generation (deterministic, collision-resistant for same-named symbols)
- [ ] Unit tests for content hash generation

---

## Architecture References

- [[04-code-intelligence-and-rag]] — "Component: Parsing Pipeline" (Chunk Constraints), "Component: Embedding Pipeline" (Dimensions, Batching), "Component: Vector Store" (Schema, Operations), "Component: Searcher"
- [[06-context-assembly]] — "Component: Retrieval Execution" (defines how downstream code calls into Searcher and GraphStore)
- topham source: `internal/rag/types.go`

---

## Notes

- The `Searcher` interface must be sufficient for both the `search_semantic` agent tool ([[05-agent-loop]]) and the context assembly retrieval path ([[06-context-assembly]]). Design it to accept multiple queries with per-query topK, plus options for hop expansion and deduplication.
- The `Store` interface must support both vector search (cosine similarity with metadata filters) and metadata-only queries (by file path, by name) needed for call graph lookups and change detection.
- The `GraphStore` interface is separate from `Store` — structural graph data lives in SQLite, not LanceDB. Keep them distinct.
- Relationship fields on `Chunk` (Calls, CalledBy, etc.) are `[]string` — stored as JSON strings in LanceDB. The Go types should use native slices; serialization is the store's responsibility.
- Layer 3 owns `ContextNeeds`, `TurnAnalyzer`, `ContextAssemblyReport`, and related context-assembly observability types in `internal/context/`. Epic 01 intentionally stops at Layer 1 contracts and does not redefine those downstream types here.
