# sirtopham

Local Go implementation of the sirtopham coding-agent harness.

## Current Phase

v0.1 harness closeout is effectively complete.

The project is now in incremental v0.2 brain work. The first proactive slice is already live and the current operator-facing contract is intentionally explicit:
- proactive MCP/vault-backed keyword brain retrieval during context assembly
- brain-aware budget fitting / serialization / inspector reporting
- dedicated ordered signal-flow observability at `/api/metrics/conversation/:id/context/:turn/signals`
- reserve/estimate/reconcile token-budget reporting on the context report surface
- a repeatable live validation package at `docs/v2-b4-brain-retrieval-validation.md` plus `scripts/validate_brain_retrieval.py`
- maintained live validation scenarios for a fact canary, a rationale note family, and a prior-debugging/history note family
- keyword-backed brain retrieval remains the real runtime path today; semantic/index-backed brain retrieval is reserved future work unless code and validation land for it

If you are resuming implementation work, read these first:
- `TECH-DEBT.md`
- `NEXT_SESSION_HANDOFF.md`
- `docs/specs/09-project-brain.md`
- `docs/specs/06-context-assembly.md`

## What Matters For Bring-Up

- The backend binary is built from `./cmd/sirtopham`.
- The production build embeds the frontend from `web/dist/`.
- Real retrieval depends on running `sirtopham index` before `sirtopham serve`.
- Indexing requires the local embedding service; qwen-coder is optional today because the current indexing path uses a no-op describer.

## Requirements

- Go toolchain
- Node.js and npm
- The bundled LanceDB shared library in `lib/linux_amd64`
- At least one working provider configuration for runtime turns
- Local indexing service:
  - embeddings / nomic-embed-code at `http://localhost:12435`
- Optional local describer service if/when description generation is re-enabled:
  - qwen-coder at `http://localhost:12434`

## Quick Start

1. Initialize project state if needed.
   - `go run -tags sqlite_fts5 ./cmd/sirtopham init`
2. Review the generated config.
   - `go run -tags sqlite_fts5 ./cmd/sirtopham config`
3. Configure provider credentials in `sirtopham.yaml` or environment.
4. Check the local indexing services.
   - `go run -tags sqlite_fts5 ./cmd/sirtopham llm status`
   - switch `local_services.mode` to `auto` before using `go run -tags sqlite_fts5 ./cmd/sirtopham llm up`
5. Build the code index.
   - `go run -tags sqlite_fts5 ./cmd/sirtopham index`
6. Start the app.
   - `go run -tags sqlite_fts5 ./cmd/sirtopham serve`

## Common Commands

- `make test`
- `make build`
- `make dev-backend`
- `make dev-frontend`
- `./bin/sirtopham config`
- `./bin/sirtopham index`
- `./bin/sirtopham serve --dev`

## Notes

- `sirtopham init` creates `.<project>/` for SQLite and LanceDB state plus a repo-local `.brain/` vault.
- `sirtopham config` is the fastest way to confirm effective paths, default/fallback routing, and the embedding endpoint; it is not a full dump of every configured provider surface.
- If you skip `sirtopham index`, the app can start, but semantic retrieval and context inspection will not reflect a real indexed project.
- The older architecture docs under `docs/` are useful background, but this README, `TECH-DEBT.md`, `NEXT_SESSION_HANDOFF.md`, and the live runtime/metrics surfaces are the practical source of truth for bring-up.
