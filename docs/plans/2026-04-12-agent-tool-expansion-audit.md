# Agent Tool Expansion Audit

Date: 2026-04-12
Audit scope: commits `cbed25f..HEAD` on `main`
Related plan: `docs/plans/2026-04-12-agent-tool-expansion-implementation-plan.md`

## Goal

Audit the planned agent tool expansion work in `internal/tool/`:

1. `list_directory` — tree-style directory listing
2. `find_files` — glob-based file discovery with `**` support
3. `test_run` — structured test runner for Go/Python/TypeScript
4. `db_sqlc` — `sqlc generate/vet/diff` wrapper
5. RTK auto-prefix in `shell`

Plus registration wiring in:
- `internal/tool/register.go`
- `internal/role/builder.go`
- `cmd/tidmouth/serve.go`

## Files audited

New/modified files in the diff range:
- `cmd/tidmouth/serve.go`
- `internal/role/builder.go`
- `internal/tool/db_sqlc.go`
- `internal/tool/db_sqlc_test.go`
- `internal/tool/fileutil.go`
- `internal/tool/find_files.go`
- `internal/tool/find_files_test.go`
- `internal/tool/list_directory.go`
- `internal/tool/list_directory_test.go`
- `internal/tool/register.go`
- `internal/tool/registry_test.go`
- `internal/tool/shell.go`
- `internal/tool/shell_test.go`
- `internal/tool/test_run.go`
- `internal/tool/test_run_parse.go`
- `internal/tool/test_run_parse_test.go`
- `internal/tool/test_run_test.go`
- `internal/tool/truncate.go`

Reference files compared against existing patterns:
- `internal/tool/file_read.go`
- `internal/tool/git_status.go`
- `internal/tool/search_text.go`
- `internal/tool/command_path.go`
- `internal/tool/registry.go`
- `internal/tool/executor.go`
- `internal/tool/types.go`

## Verification performed

Targeted verification run locally:
- `go test ./internal/tool -run 'Test(RegisterDirectoryTools|AllNewToolsRegistered|ListDirectory|FindFiles|Shell|TestRun|DbSqlc|RegistryToolDefinitions)'` → pass
- `go test ./internal/role` → pass

Notes:
- Full-repo or wider-package verification was not required for this audit doc.
- This document is intended as a resolver handoff for focused fixes.

---

## CRITICAL — must fix before merge

### 1. Symlink escape breaks project-root confinement
- Files:
  - `internal/tool/fileutil.go:17-37`
  - callers in this slice:
    - `internal/tool/list_directory.go:85`
    - `internal/tool/find_files.go:78`
    - `internal/tool/test_run.go:84`
    - `internal/tool/db_sqlc.go:92`
    - `internal/tool/shell.go:126`
- Problem:
  - `resolvePath()` only validates the textual relative path using `filepath.Clean`, `filepath.Join`, and `filepath.Rel`.
  - It does not resolve symlinks before checking containment.
  - A path like `linked-outside/file` can still pass validation if `linked-outside` is a symlink inside the repo pointing outside the repo.
- Impact:
  - All new path-accepting tools in this slice can escape the intended project-root sandbox through in-repo symlinks.
- Suggested fix:
  - Resolve `projectRoot` and the candidate path via `filepath.EvalSymlinks` where possible, or walk components with `Lstat` and reject symlink traversal entirely.
  - Re-check ancestry after symlink resolution.
  - For not-yet-existing targets, validate the nearest existing parent.
- Suggested tests:
  - Add a symlink-escape regression test at the `resolvePath` level.
  - Add at least one tool-level regression test for a path-accepting tool.

### 2. `test_run` can incorrectly report success on infrastructure failure
- File: `internal/tool/test_run.go:81-93, 182-206, 243-262, 307-332`
- Problem:
  - `params.Path` is resolved but not validated with `os.Stat` before use.
  - All three runners ignore `cmd.Run()` errors.
  - This means nonexistent paths, timeouts, cancellations, or runner invocation failures can be reformatted into a misleading success summary instead of a tool failure.
- Impact:
  - The tool can lie about test state, especially in timeout/cancel/path-error cases.
- Suggested fix:
  - Validate the resolved path before ecosystem detection/execution.
  - Capture and branch on `runErr` for all ecosystems.
  - Return `Success=false` for infrastructure/runtime failures.
  - Only return `Success=true` for actual executed test runs, including legitimate test failures.
- Suggested tests:
  - Nonexistent path
  - context cancellation
  - timeout
  - broken runner / startup failure
  - stderr-only failure with empty stdout

---

## IMPORTANT — should fix

### 3. `db_sqlc diff` masks real command failures as normal “out of sync” output
- File: `internal/tool/db_sqlc.go:155-168`
- Problem:
  - Any non-nil `runErr` for `diff` is treated as a successful diff mismatch.
  - Real command failures are therefore indistinguishable from expected “differences found” cases.
- Suggested fix:
  - Distinguish expected diff-result exit behavior from actual execution failure.
  - If `sqlc diff` has documented exit-code semantics, branch on exit code; otherwise fail closed on ambiguous errors.
- Missing coverage:
  - No test currently exercises diff failure-vs-out-of-sync distinction.

### 4. `pytest-json-report` capability probe can hang before timeout logic applies
- File: `internal/tool/test_run.go:167, 209-215`
- Problem:
  - `pytestJSONReportAvailable()` runs under the caller context with no short probe timeout.
  - If pytest startup or plugin loading hangs, the tool can block before the real timed test command even begins.
- Suggested fix:
  - Put the probe under a small dedicated timeout.
  - Fall back to short-output mode on probe timeout/error.
- Missing coverage:
  - No test for a hanging/slow probe or fallback behavior.

### 5. Go subdirectory handling in `test_run` is inconsistent with the schema
- File: `internal/tool/test_run.go:43-46, 293-302`
- Problem:
  - The schema says `path` is a “Subdirectory or package path”.
  - Implementation treats any slashless value as a package path, so `"api"` becomes `go test api` instead of testing `./api` from the repo.
- Design concern:
  - In normal repos and monorepos, top-level single-segment subdirectories are common.
- Suggested fix:
  - Prefer resolved filesystem directories over string heuristics.
  - Only treat the value as a package path when explicitly requested or when it cannot resolve as an in-repo directory.
- Missing coverage:
  - No integration test for single-segment subdirectory input.

### 6. TypeScript runner detection is rooted at `projectRoot`, not the detected package boundary
- File: `internal/tool/test_run.go:219, 265-274`
- Problem:
  - Ecosystem detection walks upward from the target directory.
  - TS runner detection does not: it only inspects `projectRoot` for vitest config.
- Impact:
  - In monorepos, a nested package can be correctly identified as TypeScript but still choose the wrong runner.
- Suggested fix:
  - Detect the runner from the same directory/package boundary used for ecosystem detection, or walk upward from the target dir.
- Missing coverage:
  - No monorepo/nested-package runner detection test.

### 7. `list_directory` and `find_files` ignore context cancellation
- Files:
  - `internal/tool/list_directory.go:67, 146-198`
  - `internal/tool/find_files.go:51, 95-143`
- Problem:
  - Both tools accept `ctx` but never check it during traversal.
- Impact:
  - On large trees, they can continue expensive work after cancellation.
- Suggested fix:
  - Check `ctx.Err()` inside traversal and stop early.
  - Return a controlled cancelled/failure result consistent with package conventions.
- Missing coverage:
  - No cancellation or large-tree stress tests.

### 8. Role-based registration is implemented but not explicitly tested for new groups
- Files:
  - `internal/role/builder.go:52-57`
  - `internal/role/builder_test.go:31-206`
- Problem:
  - `directory`, `test`, and `sqlc` groups are wired in `BuildRegistry()`.
  - Existing builder tests do not assert those groups specifically.
- Suggested fix:
  - Add explicit builder tests for the new role groups.

### 9. New truncation notices exist but are only partially tested
- Files:
  - `internal/tool/truncate.go:47-56`
  - `internal/tool/truncate_test.go:55-65`
- Problem:
  - Notices exist for `list_directory`, `find_files`, `test_run`, and `db_sqlc`, but current tests only cover `shell` among the new tools.
- Suggested fix:
  - Extend truncation tests to cover each new tool name.

---

## MINOR — nice to have

### 10. `shell` uses raw `exec.LookPath` for RTK detection instead of `lookupCommandPath`
- File: `internal/tool/shell.go:34-39`
- Problem:
  - This deviates from the shared command-lookup helper used by the other external-binary tools.
- Assessment:
  - Likely intentional because RTK is optional, not required.
- Suggested fix:
  - Either switch to `lookupCommandPath()` for consistency, or document the intentional exception.

### 11. Path joins use string concatenation instead of `filepath.Join`
- Files:
  - `internal/tool/list_directory.go:189`
  - `internal/tool/db_sqlc.go:104-106`
- Problem:
  - This is less portable and inconsistent with surrounding code.
- Suggested fix:
  - Replace with `filepath.Join()`.

### 12. Error-message wording is slightly inconsistent with existing tool style
- Files:
  - `internal/tool/test_run.go:278-285`
  - compare with `internal/tool/git_status.go:58-64`, `internal/tool/search_text.go:100-106`, `internal/tool/db_sqlc.go:79-87`
- Problem:
  - Most tools use “X is required but not found in PATH”.
  - Go test runner currently says “go not found in PATH”.
- Suggested fix:
  - Normalize the wording to match package conventions.

---

## OBSERVATIONS

### Tool interface and schema conformance
All new tools correctly implement the `Tool` interface shape:
- `Name()`
- `Description()`
- `ToolPurity()`
- `Schema()`
- `Execute()`

Relevant definitions:
- `internal/tool/list_directory.go:39-67`
- `internal/tool/find_files.go:22-51`
- `internal/tool/test_run.go:27-64`
- `internal/tool/db_sqlc.go:24-49`
- `internal/tool/shell.go:65-94`

Schema validity is covered by tests for each new tool.

### Path validation usage
Every new user-supplied path in this slice goes through `resolvePath()`:
- `list_directory`: `internal/tool/list_directory.go:79-92`
- `find_files`: `internal/tool/find_files.go:74-88`
- `test_run`: `internal/tool/test_run.go:81-93`
- `db_sqlc`: `internal/tool/db_sqlc.go:89-101`
- `shell` working dir: `internal/tool/shell.go:123-135`

The remaining problem is not missing usage; it is that `resolvePath()` itself is insufficient against symlink traversal.

### Cross-cutting helper reuse
- `defaultDirExcludes` is defined once and shared between the new directory tools:
  - definition: `internal/tool/list_directory.go:22-37`
  - used by `list_directory`: `internal/tool/list_directory.go:157-161`
  - used by `find_files`: `internal/tool/find_files.go:100-105`
- `fileExists` is defined once and reused consistently:
  - definition: `internal/tool/fileutil.go:99-103`
  - used by `test_run`: `internal/tool/test_run.go:129-138, 268-270`
  - used by `db_sqlc`: `internal/tool/db_sqlc.go:103-106`

### RTK prefix / denylist semantics
- Denylist checks the original command before RTK prefixing:
  - denylist check: `internal/tool/shell.go:112-121`
  - RTK prefix application: `internal/tool/shell.go:150`
- That preserves “check the original command” semantics.
- Operational implication:
  - Denylist entries must target the unprefixed command form; patterns written as `rtk ...` will not match.

### Registration completeness
Direct registration in tidmouth is complete:
- `cmd/tidmouth/serve.go:75-86`

Role-based registration is also complete:
- `internal/role/builder.go:34-58`

Registry helper wiring is centralized:
- `internal/tool/register.go:57-72`

Registry/ToolDefinitions coverage exists:
- `internal/tool/registry_test.go:270-312`

### Output normalization and truncation pipeline
Successful tool outputs still flow through the shared executor pipeline:
- `internal/tool/executor.go:128-140`

New truncation notices exist for:
- `shell`: `internal/tool/truncate.go:47-48`
- `list_directory`: `internal/tool/truncate.go:49-50`
- `find_files`: `internal/tool/truncate.go:51-52`
- `test_run`: `internal/tool/truncate.go:53-54`
- `db_sqlc`: `internal/tool/truncate.go:55-56`

### No obvious new panic/nil/resource-leak issues found
- No clear nil-deref or leak was found in the new tools.
- Executor-level panic recovery still protects tool execution:
  - `internal/tool/executor.go:147-167`
- The main problems are correctness and sandbox-boundary issues, not crashers.

---

## Test coverage summary by tool

### `list_directory`
Tested:
- happy path
- depth limit
- excludes
- hidden file behavior
- traversal rejection
- missing path
- non-directory target
- schema validity
- directory-first sort order
- scoped subpath

Coverage file:
- `internal/tool/list_directory_test.go:12-279`

Not tested:
- invalid JSON input
- absolute-path rejection
- context cancellation
- unreadable directory handling
- large tree stress
- depth clamping for values outside 1..10

### `find_files`
Tested:
- basename glob
- recursive `**`
- scoped path
- no-results behavior
- `max_results`
- excludes
- traversal rejection
- empty pattern
- schema validity

Coverage file:
- `internal/tool/find_files_test.go:12-199`

Not tested:
- invalid JSON input
- absolute-path rejection
- context cancellation
- unreadable directory handling
- large tree stress
- complex glob patterns like `**/a/**/b`
- Windows-style path separator cases

### `test_run`
Tested:
- Go pass/fail integration
- ecosystem detection
- schema validity
- missing `pytest`
- missing `npx`
- no-ecosystem failure
- parser coverage for Go / pytest / jest JSON and short pytest output

Coverage files:
- `internal/tool/test_run_test.go:41-265`
- `internal/tool/test_run_parse_test.go:10-246`

Not tested:
- missing `go`
- unknown ecosystem override
- invalid JSON input
- path traversal / absolute path rejection
- context cancellation and timeout for all ecosystems
- positive Python runtime path
- positive TypeScript runtime path
- Go `filter` behavior
- Go package/subdirectory resolution behavior
- hanging pytest probe fallback

### `db_sqlc`
Tested:
- schema validity
- purity
- missing config
- invalid action
- traversal rejection
- `vet` success
- `diff` happy-path invocation

Coverage file:
- `internal/tool/db_sqlc_test.go:67-176`

Not tested:
- missing `sqlc`
- default `generate` action
- `generate` success/failure branches
- `vet` failure branch
- `diff` failure-vs-out-of-sync distinction
- invalid JSON input
- absolute-path rejection
- timeout / cancellation

### `shell` RTK expansion
Tested:
- normal success
- non-zero exit handling
- timeout
- denylist matching
- cancellation
- working directory traversal rejection
- schema validity
- registration helper
- `applyRTKPrefix()` unit behavior

Coverage file:
- `internal/tool/shell_test.go:11-258`

Not tested:
- end-to-end RTK-prefixed execution path through `Execute()`
- RTK detection path in `NewShell()`
- invalid JSON input
- absolute-path rejection for `working_dir`
- process-start failure path
- descendant process cleanup after timeout/cancel

---

## Resolver guidance

Recommended fix order:

1. Harden `resolvePath()` against symlink escape.
2. Fix `test_run` so infrastructure/runtime failures cannot be reported as success.
3. Fix `db_sqlc diff` error classification.
4. Add short timeout/fallback to pytest capability probing.
5. Fix Go path handling in `test_run`.
6. Add targeted regression tests for all of the above.
7. Fill remaining builder/truncation/cancellation coverage gaps.

Suggested resolver scope split:
- Resolver A: path sandbox hardening (`resolvePath` + regression tests)
- Resolver B: `test_run` correctness fixes (runErr handling, path validation, timeout/cancel, Go path semantics)
- Resolver C: `db_sqlc` diff classification + missing branch coverage
- Resolver D: test coverage cleanup (builder groups, truncation notices, cancellation tests)

## Acceptance criteria for follow-up fix PR

A follow-up PR should be considered ready only when:
- symlink escape is blocked at the shared path-validation layer
- `test_run` no longer reports false success on execution failure, missing path, timeout, or cancellation
- `db_sqlc diff` no longer hides true command failures
- pytest capability probing cannot hang indefinitely
- new regression tests cover the critical and important cases above
- role-builder coverage explicitly includes `directory`, `test`, and `sqlc`
- truncation tests cover all new tool names
