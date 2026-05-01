# 02 — Tech Stack Decisions

**Status:** Draft v0.1
**Last Updated:** 2026-05-01
**Author:** Mitchell

---

## Decision Log

Each decision records what was chosen, what alternatives were considered, and why. This is the document future-you reads when wondering "why did we pick X."

---

## Core Language: Go

**Decision:** Go is the implementation language for the entire backend, CLI, and server.

**Rationale:**
- Single binary distribution via `go build`. No runtime dependencies, no interpreters, no virtual environments.
- Carries forward from topham — existing Go code (tree-sitter parsers, LanceDB integration, SQLite patterns) ports directly.
- Native concurrency model (goroutines, channels) fits the agent loop well — streaming responses, concurrent tool execution, background indexing.
- Lower memory footprint than Python for a long-running local server.

**Alternatives considered:**
- *Python:* Hermes is Python, so the provider code could be directly copied rather than translated. Rejected because it contradicts the single-binary goal and would require users to manage a Python environment.
- *Rust:* Performance benefits don't justify the development velocity tradeoff for a personal tool. Go is fast enough.
- *TypeScript (full-stack):* Would unify frontend and backend. Rejected because topham's Go code is the foundation, and Node.js as a local server is heavier than a Go binary.

---

## CGo: Accepted

**Decision:** CGo is accepted as a build dependency. The project will require CGo-enabled compilation.

**Rationale:**
- Tree-sitter (code parsing) requires CGo bindings. There is no pure-Go equivalent with comparable quality. This is non-negotiable.
- LanceDB (vector store) also uses CGo. Since CGo is already required for tree-sitter, LanceDB's CGo dependency is not an incremental cost.
- Cross-compilation is not a concern. This builds and runs on one machine.

**Implications:**
- Build requires `CGO_ENABLED=1` and a C compiler.
- Makefile-driven build (`make build`, `make test`) rather than bare `go build`. Carries forward from topham.
- Binary is platform-specific (Linux amd64 for the target machine).

---

## Database: SQLite

**Decision:** SQLite for all structured persistence — conversations, messages, sub-calls, index state, metrics.

**Rationale:**
- Embedded, zero-config, single-file database. No external process to manage.
- Battle-tested persistence for single-user applications.
- Carries forward from topham's SQLite patterns.
- Full-text search (FTS5) available if needed for conversation search.

**Driver decision: Deferred.** Two viable options:
- `mattn/go-sqlite3` (CGo) — most widely used, already in CGo land anyway.
- `modernc.org/sqlite` (pure Go) — avoids adding another CGo dependency, but CGo is already accepted.
- Leaning toward `mattn/go-sqlite3` since CGo is a given, but this can be decided at implementation time. No architectural impact.

**Alternatives considered:**
- *PostgreSQL:* Overkill for single-user local app. Requires running a separate server process. Would enable pgvector for embeddings, but adds operational complexity.
- *Bolt/bbolt:* Key-value only, no relational queries. Too primitive for the data model.

---

## Vector Store: LanceDB

**Decision:** Use LanceDB for derived vector indexes.

**Status:** Implemented for both code and brain semantic retrieval.

**What we know:**
- LanceDB is integrated via CGo bindings.
- Code semantic chunks are stored under `.yard/lancedb/code`.
- Brain semantic chunks are stored under `.yard/lancedb/brain`.
- Both stores support similarity search through the runtime searcher layer.

**Ongoing evaluation criteria:**
- Given a natural language query ("auth middleware", "how does routing work"), does it return the right code chunks?
- Are the chunks well-formed (complete functions, not arbitrary text splits)?
- Is the ranking sensible (most relevant first)?
- How does it handle queries that span multiple files or concepts?

**Alternatives if LanceDB fails evaluation:**
- **sqlite-vec:** Keeps everything in SQLite. Simpler stack, one less dependency. May be sufficient at codebase scale (tens of thousands of chunks, not millions).
- **pgvector:** Known to be good, but requires running PostgreSQL. Significant operational overhead for a local tool. Last resort.
- **Brute-force cosine similarity in SQLite:** Store embeddings as BLOBs, compute distance in Go. At codebase scale this might be fast enough. Simplest possible approach.

**Operational note:** Retrieval quality still needs continued validation with real indexed projects, but the architectural decision is no longer pending.

---

## Code Parsing: tree-sitter (CGo)

**Decision:** tree-sitter for code parsing and symbol extraction.

**Rationale:**
- Industry-standard incremental parser. Used by every major editor and code intelligence tool.
- Produces concrete syntax trees that enable precise symbol extraction — functions, types, interfaces, structs, methods, imports, exports with full position info.
- Carries forward from topham with parsers for Go, TypeScript/TSX, and Python.
- No pure-Go alternative provides equivalent quality.

**Implications:**
- CGo dependency (accepted, see above).
- Each supported language requires its own tree-sitter grammar. Adding a new language means adding a grammar dependency and writing extraction logic.
- Grammar versions should be pinned for reproducibility.

---

## Embeddings: nomic-embed-code via Docker

**Decision:** nomic-embed-code running in a Docker container (port 8081) for code embeddings.

**Rationale:**
- Code-specialized embedding model. Better semantic understanding of code than general-purpose embedding models.
- Runs locally — no API costs, no external service dependency for indexing.
- Docker container infrastructure already exists from topham's setup.
- The target machine has an RTX 4090 (24GB VRAM), more than sufficient.

**Alternatives considered:**
- *Cloud embedding APIs (OpenAI text-embedding-3-small, Voyage Code):* Dirt cheap but introduces an API dependency for core functionality. Rejected for the default path; could be added as an optional provider later.
- *ONNX runtime embedded in Go binary:* Would eliminate Docker dependency for embeddings. Significant implementation effort for marginal benefit given Docker is already in the stack.

---

## Local LLM Inference: Docker Container

**Decision:** Local LLM inference via Docker container (port 8080) with configurable model.

**Rationale:**
- RTX 4090 with 24GB VRAM supports running 7B-32B parameter models locally.
- Docker isolates the inference runtime from the Go application.
- Model is configurable: Qwen2.5-Coder-7B, DeepSeek-R1-32B, Mercury 2, or others.
- Carries forward from topham's Docker Compose setup.

**Note:** Local LLM is optional. The primary inference path is via subscription-based cloud providers (Claude, Codex). Local models serve as a fallback, a cost-free option for simple queries, or for offline use.

---

## Frontier LLM Access: Subscription Credential Reuse

**Decision:** Access Claude via OAuth credential reuse from Claude Code and Codex via Yard-managed OpenAI device-code auth. No per-token API costs.

**Rationale:**
- This is how the developer currently works — Claude Pro/Max subscription via Claude Code, Codex subscription through Yard's `yard auth login codex` flow.
- Hermes Agent (MIT licensed) has proven this pattern works and documented the credential discovery paths.
- Reimplemented in Go (see [[03-provider-architecture]] for details).

**Credential sources:**
- Anthropic/Claude: `~/.claude/.credentials.json` (OAuth tokens from Claude Code)
- OpenAI/Codex: `~/.sirtopham/auth.json`; `CODEX_HOME/auth.json` or `~/.codex/auth.json` is a one-time bootstrap import source only

**Alternatives considered:**
- *Direct API keys with per-token billing:* Works but costs money unnecessarily when subscriptions already provide access.
- *Shelling out to Claude Code / Codex CLIs:* Simpler but loses control over streaming, tool calling format, and error handling. Hermes tried both approaches and settled on credential reuse with direct API calls.

---

## Operator Console TUI: Bubble Tea + Bubbles + Lip Gloss

**Decision:** Use the Charm stack for the target daily-driver terminal UI:

- Bubble Tea for the event loop and full-window terminal app architecture.
- Bubbles for common components such as lists, tables, text inputs, text areas, spinners, progress bars, and viewports.
- Lip Gloss for terminal styling and layout.

**Status:** Selected target direction.

**Rationale:**
- The operator works in the terminal with Codex and normal development tools, so the daily-driver interface should live there.
- The app is local, single-user, keyboard-heavy, and operational. Those constraints fit a TUI better than a full browser command center.
- The implementation stays in Go and can call the same internal runtime builders, chain store, and status services as Cobra commands and HTTP handlers.
- Bubble Tea's message/update/view model is a good fit for live chain events, background refreshes, key-driven navigation, and deterministic rendering tests.
- Bubbles avoids hand-rolling basic controls for lists, tables, text inputs, text areas, spinners, progress bars, and scrollable panes.

**Alternatives considered:**
- *tview:* Provides a traditional widget toolkit and can build forms/tables quickly. Rejected for the target app because Yard needs a bespoke operational console with live event streams and composable state more than a conventional widget tree.
- *tcell directly:* Powerful low-level terminal input/screen library. Rejected because it would force Yard to build too much framework code.
- *Browser-only command center:* Richer visual layout, but adds frontend ceremony to workflows that are naturally terminal-native.

See [[20-operator-console-tui]] for the product and implementation target.

---

## Web Inspector: React + TypeScript + Vite

**Decision:** Retain React + TypeScript + Vite for the browser inspector served by `yard serve`, compiled and embedded in the Go binary via `embed.FS`.

**Status:** Retained for rich inspection, no longer the primary operator surface.

**Rationale:**
- Rich component ecosystem for syntax highlighting, rendered markdown, diff viewers, file trees, and charts.
- TypeScript provides type safety for the existing non-trivial frontend.
- Vite keeps frontend development fast.
- Tailwind CSS + shadcn/ui remain acceptable for the browser inspector.
- `embed.FS` in Go means the compiled frontend ships inside the binary. No separate frontend server in production.

**Scope boundary:**
- The web app should not duplicate the entire TUI as a second command center.
- The web app owns views that are meaningfully better in a browser: rich context inspection, detailed tool-call rendering, side-by-side diffs, conversation transcript browsing, charts, and optional document intake.
- The terminal console owns daily operations: readiness, launch, chain control, live event following, receipt browsing, and jump-to-editor workflows.

See [[07-web-interface-and-streaming]] and [[21-web-inspector]].

---

## Build System: Makefile

**Decision:** Makefile for builds. `make build`, `make test`, `make dev`.

**Rationale:**
- Required for CGo/LanceDB compilation flags.
- Carries forward from topham.
- Simple, universal, no additional tooling required.

---

## Hardware Target

**Primary development and runtime environment:**
- RTX 4090 (24GB VRAM) for local model inference
- Docker Compose for LLM container (port 8080) + embeddings container (port 8081)
- Linux (Ubuntu/similar)

This is a single-machine, single-developer tool. The hardware is known and fixed. No need to design for variable environments.

---

## Summary Table

| Component | Choice | Status |
|---|---|---|
| Language | Go | ✅ Decided |
| CGo | Accepted | ✅ Decided |
| Database | SQLite | ✅ Decided |
| Vector store | LanceDB | Implemented |
| Code parsing | tree-sitter (CGo) | ✅ Decided |
| Embeddings | nomic-embed-code (Docker) | ✅ Decided |
| Local LLM | Docker container, configurable model | ✅ Decided |
| Frontier LLM | Credential reuse (Claude + Codex subs) | ✅ Decided |
| Operator TUI | Bubble Tea + Bubbles + Lip Gloss | Selected target |
| Web inspector | React + TypeScript + Vite | Retained |
| Web styling | Tailwind CSS + shadcn/ui | Retained |
| Frontend embed | Go `embed.FS` | ✅ Decided |
| Build | Makefile | ✅ Decided |
| Streaming | WebSocket for web; internal event subscription/polling for TUI | Decided direction |
