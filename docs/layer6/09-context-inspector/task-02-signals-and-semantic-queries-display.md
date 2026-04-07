# Task 02: Turn Analyzer Signals and Semantic Queries Display

**Epic:** 09 — Context Inspector
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Display the turn analyzer's detected signals plus the ordered signal/query flow and semantic queries derived from the user's message. The signals section shows what the analyzer detected in the user's input (file references, symbol references, intent verbs, momentum patterns). The signal-flow section shows the ordered stream exposed by `/api/metrics/conversation/:id/context/:turn/signals`. The semantic queries section shows the queries that were sent to retrieval. Together, these sections answer the question: "What did the system understand from my message, and what did it search for?"

## Acceptance Criteria

- [ ] **Signals section:** Displayed as a collapsible section within the context inspector panel, labeled "Signals" or "Turn Analysis"
- [ ] Signals rendered from the `signals_json` array in the context report
- [ ] Ordered signal flow rendered from `GET /api/metrics/conversation/:id/context/:turn/signals`
- [ ] Each signal displays: type (e.g., `file_ref`, `symbol_ref`, `intent_verb`, `momentum`), source text (the part of the user's message that triggered the signal), and extracted value (the file path, symbol name, or intent)
- [ ] Signal types are visually distinguished (e.g., different badge colors or icons per type)
- [ ] Signals are displayed in the order they appear in the array
- [ ] If no signals were detected, display "No signals detected" rather than an empty section
- [ ] **Semantic queries section:** Displayed as a collapsible section labeled "Queries" or "Search Queries"
- [ ] Queries rendered from the `needs_json` field in the context report
- [ ] Signal-flow entries preserve order across signals, semantic queries, explicit files/symbols, momentum entries, and flags
- [ ] Each query shows the query text that was sent to the RAG pipeline for semantic search
- [ ] If needs include explicit file paths or symbol lookups (in addition to semantic queries), these are displayed with their type (e.g., "explicit file: auth/middleware.go", "semantic: authentication middleware error handling")
- [ ] If no queries were generated, display "No queries generated"
- [ ] Both sections are collapsed by default to save space, expandable on click
