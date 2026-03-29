# Task 05: Previously-Viewed Annotations and Section Formatting

**Epic:** 05 — Budget Manager & Context Serialization
**Status:** ⬚ Not started
**Dependencies:** Task 04

---

## Description

Implement the previously-viewed annotation system and finalize section formatting. Chunks from files in the `seenFiles` set (maintained at the session level by the agent loop, passed as input) are annotated with `[previously viewed in turn N]` in their header. This helps the LLM understand which code the user has already seen, reducing redundant explanations. Ensure all sections (code, conventions, git) have properly nested headers, properly closed code fences, and handle edge cases like empty sections gracefully.

## Acceptance Criteria

- [ ] Chunks from files in the `seenFiles` set are annotated with `[previously viewed in turn N]` in the header
- [ ] The `seenFiles` set is received as input (not maintained by the serializer)
- [ ] Empty sections are omitted entirely (for example, no empty `## Project Conventions` or `## Recent Changes` section when that content is absent)
- [ ] All code fences are properly closed
- [ ] Headers are properly nested (`##` for sections, `###` for individual items)
- [ ] Empty `RetrievalResults` (no hits from any source) produces an empty or minimal markdown output without crashing
- [ ] Output remains deterministic with annotations applied
- [ ] Package compiles with no errors
