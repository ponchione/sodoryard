# Layer 4 — Epic 05: Shell Tool

**Layer:** 4 (Tool System)
**Package:** `internal/tool/`
**Status:** ⬚ Not Started
**Dependencies:**
- Layer 4 Epic 01: Tool Interface, Registry & Executor
- Layer 0 Epic 03: Configuration (`internal/config/` — `shell_timeout_seconds`, `shell_denylist`)

**Architecture Refs:**
- [[05-agent-loop]] §Tool Set — shell tool definition, denylist, timeout
- [[05-agent-loop]] §Cancellation — SIGTERM/SIGKILL process management during cancellation
- [[05-agent-loop]] §Configuration — `shell_timeout_seconds: 120`, `shell_denylist` patterns

---

## What This Epic Covers

The `shell` tool — arbitrary command execution with safety guardrails, timeout management, and process lifecycle control. This is the most complex individual tool because it must handle:

- stdout/stderr capture with combined or separate output
- Configurable timeout with process group termination
- Denylist matching for catastrophic commands
- Context cancellation with SIGTERM -> SIGKILL escalation
- Working directory enforcement (always project root)

The shell tool is `Mutating` — it can modify the filesystem, run builds, execute tests, install dependencies, or anything else. It's the agent's escape valve for operations not covered by the structured tools.

---

## Definition of Done

- [ ] Implements `Tool` interface with purity `Mutating`
- [ ] Parameters: `command` (required — the shell command string), `timeout_seconds` (optional — overrides default), `working_dir` (optional — subdirectory within project root, default is project root)
- [ ] Executes via `sh -c <command>` (POSIX shell, not bash-specific)
- [ ] Working directory is `projectRoot` (or `projectRoot/working_dir` if subdirectory specified). Subdirectory must not escape project root via `..`
- [ ] Captures stdout and stderr separately. Returns formatted output:
  ```
  Exit code: 0

  STDOUT:
  <stdout content>

  STDERR:
  <stderr content>
  ```
- [ ] If stderr is empty, omits the STDERR section. If stdout is empty, omits the STDOUT section.
- [ ] **Timeout:** Default from config `shell_timeout_seconds` (120s). Per-call override via parameter. On timeout: SIGTERM to process group -> 5 second grace period -> SIGKILL. Returns whatever output was captured before timeout, with message: `[Command timed out after Ns. Process killed. Partial output above.]`
- [ ] **Process group:** Command runs in its own process group (`Setpgid: true`). SIGTERM/SIGKILL target the group (`-pid`), not just the parent process. This ensures child processes (e.g., spawned by build tools) are also terminated.
- [ ] **Denylist:** Before execution, check the command string against `shell_denylist` patterns from config. If matched, return error: `"Command rejected by safety denylist: matches pattern '<pattern>'. This is a safeguard against catastrophic mistakes."` Default denylist per [[05-agent-loop]]: `rm -rf /`, `git push --force`. Matching is substring-based (simple, not regex — these are catastrophic patterns, not fine-grained rules).
- [ ] **Context cancellation:** When the executor's context is cancelled (user pressed cancel), send SIGTERM to the process group. Wait up to 5 seconds. If still running, SIGKILL. Return captured output with `[Command cancelled by user. Partial output above.]`
- [ ] **Exit code handling:** Non-zero exit codes are NOT errors in the `ToolResult` sense. A failed build (exit code 1) returns `Success=true` with the build output — the LLM needs to see the errors to fix them. Only infrastructure failures (command not found, permission denied to create process) set `Success=false`.
- [ ] JSON Schema accurately describes all parameters, with description noting the timeout default and denylist behavior
- [ ] Unit tests: successful command, non-zero exit code, timeout with SIGTERM/SIGKILL, denylist rejection, cancellation, subdirectory working dir, path traversal rejection
- [ ] Integration test: run a real command (e.g., `echo hello`), verify stdout capture and exit code

---

## Key Design Notes

**Exit code semantics.** A crucial design point: non-zero exit codes are success from the tool's perspective. The LLM asked to run a command, the command ran, it returned output. Whether the output indicates a build failure or test failure is for the LLM to interpret. Only tool-level infrastructure failures (can't spawn process, can't access working directory) are `Success=false`.

**Denylist is minimal.** Per [[05-agent-loop]]: "Since this is a personal tool, the denylist is minimal — just catastrophic mistakes, not general safety theater." The denylist is a comma-separated list of substring patterns in config, not a security boundary.

**Streaming output (future).** [[05-agent-loop]] §Open Questions discusses streaming shell output to the UI in real-time via `tool_call_output` events for long-running commands. This is NOT in scope for this epic. The shell tool captures all output and returns it as a single result. Real-time streaming is a Layer 5/7 concern that can be added later by having the executor emit incremental events.

**Process group cleanup.** This is important for correctness. If the agent runs `go test ./...` and the user cancels, we need to kill not just the `sh` process but all the `go` subprocesses it spawned. Process group management (`Setpgid` + `syscall.Kill(-pgid, signal)`) handles this.

---

## Consumed By

- [[layer4-epic01-tool-interface]] — registered in the tool registry
- Layer 5 (Agent Loop) — dispatched via the executor; cancellation propagated via context
