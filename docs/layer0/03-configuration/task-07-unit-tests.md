# Task 07: Configuration Unit Tests

**Epic:** 03 — Configuration Loading
**Status:** ⬚ Not started
**Dependencies:** Task 04, Task 05, Task 06

---

## Description

Write unit tests covering the full configuration loading pipeline: defaults, partial overrides, validation, and environment variable precedence.

## Acceptance Criteria

- [ ] Test that loading with no file produces valid defaults for every section
- [ ] Test that partial YAML correctly overrides only specified fields
- [ ] Test that invalid values (bad port, negative tokens, unknown enum) produce specific errors
- [ ] Test that environment variables override YAML values
- [ ] Test that env var precedence: env > yaml > defaults
- [ ] All tests pass via `go test ./internal/config/...`
