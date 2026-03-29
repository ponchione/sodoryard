# Task 01: Basic Shell Execution

**Epic:** 05 — Shell Tool
**Status:** ⬚ Not started
**Dependencies:** Layer 4 Epic 01

---

## Description

Implement the core `shell` tool as a `Mutating` tool in `internal/tool/`. This task covers the basic execution path: running a command via `sh -c`, capturing stdout and stderr separately, formatting the output with exit code, and handling working directory enforcement. Timeout, denylist, and process group management are added in subsequent tasks.

## Acceptance Criteria

- [ ] Implements the `Tool` interface with `Purity() = Mutating`
- [ ] `Name()` returns `"shell"`, `Description()` returns a concise one-liner for the LLM
- [ ] Accepts JSON input with parameters: `command` (required string), `timeout_seconds` (optional int), `working_dir` (optional string)
- [ ] Executes the command via `sh -c <command>` using `exec.CommandContext`
- [ ] Working directory set to `projectRoot` by default. If `working_dir` is specified, resolved as `projectRoot/<working_dir>`.
- [ ] Subdirectory path traversal outside project root rejected (e.g., `working_dir: "../../etc"` returns `Success=false`)
- [ ] Captures stdout and stderr into separate `bytes.Buffer` instances
- [ ] Returns formatted output:
  ```
  Exit code: <N>

  STDOUT:
  <stdout content>

  STDERR:
  <stderr content>
  ```
- [ ] If stdout is empty, the STDOUT section is omitted from the output
- [ ] If stderr is empty, the STDERR section is omitted from the output
- [ ] If both are empty (e.g., `true` command), returns just `"Exit code: 0"`
- [ ] Non-zero exit codes return `Success=true` — the command ran and produced output. The LLM interprets whether the output indicates a problem.
- [ ] Infrastructure failures (e.g., `sh` not found, working directory doesn't exist) return `Success=false` with the OS-level error message
- [ ] `Schema()` returns valid JSON Schema with parameter types, descriptions, required fields, and notes about timeout/denylist behavior
