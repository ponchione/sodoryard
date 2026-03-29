# Task 03: FullContextPackage Struct

**Epic:** 01 — Context Assembly Types & Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Define the `FullContextPackage` struct, the final output of the context assembly pipeline. This struct wraps the serialized markdown string (system prompt cache block 2), its token count, the assembly report, and a frozen flag that prevents modification after creation. The frozen flag enforces the invariant that assembled context is immutable within a turn — all LLM iterations within the same turn reuse the same context block.

## Acceptance Criteria

- [ ] `FullContextPackage` struct defined with fields: `Content string` (serialized markdown), `TokenCount int`, `Report *ContextAssemblyReport`, `Frozen bool`
- [ ] GoDoc comment explains that the struct is intentionally simple — serialization logic lives in Epic 05, this is just the container
- [ ] GoDoc comment explains the two-phase lifecycle: created mutable, then frozen after assembly completes
- [ ] Package compiles with no errors
