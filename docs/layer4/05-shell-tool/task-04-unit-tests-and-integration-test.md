# Task 04: Unit Tests and Integration Test

**Epic:** 05 — Shell Tool
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, Task 03

---

## Description

Write comprehensive unit tests and an integration test for the shell tool. Tests cover the full range of behaviors: successful execution, non-zero exit codes, stdout/stderr separation, timeout with process group termination, denylist rejection, context cancellation, subdirectory working directory, and path traversal rejection. The integration test runs a real command to verify end-to-end behavior.

## Acceptance Criteria

- [ ] All tests in `internal/tool/` package (e.g., `shell_test.go`)
- [ ] All tests pass via `go test ./internal/tool/...`
- [ ] **Successful command:** Run `echo hello`, verify `Success=true`, exit code 0, stdout contains `"hello"`
- [ ] **Non-zero exit code:** Run `sh -c 'exit 42'`, verify `Success=true` (not a tool error), exit code 42 in output
- [ ] **Stdout and stderr separation:** Run a command that writes to both (e.g., `sh -c 'echo out; echo err >&2'`), verify STDOUT and STDERR sections are separate and correct
- [ ] **Stderr only:** Run a command that writes only to stderr, verify STDOUT section is omitted
- [ ] **Timeout — SIGTERM sufficient:** Run `sleep 60` with `timeout_seconds: 1`. Verify the tool returns within a few seconds (not 60), result includes `"timed out"` notice, and the `sleep` process is no longer running
- [ ] **Timeout — SIGKILL escalation:** Run a command that traps SIGTERM (e.g., `sh -c 'trap "" TERM; sleep 60'`) with `timeout_seconds: 1`. Verify SIGKILL terminates it after the grace period.
- [ ] **Denylist rejection:** Attempt to run `rm -rf /`, verify `Success=false` with denylist message and that no process was spawned
- [ ] **Denylist — partial match:** Verify that `rm -rf /tmp/testdir` does NOT match the `rm -rf /` pattern (the denylist pattern is `rm -rf /` which is a substring of `rm -rf /tmp/testdir` — decide and document whether this should match or not, and test accordingly)
- [ ] **Context cancellation:** Start a `sleep 60` command, cancel the context after 100ms. Verify the tool returns promptly with the cancellation notice.
- [ ] **Subdirectory working dir:** Create a subdirectory, run `pwd` with `working_dir` set to it. Verify the output shows the correct path.
- [ ] **Path traversal rejection:** Attempt `working_dir: "../../.."`, verify `Success=false` with traversal error
- [ ] **Integration test:** Register the shell tool, dispatch via executor, run `echo integration-test`, verify full pipeline produces correct result
- [ ] Schema validation: `Schema()` output is valid JSON and contains expected parameter names
