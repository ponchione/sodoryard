# Task 03: RAG, Brain, and Structural Graph Results Display

**Epic:** 09 — Context Inspector
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Display the retrieval results from the three context sources: RAG (code chunk semantic search), brain (project knowledge documents), and structural graph (symbol relationships). Each result shows its source, relevance score, and whether it was included or excluded from the final assembled context. Excluded results show the reason for exclusion. This is the most actionable section of the inspector — it answers "what did the system find, what made it in, and what was left out?"

## Acceptance Criteria

- [ ] **RAG results section:** Collapsible section labeled "Code Chunks (RAG)"
- [ ] Rendered from `rag_results_json` in the context report
- [ ] Each result displays: file path, chunk name/identifier, similarity score (0.0-1.0), and included/excluded status
- [ ] Results sorted by similarity score descending (most relevant first)
- [ ] **Included** results badged/colored green. **Excluded** results badged/colored red/grey with the exclusion reason (e.g., "below_threshold", "budget_exceeded")
- [ ] The relevance threshold (e.g., 0.35) is visually indicated — results above it are candidates, results below were filtered pre-budget
- [ ] **Included vs excluded summary** shown at the top of the section: "12 included, 8 excluded" (counts)
- [ ] Excluded chunks are browsable — not hidden by default. This is the most useful debugging data
- [ ] **Brain results section:** Collapsible section labeled "Brain Documents"
- [ ] Rendered from `brain_results_json` in the context report
- [ ] Each result displays: document vault path, title, match score, match mode (keyword/semantic/graph), included/excluded status
- [ ] Brain document paths displayed as text for v0.1 (clickable `obsidian://` links deferred to v0.3)
- [ ] **Structural graph results section:** Collapsible section labeled "Structural Graph"
- [ ] Rendered from `graph_results_json` in the context report (may be empty if no structural analysis was performed)
- [ ] Each result displays: symbol name, relationship type (caller, callee, implements, extends), depth from the reference symbol, and file path
- [ ] If no structural graph results exist for a turn, the section shows "No structural analysis" and remains collapsed
- [ ] All three sections handle empty data gracefully — display an informative message rather than a blank section
