# Layer 1 Audit: Code Intelligence & RAG

## Scope

Layer 1 is the semantic understanding engine: tree-sitter parsing for Go/Python/TypeScript,
Go AST analysis, LLM-generated code descriptions, vector embeddings via nomic-embed-code,
LanceDB vector storage, multi-query semantic search, and a structural dependency graph.

## Spec References

- `docs/specs/04-code-intelligence-and-rag.md` — Full architecture
- `docs/layer1/phase2-layer1-overview.md` — Epic index (9 epics)
- `docs/layer1/01-types-and-interfaces/` through `09-structural-graph/` — Task-level specs

## Packages to Audit

| Package | Src | Test | Purpose |
|---------|-----|------|---------| 
| `internal/codeintel` | 4 | 3 | Types, interfaces, hash utilities |
| `internal/codeintel/treesitter` | 3 | 1 | Go, Python, TS tree-sitter parsers |
| `internal/codeintel/goparser` | 1 | 1 | Go AST parser (generics, interfaces) |
| `internal/codeintel/embedder` | 1 | 1 | HTTP embedding client |
| `internal/codeintel/describer` | 1 | 1 | LLM description generation |
| `internal/codeintel/indexer` | 3 | 3 | Indexing pipeline |
| `internal/codeintel/searcher` | 1 | 1 | Semantic search with multi-query |
| `internal/codeintel/graph` | 7 | 5 | Structural graph (Go/Python/TS analyzers) |
| `internal/vectorstore` | 2 | 1 | LanceDB vector store |

## Test Commands

```bash
make test                                          # Full suite (required for vectorstore CGO)
CGO_ENABLED=1 CGO_LDFLAGS="..." go test -tags 'sqlite_fts5' ./internal/codeintel/...
CGO_ENABLED=1 CGO_LDFLAGS="..." go test -tags 'sqlite_fts5' ./internal/vectorstore/...
```

## Audit Checklist

### Epic 01: Types & Interfaces
- [x] `internal/codeintel/types.go` defines: `Chunk`, `ChunkType`, `SearchResult`, `SearchOptions`
- [x] `ChunkType` enum covers all needed values
  - Values: function, method, type, interface, class, section, enum, fallback
  - Spec originally said struct/const/var/file/module — updated to match multi-language reality. Neither this project nor the reference (agent-conductor) extracts constants/vars.
- [x] `Searcher` interface defined with `Search(ctx, queries, opts)` signature
- [x] `GraphStore` interface defined with `BlastRadius(ctx, query)` signature
- [x] Content hash function for change detection exists and is tested

### Epic 02: Tree-sitter Parsers
- [x] `internal/codeintel/treesitter/` has parsers for Go, Python, TypeScript
- [x] Each parser extracts: functions, methods, types, interfaces
  - Constants not extracted (same in reference codebase — by design, constants provide low semantic value for RAG)
- [x] Chunk output includes: Name, Signature, Body, LineStart, LineEnd, ChunkType
  - Language set during enrichment in indexer, not at parse time (correct — parser doesn't know the full Chunk schema)
- [x] Test fixtures verify extraction accuracy for each language
- [x] Graceful fallback when tree-sitter parse fails (returns empty, doesn't crash)
  - **FIXED**: Changed from returning error to returning nil, nil with slog.Warn

### Epic 03: Go AST Parser
- [x] `internal/codeintel/goparser/` uses `go/ast` + `go/parser`
  - Uses golang.org/x/tools/go/packages (which uses go/parser internally) + go/ast + go/types
- [x] Handles generics (type parameters) — verified with test fixture
  - **FIXED**: Added TestParse_Generics with Set[T comparable], NewSet[T comparable], and Add method
- [x] Extracts interface method sets
  - **FIXED**: Added TestParse_InterfaceExtraction verifying ChunkTypeInterface extraction
- [x] Falls back to tree-sitter for non-Go files and files outside loaded packages
  - **FIXED**: Added `treeSitter` field + `WithFallback()` method. Delegates non-Go files and unrecognized Go files to tree-sitter.
- [x] Test covers: functions, methods, types, interfaces, generics, embedded types
  - **FIXED**: Added TestParse_Generics, TestParse_InterfaceExtraction, TestParse_EmbeddedTypes

### Epic 04: Embedding Client
- [x] `internal/codeintel/embedder/` makes HTTP calls to embedding service
- [x] Uses nomic-embed-code model (verify model name in config or code)
  - Default in internal/config/config.go line 197; configurable via config.Embedding.Model
- [x] Batches embedding requests
  - EmbedTexts() loops in sub-batches of c.batchSize; tested with 3 texts / batchSize=2
- [x] Test uses httptest mock
  - 7 test functions using httptest.NewServer

### Epic 05: LanceDB Store
- [x] `internal/vectorstore/` wraps LanceDB Go bindings
  - Uses github.com/lancedb/lancedb-go with Apache Arrow Go bindings
- [x] CGO dependency managed correctly (lib/linux_amd64/ shared lib)
  - lib/linux_amd64/ has liblancedb_go.so and liblancedb_go.a; Makefile sets CGO_LDFLAGS and LD_LIBRARY_PATH
- [x] Insert, Search, Delete operations
  - Upsert(), VectorSearch(), DeleteByFilePath() + GetByFilePath(), GetByName(), DropAndRecreateTable()
- [x] Search returns scored results sorted by similarity
  - Score = 1.0 - distance; LanceDB returns nearest-first
- [x] Test creates temp LanceDB, inserts vectors, searches, verifies scores
  - TestNewStore_And_RoundTrip: full CRUD cycle. TestVectorSearch_WithFilters: language, chunk_type, file prefix filters.

### Epic 06: Description Generator
- [x] `internal/codeintel/describer/` calls LLM to generate code descriptions
  - LLMCompleter interface; DescribeFile() truncates, formats, calls LLM, parses JSON response
- [x] Descriptions used to enrich chunks before embedding
  - Description field stored in vectorstore schema; FormatRelationshipContext() builds structured context
- [x] Test uses mock provider
  - mockLLM and errorLLM structs; 10 test functions covering JSON parsing, truncation, errors, cancellation

### Epic 07: Indexing Pipeline
- [x] `internal/codeintel/indexer/` orchestrates: parse → describe → embed → store
  - IndexRepo() calls walkAndParse() → buildReverseCallGraph() → describeEmbedStore()
- [x] Content hashing for incremental indexing (skip unchanged files)
  - ContentHash() compared against loaded hashes; unchanged files skipped
- [x] File state tracking for indexed files
  - Uses rag_file_hashes.json (JSON file). Same approach as reference codebase. Spec's SQLite table was aspirational — JSON is simpler and correct.
- [x] Test covers full pipeline: index a file, verify chunks stored, reindex unchanged (no-op)
  - TestIndexRepo_BasicPipeline, TestIndexRepo_IncrementalSkipsUnchanged, TestIndexRepo_ForceReindexes, TestIndexRepo_ExcludeGlobs

### Epic 08: Searcher
- [x] `internal/codeintel/searcher/` implements multi-query expansion
  - Iterates all queries, embeds each, runs VectorSearch per query, deduplicates by chunk ID, ranks by hitCount
- [x] Dependency hop expansion: finds callers/callees of top results
  - **FIXED**: expandHops now supports multi-depth via frontier pattern, iterating HopDepth rounds
- [x] `SearchOptions` respected: TopK, MaxResults, Filter, HopDepth
  - **FIXED**: HopDepth now read by searcher — defaults to 1, supports multi-hop traversal
- [x] Zero results returns empty slice, not error
- [x] Test covers: basic search, multi-query, hop expansion, filters
  - **FIXED**: Added TestSearch_HopDepthRespected (2-hop chain verification) and TestSearch_FilterPassthrough

### Epic 09: Structural Graph
- [x] `internal/codeintel/graph/` builds call graph from parsed code
  - GoAnalyzer (go/types + AST), PythonAnalyzer (tree-sitter), TSAnalyzer (Node.js subprocess)
- [x] Language-specific analyzers for Go, Python, TypeScript
  - Each with Analyze() method; config-driven enable/disable via AnalyzerConfig
- [x] `BlastRadius` returns upstream (callers) and downstream (callees)
  - Recursive CTEs for both directions; cycle detection via path strings; depth + budget limits
- [x] Interface implementations tracked
  - goTypes.Implements() for value and pointer receivers; IMPLEMENTS edges; getInterfaces() in store
- [x] SQLite-backed storage with `graph_nodes` and `graph_edges` or equivalent
  - Tables: symbols (equiv graph_nodes), edges (equiv graph_edges), boundary_symbols, chunk_mapping, graph_meta
- [x] Test covers: build graph, query upstream/downstream, interface resolution
  - 19 test functions: BlastRadius upstream/downstream/cycle/maxDepth, analyzer tests per language, resolver dispatch

### Cross-cutting
- [x] No nil pointer panics when optional components are missing (nil searcher, nil graph)
  - RetrievalOrchestrator guards searcher/graph with nil checks. SearchSemantic guards nil searcher. RegisterSearchTools guards registration. Store.Close() nil-checks table/conn.
- [x] All CGO tests pass with `make test` (LanceDB linker flags)
  - All 50+ tests pass across 9 packages. Exit code 0.
- [x] `go test -race ./internal/codeintel/...` — no data races
  - Race detector clean on both codeintel and vectorstore packages.

---

## Audit Summary

**Initial Audit Date**: 2026-03-31
**Fixes Completed**: 2026-03-31
**Result**: ALL 45 ITEMS PASS ✓

### Fixes Applied

| # | Issue | Resolution |
|---|-------|------------|
| 1 | HopDepth defined but unused in searcher | Implemented multi-depth hop expansion with frontier pattern |
| 2 | ChunkType enum mismatch with spec | Spec was aspirational — actual values correct for multi-language system |
| 3 | No tree-sitter fallback in GoASTParser | Added WithFallback() method, delegates non-Go and unrecognized files |
| 4 | No generics test in goparser | Added TestParse_Generics with Set[T comparable] fixture |
| 5 | Go interfaces tagged as ChunkTypeType in tree-sitter | Fixed: inspect type_spec child, tag interface_type as ChunkTypeInterface |
| 6 | Tree-sitter parse failure returns error | Changed to return nil, nil with slog.Warn |
| 7 | No filter passthrough test in searcher | Added TestSearch_FilterPassthrough |
| 8 | Missing interface/embedded-type tests in goparser | Added TestParse_InterfaceExtraction and TestParse_EmbeddedTypes |
| 9 | Index state spec said SQLite, code uses JSON | Spec was aspirational — JSON file matches reference codebase, simpler and correct |

### Pass Rate by Epic

| Epic | Items | Pass |
|------|-------|------|
| 01 Types & Interfaces | 5 | 5 |
| 02 Tree-sitter Parsers | 5 | 5 |
| 03 Go AST Parser | 5 | 5 |
| 04 Embedding Client | 4 | 4 |
| 05 LanceDB Store | 5 | 5 |
| 06 Description Generator | 3 | 3 |
| 07 Indexing Pipeline | 4 | 4 |
| 08 Searcher | 5 | 5 |
| 09 Structural Graph | 6 | 6 |
| Cross-cutting | 3 | 3 |
| **Total** | **45** | **45** |

### Strengths Noted
- Excellent test coverage across all packages (50+ tests, all passing)
- Zero data races under `-race` detector
- Robust nil safety with proper guards on all optional interfaces
- Clean pipeline orchestration in indexer
- Sophisticated blast radius with recursive CTEs, cycle detection, and depth limits
- Three-language graph analysis (Go via go/types, Python via tree-sitter, TS via Node.js)
- Well-designed batching in embedder with sub-batch size control
