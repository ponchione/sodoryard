# Task 04: Compression Integration and FullContextPackage Freezing

**Epic:** 06 — Context Assembly Pipeline
**Status:** ⬚ Not started
**Dependencies:** Task 01; Epic 07 (compression engine)

---

## Description

Integrate compression signaling with the assembly pipeline. Before returning the `FullContextPackage`, check the `BudgetResult.CompressionNeeded` flag. If compression is needed, the pipeline returns a `CompressionNeeded` signal alongside the package so the agent loop can invoke the compression engine before proceeding to the LLM call. Compression does not re-trigger assembly — the assembled context is already frozen. Also implement the `FullContextPackage` freezing invariant: once frozen, any attempt to modify the package returns an error or panics.

## Acceptance Criteria

- [ ] Before returning `FullContextPackage`, checks `BudgetResult.CompressionNeeded` flag
- [ ] If compression is needed, returns a `CompressionNeeded` signal alongside the `FullContextPackage` (the pipeline does not directly invoke compression — the agent loop does)
- [ ] After compression (invoked by the agent loop), the pipeline re-measures history token count for the report if needed
- [ ] Compression does not re-trigger assembly — assembled context is already frozen
- [ ] `FullContextPackage` freezing enforced: the `Frozen` flag is set at step 8, and the struct is immutable after creation
- [ ] All LLM iterations within the same turn reuse the same frozen `FullContextPackage`
- [ ] Package compiles with no errors
