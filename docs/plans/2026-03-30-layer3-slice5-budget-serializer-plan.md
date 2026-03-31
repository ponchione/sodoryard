# Layer 3 Slice 5: Budget Manager and Serializer Plan

> For Hermes: implement only this slice, commit locally, then stop.

Goal: add deterministic budget fitting and markdown serialization in `internal/context` so retrieval results can be trimmed to fit the model window and rendered into the stable cache-block-2 format.

Architecture:
- Keep everything in `internal/context`.
- Reuse existing `BudgetResult`, `RetrievalResults`, and `SeenFileLookup` contracts.
- Keep this slice pure computation only: no DB writes, no network calls, no file I/O.
- Use rough token estimation (`(len(text)+3)/4`) and stable deterministic ordering.
- If Slice 5 exposes unrelated follow-up issues, record them in `TECH-DEBT.md` instead of widening this commit.

Tech stack: Go, `internal/context`, `internal/config`, stdlib `strings`, `sort`, `path/filepath`

---

## Scope of this single problem

Deliver only these artifacts:
- `internal/context/budget.go`
- `internal/context/serializer.go`
- `internal/context/budget_test.go`
- `internal/context/serializer_test.go`

Do not implement compression itself, assembler wiring, or report persistence in this slice.

---

## Task 1: Write failing budget tests

Objective: lock down token accounting, priority filling, and compression signalling before implementation.

Add tests for:
- token accounting with a 200k model / 50k history case
- explicit files filling before RAG and structural content
- budget exhaustion causing lower-priority exclusions with `budget_exceeded`
- below-threshold RAG exclusion with `below_threshold`
- budget breakdown accounting
- compression trigger when history exceeds the configured threshold

Verification:
- `go test ./internal/context/... -run 'TestPriorityBudgetManager|TestBudget'`
- Expected RED failure because budget implementation does not exist yet

---

## Task 2: Write failing serializer tests

Objective: lock down the markdown shape before implementation.

Add tests for:
- two RAG chunks from the same file grouped under one file header
- description before code fence
- language-tagged fences
- previously-viewed annotation from `SeenFileLookup`
- conventions and git sections appearing only when populated
- empty budget result producing empty or minimal output without crashing
- balanced code fences and stable deterministic ordering

Verification:
- `go test ./internal/context/... -run 'TestMarkdownSerializer|TestSerialize'`
- Expected RED failure because serializer implementation does not exist yet

---

## Task 3: Implement the budget manager

Objective: satisfy the budget tests with the smallest clean implementation.

Create `internal/context/budget.go` with:
- `PriorityBudgetManager` implementing `BudgetManager`
- token accounting constants for:
  - base system prompt reserve
  - tool schema reserve
  - response headroom reserve
- assembled budget calculation:
  - `available = model limit - reserves - history`
  - `budget total = min(available, cfg.MaxAssembledTokens default 30000)`
- priority filling order:
  1. explicit files
  2. top-ranked RAG hits
  3. structural hits
  4. conventions
  5. git
  6. lower-ranked RAG hits
- soft caps for conventions and git
- inclusion/exclusion tracking and budget breakdown
- `CompressionNeeded` signalling using `cfg.CompressionThreshold` default 0.5

Notes:
- Relevance threshold should still be enforced here so direct unit tests on `Fit` can record `below_threshold` exclusions even if retrieval already filtered most live inputs.
- Keep the top-vs-lower RAG split simple and explicit in code.

Verification:
- `go test ./internal/context/... -run 'TestPriorityBudgetManager|TestBudget'`

---

## Task 4: Implement the markdown serializer

Objective: satisfy the serializer tests with deterministic output.

Create `internal/context/serializer.go` with:
- `MarkdownSerializer` implementing `Serializer`
- `## Relevant Code` section
- grouping by file path
- per-chunk description before code fence
- `[previously viewed in turn N]` annotation when `seenFiles.Contains(path)` is true
- `## Project Conventions` section when conventions exist
- `## Recent Changes (last N commits)` section when git context exists
- language tag selection from chunk language or file extension

Notes:
- Omit empty sections entirely.
- Keep ordering stable: explicit files, then grouped RAG chunks, then structural summary, conventions, git.

Verification:
- `go test ./internal/context/... -run 'TestMarkdownSerializer|TestSerialize'`

---

## Task 5: Final verification and commit

Objective: prove Slice 5 is complete and isolated.

Run:
- `go test ./internal/context/... -run 'TestPriorityBudgetManager|TestMarkdownSerializer|TestSerialize|TestBudget'`
- `go test ./internal/context/...`

Commit only Slice 5 code:
- `git add internal/context/budget.go internal/context/serializer.go internal/context/budget_test.go internal/context/serializer_test.go`
- `git commit -m "feat(context): add budget manager and serializer"`

Stop after commit.

---

## Success criteria

This slice is done when:
- `BudgetManager` has a concrete implementation that computes budget totals, selected content, exclusions, and compression signalling
- `Serializer` has a concrete deterministic markdown implementation
- grouped code output, conventions, git context, and seen-file annotations all render correctly
- `go test ./internal/context/...` passes
- unrelated follow-up issues are recorded in `TECH-DEBT.md` instead of expanded into this slice
- only Slice 5 code is included in the local commit
