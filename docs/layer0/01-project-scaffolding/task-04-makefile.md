# Task 04: Create Makefile with Build Targets

**Epic:** 01 — Project Scaffolding
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 03

---

## Description

Create a Makefile at the repository root with `build`, `test`, and `clean` targets. All targets must set `CGO_ENABLED=1` since the project depends on a CGo SQLite driver.

## Acceptance Criteria

- [ ] `make build` compiles the binary with `CGO_ENABLED=1`
- [ ] `make test` runs `go test ./...` with `CGO_ENABLED=1`
- [ ] `make clean` removes build artifacts
- [ ] All three targets succeed when run in sequence
