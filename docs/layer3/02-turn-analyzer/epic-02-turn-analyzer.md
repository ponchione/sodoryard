# Layer 3, Epic 02: Turn Analyzer

**Layer:** 3 — Context Assembly
**Epic:** 02 of 07
**Status:** ⬚ Not started
**Dependencies:** [[layer-3-epic-01-context-assembly-types]]

---

## Description

Implement the rule-based `TurnAnalyzer` that examines a user message and recent conversation history to produce a `ContextNeeds` struct. This is pure string processing — no LLM calls, no I/O, no external dependencies. The analyzer extracts signals (file references, symbol references, intent verbs, git keywords, continuation markers) via regex and heuristics, recording every decision in the `Signals` trace for observability.

The `TurnAnalyzer` interface is defined in Epic 01. This epic provides the `RuleBasedAnalyzer` implementation. The interface contract ensures this implementation can be swapped for an LLM-assisted version later without changing any downstream consumers.

---

## Definition of Done

- [ ] `RuleBasedAnalyzer` struct implementing `TurnAnalyzer` interface
- [ ] **File reference extraction:**
  - Regex matches paths with file extensions: `internal/auth/middleware.go`, `./config.yaml`, `cmd/sirtopham/main.go`
  - Matches Go-convention directory references without extensions: `internal/auth/`, `pkg/server`, `cmd/sirtopham`
  - Populates `ContextNeeds.ExplicitFiles`
  - Each match produces a `Signal{Type: "file_ref", Source: <matched text>, Value: <extracted path>}`
- [ ] **Symbol reference extraction:**
  - Backtick-wrapped identifiers: `` `ValidateToken` ``, `` `AuthService` ``
  - PascalCase and camelCase words not in a stopword set (common English words like "This", "The", "When", etc.)
  - Identifiers preceded by keywords: "function", "method", "type", "struct", "interface", "func"
  - Populates `ContextNeeds.ExplicitSymbols`
  - Each match produces a `Signal{Type: "symbol_ref", ...}`
- [ ] **Modification intent detection:**
  - Verb set: "fix", "refactor", "change", "update", "edit", "modify", "rewrite", "rename", "move", "delete", "remove"
  - When a modification verb appears with an identified target (file or symbol), the target is added to `ExplicitSymbols` for structural graph lookup (blast radius)
  - Produces `Signal{Type: "modification_intent", ...}`
- [ ] **Creation intent detection:**
  - Verb set: "write", "create", "add", "implement", "build", "make"
  - Paired with structural nouns: "test", "endpoint", "handler", "middleware", "migration", "route", "model", "service"
  - Sets `ContextNeeds.IncludeConventions = true`
  - Produces `Signal{Type: "creation_intent", ...}`
- [ ] **Git context detection:**
  - Keyword set: "commit", "diff", "PR", "pull request", "merge", "branch", "recent changes", "what changed", "last push"
  - Sets `ContextNeeds.IncludeGitContext = true` with `GitContextDepth` based on keyword specificity
  - Produces `Signal{Type: "git_context", ...}`
- [ ] **Continuation detection:**
  - Signal words: "continue", "keep going", "finish", "next", "also", "too"
  - Combined with the absence of strong new-topic signals (no explicit files, no explicit symbols)
  - Populates `MomentumFiles` and `MomentumModule` from recent history scanning (delegates to momentum extraction, which may be in this epic or Epic 03)
  - Produces `Signal{Type: "continuation", ...}`
- [ ] **Stopword set** for symbol extraction — at least 50 common English words that look like PascalCase but aren't symbols (This, That, When, Where, Then, True, False, None, etc.)
- [ ] **Unit tests** covering:
  - Message with explicit file path → populates ExplicitFiles
  - Message with backtick-wrapped symbol → populates ExplicitSymbols
  - "Fix `ValidateToken` in `internal/auth/service.go`" → both ExplicitFiles and ExplicitSymbols populated, modification intent detected
  - "Create a test for the auth handler" → IncludeConventions = true, creation intent signal
  - "What changed in the last 3 commits?" → IncludeGitContext = true
  - "Keep going" with no other signals → continuation detected
  - Every test verifies the Signals trace contains the expected entries
  - Edge case: message with no recognizable signals produces empty ContextNeeds (no SemanticQueries, no ExplicitFiles, etc.)
  - Edge case: PascalCase English words ("However", "Because") are filtered by stopword set

---

## Architecture References

- [[06-context-assembly]] — "Component: Turn Analyzer" section, "Signal Extraction Rules (v0.1)" subsection, "Replaceability" subsection

---

## Implementation Notes

- Signal extraction rules should run in the documented priority order: file references → symbol references → modification intent → creation intent → git context → continuation. Earlier signals provide context for later ones (e.g., modification intent checks if a target file/symbol was already found).
- The stopword set should be a `map[string]struct{}` for O(1) lookup. Start with the most common false positives and expand during tuning.
- The analyzer does NOT produce `SemanticQueries` — that's the job of the Query Extractor (Epic 03). The analyzer produces raw signals; the query extractor translates `ContextNeeds` into search queries.
- The `recentHistory` parameter is used for momentum extraction (continuation detection). If momentum extraction is complex enough, it may live in Epic 03. The boundary: if the turn analyzer needs to scan tool call arguments from recent messages to compute MomentumFiles, that scanning logic can live here or in Epic 03. Pick whichever keeps the epic cohesive.
- This code is the primary tuning surface for context assembly quality. The Signal trace must be thorough enough to debug "why didn't the analyzer detect X?" by examining the trace.
