# Task 07: Unit Tests

**Epic:** 02 — Turn Analyzer
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, Task 03, Task 04, Task 05, Task 06

---

## Description

Comprehensive unit tests for the `RuleBasedAnalyzer` covering all six signal extraction rules, edge cases, and the Signal trace. Every test verifies both the `ContextNeeds` output and the `Signals` trace entries to ensure full observability of the analyzer's decision-making process.

## Acceptance Criteria

- [ ] Test: message with explicit file path (`internal/auth/middleware.go`) populates `ExplicitFiles`
- [ ] Test: message with backtick-wrapped symbol (`` `ValidateToken` ``) populates `ExplicitSymbols`
- [ ] Test: combined message "Fix `ValidateToken` in `internal/auth/service.go`" populates both `ExplicitFiles` and `ExplicitSymbols`, and produces a `modification_intent` signal
- [ ] Test: "Create a test for the auth handler" sets `IncludeConventions = true` and produces a `creation_intent` signal
- [ ] Test: "What changed in the last 3 commits?" sets `IncludeGitContext = true` with appropriate depth
- [ ] Test: "Keep going" with no other signals produces a `continuation` signal
- [ ] Test: every test case verifies that the `Signals` trace contains the expected entries with correct `Type`, `Source`, and `Value`
- [ ] Test: edge case — message with no recognizable signals produces empty `ContextNeeds` (no `SemanticQueries`, no `ExplicitFiles`, etc.)
- [ ] Test: edge case — PascalCase English words ("However", "Because") are filtered by the stopword set and do not appear in `ExplicitSymbols`
- [ ] All tests pass: `go test ./internal/context/...`
