# Task 03: Call Graph Extraction

**Epic:** 03 — Go AST Parser
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02

---

## Description

Implement call graph extraction for each function and method declaration. For every `*ast.CallExpr` within a function body, resolve the callee to a qualified name using `go/types` information and record it in a `Calls []string` list. This forward call data is the critical input for the indexing pipeline's reverse call graph construction (L1-E07, Pass 2).

## Acceptance Criteria

- [ ] Method on `GoParser`: `extractCalls(funcDecl *ast.FuncDecl, typesInfo *types.Info) []string`
- [ ] Walks all `*ast.CallExpr` nodes within the function body using `ast.Inspect`
- [ ] **Direct function calls** (e.g., `fmt.Println(...)`) are resolved to qualified names:
  - Uses `typesInfo.Uses` to look up the `*ast.Ident` or `*ast.SelectorExpr` callee
  - For package-qualified calls: produces `"pkg.FuncName"` format (e.g., `"fmt.Println"`, `"codeintel.NewStore"`)
  - For same-package calls: produces `"FuncName"` (unqualified, since the indexer resolves package context)
- [ ] **Method calls** (e.g., `s.Process(...)`) are resolved to `"ReceiverType.MethodName"` using the type info of the receiver expression
  - If the receiver type is a pointer, the method is still recorded without the `*` prefix (e.g., `"Parser.Parse"`, not `"*Parser.Parse"`)
- [ ] **Unresolved calls** (calls to functions in packages not loaded, or dynamically dispatched calls where type info is unavailable) are recorded as the best-effort string representation of the call expression (e.g., `"unknownPkg.Func"`) rather than silently dropped
- [ ] **Type conversions** (`int(x)`, `MyType(x)`) are excluded from the calls list — these are `*ast.CallExpr` nodes but not actual function calls. Detection: the callee resolves to a `*types.TypeName` rather than a `*types.Func`
- [ ] **Built-in function calls** (`len`, `cap`, `make`, `new`, `append`, `delete`, `close`, `panic`, `recover`, `copy`, `print`, `println`) are excluded from the calls list — these are not user-defined and do not contribute to the call graph
- [ ] The returned `[]string` is deduplicated (a function that calls `fmt.Println` three times lists it once)
- [ ] The returned `[]string` is sorted alphabetically for deterministic output
