# Task 10: Context Assembly Types (TurnAnalyzer, ContextNeeds, Signal)

**Epic:** 01 — Types and Interfaces
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Define the types and interfaces used by the context assembly layer (doc 06). The `TurnAnalyzer` interface examines user messages and conversation history to determine what codebase context is needed. Its output, `ContextNeeds`, is a structured specification of retrieval targets — not a boolean "needs context / doesn't" but a full description of what to fetch. The `Signal` struct provides observability into every decision the analyzer makes. These types live in `internal/rag/` because they are consumed by both the context assembly layer and the RAG pipeline.

## Acceptance Criteria

- [ ] `Message` struct defined in `internal/rag/types.go` with the following fields (minimal representation for the analyzer's input — not the full persistence model):
  - `Role    string` — `"user"`, `"assistant"`, or `"tool"`
  - `Content string` — message text content
- [ ] `Signal` struct defined in `internal/rag/types.go` with the following fields:
  - `Type   string` — signal category: `"file_ref"`, `"symbol_ref"`, `"intent_modify"`, `"intent_create"`, `"git_context"`, `"continuation"`, `"momentum"`, `"keyword"`
  - `Source string` — the literal text from the user message that triggered this signal
  - `Value  string` — the extracted value (e.g., the file path, symbol name, or keyword)
- [ ] `ContextNeeds` struct defined in `internal/rag/types.go` with the following fields:
  - `SemanticQueries    []string`  — search queries derived from the user message (max 3)
  - `ExplicitFiles      []string`  — file paths explicitly mentioned by the user (fetched directly, not searched)
  - `ExplicitSymbols    []string`  — symbol names identified for structural graph lookup (blast radius)
  - `IncludeConventions bool`      — whether to load cached project conventions (true when creation intent detected)
  - `IncludeGitContext  bool`      — whether to include recent git history (true when git-related keywords detected)
  - `GitContextDepth    int`       — number of recent commits to include (e.g., 5 for "recent changes", 20 for "what changed this week")
  - `MomentumFiles      []string`  — file paths from recent tool calls in the last 2 turns
  - `MomentumModule     string`    — longest common directory prefix among momentum files (e.g., `"internal/auth"`)
  - `Signals            []Signal`  — all signals detected during analysis (observability hook)
- [ ] `TurnAnalyzer` interface defined in `internal/rag/interfaces.go` with the following method:
  ```go
  type TurnAnalyzer interface {
      // AnalyzeTurn examines the user's message and recent conversation history
      // to determine what codebase context is needed for this turn.
      //
      // message is the current user message text.
      // recentHistory is the last N messages from the conversation (typically
      // the last 2 turns worth of messages, for momentum extraction).
      //
      // Always returns a non-nil *ContextNeeds. If nothing is detected,
      // returns a ContextNeeds with empty/zero fields (the retrieval layer
      // handles this by running no queries).
      AnalyzeTurn(message string, recentHistory []Message) *ContextNeeds
  }
  ```
  > **Note:** `AnalyzeTurn` intentionally returns no error because the rule-based analyzer is infallible by design — it uses only regex and heuristics (<5ms). Nil or empty input produces a zero-valued `ContextNeeds`.
- [ ] File compiles cleanly: `go build ./internal/rag/...`
