# Task 03: Modification Intent Detection

**Epic:** 02 — Turn Analyzer
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02

---

## Description

Implement modification intent detection as the third signal rule. When a modification verb (fix, refactor, change, update, edit, modify, rewrite, rename, move, delete, remove) appears in the user message alongside an identified target (file or symbol from earlier extraction rules), the target is added to `ExplicitSymbols` for structural graph lookup to determine blast radius. This rule depends on file and symbol extraction running first so it can check whether a target was already found.

## Acceptance Criteria

- [ ] Modification verb set defined: "fix", "refactor", "change", "update", "edit", "modify", "rewrite", "rename", "move", "delete", "remove"
- [ ] When a modification verb appears with an identified target (file or symbol), the target is added to `ExplicitSymbols` for structural graph lookup
- [ ] Each detection produces a `Signal{Type: "modification_intent", Source: <verb + target>, Value: <target>}`
- [ ] Runs after file and symbol extraction (uses their results to identify targets)
- [ ] Message without an identified target but with a modification verb does not produce a false signal
- [ ] Package compiles with no errors
