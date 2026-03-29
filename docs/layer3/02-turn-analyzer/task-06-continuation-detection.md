# Task 06: Continuation Detection

**Epic:** 02 — Turn Analyzer
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02

---

## Description

Implement continuation detection as the sixth and final signal rule. When the user message contains signal words (continue, keep going, finish, next, also, too) combined with the absence of strong new-topic signals (no explicit files, no explicit symbols from earlier rules), the analyzer flags this as a continuation turn. Continuation populates `MomentumFiles` and `MomentumModule` from recent history scanning, enabling the momentum tracker (Epic 03) to provide context continuity across turns.

## Acceptance Criteria

- [ ] Continuation signal words defined: "continue", "keep going", "finish", "next", "also", "too"
- [ ] Continuation detected when signal words are present AND no explicit files or symbols were extracted by earlier rules
- [ ] When continuation is detected, recent history (`recentHistory` parameter) is scanned for file paths to populate `MomentumFiles` and `MomentumModule`
- [ ] Each detection produces a `Signal{Type: "continuation", Source: <matched signal word>, Value: "momentum_applied"}`
- [ ] If signal words are present but strong new-topic signals also exist, continuation is NOT detected (new topic takes priority)
- [ ] Package compiles with no errors
