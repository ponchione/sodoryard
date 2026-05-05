# Next Session Handoff

Date: 2026-05-05

## Current State

The repository was clean before this handoff document was added. The latest completed work is:

- `5c412f6 Persist actual headless usage metrics`
- `44b3109 Add chain dogfooding metrics`
- `87a3337 Tighten Shunter audit smoke surfaces`

Shunter is the base brain/project-memory design. Do not reintroduce legacy migration, vault, SQLite import/export, compatibility aliases, public memory commands, or backwards-compatible command surfaces.

## What Changed Tonight

`yard chain metrics <chain-id>` now exists and is the primary quick check for dogfooding chain health. It reports:

- chain health: `ok`, `attention`, or `failing`
- step counts and per-step rows
- token, turn, duration, resolver-loop, and budget usage
- event counts including output, step failures, safety limits, reindexing, and child process start/exit counts
- concrete warning lines for suspicious harness behavior

The live metrics smoke found two real issues, both fixed:

- Headless child receipts could contain model-authored `tokens_used: 0` / `turns_used: 0`; the headless runner now rewrites receipt usage fields from actual `RunTurn` metrics before returning.
- `./bin/yard` could spawn a stale `tidmouth` from `PATH`; the spawn tool now prefers a sibling `tidmouth` beside the running `yard` binary when available.

The smoke-test docs and README now include the metrics command in the dogfooding path.

## Validation Already Run

These passed after the final commits:

```bash
rtk make test
rtk make build
rtk ./bin/yard config
rtk ./bin/yard auth status
rtk ./bin/yard doctor
```

Runtime state from the final live smoke:

```text
chain_id: 019df9f0-fd8f-768b-a4d8-bd8094922000
metrics: health=ok warnings=0 tokens=251436 turns=10 duration=39s
provider/model: codex / gpt-5.5
auth: healthy, yard_store, expires 2026-05-15T20:19:35Z
local services: docker/compose available; nomic-embed and qwen-coder healthy/reachable/model-ready
```

Inspect command:

```bash
rtk ./bin/yard chain metrics 019df9f0-fd8f-768b-a4d8-bd8094922000
```

## Next Agent Starting Point

Start by confirming the same baseline:

```bash
rtk git status --short
rtk git log --oneline -8
rtk make test
rtk make build
rtk ./bin/yard config
rtk ./bin/yard auth status
rtk ./bin/yard doctor
```

Then dogfood with a real but bounded one-step chain:

```bash
rtk ./bin/yard chain start \
  --role coder \
  --max-steps 1 \
  --max-duration 2m \
  --task "<small read-only or tightly scoped task>"

rtk ./bin/yard chain metrics <chain-id>
rtk ./bin/yard chain receipt <chain-id> 1
```

## Useful Next Work

The most useful next slice is not more legacy cleanup. It is dogfooding and performance/ergonomics tuning around the active Shunter-native harness.

Good candidates:

- Investigate why a simple one-sentence read-only chain used 10 turns and about 251k tokens. The metrics command now makes this visible; the next useful work is reducing that behavior.
- Surface the same chain metrics report in the TUI or web inspector if dogfooding shows the CLI is not enough.
- Improve launch prompts or role instructions so small read-only tasks finish faster and avoid unnecessary broad searches.
- Keep watching for metrics warnings after real chains: zero token/turn usage, missing receipts, process started/exited mismatch, failed events, or aggregate-vs-step drift.

## Guardrails

- Use `rtk` for shell commands.
- Prefer `rtk make test` and `rtk make build`.
- If running Go directly, use `-tags sqlite_fts5` with the Makefile CGO/LanceDB environment.
- Do not churn `yard.yaml`, `.yard/`, `.brain/`, generated web output, or local runtime state unless the task requires it.
- Do not restore legacy vault, migration, import/export, or compatibility command surfaces.
- Keep `tidmouth` limited to the internal engine subprocess contract unless intentionally redesigning the spawn contract.
