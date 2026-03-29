# Task 02: Symbol Reference Extraction with Stopword Set

**Epic:** 02 — Turn Analyzer
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Implement symbol reference extraction as the second signal rule in the `RuleBasedAnalyzer`. Detect identifiers via three patterns: backtick-wrapped identifiers (e.g., `` `ValidateToken` ``), PascalCase/camelCase words not in the stopword set, and identifiers preceded by programming keywords (`function`, `method`, `type`, `struct`, `interface`, `func`). Also define the stopword set (at least 50 common English words that look like PascalCase but are not code symbols) as a `map[string]struct{}` for O(1) lookup.

## Acceptance Criteria

- [ ] Backtick-wrapped identifiers extracted: `` `ValidateToken` ``, `` `AuthService` ``
- [ ] PascalCase and camelCase words detected and filtered against the stopword set
- [ ] Identifiers preceded by keywords (`function`, `method`, `type`, `struct`, `interface`, `func`) extracted
- [ ] Matched symbols populate `ContextNeeds.ExplicitSymbols`
- [ ] Each match produces a `Signal{Type: "symbol_ref", Source: <matched text>, Value: <extracted symbol>}`
- [ ] Stopword set contains at least 50 common English words that are false-positive PascalCase matches: This, That, When, Where, Then, True, False, None, However, Because, etc.
- [ ] Stopword set implemented as `map[string]struct{}` for O(1) lookup
- [ ] Package compiles with no errors
