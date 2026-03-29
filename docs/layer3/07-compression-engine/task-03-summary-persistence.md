# Task 03: Summary Persistence and Sequence Numbering (Steps 5-6)

**Epic:** 07 — Compression Engine
**Status:** ⬚ Not started
**Dependencies:** Task 02; Layer 0: Epic 04 (SQLite), Epic 06 (schema/sqlc)

---

## Description

Implement the persistence of compression results to SQLite (steps 5-6 of the algorithm). Mark all middle messages as compressed (`is_compressed = 1`), insert a synthetic summary message with proper metadata (`is_summary = 1`, turn range coverage), and compute the REAL-type sequence number as the midpoint between the last head message and first tail message. The sequence bisection approach (IEEE 754 doubles) supports thousands of compressions without integer collision.

## Acceptance Criteria

- [ ] **Step 5 — Persist compression:**
  - Set `is_compressed = 1` on all middle messages in the `messages` table
  - INSERT a synthetic summary message with: `role = 'user'`, `is_summary = 1`, `compressed_turn_start` and `compressed_turn_end` recording the turn range covered
  - Summary message `sequence` = `(lastHeadSeq + firstTailSeq) / 2.0` (REAL midpoint bisection)
- [ ] **Step 6 — Orphaned tool call sanitization:**
  - Scan remaining uncompressed messages for assistant messages containing `tool_calls` in their content JSON where the corresponding `role=tool` result messages have been compressed
  - For orphaned tool_use blocks, remove them from the assistant message content JSON
  - This prevents API rejections from malformed conversation structure (assistant tool_use without matching tool result)
- [ ] Sequence midpoint calculation uses IEEE 754 float64 arithmetic
- [ ] After compression, new messages appended to the conversation get the next integer sequence (no collision with bisected summary sequence)
- [ ] Package compiles with no errors
