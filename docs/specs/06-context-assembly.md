# 06 — Context Assembly

**Status:** Draft, v0.1 implemented and first v0.2 brain retrieval slice landed **Last Updated:** 2026-04-07 **Author:** Mitchell

---

## Overview

Context assembly is the implementation of sodoryard's core thesis: programmatic, task-specific, minimal context beats static context files. Every turn, before the first LLM call, the system examines the user's message and recent conversation history, retrieves relevant code and project knowledge via code RAG, explicit file reads, structural analysis, project-brain retrieval, conventions, and git context, and packages it within the available token budget. The project brain was intentionally reactive-only in v0.1; the first v0.2 slice now wires proactive keyword-backed brain retrieval into this layer, with broader semantic/graph brain work still deferred.

This is what makes sodoryard "understand" the codebase. Get this right and the agent feels like a senior engineer who's read every file. Get it wrong and it's just another chatbot.

This layer is entirely net-new. No prior art exists for per-turn RAG-driven context assembly in a conversational coding agent. Hermes Agent relies on static context files (AGENTS.md, SOUL.md) loaded once at session start. Claude Code uses CLAUDE.md. sodoryard takes a different approach — dynamic, per-turn retrieval that assembles exactly the context the current turn needs. Because it's novel, heavy instrumentation is built in from day one to measure whether it's working.

---

## Design Principles

**1. Always-on retrieval, never classification.** The system runs RAG queries every turn. If nothing relevant comes back above the relevance threshold, nothing gets injected. The cost of a false positive (an unnecessary embedding call + vector search, ~100ms, all local) is orders of magnitude cheaper than a false negative (the LLM burns an iteration using `search_semantic` reactively, costing seconds and thousands of tokens). There is no turn classifier, no "should we run RAG?" decision. The trigger problem is dissolved by making retrieval cheap enough to always run.

**2. Rule-based analysis, not LLM-assisted.** The turn analyzer uses regex and heuristics, not an LLM call. An LLM classification step would add hundreds of milliseconds and require its own context — turtles all the way down. Rules handle 70-80% of cases well; the remaining 20-30% degrade gracefully because the agent has `search_semantic` as a reactive fallback tool. The analyzer has a clean interface so it can be swapped to an LLM-assisted implementation later if rule quality proves insufficient.

**3. Frozen per turn for cache stability.** Assembled context fires once at the start of each turn and is immutable for all iterations within that turn. This enables Anthropic prompt caching — the assembled context block is an exact prefix match across iterations 2-N. If the agent needs additional codebase information mid-turn, it uses the `search_semantic` tool (reactive retrieval) rather than re-triggering assembly.

**4. Instrumented by default.** Every turn produces a `ContextAssemblyReport` that records what was analyzed, retrieved, filtered, included, and excluded. Quality metrics track whether the agent needed to reactively search for things that should have been proactively assembled. This is the feedback loop that makes a novel approach viable.

---

## Prompt Cache Layout

Anthropic's prompt caching is keyed on exact prefix matching. The cache layout is critical — every design decision about where context goes must be evaluated against "does this break prompt caching?"

### Three Cache Breakpoints

```
SYSTEM MESSAGE:
  ├─ [text block] Base system prompt              ← CACHE BREAKPOINT 1
  │   Agent personality, behavioral guidelines,
  │   tool usage instructions, project conventions.
  │   Stable across ALL turns in ALL sessions.
  │
  └─ [text block] Assembled context               ← CACHE BREAKPOINT 2
      RAG results, explicit file contents,
      structural graph results, git context.
      Frozen at turn start. Stable across all
      iterations within a turn. Changes between turns.

MESSAGES ARRAY:
  ├─ Previous turns (completed)                   ← CACHE BREAKPOINT 3
  │   All messages from prior turns in this session.
  │   Grows monotonically. Each new turn appends;
  │   existing prefix is unchanged.
  │
  └─ Current turn (fresh, uncached)
      User message, in-progress tool calls and
      results from earlier iterations in this turn.
```

### Cache Hit Behavior

|Block|Scope|Hit Rate|
|---|---|---|
|Block 1 (base prompt)|Across all turns and sessions|~100%|
|Block 2 (assembled context)|Across iterations within a single turn|High (iterations 2-N hit; 0% across turns)|
|Block 3 (history prefix)|Across iterations within a turn|High (prefix is stable; only fresh content appends)|

### Why Not User Message Injection?

Hermes/Honcho puts per-turn context on the user message instead of the system prompt, preserving system prompt caching across turns. We considered this but rejected it for sodoryard because:

- Assembled context is 10-30k tokens — large enough that within-turn caching across iterations provides significant latency savings.
- Stuffing 30k tokens of code into a user message is semantically incorrect. It's not something the user said; it's system-provided context. This matters for model behavior — the LLM treats system prompt content differently from user content.
- Hermes's approach makes sense when "context" is small (Honcho recall snippets) and unpredictable. Our context is large and frozen per-turn — exactly what cache breakpoints are designed for.

### Cache Invalidation

The only event that invalidates the prompt-cacheable history prefix is **conversation compression** (see Compression section). When compression fires, the conversation history changes shape, so the history breakpoint is invalidated. The assembled context (block 2) was built for the current turn and doesn't change. The base prompt (block 1) never changes.

Current implementation note: `PromptBuilder.BuildPrompt()` rebuilds the provider request each iteration and only emits explicit cache markers for Anthropic, gated by `agent.cache_system_prompt`, `agent.cache_assembled_context`, and `agent.cache_conversation_history`. There is no long-lived `_cached_system_prompt` byte buffer in code today.

---

## Component: Turn Analyzer

The turn analyzer examines the user's message and recent conversation history to produce a `ContextNeeds` struct — not a boolean "needs context / doesn't" but a specification of what to retrieve.

### Interface

```go
// AnalyzeTurn examines the message and recent history to determine
// what codebase context is needed for this turn.
type TurnAnalyzer interface {
    AnalyzeTurn(message string, recentHistory []Message) *ContextNeeds
}

type ContextNeeds struct {
    // Semantic search queries derived from the message
    SemanticQueries   []string

    // Files explicitly mentioned by path
    ExplicitFiles     []string

    // Symbols identified for structural graph lookup
    ExplicitSymbols   []string

    // Category flags
    IncludeConventions bool
    IncludeGitContext   bool
    GitContextDepth    int

    // Momentum from previous turns
    MomentumFiles     []string
    MomentumModule    string    // common directory prefix

    // Observability: what triggered each decision
    Signals           []Signal
}

type Signal struct {
    Type   string // "file_ref", "symbol_ref", "intent_verb", "momentum", etc.
    Source string // the text that triggered this signal
    Value  string // the extracted value
}
```

The `Signals` field is the observability hook. Every decision the analyzer makes records why — what regex matched, what text triggered it, what value was extracted. This feeds the context inspector debug panel and enables systematic tuning.

### Signal Extraction Rules (v0.1)

The rule-based analyzer extracts signals in priority order:

**File references:** Regex for paths with file extensions (`internal/auth/middleware.go`, `./config.yaml`). Also matches Go-convention directory references without extensions (`internal/auth/`, `pkg/server`, `cmd/tidmouth`).

**Symbol references:** Backtick-wrapped identifiers (`` `ValidateToken` ``). PascalCase and camelCase words that aren't common English, checked against a small stopword set. Identifiers preceded by keywords: "function", "method", "type", "struct", "interface", "func".

**Modification intent:** Verbs — "fix", "refactor", "change", "update", "edit", "modify", "rewrite", "rename", "move", "delete", "remove". When these appear with an identified target (file or symbol), flag `ExplicitSymbols` for structural graph lookup (blast radius analysis).

**Creation intent:** Verbs — "write", "create", "add", "implement", "build", "make". Paired with structural nouns — "test", "endpoint", "handler", "middleware", "migration", "route", "model", "service". Flag `IncludeConventions = true`.

**Git context:** Keywords — "commit", "diff", "PR", "pull request", "merge", "branch", "recent changes", "what changed", "last push". Flag `IncludeGitContext = true` with appropriate depth.

**Continuation:** Signals — "continue", "keep going", "finish", "next", "also", "too", combined with the absence of strong new-topic signals. Flag momentum lookup.

### Replaceability

The `TurnAnalyzer` interface is the firewall. If rule-based analysis proves insufficient (context inspector shows consistently poor signal extraction), the implementation can be swapped to an LLM-assisted version — a local model call with a structured output prompt — without changing anything downstream. The interface, the `ContextNeeds` struct, and all consumers remain identical.

---

## Component: Query Extraction

Query extraction translates the turn analyzer's output into concrete search queries for the RAG pipeline. No LLM call — string processing only.

### Three-Source Strategy

**Source 1: Cleaned message.** Take the user's message, strip conversational filler ("hey", "can you", "please", "I think", "I want you to", "could you", "help me", "let's"), strip punctuation. Cap at ~50 words. This is the primary semantic query. For long messages with multiple sentences, split at sentence boundaries and take up to 2 queries.

**Source 2: Technical keyword extraction.** Pull out noun phrases that look like technical terms — words with underscores, camelCase, PascalCase, dot notation, HTTP methods (GET, POST), status codes (401, 500), programming domain terms (middleware, handler, router, schema, migration, query, endpoint). Join these into a supplementary query if they differ meaningfully from the cleaned message.

**Source 3: Momentum-enhanced query.** If the momentum module is active (e.g., `internal/auth`), prepend it to the cleaned message query. "Fix the tests" becomes "internal/auth fix the tests." This narrows the search space toward the module the conversation has been working in.

### Query Cap

Maximum 3 queries per turn. These feed into the searcher's multi-query expansion, which runs each with `topK=10`, deduplicates by chunk ID, and re-ranks by hit count (chunks matching multiple queries rank higher). This produces ~15-25 ranked chunks before relevance filtering and budget fitting.

### Explicit Entity Handling

File paths and symbol names extracted by the turn analyzer do NOT become RAG queries. They are handled separately:

- **Explicit files:** Direct file reads. Deterministic, not probabilistic.
- **Explicit symbols:** Structural graph lookups (blast radius). Also deterministic.

This separation ensures that when the user says "fix `internal/auth/middleware.go`", the file is fetched directly — not searched for semantically, which could return a different file with similar content.

---

## Component: Conversation Momentum

Momentum prevents "context amnesia" across turns — the problem where each turn's context assembly is treated in isolation, losing track of what the conversation is "about."

### Implementation (Minimal v0.1)

Scan the last 2 turns of conversation history. Extract file paths from tool calls:

- `file_read` calls: extract the path argument
- `file_write` / `file_edit` calls: extract the path argument
- `search_text` / `search_semantic` results: extract file paths from result content

Compute `MomentumModule` as the longest common directory prefix among these paths. If all recent tool activity was in `internal/auth/`, the momentum module is `internal/auth`.

`MomentumFiles` is the deduplicated list of file paths from these tool calls.

### How Momentum Is Used

1. **Weak-signal turns:** If the current message has no explicit file references and vague language ("now fix the tests"), momentum files become additional explicit file fetches. The momentum module narrows the third semantic query.
    
2. **Strong-signal turns:** If the current message has clear signals (explicit files, specific technical terms), momentum results get lower priority in budget fitting. They may not make it into the final context package. This naturally handles topic switches — if the user provides clear new signals, momentum yields.
    

### What Momentum Does NOT Do

- Pronoun resolution ("fix it" → "fix auth.go"). The analyzer doesn't try to resolve pronouns. It uses momentum as a probabilistic hint, not a definitive reference.
- Multi-turn topic tracking. Momentum looks at the last 2 turns only. It doesn't maintain a topic model across the session.
- Overriding user intent. If the user says "now let's work on routing", momentum from the auth module doesn't contaminate the routing context.

---

## Component: Retrieval Execution

After the turn analyzer produces `ContextNeeds` and query extraction builds the query set, retrieval runs in parallel. All retrieval is local — no external API calls, no network dependencies.

### Parallel Retrieval Paths

```
ContextNeeds
  ↓
┌─────────────────────────────────────────────────────────┐
│ Parallel execution (~100-200ms total, all local)        │
│                                                         │
│  ┌─ Semantic search ────────────────────────────────┐   │
│  │  Embed queries → LanceDB vector search →         │   │
│  │  dedup/re-rank → one-hop call graph expansion    │   │
│  └──────────────────────────────────────────────────┘   │
│                                                         │
│  ┌─ Explicit file reads ────────────────────────────┐   │
│  │  Read file contents for mentioned paths          │   │
│  └──────────────────────────────────────────────────┘   │
│                                                         │
│  ┌─ Structural graph ──────────────────────────────┐    │
│  │  Blast radius for identified symbols (if any)   │    │
│  └─────────────────────────────────────────────────┘    │
│                                                         │
│  ┌─ Project brain (v0.2) ──────────────────────────┐    │
│  │  Keyword retrieval via MCP/vault backend        │    │
│  │  Semantic/wikilink expansion remains future     │    │
│  └─────────────────────────────────────────────────┘    │
│                                                         │
│  ┌─ Convention cache ──────────────────────────────┐    │
│  │  Load cached conventions (if flagged)           │    │
│  └─────────────────────────────────────────────────┘    │
│                                                         │
│  ┌─ Git context ───────────────────────────────────┐    │
│  │  git log --oneline -N (if flagged)              │    │
│  └─────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────┘
  ↓
Relevance filtering → Budget fitting → Serialization
```

### Semantic Search Details

Uses the searcher from [[04-code-intelligence-and-rag]]:

1. **Embed queries** via the nomic-embed-code container (port 8081). Each query prefixed with the recommended retrieval prefix.
2. **Vector search** against LanceDB with `topK=10` per query.
3. **Deduplication** by chunk ID. Chunks matching multiple queries get higher rank (hit count re-ranking).
4. **One-hop call graph expansion** for the top hits: look up functions each hit calls and functions that call each hit. Budget allocation: 60% for direct vector hits, 40% for dependency hops.

### Relevance Filtering

After retrieval, apply a cosine similarity threshold. Chunks below the threshold are discarded entirely — they don't compete for budget.

**Default threshold: 0.35.** This is a starting guess that requires empirical tuning. The context inspector debug panel is the primary mechanism for tuning — observe what's retrieved, what passes the threshold, and whether the right code is getting through.

If no chunks pass the threshold for a turn, nothing is injected. The agent operates with no assembled context and uses tools reactively if it needs codebase information. This is the correct behavior for turns like "What's the Go syntax for a goroutine?"

---

## Component: Budget Manager

The context window is finite. The budget manager allocates tokens across all blocks and enforces a hard cap on assembled context.

### Token Accounting

```
Total budget = model context limit (e.g., 200k for Sonnet, 1M for Opus)

Reserved (non-negotiable):
  System prompt block 1:      ~3k tokens (measured, stable)
  Tool schemas:                ~3k tokens (measured, stable)
  Response headroom:           ~16k tokens (max_tokens setting)

Measured (from current state):
  Conversation history:        measured from persisted messages

Available for assembled context:
  = Total - Reserved - History - Response headroom

Hard cap on assembled context:
  = min(available, MAX_CONTEXT_BUDGET)
```

### MAX_CONTEXT_BUDGET

Default: **30,000 tokens.** Roughly 20-25 complete functions with descriptions.

This is a design parameter, not a hard truth. Too small and the agent constantly reaches for `search_semantic` to supplement. Too large and attention quality degrades — the LLM pays less attention to content in the middle of very long contexts. The context inspector's `AgentUsedSearchTool` metric tells you if it's too small. Observation of response quality tells you if it's too large.

### Priority Allocation

When the budget is tight, allocate in priority order:

1. **Explicit files** (user mentioned them directly — highest signal)
2. **Proactive brain hits** (project knowledge from the MCP/vault backend)
3. **Top RAG code hits** (above threshold, de-duped, re-ranked by hit count)
4. **Structural graph results** (callers/callees of identified symbols)
5. **Conventions** (derived from code analysis — when writing new code)
6. **Git context** (recent commits — minimal, just one-line summaries)
7. **Lower-ranked RAG code hits** (fill remaining budget)

Current implementation note: proactive brain hits now serialize as a first-class Project Brain section, contribute to `budget_breakdown.brain`, and operational brain log notes like `_log.md` are filtered so they do not outrank real knowledge notes. For turns with an explicit brain intent signal (for example prompts that directly ask about the project brain), context assembly can now prefer brain retrieval and skip generic code semantic search when there are no explicit code references to chase.

If the budget runs out mid-priority-3, that's fine — the agent has `search_semantic` as a tool to fetch anything else it needs. The budget manager's job is to front-load the highest-signal context, not to be comprehensive.

### History Compression Trigger

The budget manager monitors conversation history growth. When history exceeds **50%** of the total context window, it signals the agent loop that compression is needed. The budget manager does not perform compression — it sets a flag that the agent loop checks. See the Compression section for details.

---

## Component: Context Serialization

The assembled context is serialized into a markdown-formatted text block and placed as the second text block in the system message (cache block 2).

### Format

````markdown
## Project Knowledge

### auth-architecture.md
The auth system uses JWT tokens validated by middleware. Token refresh is
handled by the AuthService, not the middleware. The middleware only validates —
it never issues or refreshes tokens. This separation exists because the refresh
flow requires database access that the middleware layer shouldn't have.

Related: [[provider-design]], [[error-handling]]

## Relevant Code

### internal/auth/middleware.go (lines 15-48)
Authentication middleware that validates JWT tokens on incoming requests.

```go
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := r.Header.Get("Authorization")
        // ... full function body ...
    })
}
`` `

### internal/auth/service.go (lines 22-45)
Validates a JWT token and returns the associated user claims.

```go
func (s *AuthService) ValidateToken(token string) (*Claims, error) {
    // ... full function body ...
}
`` `

### internal/auth/service.go (lines 47-63) [previously viewed in turn 2]
Generates a new JWT token for an authenticated user.

```go
func (s *AuthService) GenerateToken(user *User) (string, error) {
    // ...
}
`` `

## Project Conventions

- Tests use the `_test.go` suffix and table-driven test patterns
- Error handling follows `if err != nil { return fmt.Errorf("context: %w", err) }`
- HTTP handlers are in `handler.go`, business logic in `service.go`

## Recent Changes (last 3 commits)

- abc1234: Fix token expiry check in AuthMiddleware
- def5678: Add rate limiting to API gateway
- 789abcd: Update Go dependencies
````

### Format Principles

**Current runtime includes proactive brain content.** Project-brain documents can now be serialized into assembled context through the MCP/vault-backed proactive keyword retrieval path. The current operator truth is intentionally narrow: brain hits are keyword-backed, `_log.md` operational notes are filtered, and broader semantic/index-backed brain retrieval is still future work unless it is explicitly landed.

**File path + line range as headers.** The LLM needs exact paths to make correct tool calls later (`file_read`, `file_edit` require paths and line ranges).

**Description before code.** The 1-2 sentence description from the RAG pipeline precedes the code block. The LLM can quickly scan what each chunk is about without reading every line of code.

**Grouped by file.** When multiple chunks come from the same file, group them under file-level organization. Reduces repetition and communicates file structure.

**Markdown with language-tagged code fences.** The LLM understands markdown well. Language tags (`go,` typescript) activate the model's code understanding.

**Previously-viewed annotation.** Chunks from files that appeared in tool results during the current conversation are annotated with `[previously viewed in turn N]`. This tells the LLM "you've seen this before, it's here for reference, not because it's new." No deduplication logic needed — just a cheap signal.

**Conventions as a short list.** 5-10 bullet points, not a style guide. The LLM infers patterns from the code chunks; conventions reinforce key ones explicitly.

**Git context is minimal.** Just commit hashes and one-line messages. The agent uses `git_diff` if it needs actual changes.

### What Is NOT Included

- Full file contents (unless explicitly requested — use code chunks instead)
- File tree (the agent can use tools to explore; a full tree wastes tokens)
- Build/test instructions (these belong in the base system prompt block 1, not assembled context)
- Content identical to what's in conversation history (but see the annotation approach above — minimal deduplication, not full dedup)

---

## Interaction with Conversation History

### The Pragmatic Approach: Don't Over-Optimize Deduplication

The 1M context window and subscription-based access (no per-token cost) mean the penalty for some redundancy is zero dollars and minimal quality impact. Context assembly and conversation history serve different purposes:

- **Conversation history** contains what the LLM _requested and received_ via tools — reactive, specific to what was asked.
- **Assembled context** contains what the system _proactively provides_ — it may include related code the LLM hasn't seen, structural relationships, and convention patterns.

Trying to perfectly deduplicate between these two sources adds complexity for minimal gain. A function that appeared in a tool result 3 turns ago may have been edited since then. Assembled context always reflects the _current_ state of the file.

### The Annotation Approach

Track a `seenFiles` set for the session — files that appeared in tool results during the current conversation. When assembled context includes a chunk from a seen file, annotate it with the `[previously viewed in turn N]` marker. This tells the LLM the context is for reference; it doesn't require expensive deduplication logic.

### Post-Compression Behavior

When conversation history gets compressed (older turns summarized), the LLM loses detailed access to files it read earlier. In this case, context assembly's proactive inclusion becomes critical — it's the only way those files get back into context. Post-compression, the assembled context may be more important than pre-compression, which reinforces the decision not to suppress it based on conversation history.

---

## Compression

Context compression is adopted from Hermes Agent's proven implementation, with adaptations for sodoryard's architecture.

### Trigger

Two-phase checking, matching Hermes:

**Preflight (rough estimate):** Before each API call, estimate total message tokens as `total_chars / 4`. If this exceeds the compression threshold, compress before sending. This is cheap (~0ms) and catches obvious cases.

**Post-response (exact):** After each API call, record the actual `prompt_tokens` from the API usage response. If this exceeds the compression threshold, compress before the next iteration. This catches edge cases where the rough estimate was too low.

**Reactive (error recovery):** If the API returns HTTP 413 (Entity Too Large) or HTTP 400 with "context_length_exceeded", compress immediately and retry once.

### Threshold

Default: **50% of the model's context window.** For a 200k context model, compression triggers at 100k prompt tokens. Configurable.

### Algorithm: Head-Tail Preservation

1. **Protect the first 3 messages** (system prompt context, initial user message, first assistant response — foundational context).
2. **Protect the last 4 messages** (most recent turns — the active working context).
3. **Extract the middle turns** (everything between head and tail).
4. **Summarize** the middle using a fast auxiliary model (local Qwen/DeepSeek via Docker, or configurable). The summary prompt instructs the model to create a concise structured summary preserving key decisions, file paths mentioned, and work completed.
5. **Insert the summary** as a single user message with `[CONTEXT COMPACTION]` prefix, replacing the middle turns.
6. **Sanitize tool call pairs.** Scan for orphaned assistant messages with `tool_calls` that no longer have corresponding `tool` result messages. Remove the `tool_calls` field from these messages. This prevents API rejections from malformed conversation structure.

### Fallback

If auxiliary model summarization fails (model unavailable, API error, timeout), truncate the middle without a summary rather than failing the entire conversation. The conversation continues with less context but doesn't crash. Log the failure for debugging.

### Cache Invalidation After Compression

After compression, the conversation history has changed shape:

- Cache block 1 (base prompt): unaffected.
- Cache block 2 (assembled context): unaffected — frozen for the current turn.
- Cache block 3 (history prefix): **invalidated** — the compressed history is a different prefix.

In the current implementation, rebuild the provider request on the next iteration so the history-prefix cache breakpoint reflects the compressed history shape.

---

## Component: Context Assembly Report

The observability system that makes this novel approach viable. Every turn produces a report.

### Structure

```go
type ContextAssemblyReport struct {
    TurnNumber          int
    AnalysisLatencyMs   int64     // time spent in turn analyzer
    RetrievalLatencyMs  int64     // time spent in parallel retrieval
    TotalLatencyMs      int64     // total assembly time

    // What the analyzer decided
    Needs               ContextNeeds

    // What retrieval returned (pre-filtering)
    RAGResults          []RAGHit       // chunk ID, file, similarity score
    BrainResults        []BrainHit     // reserved for v0.2 proactive brain retrieval; empty in v0.1
    ExplicitFileResults []FileResult   // file path, size in tokens, truncated?
    GraphResults        []GraphHit     // symbol, relationship type, depth

    // What survived budget fitting (post-filtering)
    IncludedChunks      []string       // chunk IDs in the final context
    ExcludedChunks      []string       // chunk IDs cut for budget or threshold
    ExclusionReasons    map[string]string // chunk ID → "below_threshold" | "budget_exceeded"

    // Budget accounting
    BudgetTotal         int            // tokens available for context
    BudgetUsed          int            // tokens consumed
    BudgetBreakdown     map[string]int // "rag": 15000, "conventions": 2500, etc. (`brain` reserved for v0.2 if added)

    // Quality signals (computed after the turn completes)
    AgentUsedSearchTool bool           // did the agent use search_semantic this turn?
    AgentReadFiles      []string       // files the agent read via tool calls
    ContextHitRate      float64        // % of agent-read files in assembled context
}
```

### Quality Metrics

Three metrics measure whether context assembly is doing its job:

**AgentUsedSearchTool:** If `true`, the assembled context was insufficient — the agent had to reactively search for more. High frequency of this across turns means the query extraction or relevance threshold needs tuning. Track as a percentage over a sliding window of recent turns.

**ContextHitRate:** Overlap between proactively assembled context and what the agent actually needed. If you assemble 15 code chunks and the agent reads 3 files via tool calls, and 2 of those 3 were in the assembled context, the hit rate is 67%. Track over time — rising hit rate means tuning is working.

**ExcludedChunks cross-referenced with AgentReadFiles:** If the agent read a file via tool call that was in `ExcludedChunks` (cut for budget), the budget is too small or the priority ranking is wrong. This is the most actionable signal — it tells you exactly what was missed and why.

### Storage

Reports are stored per-turn in SQLite in `context_reports`. The web UI's context inspector reads the full report from `GET /api/metrics/conversation/:id/context/:turn`, and a narrower ordered signal-flow view is available at `GET /api/metrics/conversation/:id/context/:turn/signals`. On historical loads, the inspector fetches both; on live turns, it can also hydrate from the `context_debug` WebSocket event.

### Context Inspector Debug Panel

The web UI displays the assembly report for each turn:

- Which RAG chunks were retrieved, with similarity scores
- Which chunks made it past the relevance threshold
- Which chunks were included vs excluded by budget fitting, with reasons
- Token budget allocation breakdown
- Turn analyzer signals (what was detected, what was missed)
- Ordered signal flow, including semantic queries and key flags when available
- Quality metrics (search tool usage, hit rate)

If report loading fails, the inspector should surface that failure explicitly rather than falling back to an ambiguous empty state.

This panel is not a nice-to-have. It is the primary mechanism for tuning the relevance threshold, budget cap, and query extraction rules. For a system with no prior art to benchmark against, the debug panel IS the benchmark.

---

## The Full Assembly Flow

Step by step, from user message to frozen context package:

```
1. User message arrives via WebSocket
   ↓
2. Turn Analyzer (rule-based, <5ms)
   - Extract file path references (regex)
   - Extract symbol names (heuristic)
   - Detect intent signals (modification, creation, git)
   - Check conversation momentum (last 2 turns)
   - Produce ContextNeeds struct with Signal trace
   ↓
3. Query Extraction (<1ms)
   - Clean message (strip filler, cap length)
   - Extract technical keywords
   - Build momentum-enhanced query (if active)
   - Produce 1-3 semantic queries
   ↓
4. Parallel Retrieval (~100-200ms total, all local)
   ├─ Semantic code search: embed → LanceDB → dedup/re-rank → hop expansion
   ├─ Explicit file reads: direct reads for mentioned paths
   ├─ Structural graph: blast radius for identified symbols
   ├─ Convention cache: load if flagged
   └─ Git context: git log if flagged
   ↓
5. Relevance Filtering
   - Apply similarity threshold (default 0.35) to code RAG results
   - Discard chunks below threshold
   - Merge RAG + structural graph results (dedup by ID)
   ↓
6. Budget Fitting
   - Measure token cost of each piece
   - Fill by priority: explicit files → top RAG → structural → conventions → git → lower RAG
   - Truncate at MAX_CONTEXT_BUDGET (default 30k tokens)
   ↓
7. Serialization
   - Format into markdown structure (see Context Serialization)
   - Annotate chunks from previously-seen files
   ↓
8. Freeze
   - FullContextPackage is immutable for the rest of this turn
   - Injected as system prompt block 2 (cache breakpoint 2)
   - ContextAssemblyReport written to SQLite
   ↓
9. Agent loop proceeds to LLM call (step 4 of agent loop)
```

Total wall-clock time for steps 2-8: **~200-300ms.** All local — embedding call to Docker container, vector search against LanceDB, file reads, graph queries. No LLM calls, no external network calls. The user perceives zero delay — the WebSocket shows a "thinking" status for the LLM call that follows, not for context assembly.

---

## Configuration

```yaml
context:
  # Budget
  max_assembled_tokens: 30000         # Hard cap on injected context
  max_chunks: 25                      # Maximum code chunks from RAG
  max_explicit_files: 5               # Maximum directly-fetched files
  convention_budget_tokens: 3000      # Token reservation for conventions
  git_context_budget_tokens: 2000     # Token reservation for git context

  # Quality
  relevance_threshold: 0.35           # Minimum cosine similarity for code RAG results
  structural_hop_depth: 1             # Call graph expansion depth
  structural_hop_budget: 10           # Max chunks from graph expansion
  momentum_lookback_turns: 2          # How many prior turns to scan for momentum

  # History management
  compression_threshold: 0.50         # Compress when history exceeds 50% of context window
  compression_head_preserve: 3        # Messages to protect at the start
  compression_tail_preserve: 4        # Messages to protect at the end
  compression_model: "local"          # Model for summarization (local Docker container)

  # Debug
  emit_context_debug: true            # Send context_debug events to web UI
  store_assembly_reports: true        # Persist reports to SQLite
```

---

## Dependencies

- [[04-code-intelligence-and-rag]] — Searcher (semantic search), structural graph (blast radius), convention extractor, embedding pipeline
- [[05-agent-loop]] — Consumes the FullContextPackage; fires context assembly at turn start; provides `search_semantic` tool as reactive fallback
- [[03-provider-architecture]] — Prompt caching API (cache_control markers), context window limits per model
- [[07-web-interface-and-streaming]] — Context inspector debug panel, `context_debug` WebSocket events
- [[08-data-model]] — ContextAssemblyReport storage, `seenFiles` tracking
- [[09-project-brain]] — Reactive brain tools in v0.1; proactive brain retrieval design for v0.2

---

## What Ports from topham

**Concepts, not code:**

- **Assembler pattern:** topham's `Assembler → FullContextPackage` concept carries forward, but the implementation is entirely different. topham assembled context for pipeline phases with structured work orders. sodoryard assembles for conversational turns with free-form messages.
- **RAG query construction:** topham's multi-query expansion and dependency hop patterns are preserved in the searcher layer ([[04-code-intelligence-and-rag]]). Context assembly calls into the same searcher.
- **Convention extraction:** topham's convention cache is reused directly.

**Net-new:**

- Turn analyzer (rule-based signal extraction)
- Query extraction from conversational messages
- Conversation momentum tracking
- Context budget manager
- Relevance threshold filtering
- Context serialization format
- Assembly observability (ContextAssemblyReport)
- Integration with Anthropic prompt caching layout
- Reserved report/data-model hooks for future project-brain retrieval integration ([[09-project-brain]])
- Compression (adapted from Hermes)

---

## Tuning Guide

Parameters most likely to need adjustment, in order of impact:

**1. relevance_threshold (0.35).** Start here. If the context inspector shows relevant code being filtered out, lower the threshold. If it shows noise (irrelevant chunks consuming budget), raise it. This is the single most impactful parameter.

**2. max_assembled_tokens (30000).** If `AgentUsedSearchTool` is frequently true and `ExcludedChunks` contains files the agent later read, increase the budget. If the LLM seems to ignore assembled context or gives unfocused responses, decrease it.

**3. Query extraction rules.** If the context inspector shows the right code exists in the index but wasn't retrieved, the queries are wrong. Review both the raw Signals section and the ordered Signal Flow view to see what the analyzer detected and what semantic queries actually drove retrieval. Add new regex patterns or keyword rules as needed.

**4. momentum_lookback_turns (2).** If context amnesia is a problem (the agent loses track of what it was working on), increase to 3. If momentum is injecting stale context from old turns, decrease to 1.

**5. compression_threshold (0.50).** If compression fires too often and loses important context, increase. If conversations hit context limits before compression fires, decrease.

---

## Open Questions

- **Anthropic cache_control with OAuth:** Need to verify that 3 cache breakpoints work identically with OAuth tokens as with API keys. The API docs describe cache_control for API-key users — confirm for subscription OAuth access.
- **Cross-language retrieval quality:** How well does the semantic search work when the query is in English but code is in Go/Python/TS? The description layer should bridge this, but worth testing against real queries.
- **Conventions staleness:** Conventions are cached and refreshed on reindex. If a convention changes mid-session (rare), the cached version is stale. Worth detecting? Probably not for v0.1.
- **Budget allocation ratios:** The current priority order (explicit files → proactive brain hits → top RAG → structural → conventions → git → lower-priority RAG overflow) is still a design guess. The context inspector should continue to validate whether that ordering produces better hit rates or needs adjustment.

---

## References

- ETH Zurich AGENTS.md paper: arXiv:2602.11988v1 (research on static context file effectiveness)
- MIT CSAIL RLM paper: arXiv:2512.24601v1 (decompose via code intelligence, not context stuffing)
- Hermes Agent prompt builder: `agent/prompt_builder.py` (static context file loading — what we're replacing)
- Hermes Agent context compressor: `agent/context_compressor.py` (compression algorithm we're adopting)
- Hermes Agent prompt caching: `agent/prompt_caching.py` (cache preservation patterns)
- Anthropic prompt caching: https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching
- topham assembler: `internal/rag/searcher.go` (multi-query expansion, dependency hops)
- topham conventions: `internal/rag/conventions.go` (convention extraction and caching)
- Project brain: [[09-project-brain]] (Obsidian-backed project knowledge base)