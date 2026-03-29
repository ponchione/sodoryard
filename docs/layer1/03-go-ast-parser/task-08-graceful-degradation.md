# Task 08: Graceful Degradation

**Epic:** 03 — Go AST Parser
**Status:** ⬚ Not started
**Dependencies:** Task 07

---

## Description

Implement graceful degradation behavior so that when `GoParser` cannot parse a specific file (because it was not loaded by `go/packages`, has syntax errors, or has missing dependencies), the failure is reported cleanly to the caller. The parser dispatcher (L1-E02 or the indexing pipeline) is responsible for falling back to tree-sitter parsing — this task ensures the Go AST parser fails gracefully with typed errors rather than panicking or returning corrupt data.

## Acceptance Criteria

- [ ] Define a sentinel error: `var ErrFileNotLoaded = errors.New("goparser: file not found in loaded packages")`
- [ ] `Parse` and `ParseWithRelationships` return `ErrFileNotLoaded` (wrapped with context) when the given `filePath` is not present in `pkgsByFile`
- [ ] Define a sentinel error: `var ErrPackageErrors = errors.New("goparser: package has type-check errors")`
- [ ] When a file's package has type-check errors (`pkg.TypesInfo` is nil or `pkg.Errors` is non-empty), the parser still attempts extraction using whatever AST information is available, but wraps the result error with `ErrPackageErrors` to signal degraded quality
- [ ] In the degraded path (package has errors but AST is available): declaration extraction proceeds normally, call graph extraction skips unresolvable calls (records them as unresolved strings), type usage extraction skips unresolvable types, interface detection returns empty results
- [ ] The caller can use `errors.Is(err, ErrFileNotLoaded)` to decide whether to fall back to tree-sitter
- [ ] The caller can use `errors.Is(err, ErrPackageErrors)` to decide whether relationship data should be treated as incomplete
- [ ] The parser never panics on malformed input. All AST node access is guarded against nil (nil function body, nil type spec type, nil receiver list, nil field list, nil function type, nil ident, nil selector expr)
