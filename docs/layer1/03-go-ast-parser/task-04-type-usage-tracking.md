# Task 04: Type Usage Tracking

**Epic:** 03 — Go AST Parser
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02

---

## Description

Implement type usage tracking for type declarations (structs and interfaces). For each type declaration, identify the other types referenced in its field definitions and embedded types, producing a `TypesUsed []string` list. This metadata feeds the structural graph's blast radius analysis — when a type changes, the graph knows which other types depend on it.

## Acceptance Criteria

- [ ] Method on `GoParser`: `extractTypesUsed(typeSpec *ast.TypeSpec, typesInfo *types.Info) []string`
- [ ] **Struct field types**: for each field in a `*ast.StructType`, the field's type is resolved and recorded
  - Named types are recorded as qualified names (e.g., `"time.Duration"`, `"Config"`)
  - Pointer types record the underlying named type (e.g., `*Config` records `"Config"`)
  - Slice/array types record the element type (e.g., `[]Config` records `"Config"`)
  - Map types record both the key and value types (e.g., `map[string]Config` records `"Config"` — `string` is a builtin, so excluded)
  - Channel types record the element type
- [ ] **Embedded types**: embedded struct/interface types (anonymous fields) are recorded as type references (e.g., `type Foo struct { Bar }` records `"Bar"`)
- [ ] **Interface method signatures**: for each method in an `*ast.InterfaceType`, parameter types and return types are extracted and recorded
- [ ] **Embedded interfaces**: interfaces that embed other interfaces record those as type references (e.g., `type ReadWriter interface { Reader; Writer }` records `"Reader"` and `"Writer"`)
- [ ] **Builtin types are excluded**: `string`, `int`, `int8`, `int16`, `int32`, `int64`, `uint`, `uint8`, `uint16`, `uint32`, `uint64`, `uintptr`, `float32`, `float64`, `complex64`, `complex128`, `bool`, `byte`, `rune`, `error`, `any` are not recorded
- [ ] **Function types in fields**: fields with function types (e.g., `Handler func(r *Request) error`) have their parameter and return types extracted
- [ ] The returned `[]string` is deduplicated and sorted alphabetically
