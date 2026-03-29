# Task 01: LanceDB Dependency and Connection Lifecycle

**Epic:** 05 — LanceDB Store
**Status:** ⬚ Not started
**Dependencies:** L0-E01 (project scaffolding), L0-E03 (config — provides LanceDB data directory path)

---

## Description

Add the LanceDB Go/CGo dependency to the project and implement the connection open and close lifecycle. This creates the `LanceStore` struct in `internal/vectorstore/` that holds the LanceDB connection handle and exposes `Open` and `Close` methods. The data directory path is derived from config: `~/.sirtopham/projects/<project-name>/lancedb/`. Parent directories are created if they do not exist.

## Acceptance Criteria

- [ ] `github.com/lancedb/lancedb-go` (or the current canonical LanceDB Go binding) added to `go.mod`
- [ ] Project compiles with `CGO_ENABLED=1` and LanceDB CGo bindings link successfully
- [ ] Makefile updated with any LanceDB-specific CGo linker flags (e.g., `CGO_LDFLAGS`, `CGO_CFLAGS`) if required by the binding
- [ ] `LanceStore` struct defined in `internal/vectorstore/store.go` with fields: `db` (LanceDB connection handle), `tableName string` (always `"chunks"`), `projectName string`
- [ ] Constructor function: `NewLanceStore(ctx context.Context, dataDir string, projectName string) (*LanceStore, error)`
  - Creates parent directories via `os.MkdirAll` if they do not exist
  - Opens a LanceDB connection at `dataDir`
  - Returns a descriptive error if directory creation fails or LanceDB open fails
- [ ] `Close() error` method cleanly shuts down the LanceDB connection
- [ ] Calling `Close` on an already-closed store does not panic (idempotent) and returns `nil`. The store uses a `sync.Once` or closed flag to ensure the underlying LanceDB connection is closed exactly once.
- [ ] Error returned from `NewLanceStore` includes the data directory path for debuggability

### Interface Compliance

- [ ] Compile-time interface assertion: `var _ codeintel.Store = (*LanceStore)(nil)` in `store.go`
- [ ] Any return type adjustments needed so `*LanceStore` satisfies the `codeintel.Store` interface (e.g., `NewLanceStore` returns `(codeintel.Store, error)`)
- [ ] `go build ./internal/vectorstore/...` compiles without errors
- [ ] `go vet ./internal/vectorstore/...` reports no issues
