# Task 01: Query Extractor - Cleaned Message (Source 1)

**Epic:** 03 — Query Extraction & Momentum
**Status:** ⬚ Not started
**Dependencies:** Epic 01

---

## Description

Implement the first query source in the `QueryExtractor`: the cleaned message query. Strip conversational filler words and phrases from the user's message, remove punctuation, and cap at approximately 50 words to produce a focused semantic search query. For long messages with multiple sentences, split at sentence boundaries and produce up to 2 queries. This is the primary query source and produces the most semantically faithful representation of the user's intent.

## Acceptance Criteria

- [ ] `QueryExtractor` function or struct defined that takes a user message and `ContextNeeds` and produces `[]string` (1-3 semantic queries)
- [ ] Conversational filler stripped: "hey", "can you", "please", "I think", "I want you to", "could you", "help me", "let's" (and similar phrases)
- [ ] Punctuation stripped from the output query
- [ ] Output capped at approximately 50 words
- [ ] For long messages with multiple sentences, split at sentence boundaries (`.`, `?`, `!` followed by whitespace) and produce up to 2 queries
- [ ] Short single-sentence messages produce exactly 1 query
- [ ] **Explicit entity exclusion:** File paths and symbol names from `ContextNeeds.ExplicitFiles` and `ContextNeeds.ExplicitSymbols` are NOT included in queries (they are handled deterministically by the retrieval orchestrator)
- [ ] Package compiles with no errors
