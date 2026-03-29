# Task 05: Momentum Unit Tests and Integration Tests

**Epic:** 03 — Query Extraction & Momentum
**Status:** ⬚ Not started
**Dependencies:** Task 03, Task 04

---

## Description

Unit tests for the momentum tracker and integration tests verifying the interaction between momentum and query extraction. Momentum tests verify file path extraction from tool calls, directory prefix computation, and edge cases. Integration tests verify that weak-signal turns with active momentum produce momentum-enhanced queries, while strong-signal turns ignore stale momentum.

## Acceptance Criteria

- [ ] Test: conversation history with `file_read` calls to `internal/auth/middleware.go` and `internal/auth/service.go` produces `MomentumModule = "internal/auth"` and `MomentumFiles = ["internal/auth/middleware.go", "internal/auth/service.go"]`
- [ ] Test: no tool calls in recent history produces empty momentum (empty `MomentumFiles`, empty `MomentumModule`)
- [ ] Test: files spanning multiple directories (e.g., `internal/auth/` and `internal/config/`) produce `MomentumModule = "internal"` (or empty if no meaningful common prefix)
- [ ] Test: single file produces prefix equal to its parent directory
- [ ] Integration test: weak-signal message ("keep going") with active momentum produces momentum files and momentum-enhanced query
- [ ] Integration test: strong-signal message ("fix `internal/config/loader.go`") with stale momentum from auth does not apply momentum (explicit signals take priority)
- [ ] All tests pass: `go test ./internal/context/...`
