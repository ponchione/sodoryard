# Phase 3 — SirTopham Orchestrator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `cmd/sirtopham/` into a working chain orchestrator binary that runs an LLM-driven agent (the orchestrator) with custom tools (`spawn_agent`, `chain_complete`), executes ordered chain steps against `tidmouth run` subprocesses, tracks state in SQLite, and produces a receipt at `receipts/orchestrator/<chain-id>.md` when done.

**Architecture:** SirTopham is itself a headless agent session that uses a narrowed toolkit (brain + two custom tools). The `spawn_agent` custom tool execs `tidmouth run` as a subprocess and blocks until it completes. Chain state (`chains`, `steps`, `events` tables) lives alongside existing Tidmouth state in `.yard/yard.db`. The orchestrator agent reads receipts from the brain via its normal brain tools, decides what to do next, and dispatches engines by calling `spawn_agent` again. The `chain_complete` tool signals the AgentLoop to stop via a new sentinel error `tool.ErrChainComplete`.

**Tech Stack:** Go 1.22+, SQLite via `internal/db` (sqlc for chain queries), existing `internal/agent` AgentLoop reused as the orchestrator's runtime, existing `internal/brain` for brain access, new `os/exec`-based subprocess management for `spawn_agent`.

---

## Scope and Context

This plan executes Phase 3 of `sodor-migration-roadmap.md`, driven by `docs/specs/15-chain-orchestrator.md`. The design is already written and does not need re-designing — this plan translates it into task-by-task code.

**Roadmap translation:** this document is the concrete implementation plan for roadmap Phase 3, not a second design pass. Each roadmap step is implemented here as follows:

- **Roadmap 3.1 — Chain state schema** → Tasks 2-3 (`internal/db` schema + sqlc queries + `internal/chain` store)
- **Roadmap 3.2 — Chain config format** → this plan pins an MVP execution contract instead of inventing a separate declarative chain YAML format. In Phase 3, chain inputs come from `sirtopham chain` CLI flags (`--specs`, `--task`, `--chain-id`, limit flags), the existing `yard.yaml` orchestrator role, and persisted chain rows in `.yard/yard.db`. A standalone chain-definition file format is deferred.
- **Roadmap 3.3 — `spawn_agent` tool** → Task 6
- **Roadmap 3.4 — `chain_complete` tool** → Task 5
- **Roadmap 3.5 — Receipt parser** → Task 1
- **Roadmap 3.6 — Orchestrator agent loop** → Tasks 4, 5, and 7
- **Roadmap 3.7 — Reindex hooks** → Task 6 plus Task 11 verification; the MVP lands `reindex_before` only
- **Roadmap 3.8 — `cmd/sirtopham/main.go` / CLI** → Tasks 7-9
- **Roadmap 3.9 — Verify** → Task 12

Where this plan differs from the roadmap's older wording, the code-facing choices below are authoritative for Phase 3 implementation.

**Agent handoff contract:** another coding agent should be able to execute this plan without reopening product questions. Treat this document as implementation-authoritative for Phase 3. If code reality forces a change, the agent should stop only for one of these reasons:

1. A referenced file/function/package no longer exists and there is no obvious equivalent.
2. A task cannot be completed without changing user-pinned behavior listed below.
3. A live runtime/test result contradicts the plan's intended behavior in a way that cannot be resolved with a narrow implementation edit.

Otherwise, adapt mechanically and keep going. Do not redesign the orchestrator architecture, invent a second config surface, or broaden scope into Phase 4/5/6 work.

**Execution order for handoff:** complete tasks in this exact dependency order unless a later task is explicitly marked verification-only:

1. Task 1 — shared receipt parser
2. Tasks 2-3 — schema, sqlc, and `internal/chain` store
3. Tasks 4-6 — custom-tools plumbing, `chain_complete`, `spawn_agent`
4. Tasks 7-9 — `cmd/sirtopham` CLI integration and operator commands
5. Tasks 10-11 — prompt refinement and reindex verification
6. Tasks 12-13 — live verification and docs alignment

A handoff agent should not start Tasks 7-13 until Tasks 1-6 are green in local tests.

**Definition of done for Phase 3:**

- `bin/sirtopham` is a real CLI binary with `chain`, `status`, `logs`, `receipt`, `cancel`, `pause`, and `resume` subcommands.
- A chain run persists rows in `.yard/yard.db` across `chains`, `steps`, and `events`.
- The orchestrator can call `spawn_agent`, receive a parsed receipt, and terminate cleanly via `chain_complete`.
- Pause/cancel status is enforced at step boundaries by `spawn_agent`/chain limit checks.
- At least one end-to-end smoke chain completes successfully and writes an orchestrator receipt plus an engine receipt into the brain.
- `docs/specs/15-chain-orchestrator.md` is updated to match the shipped Phase 3 implementation choices.

**Prerequisite commits already on main:**

- `v0.2.1-yard-paths` (commit `c080551`) — canonical `.yard/yard.db` state dir, constants in `internal/config`
- `8949c84` — `search_text` excludes `.yard/`
- `c080551` — `buildAppRuntime` bootstraps schema before upgrade helpers

**What's in scope:**

- `internal/receipt/` — pure frontmatter parser, usable by SirTopham
- SQLite `chains`, `steps`, `events` tables via the existing `internal/db` schema pipeline (sqlc)
- `internal/chain/` — Go wrappers around the sqlc queries + limit enforcement + event logging
- `internal/role/` extension: custom tools factory via `BuilderDeps`
- `internal/tool/` extension: `ErrChainComplete` sentinel
- `internal/agent/loop.go` extension: detect sentinel and stop iteration
- `internal/spawn/` — `spawn_agent` custom tool implementation (subprocess exec, receipt read, metrics aggregation)
- `cmd/sirtopham/` — CLI with `chain`, `status`, `logs`, `receipt`, `cancel`, `pause`, `resume` subcommands
- `agents/orchestrator.md` — refined stub prompt (NOT the Phase 4 production prompt — good enough to drive a minimal smoke-test chain)
- Reindex hooks (`reindex_before` flag on `spawn_agent`)
- Final regression smoke test + tag `v0.4-orchestrator`

**What's explicitly out of scope (future work):**

- **Parallel engine execution** — spec 15 "Future Extensions". Serialized only.
- **Standalone declarative chain YAML format** — roadmap Step 3.2 asked for a chain config format; for Phase 3 MVP we use CLI inputs + `yard.yaml` role config + persisted chain state instead.
- **Chain templates** — spec 15 "Future Extensions".
- **Chain forking** — spec 15 "Future Extensions".
- **Cost-aware routing** — spec 15 "Future Extensions".
- **`request_human_input` checkpoint tool** — spec 15 "Future Extensions".
- **`reindex_after` hook** — roadmap Step 3.7 mentioned before/after hooks; Phase 3 MVP implements `reindex_before` only.
- **Full session resumption on `sirtopham resume`** — MVP only writes the flag; starting a fresh orchestrator session with "here's what already happened" context is a follow-up.
- **Production orchestrator system prompt** — Phase 4.
- **Knapford dashboard integration** — Phase 6.
- **`sirtopham.db` as a separate database file** — spec 15 mentioned this but the user pinned `.yard/yard.db` as the single shared DB in the Phase 5a brainstorming. Tables for chain state go into the existing Tidmouth DB.

**Pinned design decisions (don't reopen without asking):**

1. **Custom tool name is `spawn_agent`** (not `spawn_engine`). This matches the already-checked-in `yard.yaml` orchestrator role (`custom_tools: [spawn_agent, chain_complete]`). Spec 15 uses `spawn_engine` in prose — update the spec to match the code in Task 13 (docs follow-up), do NOT flip `yard.yaml` to match the spec.
2. **Loop termination is a sentinel error**, not a return-value flag. The `chain_complete` tool writes the receipt, updates chain state, then returns `tool.ErrChainComplete`. `internal/agent/loop.go` detects this specific error and exits its iteration loop with a clean `TurnResult`. Rationale: adding a bool field to every tool result would pollute the tool interface for one use case; sentinel errors are cheap, backwards-compatible, and already how the loop handles other stop conditions (timeout, cancel, max iterations).
3. **Custom tools extension point is a factory map on `BuilderDeps`**. Today `internal/role/builder.go:28` rejects any role with `custom_tools`. Phase 3 makes `BuilderDeps` carry a `CustomToolFactory map[string]func() tool.Tool` field. When the caller is `cmd/tidmouth`, it passes nil and the old rejection behavior is preserved. When the caller is `cmd/sirtopham`, it passes a factory containing `spawn_agent` and `chain_complete` constructors. This means `tidmouth run` cannot register custom tools even if invoked with an orchestrator role config — exactly what we want, because the orchestrator should only run through `sirtopham chain`.
4. **Roadmap Step 3.2 is implemented as an execution contract, not a new file format.** `sirtopham chain` accepts the chain's source material (`--specs` or `--task`) and execution limits via CLI flags; `yard.yaml` supplies the orchestrator role definition; `.yard/yard.db` persists the live chain row and step/event history. Do NOT add a second "chain YAML" surface in this phase.
5. **`.yard/yard.db` is the shared database for both Tidmouth and SirTopham state.** The roadmap's later prose sometimes implies a separate chain DB, but for Phase 3 the code should treat `.yard/yard.db` as the single canonical SQLite file. Chain tables are added to `internal/db/schema.sql` alongside the existing ten tables. `EnsureChainSchema` in `internal/db/init.go` is the idempotent migration helper.
6. **MVP pause/resume**: `sirtopham pause <chain-id>` writes `status='paused'` to the `chains` row. `spawn_agent` checks this flag before exec and returns a "chain paused" tool error. `sirtopham resume <chain-id>` writes `status='running'` back but does NOT restart the orchestrator session — the user must rerun `sirtopham chain --chain-id <chain-id>` to start a fresh session that reads the existing receipts. This is explicitly documented in the `resume` subcommand output. Full auto-resume is follow-up work.
7. **MVP cancel**: `sirtopham cancel <chain-id>` writes `status='cancelled'`. If the orchestrator session is running in a foreground shell, the user can Ctrl-C it. If it's running headless (not implemented in this phase), the cancel flag is checked by `spawn_agent` at step boundaries.
8. **Roadmap Step 3.7 is narrowed for MVP.** Implement `reindex_before` on `spawn_agent` now. Do not add `reindex_after` unless asked; track it as deferred work instead.

---

## File Structure

**New files:**

- `internal/receipt/types.go` — `Receipt`, `Verdict` types
- `internal/receipt/parser.go` — `Parse(content string) (Receipt, error)` + `ParseFrontmatter`
- `internal/receipt/parser_test.go`
- `internal/db/query/chains.sql` — sqlc source for chain/step/event queries
- `internal/db/chains.sql.go` — sqlc-generated (do not hand-edit)
- `internal/chain/state.go` — Go wrappers around the sqlc queries: `StartChain`, `StartStep`, `CompleteStep`, etc.
- `internal/chain/limits.go` — limit check helpers
- `internal/chain/events.go` — event logging helper
- `internal/chain/state_test.go`
- `internal/chain/limits_test.go`
- `internal/chain/events_test.go`
- `internal/spawn/spawn_agent.go` — `SpawnAgentTool` implementing `tool.Tool`
- `internal/spawn/subprocess.go` — exec helper with SIGTERM → SIGKILL timeout handling
- `internal/spawn/spawn_agent_test.go`
- `internal/spawn/subprocess_test.go`
- `cmd/sirtopham/main.go` — REPLACE the placeholder
- `cmd/sirtopham/runtime.go` — `buildOrchestratorRuntime` (narrower than tidmouth's)
- `cmd/sirtopham/chain.go` — `sirtopham chain` subcommand
- `cmd/sirtopham/status.go` — `sirtopham status` subcommand
- `cmd/sirtopham/logs.go` — `sirtopham logs` subcommand
- `cmd/sirtopham/receipt.go` — `sirtopham receipt` subcommand
- `cmd/sirtopham/cancel.go` — `sirtopham cancel` subcommand
- `cmd/sirtopham/pause_resume.go` — `sirtopham pause` and `sirtopham resume` subcommands
- `cmd/sirtopham/chain_test.go`

**Modified files:**

- `internal/db/schema.sql` — add `chains`, `steps`, `events` CREATE TABLE statements (plus indexes)
- `internal/db/init.go` — add `EnsureChainSchema` helper following the `EnsureContextReportsIncludeTokenBudget` pattern
- `cmd/tidmouth/runtime.go` — call `EnsureChainSchema` in the startup block
- `internal/role/builder.go` — replace the flat `custom_tools` rejection with a factory-based wire-up; update `BuilderDeps` type
- `internal/role/builder_test.go` — add coverage for both factory paths
- `internal/tool/registry.go` (or a new `internal/tool/errors.go`) — add `ErrChainComplete` sentinel
- `internal/agent/loop.go` — detect `ErrChainComplete` after tool execution and exit iteration with a clean `TurnResult`
- `internal/agent/loop_test.go` — cover the sentinel-error exit path
- `cmd/tidmouth/receipt.go` — factor `validateReceiptContent` into `internal/receipt/` or call into it. (The exact factoring depends on what the new receipt package exposes — Task 1 decides.)
- `agents/orchestrator.md` — expand the stub
- `docs/specs/15-chain-orchestrator.md` — docs follow-up: rename `spawn_engine` → `spawn_agent`, pin `.yard/yard.db`

**Not touched (verify at review time):**

- `internal/codeintel/`, `internal/vectorstore/`, `internal/brain/indexer/` — codeintel-side package, unchanged
- `internal/config/` — no config schema changes (orchestrator limits live in the CLI flags and the SQL defaults, not in `yard.yaml`)
- `web/`, `webfs/` — Knapford's problem, Phase 6
- `cmd/knapford/` — stays as a placeholder
- All existing Tidmouth commands (`serve`, `run`, `index`, `init`, `auth`, `llm`, `doctor`, `config`, `brain serve`) — no regressions permitted

## Execution checkpoints

Use these as explicit handoff gates between sessions/agents.

| Checkpoint | Tasks | Required evidence before moving on |
|---|---|---|
| CP1: shared parsing + state foundation | 1-3 | `internal/receipt`, `internal/db`, and `internal/chain` tests pass; chain tables exist in schema/init/sqlc surfaces; no breakage in `cmd/tidmouth` receipt tests |
| CP2: orchestrator runtime primitives | 4-6 | custom tools register only through `cmd/sirtopham`; `chain_complete` cleanly terminates the loop; `spawn_agent` records steps/events/metrics and reads receipts |
| CP3: CLI integration | 7-9 | `bin/sirtopham` builds; `chain/status/logs/receipt/cancel/pause/resume` compile and have test coverage; chain status transitions work through the store |
| CP4: phase completion gate | 10-13 | orchestrator prompt is minimally usable; `reindex_before` is verified; live smoke chain passes; spec 15 docs are aligned |

If a session ends mid-checkpoint, update `NEXT_SESSION_HANDOFF.md` with the failing command/test, the exact touched files, and the next unresolved sub-step.

---

## Task 1: `internal/receipt/` — frontmatter parser

**Files:**

- Create: `internal/receipt/types.go`
- Create: `internal/receipt/parser.go`
- Create: `internal/receipt/parser_test.go`
- Modify: `cmd/tidmouth/receipt.go` (delegate validation to the new package)

**Background:** Spec 15's `spawn_agent` tool must read a receipt from the brain after a subprocess exits and return its content to the orchestrator. `cmd/tidmouth/receipt.go` already has `validateReceiptContent` and `splitFrontmatter` helpers, but they live in `package main` and cannot be imported. Phase 3 needs this logic in a shared package. Rather than duplicating, this task extracts the parsing into `internal/receipt/` and has `cmd/tidmouth/receipt.go` call into it.

The new package exposes a `Parse(content []byte) (Receipt, error)` function and a `Verdict` type. The existing `cmd/tidmouth/receipt.go` `receiptFrontmatter` struct becomes an alias or a reference into the new package's type.

- [ ] **Step 1.1: Define types in `internal/receipt/types.go`**

Create the file:

```go
package receipt

import "time"

// Verdict is the outcome verdict an engine agent writes into its receipt
// frontmatter. Spec 13 defines the canonical set.
type Verdict string

const (
	VerdictCompleted          Verdict = "completed"
	VerdictCompletedNoReceipt Verdict = "completed_no_receipt"
	VerdictBlocked            Verdict = "blocked"
	VerdictEscalate           Verdict = "escalate"
	VerdictSafetyLimit        Verdict = "safety_limit"
)

// Receipt is the parsed spec-13 frontmatter block from a receipt doc in the
// brain. Body text (the markdown after the frontmatter) is preserved in
// RawBody so tools that need it can show it to the caller.
type Receipt struct {
	Agent           string        `yaml:"agent"`
	ChainID         string        `yaml:"chain_id"`
	Step            int           `yaml:"step"`
	Verdict         Verdict       `yaml:"verdict"`
	Timestamp       time.Time     `yaml:"timestamp"`
	TurnsUsed       int           `yaml:"turns_used"`
	TokensUsed      int           `yaml:"tokens_used"`
	DurationSeconds int           `yaml:"duration_seconds"`
	RawBody         string        `yaml:"-"`
}
```

- [ ] **Step 1.2: Write parser tests first**

Create `internal/receipt/parser_test.go` with table-driven tests covering:

1. Happy path: a valid receipt with all fields present, timestamp parses correctly, verdict is `completed`, body is preserved.
2. Missing frontmatter delimiter: returns `ErrMissingFrontmatter`.
3. Malformed YAML inside frontmatter: returns a wrapping `yaml:` error.
4. Missing required field `agent`: returns `ErrMissingField` with field name.
5. Negative `step`: returns `ErrInvalidField` with field name.
6. Unknown verdict string: returns `ErrInvalidVerdict` with the bad value.
7. Missing body: still valid (body is optional), `RawBody == ""`.
8. Extra unknown fields: ignored (YAML default, but assert they don't break parsing).

Use this fixture for the happy path:

```go
const happyPath = `---
agent: correctness-auditor
chain_id: smoke-test-p5a
step: 1
verdict: completed
timestamp: 2026-04-11T00:00:00Z
turns_used: 2
tokens_used: 0
duration_seconds: 0
---

Receipt created per request.
`
```

Expected parse result:

```go
Receipt{
    Agent:           "correctness-auditor",
    ChainID:         "smoke-test-p5a",
    Step:            1,
    Verdict:         VerdictCompleted,
    Timestamp:       time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC),
    TurnsUsed:       2,
    TokensUsed:      0,
    DurationSeconds: 0,
    RawBody:         "\nReceipt created per request.\n",
}
```

- [ ] **Step 1.3: Run the tests — confirm they fail**

```bash
make test 2>&1 | grep -E "internal/receipt|FAIL" | head -20
```

Expected: FAIL (package does not exist or does not compile).

- [ ] **Step 1.4: Implement `internal/receipt/parser.go`**

```go
package receipt

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	// ErrMissingFrontmatter is returned when the document body does not
	// contain a leading YAML frontmatter block delimited by ---.
	ErrMissingFrontmatter = errors.New("receipt: missing or malformed YAML frontmatter")
	// ErrMissingField is returned when a required spec-13 frontmatter field
	// is absent. The field name is wrapped into the error message.
	ErrMissingField = errors.New("receipt: missing required field")
	// ErrInvalidField is returned when a field fails its type or value
	// constraint (e.g. negative step).
	ErrInvalidField = errors.New("receipt: invalid field")
	// ErrInvalidVerdict is returned when the verdict value is outside the
	// spec-13 allowed set.
	ErrInvalidVerdict = errors.New("receipt: invalid verdict")
)

// Parse decodes a receipt document's YAML frontmatter and returns a Receipt
// value. The body after the second --- is preserved in Receipt.RawBody.
func Parse(content []byte) (Receipt, error) {
	front, body, ok := splitFrontmatter(content)
	if !ok {
		return Receipt{}, ErrMissingFrontmatter
	}

	var r Receipt
	if err := yaml.Unmarshal(front, &r); err != nil {
		return Receipt{}, fmt.Errorf("receipt: decode yaml: %w", err)
	}
	r.RawBody = string(body)

	if err := r.validate(); err != nil {
		return Receipt{}, err
	}
	return r, nil
}

func (r *Receipt) validate() error {
	if strings.TrimSpace(r.Agent) == "" {
		return fmt.Errorf("%w: agent", ErrMissingField)
	}
	if strings.TrimSpace(r.ChainID) == "" {
		return fmt.Errorf("%w: chain_id", ErrMissingField)
	}
	if r.Step <= 0 {
		return fmt.Errorf("%w: step (must be >= 1, got %d)", ErrInvalidField, r.Step)
	}
	if r.Verdict == "" {
		return fmt.Errorf("%w: verdict", ErrMissingField)
	}
	if !validVerdict(r.Verdict) {
		return fmt.Errorf("%w: %q", ErrInvalidVerdict, r.Verdict)
	}
	if r.Timestamp.IsZero() {
		return fmt.Errorf("%w: timestamp", ErrMissingField)
	}
	if r.TurnsUsed < 0 {
		return fmt.Errorf("%w: turns_used (must be >= 0, got %d)", ErrInvalidField, r.TurnsUsed)
	}
	if r.TokensUsed < 0 {
		return fmt.Errorf("%w: tokens_used (must be >= 0, got %d)", ErrInvalidField, r.TokensUsed)
	}
	if r.DurationSeconds < 0 {
		return fmt.Errorf("%w: duration_seconds (must be >= 0, got %d)", ErrInvalidField, r.DurationSeconds)
	}
	return nil
}

func validVerdict(v Verdict) bool {
	switch v {
	case VerdictCompleted, VerdictCompletedNoReceipt, VerdictBlocked, VerdictEscalate, VerdictSafetyLimit:
		return true
	}
	return false
}

// splitFrontmatter returns (frontBytes, bodyBytes, true) if content begins
// with a valid YAML frontmatter block delimited by --- lines. Otherwise it
// returns (nil, nil, false).
func splitFrontmatter(content []byte) ([]byte, []byte, bool) {
	// Must begin with --- on the first line.
	if !bytes.HasPrefix(content, []byte("---\n")) && !bytes.HasPrefix(content, []byte("---\r\n")) {
		return nil, nil, false
	}
	// Find the closing --- line (must be at the start of a line).
	rest := content[4:] // skip the opening "---\n"
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		end = bytes.Index(rest, []byte("\n---\r\n"))
		if end < 0 {
			return nil, nil, false
		}
	}
	front := rest[:end]
	// Skip past "\n---\n" (5 bytes) or "\n---\r\n" (6 bytes).
	delim := []byte("\n---\n")
	if bytes.HasPrefix(rest[end:], []byte("\n---\r\n")) {
		delim = []byte("\n---\r\n")
	}
	body := rest[end+len(delim):]
	return front, body, true
}
```

- [ ] **Step 1.5: Run the tests — confirm they pass**

```bash
make test 2>&1 | grep -E "internal/receipt|FAIL" | head -20
```

Expected: `ok  github.com/ponchione/sodoryard/internal/receipt`

- [ ] **Step 1.6: Delegate `cmd/tidmouth/receipt.go` validation to the new package**

Open `cmd/tidmouth/receipt.go`. Find `validateReceiptContent` (lines 41-75). Replace its body with a call into `internal/receipt`:

```go
func validateReceiptContent(content string) error {
	_, err := receipt.Parse([]byte(content))
	return err
}
```

Add the import `"github.com/ponchione/sodoryard/internal/receipt"` (aliased or not — either works).

The existing `receiptFrontmatter` type can stay in the tidmouth package if `splitFrontmatter` is still used locally, OR be replaced by the new `receipt.Receipt`. Prefer the latter for consistency — delete the local type and any helpers that are now in `internal/receipt`.

**Important:** the existing `cmd/tidmouth/receipt_test.go` tests must still pass. If the test cases there exercise any behavior not covered by the new `internal/receipt` tests, DO NOT delete those tests — port them to `internal/receipt` or leave them in place.

- [ ] **Step 1.7: Run all tests**

```bash
make test 2>&1 | grep -E "receipt|cmd/tidmouth|FAIL" | head -30
```

Expected: both `internal/receipt` and `cmd/tidmouth` green.

- [ ] **Step 1.8: Commit**

```bash
git add internal/receipt/ cmd/tidmouth/receipt.go cmd/tidmouth/receipt_test.go
git commit -m "feat(receipt): add shared frontmatter parser package

Phase 3 task 1 — extract receipt-frontmatter parsing out of cmd/tidmouth
and into internal/receipt so SirTopham's spawn_agent tool can consume it
without pulling in the tidmouth main package.

The package exposes Parse([]byte) (Receipt, error) with typed Verdict
values covering the spec-13 allowed set (completed, completed_no_receipt,
blocked, escalate, safety_limit). cmd/tidmouth/receipt.go's
validateReceiptContent now delegates to receipt.Parse.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Chain schema and migration helper

**Files:**

- Modify: `internal/db/schema.sql` — add three CREATE TABLE statements and their indexes
- Modify: `internal/db/init.go` — add `EnsureChainSchema` helper
- Modify: `cmd/tidmouth/runtime.go` — call `EnsureChainSchema` from `buildAppRuntime`
- Test: `internal/db/schema_integration_test.go` (existing file — add new test cases)

**Background:** Spec 15 §"Chain State Schema" defines the three tables (`chains`, `steps`, `events`) with their columns, indexes, and constraints. This task adds them to the existing `internal/db/schema.sql` file and mirrors the existing "Ensure*" migration pattern (`EnsureMessageSearchIndexesIncludeTools`, `EnsureContextReportsIncludeTokenBudget`) so upgrade-in-place works for projects whose `.yard/yard.db` already exists.

- [ ] **Step 2.1: Add the three tables to `internal/db/schema.sql`**

Append to the end of `internal/db/schema.sql`:

```sql
-- ---------------------------------------------------------------------------
-- Phase 3: SirTopham chain orchestrator state
--
-- chains: one row per chain execution
-- steps: one row per engine invocation within a chain
-- events: append-only event log for observability / dashboards
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS chains (
    id                  TEXT PRIMARY KEY,
    source_specs        TEXT,           -- JSON array of brain-relative spec paths, nullable
    source_task         TEXT,           -- free-form task string, nullable
    status              TEXT NOT NULL DEFAULT 'running',
                                        -- running, paused, completed, failed, cancelled
    summary             TEXT,
    total_steps         INTEGER NOT NULL DEFAULT 0,
    total_tokens        INTEGER NOT NULL DEFAULT 0,
    total_duration_secs INTEGER NOT NULL DEFAULT 0,
    resolver_loops      INTEGER NOT NULL DEFAULT 0,

    max_steps           INTEGER NOT NULL DEFAULT 100,
    max_resolver_loops  INTEGER NOT NULL DEFAULT 3,
    max_duration_secs   INTEGER NOT NULL DEFAULT 14400,
    token_budget        INTEGER NOT NULL DEFAULT 5000000,

    started_at          TEXT NOT NULL DEFAULT (datetime('now')),
    completed_at        TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS steps (
    id                  TEXT PRIMARY KEY,
    chain_id            TEXT NOT NULL REFERENCES chains(id),
    sequence_num        INTEGER NOT NULL,
    role                TEXT NOT NULL,
    task                TEXT NOT NULL,
    task_context        TEXT,           -- for resolver-loop tracking
    status              TEXT NOT NULL DEFAULT 'pending',
                                        -- pending, running, completed, failed
    verdict             TEXT,
    receipt_path        TEXT,
    tokens_used         INTEGER NOT NULL DEFAULT 0,
    turns_used          INTEGER NOT NULL DEFAULT 0,
    duration_secs       INTEGER NOT NULL DEFAULT 0,
    exit_code           INTEGER,
    error_message       TEXT,

    started_at          TEXT,
    completed_at        TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_steps_chain ON steps(chain_id);
CREATE INDEX IF NOT EXISTS idx_steps_status ON steps(status);

CREATE TABLE IF NOT EXISTS events (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    chain_id            TEXT NOT NULL REFERENCES chains(id),
    step_id             TEXT REFERENCES steps(id),
    event_type          TEXT NOT NULL,
    event_data          TEXT,           -- JSON blob
    created_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_events_chain ON events(chain_id);
CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at);
```

- [ ] **Step 2.2: Add `EnsureChainSchema` to `internal/db/init.go`**

Append after `EnsureContextReportsIncludeTokenBudget`:

```go
// EnsureChainSchema creates the SirTopham chain orchestrator tables if they
// are absent. The DDL is written as CREATE TABLE IF NOT EXISTS so this helper
// is safe to call on every startup, against both fresh and initialized
// databases.
func EnsureChainSchema(ctx context.Context, db *sql.DB) error {
	if ctx == nil {
		ctx = context.Background()
	}
	const ddl = `
CREATE TABLE IF NOT EXISTS chains (
    id                  TEXT PRIMARY KEY,
    source_specs        TEXT,
    source_task         TEXT,
    status              TEXT NOT NULL DEFAULT 'running',
    summary             TEXT,
    total_steps         INTEGER NOT NULL DEFAULT 0,
    total_tokens        INTEGER NOT NULL DEFAULT 0,
    total_duration_secs INTEGER NOT NULL DEFAULT 0,
    resolver_loops      INTEGER NOT NULL DEFAULT 0,
    max_steps           INTEGER NOT NULL DEFAULT 100,
    max_resolver_loops  INTEGER NOT NULL DEFAULT 3,
    max_duration_secs   INTEGER NOT NULL DEFAULT 14400,
    token_budget        INTEGER NOT NULL DEFAULT 5000000,
    started_at          TEXT NOT NULL DEFAULT (datetime('now')),
    completed_at        TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS steps (
    id                  TEXT PRIMARY KEY,
    chain_id            TEXT NOT NULL REFERENCES chains(id),
    sequence_num        INTEGER NOT NULL,
    role                TEXT NOT NULL,
    task                TEXT NOT NULL,
    task_context        TEXT,
    status              TEXT NOT NULL DEFAULT 'pending',
    verdict             TEXT,
    receipt_path        TEXT,
    tokens_used         INTEGER NOT NULL DEFAULT 0,
    turns_used          INTEGER NOT NULL DEFAULT 0,
    duration_secs       INTEGER NOT NULL DEFAULT 0,
    exit_code           INTEGER,
    error_message       TEXT,
    started_at          TEXT,
    completed_at        TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_steps_chain ON steps(chain_id);
CREATE INDEX IF NOT EXISTS idx_steps_status ON steps(status);
CREATE TABLE IF NOT EXISTS events (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    chain_id            TEXT NOT NULL REFERENCES chains(id),
    step_id             TEXT REFERENCES steps(id),
    event_type          TEXT NOT NULL,
    event_data          TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_events_chain ON events(chain_id);
CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at);
`
	if _, err := db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("ensure chain schema: %w", err)
	}
	return nil
}
```

Note: the DDL is duplicated between `schema.sql` (used by `InitIfNeeded` on a fresh db) and the Go string in `EnsureChainSchema` (used on an existing db). This duplication matches the pattern used elsewhere in `init.go` — don't try to eliminate it.

- [ ] **Step 2.3: Wire `EnsureChainSchema` into `buildAppRuntime`**

Open `cmd/tidmouth/runtime.go`. Find the schema-upgrade block (around line 79-88, after `EnsureContextReportsIncludeTokenBudget`). Add:

```go
	if err := appdb.EnsureChainSchema(ctx, database); err != nil {
		return closeOnError(fmt.Errorf("ensure chain schema: %w", err))
	}
```

Order: AFTER `InitIfNeeded` (Phase 5a fix) and AFTER the two existing Ensure* calls. SirTopham's own runtime builder in Task 7 will call this same helper.

- [ ] **Step 2.4: Add a test to `internal/db/schema_integration_test.go`**

Add a test case verifying the three tables exist after `InitIfNeeded` + `EnsureChainSchema`:

```go
func TestEnsureChainSchemaCreatesTables(t *testing.T) {
	db := openTestDB(t) // follows the existing helper pattern in this file
	ctx := context.Background()

	if _, err := InitIfNeeded(ctx, db); err != nil {
		t.Fatalf("InitIfNeeded: %v", err)
	}
	if err := EnsureChainSchema(ctx, db); err != nil {
		t.Fatalf("EnsureChainSchema: %v", err)
	}

	for _, table := range []string{"chains", "steps", "events"} {
		var count int
		err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`,
			table,
		).Scan(&count)
		if err != nil {
			t.Fatalf("query sqlite_master for %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("table %s not created (count=%d)", table, count)
		}
	}

	// Idempotency: calling again should be a no-op.
	if err := EnsureChainSchema(ctx, db); err != nil {
		t.Fatalf("EnsureChainSchema second call: %v", err)
	}
}
```

If `openTestDB` doesn't exist, look at the existing helpers in `schema_integration_test.go` and mimic them. This test file uses a real temp sqlite file, not `:memory:`.

- [ ] **Step 2.5: Run the test — confirm green**

```bash
make test 2>&1 | grep -E "internal/db|FAIL" | head -20
```

Expected: `ok github.com/ponchione/sodoryard/internal/db`.

- [ ] **Step 2.6: Commit**

```bash
git add internal/db/schema.sql internal/db/init.go internal/db/schema_integration_test.go cmd/tidmouth/runtime.go
git commit -m "feat(db): add phase 3 chain orchestrator tables

Phase 3 task 2 — add chains, steps, events tables plus their indexes
to the SirTopham schema. These are shared with Tidmouth's state in
.yard/yard.db (pinned as the single canonical database in phase 5a).

EnsureChainSchema follows the existing migration-in-place pattern and
is called from buildAppRuntime so tidmouth startup upgrades any older
database in place.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: sqlc queries and `internal/chain/` state management

**Files:**

- Create: `internal/db/query/chains.sql` — sqlc source file
- Create: `internal/db/chains.sql.go` — sqlc-generated (run `sqlc generate` at the end of this task; do not hand-edit)
- Create: `internal/chain/state.go` — Go wrappers that own the chain lifecycle
- Create: `internal/chain/limits.go` — limit check helpers
- Create: `internal/chain/events.go` — event logging helper
- Create: `internal/chain/state_test.go`
- Create: `internal/chain/limits_test.go`
- Create: `internal/chain/events_test.go`

**Background:** `internal/chain/` is the Go-side state machine for chains. It owns the lifecycle transitions (`StartChain`, `StartStep`, `CompleteStep`, `FailStep`, `CompleteChain`, `CancelChain`, `PauseChain`, `ResumeChain`), checks safety limits before each `spawn_agent` call, and appends events to the event log. It does NOT exec subprocesses — that's Task 4 (`internal/spawn/`). It does NOT know about the brain — receipts are passed in from the caller.

The package exposes a `Store` type that holds a `*sqlc.Queries` and a `clock.Clock` (for test-determinism). All methods take a `context.Context` and return explicit errors.

- [ ] **Step 3.1: Write sqlc queries**

Create `internal/db/query/chains.sql`:

```sql
-- name: CreateChain :exec
INSERT INTO chains (
    id, source_specs, source_task, status,
    max_steps, max_resolver_loops, max_duration_secs, token_budget
) VALUES (?, ?, ?, 'running', ?, ?, ?, ?);

-- name: GetChain :one
SELECT * FROM chains WHERE id = ?;

-- name: ListChains :many
SELECT * FROM chains ORDER BY created_at DESC LIMIT ?;

-- name: UpdateChainStatus :exec
UPDATE chains
SET status = ?, updated_at = datetime('now')
WHERE id = ?;

-- name: UpdateChainMetrics :exec
UPDATE chains
SET total_steps         = ?,
    total_tokens        = ?,
    total_duration_secs = ?,
    resolver_loops      = ?,
    updated_at          = datetime('now')
WHERE id = ?;

-- name: CompleteChain :exec
UPDATE chains
SET status       = ?,
    summary      = ?,
    completed_at = datetime('now'),
    updated_at   = datetime('now')
WHERE id = ?;

-- name: CreateStep :exec
INSERT INTO steps (
    id, chain_id, sequence_num, role, task, task_context, status
) VALUES (?, ?, ?, ?, ?, ?, 'pending');

-- name: StartStep :exec
UPDATE steps
SET status     = 'running',
    started_at = datetime('now')
WHERE id = ?;

-- name: CompleteStep :exec
UPDATE steps
SET status        = ?,
    verdict       = ?,
    receipt_path  = ?,
    tokens_used   = ?,
    turns_used    = ?,
    duration_secs = ?,
    exit_code     = ?,
    error_message = ?,
    completed_at  = datetime('now')
WHERE id = ?;

-- name: GetStep :one
SELECT * FROM steps WHERE id = ?;

-- name: ListStepsByChain :many
SELECT * FROM steps WHERE chain_id = ? ORDER BY sequence_num ASC;

-- name: CountResolverStepsForTaskContext :one
SELECT COUNT(*) FROM steps
WHERE chain_id = ? AND role = 'resolver' AND task_context = ?;

-- name: CreateEvent :exec
INSERT INTO events (chain_id, step_id, event_type, event_data)
VALUES (?, ?, ?, ?);

-- name: ListEventsByChain :many
SELECT * FROM events WHERE chain_id = ? ORDER BY id ASC;

-- name: ListEventsByChainSince :many
SELECT * FROM events
WHERE chain_id = ? AND id > ?
ORDER BY id ASC;
```

- [ ] **Step 3.2: Run `sqlc generate`**

```bash
sqlc generate
```

Expected output: `internal/db/chains.sql.go` is created (and possibly `internal/db/models.go` updated to include `Chain`, `Step`, `Event` struct types). Verify:

```bash
ls -la internal/db/chains.sql.go
```

If sqlc is not installed, install per the project README or use `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`. If `sqlc.yaml` needs updating to include `query/chains.sql`, it may already glob all `*.sql` files — verify by reading `sqlc.yaml` and adjusting the `queries:` value if necessary.

- [ ] **Step 3.3: Write tests for `internal/chain/state.go`**

Create `internal/chain/state_test.go` with table-driven tests exercising:

1. `StartChain` — creates row, returns the chain ID (if auto-generated)
2. `StartStep` — creates a step in `pending` state, then transitions to `running`
3. `CompleteStep` — updates verdict, metrics, status
4. `CompleteStep` with failure — sets `status='failed'` + error message
5. Concurrent chain access — two goroutines updating different chains don't conflict (acceptable if sqlite's default locking handles this, else add retries)

Use an in-memory sqlite database for the test via `appdb.OpenDB(ctx, ":memory:")` + `InitIfNeeded` + `EnsureChainSchema`.

- [ ] **Step 3.4: Implement `internal/chain/state.go`**

```go
package chain

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/id"
)

// Store wraps the sqlc queries with chain-lifecycle methods. All methods are
// context-scoped and return explicit errors.
type Store struct {
	q     *appdb.Queries
	db    *sql.DB
	clock func() time.Time
}

// NewStore returns a Store backed by the given database. The clock defaults
// to time.Now; tests can override via StoreWithClock.
func NewStore(db *sql.DB) *Store {
	return &Store{q: appdb.New(db), db: db, clock: time.Now}
}

// StoreWithClock returns a Store whose clock returns the given function's
// output. Used by tests for determinism.
func StoreWithClock(db *sql.DB, clk func() time.Time) *Store {
	s := NewStore(db)
	s.clock = clk
	return s
}

// ChainSpec is the input for starting a new chain.
type ChainSpec struct {
	ChainID          string // optional; auto-generated if empty
	SourceSpecs      []string
	SourceTask       string
	MaxSteps         int
	MaxResolverLoops int
	MaxDuration      time.Duration
	TokenBudget      int
}

// StartChain creates a new chain row in status=running and returns the
// resulting chain ID.
func (s *Store) StartChain(ctx context.Context, spec ChainSpec) (string, error) {
	chainID := spec.ChainID
	if chainID == "" {
		chainID = id.NewChainID(s.clock())
	}
	sourceSpecs := sql.NullString{}
	if len(spec.SourceSpecs) > 0 {
		sourceSpecs.String = encodeJSON(spec.SourceSpecs)
		sourceSpecs.Valid = true
	}
	sourceTask := sql.NullString{String: spec.SourceTask, Valid: spec.SourceTask != ""}

	if err := s.q.CreateChain(ctx, appdb.CreateChainParams{
		ID:               chainID,
		SourceSpecs:      sourceSpecs,
		SourceTask:       sourceTask,
		MaxSteps:         int64(spec.MaxSteps),
		MaxResolverLoops: int64(spec.MaxResolverLoops),
		MaxDurationSecs:  int64(spec.MaxDuration.Seconds()),
		TokenBudget:      int64(spec.TokenBudget),
	}); err != nil {
		return "", fmt.Errorf("create chain: %w", err)
	}
	return chainID, nil
}

// ... (StartStep, CompleteStep, RecordStepFailure, GetChain, ListSteps,
//      UpdateChainMetrics, CompleteChain, CancelChain, PauseChain,
//      ResumeChain — one method per sqlc query, thin wrappers)
```

**Note:** Filling in every method body here would double the plan's length. The pattern is: each method takes a `context.Context` and a typed parameter struct, calls the corresponding sqlc method, and wraps errors with the method name. Follow the pattern from `internal/conversation/manager.go` or similar existing wrappers in this codebase — look at one before writing.

The methods needed by the rest of the plan are:

- `StartChain(ctx, ChainSpec) (chainID string, err error)`
- `StartStep(ctx, StepSpec) (stepID string, err error)` — creates pending row, returns ID
- `StepRunning(ctx, stepID string) error`
- `CompleteStep(ctx, CompleteStepParams) error`
- `FailStep(ctx, FailStepParams) error`
- `CompleteChain(ctx, chainID, status, summary string) error`
- `UpdateChainMetrics(ctx, chainID string, m ChainMetrics) error`
- `GetChain(ctx, chainID string) (*Chain, error)`
- `ListSteps(ctx, chainID string) ([]Step, error)`
- `SetChainStatus(ctx, chainID, status string) error` — used by pause/resume/cancel
- `CountResolverStepsForContext(ctx, chainID, taskContext string) (int, error)`

Return the `Chain` and `Step` types from `internal/chain/` (re-wrap the sqlc-generated types so consumers don't need to import `internal/db`).

A `ChainID` auto-generation helper should live in `internal/id/` if not already there; add one if missing:

```go
// internal/id/chain.go — only if internal/id doesn't already have a chain helper
func NewChainID(now time.Time) string {
	return fmt.Sprintf("chain-%s-%04d", now.UTC().Format("2006-01-02"), randInt(10000))
}
```

Check `internal/id/` first before adding anything — the project already has an ID generation pattern somewhere.

- [ ] **Step 3.5: Implement `internal/chain/limits.go`**

```go
package chain

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Limit errors are returned by CheckLimits to indicate which limit was hit.
// spawn_agent uses errors.Is against these sentinels to decide how to report
// the limit violation to the orchestrator agent.
var (
	ErrMaxStepsExceeded    = errors.New("chain limit: max_steps exceeded")
	ErrTokenBudgetExceeded = errors.New("chain limit: token_budget exceeded")
	ErrMaxDurationExceeded = errors.New("chain limit: max_duration exceeded")
	ErrResolverLoopCapHit  = errors.New("chain limit: max_resolver_loops exceeded")
	ErrChainNotRunning     = errors.New("chain limit: chain is not in running state (paused or cancelled)")
)

// LimitCheckInput is the input to CheckLimits. It carries the role and
// task_context so resolver-loop tracking can count matching prior steps.
type LimitCheckInput struct {
	Role        string
	TaskContext string
}

// CheckLimits returns a non-nil sentinel error if spawning a new step would
// exceed any configured chain limit. It does not acquire a lock — the caller
// (spawn_agent) is responsible for sequencing.
func (s *Store) CheckLimits(ctx context.Context, chainID string, in LimitCheckInput) error {
	ch, err := s.GetChain(ctx, chainID)
	if err != nil {
		return fmt.Errorf("limit check: load chain: %w", err)
	}
	if ch.Status != "running" {
		return ErrChainNotRunning
	}
	if int(ch.TotalSteps) >= int(ch.MaxSteps) {
		return fmt.Errorf("%w (current=%d, max=%d)", ErrMaxStepsExceeded, ch.TotalSteps, ch.MaxSteps)
	}
	if int(ch.TotalTokens) >= int(ch.TokenBudget) {
		return fmt.Errorf("%w (current=%d, budget=%d)", ErrTokenBudgetExceeded, ch.TotalTokens, ch.TokenBudget)
	}
	if ch.StartedAt != (time.Time{}) {
		elapsed := s.clock().Sub(ch.StartedAt)
		if elapsed >= time.Duration(ch.MaxDurationSecs)*time.Second {
			return fmt.Errorf("%w (elapsed=%s, max=%ds)", ErrMaxDurationExceeded, elapsed, ch.MaxDurationSecs)
		}
	}
	if in.Role == "resolver" && in.TaskContext != "" {
		count, err := s.CountResolverStepsForContext(ctx, chainID, in.TaskContext)
		if err != nil {
			return fmt.Errorf("limit check: count resolver steps: %w", err)
		}
		if count >= int(ch.MaxResolverLoops) {
			return fmt.Errorf("%w (task_context=%q, count=%d, max=%d)",
				ErrResolverLoopCapHit, in.TaskContext, count, ch.MaxResolverLoops)
		}
	}
	return nil
}
```

Tests: one test per limit, each constructing a chain at the edge and asserting the correct sentinel. Use `StoreWithClock` to control elapsed-time checks.

- [ ] **Step 3.6: Implement `internal/chain/events.go`**

```go
package chain

import (
	"context"
	"encoding/json"
	"fmt"
)

// EventType is a typed wrapper around the event_type TEXT column to reduce
// magic strings.
type EventType string

const (
	EventChainStarted     EventType = "chain_started"
	EventStepStarted      EventType = "step_started"
	EventStepCompleted    EventType = "step_completed"
	EventStepFailed       EventType = "step_failed"
	EventReindexStarted   EventType = "reindex_started"
	EventReindexCompleted EventType = "reindex_completed"
	EventResolverLoop     EventType = "resolver_loop"
	EventSafetyLimitHit   EventType = "safety_limit_hit"
	EventChainPaused      EventType = "chain_paused"
	EventChainResumed     EventType = "chain_resumed"
	EventChainCompleted   EventType = "chain_completed"
	EventChainCancelled   EventType = "chain_cancelled"
)

// LogEvent appends an event to the chain's event log. eventData is
// JSON-encoded inside the helper so callers can pass a typed struct or a
// map.
func (s *Store) LogEvent(ctx context.Context, chainID string, stepID string, eventType EventType, eventData any) error {
	var payload string
	if eventData != nil {
		b, err := json.Marshal(eventData)
		if err != nil {
			return fmt.Errorf("marshal event data: %w", err)
		}
		payload = string(b)
	}
	// sqlc CreateEvent takes nullable step_id
	// ...
	return nil
}
```

Fill in the sqlc call per the generated signature.

Test with several event types and verify the JSON payload round-trips via `ListEventsByChain`.

- [ ] **Step 3.7: Run tests**

```bash
make test 2>&1 | grep -E "internal/chain|FAIL" | head -30
```

Expected: all `internal/chain/*_test.go` tests green.

- [ ] **Step 3.8: Commit**

```bash
git add internal/db/query/chains.sql internal/db/chains.sql.go internal/db/models.go internal/chain/ internal/id/chain.go
git commit -m "feat(chain): chain state machine and event log

Phase 3 task 3 — internal/chain/ owns the chain lifecycle state
machine: StartChain, StartStep, CompleteStep, FailStep, pause/resume/
cancel status flips, per-chain metric rollups, resolver-loop counting,
and event logging. Limits are exposed via typed sentinel errors
(ErrMaxStepsExceeded, ErrTokenBudgetExceeded, ErrMaxDurationExceeded,
ErrResolverLoopCapHit, ErrChainNotRunning) that spawn_agent (task 6)
will errors.Is against to decide how to report violations.

sqlc queries live in internal/db/query/chains.sql and are generated
into internal/db/chains.sql.go via \`sqlc generate\`.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Custom tools extension point

**Files:**

- Modify: `internal/role/builder.go` — extend `BuilderDeps` with a `CustomToolFactory` map, replace the flat rejection
- Modify: `internal/role/builder_test.go` — cover both the nil-factory and provided-factory paths
- Modify: any existing caller of `role.BuildRegistry` to pass nil for `CustomToolFactory` (tidmouth is currently the only caller via `cmd/tidmouth/run.go`)

**Background:** `internal/role/builder.go:28-30` currently rejects any role config that has a non-empty `CustomTools` list. That rejection is correct for Tidmouth (no custom tools at runtime) but wrong for SirTopham (which needs `spawn_agent` and `chain_complete`). This task makes the rejection opt-in: if the caller passes a `CustomToolFactory`, look each name up in the factory and register it; otherwise, keep the old rejection.

- [ ] **Step 4.1: Extend `BuilderDeps`**

Open `internal/role/builder.go`. Find the `BuilderDeps` struct. Add:

```go
type BuilderDeps struct {
	// ... existing fields unchanged

	// CustomToolFactory maps custom tool name → constructor function. When
	// non-nil, role configs may reference any name present in this map via
	// agent_roles.<role>.custom_tools, and BuildRegistry will register the
	// returned tool alongside the built-in groups. When nil, any role with
	// a non-empty custom_tools list is rejected as "custom_tools are not
	// implemented".
	//
	// The factory returns a fresh tool instance per call so each session
	// gets its own state. Callers that want a shared state across sessions
	// must close over a singleton inside the constructor.
	CustomToolFactory map[string]func() tool.Tool
}
```

- [ ] **Step 4.2: Replace the flat rejection with factory-aware wiring**

Find the current rejection block (around `internal/role/builder.go:28-30`):

```go
if len(roleCfg.CustomTools) > 0 {
	return nil, appconfig.BrainConfig{}, fmt.Errorf("custom_tools are not implemented by SirTopham and must be provided by the external orchestrator")
}
```

Replace with:

```go
if len(roleCfg.CustomTools) > 0 {
	if deps.CustomToolFactory == nil {
		return nil, appconfig.BrainConfig{}, fmt.Errorf("custom_tools are not implemented: caller did not provide a CustomToolFactory")
	}
	for _, name := range roleCfg.CustomTools {
		ctor, ok := deps.CustomToolFactory[name]
		if !ok {
			return nil, appconfig.BrainConfig{}, fmt.Errorf("custom tool %q not provided by factory", name)
		}
		registry.Register(ctor())
	}
}
```

Note: this assumes `registry` is already constructed at the point of the check. If the current builder constructs the registry later, move the check accordingly. Read the current function top-to-bottom before editing.

- [ ] **Step 4.3: Update `cmd/tidmouth/run.go` to pass nil explicitly**

The existing call to `role.BuildRegistry` already compiles against the new struct (since new fields default to zero values), so no code change is needed unless the compiler complains. Verify by building:

```bash
make build 2>&1 | tail -10
```

Expected: clean build. If any `role.BuilderDeps{...}` struct literal needs updating, add `CustomToolFactory: nil` for documentation value.

- [ ] **Step 4.4: Write tests covering both paths**

In `internal/role/builder_test.go`:

1. **Nil factory + custom tools** — expect the existing rejection error. Keep any existing test that covers this.
2. **Non-nil factory + custom tools, factory contains name** — expect success, registry contains the custom tool.
3. **Non-nil factory + custom tools, factory missing name** — expect error wrapping `"not provided by factory"`.
4. **Non-nil factory + no custom tools** — factory is ignored (no custom tools registered).

Use a trivial `tool.Tool` implementation for the test fake (maybe there's an existing `tool.NoopTool` or similar; check).

- [ ] **Step 4.5: Run tests**

```bash
make test 2>&1 | grep -E "internal/role|FAIL" | head -20
```

Expected: all `internal/role` tests green, including new ones.

- [ ] **Step 4.6: Commit**

```bash
git add internal/role/builder.go internal/role/builder_test.go cmd/tidmouth/run.go
git commit -m "feat(role): custom tools extension point via BuilderDeps factory

Phase 3 task 4 — role.BuildRegistry now accepts an optional
CustomToolFactory map through BuilderDeps. When non-nil, role configs
with non-empty custom_tools lists resolve each name against the
factory and register the resulting tool alongside the built-in groups.
When nil (Tidmouth's case), the existing rejection is preserved.

This is the plumbing that lets SirTopham register spawn_agent and
chain_complete for its orchestrator session, while keeping tidmouth
run strictly limited to the built-in tool groups.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: `chain_complete` tool and loop termination signal

**Files:**

- Create: `internal/tool/errors.go` — `ErrChainComplete` sentinel (new file in `internal/tool/`)
- Modify: `internal/agent/loop.go` — detect the sentinel and exit iteration
- Modify: `internal/agent/loop_test.go` — add a test case
- Create: `internal/spawn/chain_complete.go` — `ChainCompleteTool` implementation (yes, it lives in `internal/spawn/` alongside `spawn_agent` — they share helpers)
- Create: `internal/spawn/chain_complete_test.go`

**Background:** The AgentLoop today terminates only via timeout, iteration cap, or text-only LLM response (no tool use). Phase 3 needs a fourth termination path: an explicit "I'm done" signal from a tool. Spec 15 says the chain_complete tool "signal[s] the orchestrator's agent loop to stop (return a special tool result that the headless driver recognizes as a termination signal)."

The cleanest mechanism in Go: a sentinel error. The tool's `Execute` method returns `tool.ErrChainComplete` (possibly wrapped) after writing the chain receipt and updating state. The loop's tool-execution loop checks `errors.Is(err, tool.ErrChainComplete)` and, if true, returns a normal `TurnResult` marked with a termination reason. This preserves the clean-exit path and the receipt-writing side effect happens before the signal.

- [ ] **Step 5.1: Add the sentinel in `internal/tool/errors.go`**

Create the file:

```go
package tool

import "errors"

// ErrChainComplete is returned by the chain_complete custom tool to signal
// the agent loop that the chain is finished and the orchestrator session
// should exit cleanly. The loop detects this via errors.Is and terminates
// the iteration with a normal TurnResult marked as "chain complete".
//
// Tools other than chain_complete should NOT return this error.
var ErrChainComplete = errors.New("tool: chain complete signal")
```

- [ ] **Step 5.2: Detect the sentinel in `internal/agent/loop.go`**

Read the current tool execution path in `internal/agent/loop.go` (the area around where tools run inside `RunTurn`). After each tool execution, add:

```go
if errors.Is(toolErr, tool.ErrChainComplete) {
    // A chain_complete tool signalled that the orchestrator is done.
    // Exit the iteration loop with a clean TurnResult and let the driver
    // interpret the ChainComplete flag.
    result.ChainComplete = true
    // Any events/metrics the loop normally finalizes go here.
    return result, nil
}
```

Add a `ChainComplete bool` field to `TurnResult` (wherever it's defined — probably `internal/agent/types.go` or similar). Document that the driver is responsible for what to do with it.

**Important:** the sentinel must only terminate the iteration loop, not the entire `AgentLoop`. A single `RunTurn` call corresponds to one "turn" — for the orchestrator, a turn runs until `chain_complete` is called or the iteration cap is hit. The caller (SirTopham's runtime) invokes `RunTurn` once and decides what to do based on `result.ChainComplete`.

Read `loop.go` carefully before editing. The exact structure of the iteration loop (where to place the check, where to set the flag) depends on the current control flow. Spend 5 minutes reading before 30 seconds editing.

- [ ] **Step 5.3: Write the loop-termination test**

Add to `internal/agent/loop_test.go`:

```go
func TestAgentLoopExitsOnChainCompleteSentinel(t *testing.T) {
	// Arrange a loop with a single fake tool that returns tool.ErrChainComplete.
	// Run one turn. Assert the result has ChainComplete=true and no error.
	// Assert iteration stopped on the first tool call (i.e. the LLM was only
	// called once, not once-per-iteration).
}
```

Fill in the test body using whatever test fakes are already in `loop_test.go`. The assertion is: `result.ChainComplete == true`, `err == nil`, `iteration count == 1`.

- [ ] **Step 5.4: Run the loop tests — confirm pass**

```bash
make test 2>&1 | grep -E "internal/agent|FAIL" | head -20
```

Expected: all `internal/agent` tests green.

- [ ] **Step 5.5: Implement the `chain_complete` tool**

Create `internal/spawn/chain_complete.go`:

```go
package spawn

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/tool"
)

// ChainCompleteTool is the custom tool the orchestrator agent calls when it
// is done. It writes a chain-completion receipt to the brain, updates the
// chains row with the final status, and returns tool.ErrChainComplete to
// signal the agent loop to exit.
type ChainCompleteTool struct {
	Store   *chain.Store
	Backend brain.Backend
	ChainID string
}

// NewChainCompleteTool constructs the tool. Call once per chain — the tool
// is bound to a specific chain ID.
func NewChainCompleteTool(store *chain.Store, backend brain.Backend, chainID string) *ChainCompleteTool {
	return &ChainCompleteTool{Store: store, Backend: backend, ChainID: chainID}
}

func (t *ChainCompleteTool) Name() string { return "chain_complete" }
func (t *ChainCompleteTool) Description() string {
	return "Signal that the chain is complete. Provide a summary of what was accomplished."
}

func (t *ChainCompleteTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "chain_complete",
		"description": "Signal that the chain is complete. Provide a summary of what was accomplished.",
		"input_schema": {
			"type": "object",
			"properties": {
				"summary": {
					"type": "string",
					"description": "Summary of the chain execution — what was built, what passed, any remaining concerns."
				},
				"status": {
					"type": "string",
					"enum": ["success", "partial", "failed"],
					"description": "Overall chain outcome."
				}
			},
			"required": ["summary", "status"]
		}
	}`)
}

type chainCompleteInput struct {
	Summary string `json:"summary"`
	Status  string `json:"status"`
}

func (t *ChainCompleteTool) Execute(ctx context.Context, raw json.RawMessage) (tool.Result, error) {
	var in chainCompleteInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return tool.Result{}, fmt.Errorf("chain_complete: parse input: %w", err)
	}
	// Normalize status → chain status column
	var chainStatus string
	switch in.Status {
	case "success":
		chainStatus = "completed"
	case "partial":
		chainStatus = "completed" // partial still counts as completed
	case "failed":
		chainStatus = "failed"
	default:
		return tool.Result{}, fmt.Errorf("chain_complete: invalid status %q", in.Status)
	}

	// Write the orchestrator receipt to the brain.
	receiptPath := fmt.Sprintf("receipts/orchestrator/%s.md", t.ChainID)
	body := formatOrchestratorReceipt(t.ChainID, in, time.Now())
	if err := t.Backend.WriteDocument(ctx, receiptPath, body); err != nil {
		return tool.Result{}, fmt.Errorf("chain_complete: write receipt: %w", err)
	}

	// Update chain state.
	if err := t.Store.CompleteChain(ctx, t.ChainID, chainStatus, in.Summary); err != nil {
		return tool.Result{}, fmt.Errorf("chain_complete: update chain state: %w", err)
	}

	// Return the sentinel. The agent loop will see this and exit.
	return tool.Result{
		Content: fmt.Sprintf("Chain %s marked as %s. Receipt at %s.", t.ChainID, chainStatus, receiptPath),
	}, tool.ErrChainComplete
}

func formatOrchestratorReceipt(chainID string, in chainCompleteInput, now time.Time) string {
	return fmt.Sprintf(`---
agent: orchestrator
chain_id: %s
step: 1
verdict: completed
timestamp: %s
turns_used: 0
tokens_used: 0
duration_seconds: 0
---

# Chain summary

Status: %s

%s
`, chainID, now.UTC().Format(time.RFC3339), in.Status, in.Summary)
}
```

**Note:** the `tool.Result` and `tool.Tool` types must match the project's actual interface. Read `internal/tool/registry.go` and `internal/tool/types.go` (or wherever the interface lives) before writing this file — the method signatures shown here may need adjustment.

- [ ] **Step 5.6: Write the chain_complete tests**

Create `internal/spawn/chain_complete_test.go`. Cover:

1. Happy path with `status=success` — receipt is written, chain is completed, `ErrChainComplete` is returned.
2. Invalid status — no receipt, no DB update, error returned (not `ErrChainComplete`).
3. Backend write failure — receipt fails, error propagates (not `ErrChainComplete`).
4. Chain store update failure — receipt written BUT DB update fails — this is an awkward partial state. Decide: should we reverse the receipt? (No — just log and return error.) Test that this case returns an error but does not return `ErrChainComplete`.

Use an in-memory brain backend fake for the test, and an in-memory sqlite `chain.Store`.

- [ ] **Step 5.7: Run tests**

```bash
make test 2>&1 | grep -E "internal/spawn|internal/tool|internal/agent|FAIL" | head -30
```

Expected: all green.

- [ ] **Step 5.8: Commit**

```bash
git add internal/tool/errors.go internal/agent/loop.go internal/agent/loop_test.go internal/spawn/chain_complete.go internal/spawn/chain_complete_test.go
git commit -m "feat(agent): chain_complete tool and loop termination sentinel

Phase 3 task 5 — add the chain_complete custom tool alongside a new
termination mechanism for the agent loop. chain_complete writes a
receipt to receipts/orchestrator/<chain-id>.md via the brain backend,
updates the chain row to status=completed/failed, and returns the
new tool.ErrChainComplete sentinel. The agent loop (loop.go) detects
the sentinel via errors.Is and exits iteration with a TurnResult
whose new ChainComplete field is true.

Tools other than chain_complete must not return this sentinel.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: `spawn_agent` tool

**Files:**

- Create: `internal/spawn/spawn_agent.go` — `SpawnAgentTool` implementing `tool.Tool`
- Create: `internal/spawn/subprocess.go` — `RunCommand` helper with SIGTERM→SIGKILL timeout
- Create: `internal/spawn/spawn_agent_test.go`
- Create: `internal/spawn/subprocess_test.go`

**Background:** `spawn_agent` is the orchestrator's primary tool. It validates the role, checks chain limits, optionally reindexes, creates a step row, execs `tidmouth run --role <role> --task <task> --chain-id <chain-id> --quiet`, waits for exit, reads the receipt from the brain, parses it via `internal/receipt`, updates the step row, updates chain metrics, and returns the receipt content to the orchestrator agent.

- [ ] **Step 6.1: Write the subprocess helper**

`internal/spawn/subprocess.go`:

```go
package spawn

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"
)

// RunCommandInput configures a subprocess invocation.
type RunCommandInput struct {
	Name    string
	Args    []string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Env     []string
	Timeout time.Duration
	// TerminateGracePeriod is how long to wait after SIGTERM before SIGKILL.
	TerminateGracePeriod time.Duration
}

// RunResult captures the outcome of a subprocess run.
type RunResult struct {
	ExitCode int
	Err      error
}

// RunCommand execs the given command and waits for it to exit or for the
// timeout to elapse. On timeout it sends SIGTERM, waits TerminateGracePeriod,
// then sends SIGKILL if the process is still alive.
func RunCommand(ctx context.Context, in RunCommandInput) RunResult {
	if in.TerminateGracePeriod == 0 {
		in.TerminateGracePeriod = 10 * time.Second
	}
	if in.Timeout == 0 {
		in.Timeout = 30 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, in.Timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, in.Name, in.Args...)
	cmd.Stdin = in.Stdin
	cmd.Stdout = in.Stdout
	cmd.Stderr = in.Stderr
	cmd.Env = in.Env
	// Cancel function: send SIGTERM first; exec.CommandContext's default is SIGKILL.
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = in.TerminateGracePeriod // SIGKILL after this if SIGTERM didn't work

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return RunResult{ExitCode: exitErr.ExitCode(), Err: nil}
		}
		return RunResult{ExitCode: -1, Err: fmt.Errorf("run command: %w", err)}
	}
	return RunResult{ExitCode: 0, Err: nil}
}
```

Tests: trivial happy path (`/bin/true`), non-zero exit (`/bin/false`), timeout (`sleep 10` with 1s timeout), stdout/stderr capture.

- [ ] **Step 6.2: Implement `SpawnAgentTool`**

`internal/spawn/spawn_agent.go`:

```go
package spawn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/receipt"
	"github.com/ponchione/sodoryard/internal/tool"
)

// SpawnAgentTool execs `tidmouth run --role <role> --task <task> --chain-id
// <chain-id> --quiet` as a subprocess, waits for exit, reads the receipt
// from the brain, and returns the receipt content to the orchestrator.
type SpawnAgentTool struct {
	Store         *chain.Store
	Backend       brain.Backend
	Config        *appconfig.Config
	ChainID       string
	EngineBinary  string // default: "tidmouth" from PATH
	ProjectRoot   string
	SubprocessEnv []string

	// Injection points for tests
	runCommand func(ctx context.Context, in RunCommandInput) RunResult
	now        func() time.Time
}

func NewSpawnAgentTool(deps SpawnAgentDeps) *SpawnAgentTool {
	t := &SpawnAgentTool{
		Store:        deps.Store,
		Backend:      deps.Backend,
		Config:       deps.Config,
		ChainID:      deps.ChainID,
		EngineBinary: deps.EngineBinary,
		ProjectRoot:  deps.ProjectRoot,
		SubprocessEnv: deps.SubprocessEnv,
	}
	if t.EngineBinary == "" {
		t.EngineBinary = "tidmouth"
	}
	t.runCommand = RunCommand
	t.now = time.Now
	return t
}

type SpawnAgentDeps struct {
	Store         *chain.Store
	Backend       brain.Backend
	Config        *appconfig.Config
	ChainID       string
	EngineBinary  string
	ProjectRoot   string
	SubprocessEnv []string
}

func (t *SpawnAgentTool) Name() string        { return "spawn_agent" }
func (t *SpawnAgentTool) Description() string { return "Spawn a headless engine agent and block until it completes." }

func (t *SpawnAgentTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "spawn_agent",
		"description": "Spawn a headless engine agent with the given role and task. Blocks until the engine completes. Returns the engine's receipt content.",
		"input_schema": {
			"type": "object",
			"properties": {
				"role": {
					"type": "string",
					"description": "Engine role name from the config (e.g., 'thomas', 'percy', 'gordon')"
				},
				"task": {
					"type": "string",
					"description": "Task description for the engine. Be specific — include brain doc paths the engine should read."
				},
				"task_context": {
					"type": "string",
					"description": "Optional context identifier for resolver-loop tracking (e.g., 'auth/01-jwt-middleware'). Required when role is 'resolver'."
				},
				"reindex_before": {
					"type": "boolean",
					"description": "Run code/brain reindexing before starting the engine. Use after code changes.",
					"default": false
				}
			},
			"required": ["role", "task"]
		}
	}`)
}

type spawnAgentInput struct {
	Role          string `json:"role"`
	Task          string `json:"task"`
	TaskContext   string `json:"task_context,omitempty"`
	ReindexBefore bool   `json:"reindex_before,omitempty"`
}

func (t *SpawnAgentTool) Execute(ctx context.Context, raw json.RawMessage) (tool.Result, error) {
	var in spawnAgentInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return tool.Result{}, fmt.Errorf("spawn_agent: parse input: %w", err)
	}

	// 1. Validate role exists in config
	roleCfg, ok := t.Config.AgentRoles[in.Role]
	if !ok {
		return tool.Result{}, fmt.Errorf("spawn_agent: role %q not defined in config", in.Role)
	}
	_ = roleCfg // used for timeout defaults in a later step

	// 2. Check chain limits before doing anything else
	if err := t.Store.CheckLimits(ctx, t.ChainID, chain.LimitCheckInput{
		Role:        in.Role,
		TaskContext: in.TaskContext,
	}); err != nil {
		_ = t.Store.LogEvent(ctx, t.ChainID, "", chain.EventSafetyLimitHit, map[string]any{
			"role":  in.Role,
			"limit": err.Error(),
		})
		return tool.Result{}, fmt.Errorf("spawn_agent: %w", err)
	}

	// 3. Optional reindex
	if in.ReindexBefore {
		if err := t.reindex(ctx); err != nil {
			return tool.Result{}, fmt.Errorf("spawn_agent: reindex: %w", err)
		}
	}

	// 4. Create step row
	stepID, err := t.Store.StartStep(ctx, chain.StepSpec{
		ChainID:     t.ChainID,
		Role:        in.Role,
		Task:        in.Task,
		TaskContext: in.TaskContext,
	})
	if err != nil {
		return tool.Result{}, fmt.Errorf("spawn_agent: create step: %w", err)
	}
	if err := t.Store.LogEvent(ctx, t.ChainID, stepID, chain.EventStepStarted, map[string]any{
		"role": in.Role,
		"task": in.Task,
	}); err != nil {
		return tool.Result{}, fmt.Errorf("spawn_agent: log step_started: %w", err)
	}

	// 5. Exec tidmouth run
	receiptPath := fmt.Sprintf("receipts/%s/%s.md", in.Role, t.ChainID)
	args := []string{
		"run",
		"--role", in.Role,
		"--task", in.Task,
		"--chain-id", t.ChainID,
		"--quiet",
	}
	started := t.now()
	var stdout, stderr bytes.Buffer
	result := t.runCommand(ctx, RunCommandInput{
		Name:    t.EngineBinary,
		Args:    args,
		Stdout:  &stdout,
		Stderr:  &stderr,
		Env:     t.SubprocessEnv,
		Timeout: 30 * time.Minute, // TODO: derive from role limits
	})
	duration := t.now().Sub(started)

	// 6. Read receipt from brain
	fullReceiptPath := filepath.Join(t.ProjectRoot, ".brain", receiptPath)
	_ = fullReceiptPath // used if we read via filesystem; prefer backend.ReadDocument
	receiptContent, readErr := t.Backend.ReadDocument(ctx, receiptPath)
	if readErr != nil {
		// No receipt means the engine crashed or exited before writing.
		if err := t.Store.FailStep(ctx, chain.FailStepParams{
			StepID:       stepID,
			ExitCode:     result.ExitCode,
			ErrorMessage: fmt.Sprintf("no receipt at %s (read error: %v)", receiptPath, readErr),
			Duration:     duration,
		}); err != nil {
			return tool.Result{}, fmt.Errorf("spawn_agent: fail step: %w", err)
		}
		return tool.Result{}, fmt.Errorf("spawn_agent: engine exited %d without receipt", result.ExitCode)
	}

	// 7. Parse receipt
	parsed, err := receipt.Parse([]byte(receiptContent))
	if err != nil {
		if err := t.Store.FailStep(ctx, chain.FailStepParams{
			StepID:       stepID,
			ExitCode:     result.ExitCode,
			ErrorMessage: fmt.Sprintf("parse receipt: %v", err),
			Duration:     duration,
		}); err != nil {
			return tool.Result{}, fmt.Errorf("spawn_agent: fail step (parse error): %w", err)
		}
		return tool.Result{}, fmt.Errorf("spawn_agent: parse receipt: %w", err)
	}

	// 8. Update step row with receipt data
	if err := t.Store.CompleteStep(ctx, chain.CompleteStepParams{
		StepID:       stepID,
		Status:       statusFromVerdict(parsed.Verdict),
		Verdict:      string(parsed.Verdict),
		ReceiptPath:  receiptPath,
		TokensUsed:   parsed.TokensUsed,
		TurnsUsed:    parsed.TurnsUsed,
		DurationSecs: int(duration.Seconds()),
		ExitCode:     result.ExitCode,
	}); err != nil {
		return tool.Result{}, fmt.Errorf("spawn_agent: complete step: %w", err)
	}

	// 9. Update chain cumulative metrics
	// (Load chain, increment totals, save. Use UpdateChainMetrics.)
	// ...

	// 10. Log step_completed event
	_ = t.Store.LogEvent(ctx, t.ChainID, stepID, chain.EventStepCompleted, map[string]any{
		"verdict":       parsed.Verdict,
		"tokens_used":   parsed.TokensUsed,
		"turns_used":    parsed.TurnsUsed,
		"duration_secs": int(duration.Seconds()),
	})

	return tool.Result{Content: receiptContent}, nil
}

func statusFromVerdict(v receipt.Verdict) string {
	switch v {
	case receipt.VerdictCompleted, receipt.VerdictCompletedNoReceipt:
		return "completed"
	case receipt.VerdictEscalate, receipt.VerdictBlocked, receipt.VerdictSafetyLimit:
		return "failed"
	}
	return "completed"
}

func (t *SpawnAgentTool) reindex(ctx context.Context) error {
	_ = t.Store.LogEvent(ctx, t.ChainID, "", chain.EventReindexStarted, nil)
	started := t.now()
	result := t.runCommand(ctx, RunCommandInput{
		Name:    t.EngineBinary,
		Args:    []string{"index", "--quiet"},
		Env:     t.SubprocessEnv,
		Timeout: 10 * time.Minute,
	})
	if result.Err != nil || result.ExitCode != 0 {
		return fmt.Errorf("tidmouth index exited %d: %v", result.ExitCode, result.Err)
	}
	_ = t.Store.LogEvent(ctx, t.ChainID, "", chain.EventReindexCompleted, map[string]any{
		"duration_secs": int(t.now().Sub(started).Seconds()),
	})
	return nil
}
```

This is long but straightforward. The `runCommand` and `now` injection points let tests substitute fakes.

**Sub-step breakdown:** Implementing this in one sitting is risky. Break it up:

- [ ] **Step 6.2a:** Skeleton — types, `Name()`, `Description()`, `Schema()`, empty `Execute` that returns a stub. Compile-check, not functional yet.
- [ ] **Step 6.2b:** Input parsing + role validation + limit check. Unit test covering invalid input, missing role, limit error.
- [ ] **Step 6.2c:** Step row creation + subprocess exec with stub runCommand. Test that the right args are passed to runCommand.
- [ ] **Step 6.2d:** Receipt read + parse + step completion. Test with a fake backend that returns a known receipt.
- [ ] **Step 6.2e:** Chain metrics update + event logging. Test that events appear in the event log.
- [ ] **Step 6.2f:** Reindex hook. Test that `--quiet index` is invoked when `reindex_before=true`.

Each sub-step gets its own TDD cycle (test-fail-implement-test-pass) but a single commit at the end of Step 6.2.

- [ ] **Step 6.3: Run the spawn tests**

```bash
make test 2>&1 | grep -E "internal/spawn|FAIL" | head -30
```

Expected: all green.

- [ ] **Step 6.4: Commit**

```bash
git add internal/spawn/
git commit -m "feat(spawn): spawn_agent custom tool

Phase 3 task 6 — implement the orchestrator's primary custom tool.
spawn_agent validates the target role against the config, checks
chain safety limits (max_steps, token_budget, max_duration,
resolver_loops) via chain.Store, optionally runs tidmouth index,
creates a step row, execs tidmouth run as a subprocess with
SIGTERM→SIGKILL timeout handling (internal/spawn/subprocess.go),
reads and parses the resulting receipt via internal/receipt, updates
the step row and chain metrics, logs events, and returns the receipt
content to the orchestrator agent as the tool result.

On any failure path (invalid role, limit violation, missing receipt,
parse error, subprocess non-zero exit) the tool records a step
failure and returns the error to the orchestrator so it can decide
whether to retry, escalate, or call chain_complete.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: `cmd/sirtopham/` main, runtime, and `chain` subcommand

**Files:**

- Modify: `cmd/sirtopham/main.go` — REPLACE the placeholder
- Create: `cmd/sirtopham/runtime.go` — `buildOrchestratorRuntime`
- Create: `cmd/sirtopham/chain.go` — the `chain` subcommand
- Create: `cmd/sirtopham/chain_test.go` — integration test

**Background:** This is the integration point. SirTopham's `main` wires Cobra subcommands, `runtime.go` builds a narrower-than-tidmouth runtime bundle (just what the orchestrator session needs), and `chain.go` runs the startup flow from spec 15 §"Orchestrator Startup Flow". This task is also where roadmap Step 3.2 becomes concrete: the Phase 3 chain execution contract is CLI-driven, not a separate chain-definition YAML file.

- [ ] **Step 7.1: Replace the placeholder `main.go`**

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	appconfig "github.com/ponchione/sodoryard/internal/config"
)

const defaultCLIConfigPath = appconfig.ConfigFilename

var version = "dev"

func newRootCmd() *cobra.Command {
	var configPath string

	rootCmd := &cobra.Command{
		Use:          "sirtopham",
		Short:        "SirTopham chain orchestrator",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(os.Stdout, "sirtopham %s\n", version)
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVar(&configPath, "config", defaultCLIConfigPath, "Path to yard.yaml config file")

	rootCmd.AddCommand(
		newChainCmd(&configPath),
		newStatusCmd(&configPath),
		newLogsCmd(&configPath),
		newReceiptCmd(&configPath),
		newCancelCmd(&configPath),
		newPauseCmd(&configPath),
		newResumeCmd(&configPath),
	)
	return rootCmd
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		if coded, ok := err.(interface{ ExitCode() int }); ok {
			os.Exit(coded.ExitCode())
		}
		os.Exit(1)
	}
}
```

- [ ] **Step 7.2: Implement `buildOrchestratorRuntime` in `runtime.go`**

SirTopham's runtime is narrower than Tidmouth's: it doesn't need the code searcher, code intel backend, or context assembler — those are tool-side concerns for headless engines, not for the orchestrator. But it DOES need:

- `*appconfig.Config`
- `*slog.Logger`
- `*sql.DB` (shared with tidmouth, via `.yard/yard.db`)
- `brain.Backend` (to let `chain_complete` write the orchestrator receipt)
- `provider.Router` (the orchestrator is an LLM agent, after all)
- `*conversation.Manager` (for the orchestrator's session)
- A narrowed `*tool.Registry` with only brain tools + the two custom tools
- Cleanup func

Read `cmd/tidmouth/runtime.go` `buildAppRuntime` and mirror the relevant subset. Call `appdb.InitIfNeeded`, `appdb.EnsureMessageSearchIndexesIncludeTools`, `appdb.EnsureContextReportsIncludeTokenBudget`, `appdb.EnsureChainSchema` in that order.

The critical difference from tidmouth: the tool registry is built with a `CustomToolFactory` that contains `spawn_agent` and `chain_complete` constructors, bound to the chain ID of the current execution.

- [ ] **Step 7.3: Implement `chain.go`**

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/chain"
	// ... other imports
)

func newChainCmd(configPath *string) *cobra.Command {
	var (
		specs         string
		task          string
		chainID       string
		maxSteps      int
		maxDuration   time.Duration
		tokenBudget   int
		dryRun        bool
	)
	cmd := &cobra.Command{
		Use:   "chain",
		Short: "Start a new chain execution",
		RunE: func(cmd *cobra.Command, args []string) error {
			if task == "" && specs == "" {
				return fmt.Errorf("one of --task or --specs is required")
			}
			return runChain(cmd.Context(), *configPath, chainFlags{
				Specs:       specs,
				Task:        task,
				ChainID:     chainID,
				MaxSteps:    maxSteps,
				MaxDuration: maxDuration,
				TokenBudget: tokenBudget,
				DryRun:      dryRun,
			})
		},
	}
	cmd.Flags().StringVar(&specs, "specs", "", "Comma-separated brain-relative paths to spec docs")
	cmd.Flags().StringVar(&task, "task", "", "Free-form task description (alternative to --specs)")
	cmd.Flags().StringVar(&chainID, "chain-id", "", "Chain execution identifier (auto-generated if empty)")
	cmd.Flags().IntVar(&maxSteps, "max-steps", 100, "Maximum total agent invocations")
	cmd.Flags().DurationVar(&maxDuration, "max-duration", 4*time.Hour, "Wall-clock timeout for the entire chain")
	cmd.Flags().IntVar(&tokenBudget, "token-budget", 5_000_000, "Total token ceiling across all agents")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Orchestrator plans the chain but does not spawn any engines")
	return cmd
}

type chainFlags struct {
	Specs       string
	Task        string
	ChainID     string
	MaxSteps    int
	MaxDuration time.Duration
	TokenBudget int
	DryRun      bool
}

func runChain(ctx context.Context, configPath string, flags chainFlags) error {
	// 1. Load config
	// 2. Build orchestrator runtime (which creates the chain row)
	// 3. Construct the orchestrator task message
	// 4. Run the orchestrator session until chain_complete or iteration cap
	// 5. Print summary, exit with appropriate code
	// ...
	return nil
}
```

Fill in the body following spec 15 §"Orchestrator Startup Flow". The key steps:

1. Load `yard.yaml` via `appconfig.Load(configPath)`.
2. Validate the `orchestrator` role exists in the config.
3. Build the orchestrator runtime (which creates the chain row in `chain.Store` and gets a chain ID back).
4. Log `chain_started` event.
5. Construct the orchestrator task message (the initial "you are managing chain X, read specs Y, begin" prompt).
6. Invoke `AgentLoop.RunTurn` once with the orchestrator role's tool registry + task message.
7. Check `result.ChainComplete`: if true, exit 0. If max-iterations hit without chain_complete, mark the chain as `failed` and exit with a non-zero code.
8. Log `chain_completed` event.
9. Print summary to stdout.

There is intentionally no `--project` flag in this phase. The project root is resolved the same way Tidmouth already resolves it: from the current working directory and/or the supplied `--config` path. If a future phase needs explicit multi-project targeting, add it then rather than reopening Phase 3's CLI surface.

- [ ] **Step 7.4: Write the integration test**

`cmd/sirtopham/chain_test.go` — this test is expensive (boots the full runtime) but essential:

```go
func TestRunChainSpawnsEngineAndCompletes(t *testing.T) {
	// Arrange:
	//   - tempdir with .brain/ vault
	//   - yard.yaml with orchestrator + one engine role (correctness-auditor)
	//   - stub tidmouth run binary on PATH that writes a valid receipt
	//   - an orchestrator system prompt that says "call spawn_agent(correctness-auditor, 'test task'), then call chain_complete"
	//   - llm provider stubbed to return exactly the tool calls above
	//
	// Act: runChain(ctx, configPath, flags)
	//
	// Assert:
	//   - chains row exists with status=completed
	//   - steps row exists for the correctness-auditor invocation with verdict=completed
	//   - orchestrator receipt exists at receipts/orchestrator/<chain-id>.md
	//   - events table has chain_started, step_started, step_completed, chain_completed rows
}
```

The test is elaborate. Consider splitting into multiple smaller tests that each stub one layer (the provider, the subprocess, etc.) and a single "full integration" test that wires real components end-to-end but against a trivial scenario.

- [ ] **Step 7.5: Build and test**

```bash
make build 2>&1 | tail -5
make test 2>&1 | grep -E "cmd/sirtopham|FAIL" | head -20
```

Expected: both green. `bin/sirtopham` is now a real binary.

- [ ] **Step 7.6: Commit**

```bash
git add cmd/sirtopham/
git commit -m "feat(sirtopham): chain subcommand runs orchestrator session

Phase 3 task 7 — replace cmd/sirtopham/main.go's placeholder with a
real Cobra-driven CLI. The chain subcommand loads yard.yaml,
validates the orchestrator role exists, builds a narrower runtime
(brain backend + provider router + conversation manager + custom
tool registry), creates the chain row via chain.Store.StartChain,
constructs the initial orchestrator task message, invokes
AgentLoop.RunTurn, and exits on TurnResult.ChainComplete (or fails
with a non-zero code if the iteration cap was hit without
chain_complete).

Subcommand stubs for status/logs/receipt/cancel/pause/resume are
wired but their bodies land in tasks 8 and 9.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: `sirtopham` status / logs / receipt subcommands

**Files:**

- Create: `cmd/sirtopham/status.go`
- Create: `cmd/sirtopham/logs.go`
- Create: `cmd/sirtopham/receipt.go`
- Create: `cmd/sirtopham/*_test.go` for each

**Background:** These are read-only observation commands. They load config, open the shared DB, and query `chain.Store` / `brain.Backend`.

- [ ] **Step 8.1: Implement `status`**

`sirtopham status <chain-id>`: print chain row + all steps in order. Format as a human-readable table:

```
chain-smoke-test-p5a  status=running  started=2026-04-11T13:36:10Z  steps=2/100  tokens=8826/5000000

  #1  correctness-auditor  completed  2.0s  8826 tokens   receipts/correctness-auditor/smoke-test-p5a.md
  #2  ...
```

Use `chain.Store.GetChain` + `chain.Store.ListSteps`.

If no chain-id is given, print the 10 most recent chains in a one-line-per-chain format.

- [ ] **Step 8.2: Implement `logs`**

`sirtopham logs <chain-id>`: print all events for the chain in chronological order. One line per event:

```
[2026-04-11T13:36:10Z] chain_started
[2026-04-11T13:36:11Z] step_started     role=correctness-auditor
[2026-04-11T13:36:14Z] step_completed   verdict=completed tokens=8826 duration=4s
[2026-04-11T13:36:14Z] chain_completed  status=completed
```

- [ ] **Step 8.3: Implement `receipt`**

`sirtopham receipt <chain-id> <step-num>`: read the receipt at `receipts/<role>/<chain-id>.md` and print it to stdout. If the step number is omitted, print the orchestrator receipt.

Use `chain.Store.ListSteps` to find the step's receipt path, then `brain.Backend.ReadDocument`.

- [ ] **Step 8.4: Tests**

One test per subcommand. Use a fixture chain created inline (not from a real chain run — mock up the DB state directly).

- [ ] **Step 8.5: Build and test**

```bash
make test 2>&1 | grep -E "cmd/sirtopham|FAIL" | head -20
```

- [ ] **Step 8.6: Commit**

```bash
git add cmd/sirtopham/status.go cmd/sirtopham/logs.go cmd/sirtopham/receipt.go cmd/sirtopham/*_test.go
git commit -m "feat(sirtopham): status, logs, receipt read-only subcommands

Phase 3 task 8 — observability commands for chain state. Each reads
from the shared .yard/yard.db and/or brain vault and prints a
human-readable summary to stdout.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: `sirtopham` cancel / pause / resume subcommands

**Files:**

- Create: `cmd/sirtopham/cancel.go`
- Create: `cmd/sirtopham/pause_resume.go`
- Tests alongside

**Background:** MVP scope — cancel flips status to `cancelled`, pause to `paused`, resume back to `running`. `spawn_agent` checks chain status as part of `CheckLimits` (via `ErrChainNotRunning`) and refuses to spawn. Full auto-session-restart on resume is explicitly deferred.

- [ ] **Step 9.1: Implement `cancel`**

`sirtopham cancel <chain-id>`:

1. Load the chain via `chain.Store.GetChain`.
2. If status is already terminal (`completed`, `failed`, `cancelled`), error out.
3. Call `chain.Store.SetChainStatus(ctx, chainID, "cancelled")`.
4. Log `chain_cancelled` event.
5. Print confirmation to stdout.

The actual subprocess termination (if a chain is running in another terminal) is out of scope — the user Ctrl-C's the `sirtopham chain` process themselves.

- [ ] **Step 9.2: Implement `pause`**

Same shape as `cancel` but sets `status='paused'` and logs `chain_paused`.

- [ ] **Step 9.3: Implement `resume`**

```go
func runResume(ctx context.Context, configPath, chainID string) error {
	// Load chain; assert status=paused
	// Flip to running
	// Log chain_resumed event
	// Print:
	//   "Chain <chain-id> resumed. The orchestrator session was not
	//    auto-restarted. Run:
	//      sirtopham chain --chain-id <chain-id>
	//    to start a fresh orchestrator session that picks up where
	//    the previous one left off (it reads receipts from the brain
	//    and reconstructs state)."
}
```

The session resumption is a documented TODO.

- [ ] **Step 9.4: Tests**

One test per subcommand. Verify:

- Status transitions correctly
- Terminal states reject the operation
- Events are logged

- [ ] **Step 9.5: Build and test**

- [ ] **Step 9.6: Commit**

```bash
git add cmd/sirtopham/cancel.go cmd/sirtopham/pause_resume.go cmd/sirtopham/*_test.go
git commit -m "feat(sirtopham): cancel, pause, resume subcommands

Phase 3 task 9 — MVP control commands. Each flips the chain's status
column and logs an event. resume explicitly does NOT auto-restart
the orchestrator session; it tells the user to run
\`sirtopham chain --chain-id <id>\` manually to start a fresh
session that reconstructs state from receipts in the brain.

Auto-session-restart is deferred to a follow-up cleanup phase.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: Refine `agents/orchestrator.md` system prompt

**Files:**

- Modify: `agents/orchestrator.md`

**Background:** The current `agents/orchestrator.md` is a short 25-line stub. Phase 3's smoke test needs enough prompt to drive one real chain end-to-end, even if imperfectly. Production-grade prompt engineering is Phase 4.

This task expands the stub to cover:

1. **Role identity.** "You are SirTopham, the chain orchestrator. You are the only engine that never writes code or reads source files — your only tools are brain access plus `spawn_agent` and `chain_complete`."
2. **Brain directory structure.** Point at `specs/`, `epics/`, `tasks/`, `plans/`, `receipts/`, `logs/`, `conventions/`, `architecture/`. Describe who writes to what.
3. **Engine roster.** List each engine role from `yard.yaml`: coder, planner, correctness-auditor, quality-auditor, performance-auditor, security-auditor, integration-auditor, test-writer, resolver, arbiter, epic-decomposer, task-decomposer, docs-arbiter. One line per role explaining what it does.
4. **Chain flow.** Describe the standard flow: spec → epic-decomposer → task-decomposer → planner → coder → auditors (correctness + quality + applicable others) → resolver (if any auditor failed) → test-writer → arbiter. Call this a default, not a rigid sequence.
5. **Decision criteria.**
   - When to spawn a resolver (an auditor returned non-completed verdict)
   - When to skip an optional auditor (e.g., skip security-auditor for docs-only changes)
   - When to escalate (resolver loop cap hit, or two auditors disagree without resolution)
   - When to call `chain_complete(status=success|partial|failed)`
6. **Receipt protocol.** Every `spawn_agent` call returns a receipt string. Parse the frontmatter: `verdict` is the key signal (`completed`, `blocked`, `escalate`, `safety_limit`). Read the body for detail when needed.
7. **Boundaries.** Never attempt work yourself. Never try to read code files directly (you can't; you have no file tools). Never bypass `spawn_agent` to "just do it quickly."

Target length: 150-250 lines. This is operator-facing guidance, not a literal script. The next session (Phase 4) will iterate on it based on real chain runs.

- [ ] **Step 10.1: Read the current stub**

```bash
cat agents/orchestrator.md
```

- [ ] **Step 10.2: Rewrite `agents/orchestrator.md`**

Expand per the outline above. Use plain prose, not JSON or YAML. Prefer short paragraphs over bullet walls. Include one concrete example:

> **Example chain decision:** the correctness-auditor returned `verdict: escalate` with a body saying "the coder's JWT middleware test file is missing edge-case coverage for expired tokens." Your next action is `spawn_agent(role="resolver", task="address the correctness-auditor's escalation in receipts/correctness-auditor/{chain-id}.md: add edge-case coverage for expired tokens to the JWT middleware test file", task_context="auth/01-jwt-middleware")`. Watch the resolver-loop counter — if it hits `max_resolver_loops`, the `spawn_agent` call will fail and you must call `chain_complete(status="partial", summary="resolver loop exceeded on auth/01-jwt-middleware")` instead of retrying.

- [ ] **Step 10.3: Commit**

```bash
git add agents/orchestrator.md
git commit -m "docs(agents): expand orchestrator system prompt for phase 3 smoke test

Phase 3 task 10 — expand the stub orchestrator prompt to cover role
identity, brain directory structure, engine roster, standard chain
flow, decision criteria for branching and escalation, receipt
parsing protocol, and hard boundaries. This is NOT the production
prompt (that's phase 4) — it's the minimum viable content that lets
the phase 3 regression smoke test drive one real chain end-to-end.

Phase 4 will iterate on this based on real chain execution data.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 11: Reindex hooks via `reindex_before`

Roadmap Step 3.7 mentioned before/after hooks. Phase 3 MVP intentionally lands only `reindex_before`. Task 6 Step 6.2f wires the `reindex_before` flag on `spawn_agent` to call `t.reindex(ctx)` before `tidmouth run`. If Task 6 deferred this sub-step, come back and finish it here.

- [ ] **Step 11.1: Verify the reindex hook is wired**

```bash
grep -n reindex internal/spawn/spawn_agent.go
```

Expected: the input schema has `reindex_before`, `Execute` calls `t.reindex(ctx)` when true, and there's a test for the hook.

- [ ] **Step 11.2: Add an integration test if missing**

The test stubs `runCommand` to capture args and asserts that a `tidmouth index --quiet` call happens BEFORE the `tidmouth run --role ...` call when `reindex_before=true`.

- [ ] **Step 11.3: Commit if any changes were needed**

Otherwise mark Task 11 as folded into Task 6's commit and move on.

---

## Task 12: End-to-end verification and tag `v0.4-orchestrator`

**Files:** none modified — this is operator verification.

**Background:** This is the gate for Phase 3. We run a real chain against `my-website` and verify the orchestrator drives it to completion. A trivial chain is fine — the goal is to prove the plumbing works, not that the prompts are production-quality.

- [ ] **Step 12.1: Pre-flight checks**

```bash
curl -s --max-time 3 http://localhost:12434/v1/models | head -c 120
curl -s --max-time 3 http://localhost:12435/v1/models | head -c 120
./bin/tidmouth auth status | head -5
./bin/sirtopham --help
```

All four should succeed. If Codex auth is expired (check `expires_at`), re-auth before proceeding.

- [ ] **Step 12.2: Write a minimal chain-smoke config**

Create `/tmp/my-website-chain-smoke.yaml` based on `/tmp/my-website-smoke.yaml` (from Phase 5a Task 7), adding an `orchestrator` role with `custom_tools: [spawn_agent, chain_complete]` and one engine role (e.g. `correctness-auditor`). Use minimal limits (`max_turns: 10`, `max_tokens: 100000` on each role).

- [ ] **Step 12.3: Run a trivial chain**

```bash
./bin/sirtopham chain \
  --config /tmp/my-website-chain-smoke.yaml \
  --task "The chain's only job: spawn the correctness-auditor once with task 'list files in the brain vault and write a receipt', then call chain_complete(status=success, summary='trivial smoke test')" \
  --chain-id phase3-smoke-1 \
  --max-steps 3 \
  --max-duration 5m \
  --token-budget 500000
```

- [ ] **Step 12.4: Verify pass criteria**

- Exit code: `0`
- `sirtopham status phase3-smoke-1` shows status `completed` with `total_steps=1`
- `sirtopham logs phase3-smoke-1` shows the event sequence `chain_started → step_started → step_completed → chain_completed`
- Receipt exists at `/home/gernsback/source/my-website/.brain/receipts/orchestrator/phase3-smoke-1.md` with valid frontmatter
- Receipt exists at `/home/gernsback/source/my-website/.brain/receipts/correctness-auditor/phase3-smoke-1.md`
- `ls /home/gernsback/source/my-website/.yard/yard.db` — the shared DB has the chain data

- [ ] **Step 12.5: Run a less trivial chain if time permits**

Optional but valuable: a two-step chain where the orchestrator spawns two different engine roles and observes the receipts in between. Use the same my-website project.

- [ ] **Step 12.6: Tag and push-back**

```bash
git tag v0.4-orchestrator
git tag | tail -5
```

Do NOT push. The user pushes manually.

- [ ] **Step 12.7: Update `NEXT_SESSION_HANDOFF.md`**

Add a "Phase 3 complete" section to the Milestones block. Move the "Next task" section to point at Phase 4 (production prompt engineering) and Phase 5b (`yard init` CLI) — see the roadmap for which makes more sense to run first.

- [ ] **Step 12.8: Commit the handoff update**

```bash
git add NEXT_SESSION_HANDOFF.md
git commit -m "docs: mark phase 3 complete and point handoff at phase 4/5b"
```

---

## Task 13: Docs follow-ups

**Files:**

- Modify: `docs/specs/15-chain-orchestrator.md`

**Background:** Spec 15 was written before Phase 5a locked `.yard/yard.db` as the shared DB, and before Phase 3 chose `spawn_agent` as the canonical tool name (matching `yard.yaml`). Align the spec with the code.

- [ ] **Step 13.1: Rename `spawn_engine` → `spawn_agent` in the spec**

Global find/replace in `docs/specs/15-chain-orchestrator.md`. Double-check the `custom_tools` example YAML block uses `spawn_agent`.

- [ ] **Step 13.2: Pin the database path**

Find every mention of `.yard/sirtopham.db` in the spec and replace with `.yard/yard.db`. Add a short paragraph at the top of the "Chain State Schema" section:

> **Database:** Chain state shares the canonical `.yard/yard.db` database with Tidmouth's conversation and message state. The three tables (`chains`, `steps`, `events`) are added by the `EnsureChainSchema` helper alongside Tidmouth's existing schema migrations. A future cleanup phase may split SirTopham into its own database, but for now the single shared DB simplifies tooling and dashboards.

- [ ] **Step 13.3: Note the MVP pause/resume scope**

Add a paragraph under "Chain Pause / Resume / Cancel":

> **Phase 3 MVP scope:** The initial Phase 3 landing implements cancel, pause, and resume as simple `chains.status` column flips. `spawn_agent` checks the status column via `chain.Store.CheckLimits` and returns `ErrChainNotRunning` if the chain is paused or cancelled. `sirtopham resume` does NOT auto-restart the orchestrator session — it flips the flag and instructs the user to re-run `sirtopham chain --chain-id <id>` to start a fresh session that reconstructs state from brain receipts. Full auto-session-restart is deferred to a post-Phase-3 cleanup.

- [ ] **Step 13.4: Commit**

```bash
git add docs/specs/15-chain-orchestrator.md
git commit -m "docs(spec15): align with phase 3 implementation choices

Post-phase-3 follow-ups:
- Rename spawn_engine → spawn_agent throughout (matches yard.yaml
  and the shipped internal/spawn/spawn_agent.go)
- Pin the database path to .yard/yard.db (shared with Tidmouth,
  locked by the phase 5a brainstorming)
- Document the phase 3 MVP pause/resume scope — the user must
  manually re-run sirtopham chain to restart a paused session

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Post-plan: deferred work

These are out of scope for this plan but should be tracked somewhere (TECH-DEBT.md or the roadmap):

1. **Auto-session-restart on `sirtopham resume`** — requires a runloop that can be awakened by a sentinel file or a small RPC, OR a "watch" mode where `sirtopham chain` polls for its chain being resumed.
2. **Parallel engine execution** — spawn multiple engines concurrently for parallel auditors. Needs concurrent brain-write safety.
3. **Standalone chain-definition/config file format** — roadmap Step 3.2's broader "chain config format" idea is deferred; Phase 3 uses CLI inputs + `yard.yaml` role config instead.
4. **Chain templates** — predefined chain flows that skip the orchestrator agent for common patterns (saves tokens).
5. **Chain forking** — split a chain into sub-chains.
6. **`request_human_input` checkpoint tool** — pause the chain and wait for Knapford-mediated human input.
7. **Production orchestrator system prompt** — iterative refinement with real chain data (Phase 4).
8. **`sirtopham.db` as a separate file** — if concurrent-writer contention becomes a problem in practice.
9. **`reindex_after` hook** — the roadmap mentioned before/after hooks; only `reindex_before` lands in Phase 3.
10. **Removing the `DefaultConfigFilename` unused parameter** — Phase 5a deferred item.
11. **`SIRTOPHAM_LOG_LEVEL` env var rename to `YARD_LOG_LEVEL`** — Phase 5a deferred item.

---

## Execution guidance

Implementation rules for a handoff agent:

1. Keep edits narrow; do not refactor unrelated Tidmouth or web code while landing Phase 3.
2. Prefer `make test` and `make build`; they carry the required native/CGO settings.
3. Treat each task's test/build step as mandatory; do not mark a task done on code inspection alone.
4. When a task says to update docs/specs after code lands, do the docs update in the same checkpoint before handing off.
5. If a command/test fails for environment reasons, record the exact failure and continue with the highest-confidence non-live tasks before stopping.
6. Do not "improve" the MVP by adding parallelism, chain templates, a second config surface, or Knapford integration.

This plan is 13 tasks, which is more than the Phase 5a plan (7 tasks) and will not fit in one session. Suggested splits:

- **Session 1** — Tasks 1-3 (receipt parser, schema, chain state). Foundation. Target: tag a checkpoint if tasks 1-3 are all green.
- **Session 2** — Tasks 4-6 (custom tools extension, chain_complete + sentinel, spawn_agent). The risky middle section where most bugs will surface.
- **Session 3** — Tasks 7-9 (cmd/sirtopham/ CLI surface). Integration of everything.
- **Session 4** — Tasks 10-13 (prompt, reindex verification, smoke test, spec alignment). Polish and verify.

Between sessions, update `NEXT_SESSION_HANDOFF.md` with the commit stack, any bugs uncovered, and what's next. Do NOT batch all of Phase 3 into one mega-session — the review and debugging overhead compounds.

**Subagent-driven execution recommended.** Tasks 1, 2, 4, 5, 10, 13 are mostly mechanical and suit a fast model. Tasks 3, 6, 7, 8, 9 have meaningful judgment calls and should use a standard model. Task 12 is live-fire verification and should be run by the operator (not a subagent) so environment issues are caught immediately.
