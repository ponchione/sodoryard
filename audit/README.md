# Audit Plans

Explicit audit plans for each layer of the sirtopham codebase. Each document
tells the auditing session exactly what to verify, where to look, and what
tests to run — no context about the build history is assumed.

## How to Use

Give the auditing session this README plus the specific layer doc. Each doc
is self-contained: it lists the packages, the spec/epic docs to cross-reference,
the exact test commands, and a checklist of things to verify.

Run audits in order (Layer 0 → 6) since later layers depend on earlier ones
being correct. Each audit is independent though — you can run any single layer
audit without the others.

## Layer Index

| File | Layer | Scope |
|------|-------|-------|
| layer0-foundation.md | 0 | Config, SQLite, logging, UUIDv7, schema/sqlc |
| layer1-code-intelligence.md | 1 | Tree-sitter, Go AST, embedder, LanceDB, graph, searcher, indexer |
| layer2-provider.md | 2 | Provider interface, Anthropic/OpenAI/Codex backends, router |
| layer3-context-assembly.md | 3 | Turn analyzer, query extraction, retrieval, budget, compression |
| layer4-tool-system.md | 4 | Tool interface, file/search/git/shell/brain tools |
| layer5-agent-loop.md | 5 | Event system, conversation manager, prompt builder, loop core |
| layer6-web-interface.md | 6 | HTTP server, REST API, WebSocket, React frontend |

## Test Command

All tests across all layers:
```
make test
```
This sets the required CGO linker flags for LanceDB. Never use `go test ./...`
directly — it will fail with linker errors on the vectorstore package.

## Project Stats (at time of audit plan creation)

- 7 layers, 49 epics, all implemented
- ~130 Go source files, ~90 test files
- ~22,800 lines Go, ~3,500 lines TypeScript
- 3 tech debt items resolved (see TECH-DEBT.md)
