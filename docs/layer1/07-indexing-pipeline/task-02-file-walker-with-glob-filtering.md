# Task 02: File Walker with Glob Filtering

**Epic:** 07 — Indexing Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 01, L0-E03 (config)

---

## Description

Implement the file discovery component that walks the project directory, applies include/exclude glob filters from config, and enforces the max file size limit. This is the entry point of Pass 1 — it produces the list of file paths to be parsed. Uses `filepath.WalkDir` for traversal and a `doublestar`-compatible library (e.g., `github.com/bmatcuk/doublestar/v4`) for `**` glob pattern support.

## Function Signature

```go
func (idx *Indexer) walkFiles(ctx context.Context) ([]FileEntry, error)
```

Where `FileEntry` is:
```go
type FileEntry struct {
    Path    string // absolute path
    RelPath string // relative to project root
    Size    int64
}
```

## Acceptance Criteria

- [ ] `FileEntry` struct defined with `Path` (absolute), `RelPath` (relative to project root), and `Size` (bytes)
- [ ] Uses `filepath.WalkDir` to traverse `IndexerConfig.ProjectRoot`
- [ ] **Include filtering:** if `IncludeGlobs` is non-empty, a file must match at least one include glob to be included. Glob matching uses the file's relative path (e.g., `internal/rag/indexer.go` matched against `**/*.go`)
- [ ] **Exclude filtering:** if a file matches any glob in `ExcludeGlobs`, it is excluded regardless of include matches. Exclude is evaluated after include
- [ ] **Default excludes** always applied even if not in config: `**/.git/**`, `**/vendor/**`, `**/node_modules/**`, `**/.sirtopham/**`
- [ ] **Max file size:** files exceeding `MaxFileSizeBytes` are skipped. A debug log message is emitted: `"skipping file exceeding size limit" path=<relpath> size=<bytes> limit=<limit>`
- [ ] **Directories:** `.git`, `vendor`, `node_modules`, `.sirtopham` directories are skipped entirely via `fs.SkipDir` (no descending into them)
- [ ] **Symlinks:** symlinks are not followed (default `WalkDir` behavior)
- [ ] Context cancellation: returns early if `ctx` is cancelled mid-walk
- [ ] Returns `[]FileEntry` sorted by relative path (deterministic ordering)
- [ ] Glob matching uses a library that supports `**` patterns (e.g., `github.com/bmatcuk/doublestar/v4`)
