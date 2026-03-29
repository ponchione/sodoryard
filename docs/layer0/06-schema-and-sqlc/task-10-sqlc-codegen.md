# Task 10: Run sqlc Code Generation and Verify

**Epic:** 06 — Schema & sqlc Code Generation
**Status:** ⬚ Not started
**Dependencies:** Task 08, Task 09

---

## Description

Run `sqlc generate` to produce type-safe Go code from the schema and query files. Verify the generated code compiles.

## Acceptance Criteria

- [ ] `sqlc generate` completes without errors
- [ ] Generated `.go` files exist in the configured output directory
- [ ] `go build ./...` succeeds with the generated code
- [ ] Generated types match the schema (correct field types, nullable handling)
