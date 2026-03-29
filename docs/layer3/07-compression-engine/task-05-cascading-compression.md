# Task 05: Cascading Compression

**Epic:** 07 — Compression Engine
**Status:** ⬚ Not started
**Dependencies:** Task 03

---

## Description

Implement cascading compression for long conversations where compression fires multiple times. When a second compression occurs, the existing summary message from the first round is marked `is_compressed = 1` along with the newly identified middle messages. A new summary is generated covering the full compressed range, with the old summary text included as input to the summarization call. Only one active (non-compressed) summary exists at any point. The reconstruction query (`WHERE is_compressed = 0 ORDER BY sequence`) must continue to work correctly after multiple compression rounds.

## Acceptance Criteria

- [ ] On second (or subsequent) compression, the existing active summary message is marked `is_compressed = 1`
- [ ] New middle messages identified using the same head-tail preservation rules
- [ ] Old summary text is included as input to the new summarization call along with new middle messages
- [ ] New summary covers the full compressed range (from original first compression through current compression)
- [ ] Only one active (non-compressed) summary exists at any point in time
- [ ] The reconstruction query (`WHERE is_compressed = 0 ORDER BY sequence`) returns correct message order after multiple compression rounds: head messages + latest summary + tail messages
- [ ] Sequence midpoints for cascading summaries use further bisection of available REAL number space
- [ ] Package compiles with no errors
