# Task 02: Timeout and Process Group Management

**Epic:** 05 — Shell Tool
**Status:** ⬚ Not started
**Dependencies:** Task 01, Layer 0 Epic 03 (config — `shell_timeout_seconds`)

---

## Description

Add timeout enforcement and process group management to the shell tool. Commands run in their own process group so that SIGTERM/SIGKILL can terminate the entire process tree (parent shell and all child processes). On timeout, the tool escalates from SIGTERM to SIGKILL after a grace period, captures any partial output, and appends a notice.

## Acceptance Criteria

- [ ] Default timeout read from config `shell_timeout_seconds` (default 120 seconds)
- [ ] Per-call timeout override via the `timeout_seconds` parameter — if provided, overrides the config default for that call only
- [ ] Command process started with `SysProcAttr.Setpgid = true` to create a new process group
- [ ] On timeout expiration:
  1. Send `SIGTERM` to the process group (`syscall.Kill(-pgid, syscall.SIGTERM)`)
  2. Wait up to 5 seconds for the process to exit
  3. If still running after 5 seconds, send `SIGKILL` to the process group
  4. Collect whatever stdout/stderr was captured before the timeout
- [ ] Timeout output appends the notice: `[Command timed out after <N>s. Process killed. Partial output above.]`
- [ ] Timeout result has `Success=true` (the command ran but took too long — the LLM needs to see the partial output)
- [ ] Process group SIGTERM/SIGKILL targets `-pgid` (negative PID), not just the parent process — ensures child processes spawned by build tools, test runners, etc. are also terminated
- [ ] If the process exits before SIGKILL is needed (within the grace period), the extra kill is not sent
