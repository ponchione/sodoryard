# Task 03: Denylist and Context Cancellation

**Epic:** 05 — Shell Tool
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, Layer 0 Epic 03 (config — `shell_denylist`)

---

## Description

Add denylist checking and context cancellation handling to the shell tool. The denylist is a minimal set of substring patterns that block catastrophic commands before they execute. Context cancellation (triggered when the user presses cancel in the UI) sends SIGTERM to the process group with the same escalation strategy as timeout, but with a different notice message.

## Acceptance Criteria

- [ ] Denylist patterns loaded from config `shell_denylist` (comma-separated list of substring patterns)
- [ ] Default denylist includes at least: `rm -rf /`, `git push --force`
- [ ] Before executing any command, the command string is checked against every denylist pattern using substring matching (case-sensitive, not regex)
- [ ] If a denylist pattern matches, execution is blocked immediately: returns `Success=false` with `"Command rejected by safety denylist: matches pattern '<pattern>'. This is a safeguard against catastrophic mistakes."`
- [ ] Denylist check runs before any process is spawned — no side effects on rejection
- [ ] Context cancellation handling: when the context passed to `Execute()` is cancelled (e.g., user cancels in UI):
  1. Send `SIGTERM` to the process group
  2. Wait up to 5 seconds for the process to exit
  3. If still running, send `SIGKILL` to the process group
  4. Collect partial stdout/stderr
- [ ] Cancellation output appends the notice: `[Command cancelled by user. Partial output above.]`
- [ ] Cancellation result has `Success=true` (the command started and produced partial output the LLM may need)
- [ ] If the context is already cancelled before execution begins, the command is not started at all — returns `Success=false` with `"Command not started: operation cancelled"`
