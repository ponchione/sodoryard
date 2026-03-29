# Task 02: Head-Tail Preservation Algorithm (Steps 1-4)

**Epic:** 07 — Compression Engine
**Status:** ⬚ Not started
**Dependencies:** Task 01; Layer 2: Epic 07 (provider router for summarization calls)

---

## Description

Implement the core compression algorithm: head-tail preservation with LLM-based summarization of middle turns. Protect the first N messages (head: system prompt context, initial user message, first assistant response) and the last M messages (tail: most recent turns, active working context). Extract all messages between head and tail as compression candidates. Summarize the middle using an auxiliary model via the Layer 2 provider interface with `purpose = "compression"`, producing a concise structured summary that preserves key decisions, file paths, completed work, and error resolutions.

## Acceptance Criteria

- [ ] `Compress(ctx context.Context, conversationID string, modelContextLimit int) error` method
- [ ] **Step 1 — Protect head:** First N messages preserved (configurable: `CompressionHeadPreserve`, default 3)
- [ ] **Step 2 — Protect tail:** Last M messages preserved (configurable: `CompressionTailPreserve`, default 4)
- [ ] **Step 3 — Extract middle:** All messages between head and tail boundaries identified as compression candidates
- [ ] **Step 4 — Summarize:** Calls configured compression model via Layer 2 provider interface with `purpose = "compression"`
- [ ] Summary prompt instructs the model to produce a concise structured summary preserving: key decisions made, file paths mentioned, work completed, errors encountered and resolutions
- [ ] Summary is prefixed with `[CONTEXT COMPACTION]`
- [ ] Summary formatted as structured bullet points, no code blocks
- [ ] Package compiles with no errors
