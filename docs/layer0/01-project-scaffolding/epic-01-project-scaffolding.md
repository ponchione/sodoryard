# Epic 01: Project Scaffolding

**Phase:** Build Phase 1 — Layer 0
**Status:** ⬚ Not started
**Dependencies:** None — this is the root epic.
**Blocks:** [[02-structured-logging]], [[03-configuration]], [[04-sqlite-connection]], [[05-uuidv7]]

---

## Description

Initialize the Go module, establish the package directory layout from the spec (`internal/config/`, `internal/db/`, `internal/logging/`, `cmd/sirtopham/`), and set up the Makefile with CGo-enabled build targets. This is the skeleton everything else is built on.

---

## Definition of Done

- [ ] `go mod init` with module path established
- [ ] Directory structure for all `internal/` packages created with placeholder files
- [ ] Makefile targets `make build`, `make test`, `make clean` work with `CGO_ENABLED=1`
- [ ] `cmd/sirtopham/main.go` compiles and runs (prints version, exits)
- [ ] `make test` runs and passes (even if the only test is a trivial one)

---

## Architecture References

- [[02-tech-stack-decisions]] — Go as core language, CGo accepted, Makefile build system
