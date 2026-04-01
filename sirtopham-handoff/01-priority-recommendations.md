# Priority recommendations for SirTopham

This document answers a practical question: which Claude Code ideas are worth turning into Go code now, and which should remain reference material until later?

## Recommendation summary

Do not do a broad translation pass.

Do a selective Go-native implementation pass focused on deterministic infrastructure that cleanly maps onto SirTopham's existing architecture.

## Highest-value items to implement or stub now

### 1. Tool output budgeting and persisted-output references

Why it is high value:
- `cc-analysis.md` identifies this as the strongest area of Claude Code differentiation.
- SirTopham already has per-tool truncation and two-phase compression thinking.
- The biggest delta is aggregate budgeting across all tool results in a merged message, not just per-tool truncation.

What to carry over conceptually:
- Per-tool result limits still exist.
- A second pass enforces an aggregate budget for all results visible to the model in the next request.
- Oversized outputs are persisted outside the prompt and replaced with a preview + stable reference.
- Tail-preserving formatting is used for shell/build/test output.
- File-read output may deserve special treatment rather than blind persistence.

Minimum worthwhile deliverable:
- a `ToolOutputManager` that accepts multiple tool results and returns model-visible replacements plus persistence metadata.

### 2. File-edit safety invariants

Why it is high value:
- It is deterministic, testable, and language-agnostic.
- It directly strengthens one of SirTopham's core mutation tools.
- Claude Code's mature behavior here mostly refines SirTopham's existing exact-match design.

What to carry over conceptually:
- full-read-before-edit, not just visibility of the file
- stale-write detection tied to prior read state
- second stale-write check immediately before write
- narrow file-creation-via-edit behavior
- deterministic self-correcting error messages

Minimum worthwhile deliverable:
- a `ReadStateStore`, `PreconditionChecker`, and `Editor` interface boundary
- explicit error codes for not-read, stale-write, zero-match, multi-match, and invalid-create

### 3. Cancellation cleanup and transcript invariants

Why it is high value:
- SirTopham already has iteration-atomic persistence and process-group cancellation.
- The Claude Code analysis highlights a missing nuance: cleaning up partially streamed assistant/tool state while leaving the transcript coherent.

What to carry over conceptually:
- incomplete assistant messages should not remain as if complete
- partially started tool calls may need synthesized result placeholders or tombstones
- "user interrupted and moved on" is meaningfully different from generic cancellation
- durable transcript state can stay simple if live stream state is cleaned up carefully

Minimum worthwhile deliverable:
- a `TurnCleanup` component that finalizes or tombstones in-flight assistant/tool records during cancellation

### 4. Prompt cache stability and latching

Why it is high value:
- SirTopham already has a block-based prompt caching design.
- The analysis shows Claude Code treats byte stability as a first-class architectural constraint.

What to carry over conceptually:
- stable vs dynamic sections should be explicit in code
- cache-relevant request parameters should latch per session or per turn where needed
- tool definitions are cache-key-sensitive and should remain stable once announced
- forks/retries/subagents should inherit identical stable bytes when intended

Minimum worthwhile deliverable:
- an explicit prompt block renderer and a cache-latch state object

### 5. Better token-budget accounting

Why it is medium-high value:
- The analysis suggests SirTopham's `chars / 4` heuristic is serviceable but not enough as the primary mechanism.
- This area is important, but it can come after tool-output persistence because the latter reduces pressure immediately.

What to carry over conceptually:
- reserve explicit response budget
- track estimated prompt cost before request
- reconcile with actual usage after request
- react to context overflow with targeted fallback paths

Minimum worthwhile deliverable:
- a `BudgetTracker` that combines rough estimation, explicit reserve, and post-response usage reconciliation

## Lower-priority items that should remain design inputs for now

### System prompt wording
Adapt the behavior, not the exact text. The useful takeaways are:
- verification before declaring success
- read files before proposing edits
- use dedicated tools before shell equivalents
- provide short progress updates

### Prompt-cache editing tricks
Claude Code's cache-edit / cache-reference mechanics are clever, but this is an optimization layer. SirTopham should first prove a simpler persisted-output and prompt-block model.

### Streaming transport details
Claude Code supports SSE and WebSocket, but SirTopham already has a strong WebSocket event model. The architectural lesson is to separate durable transcript semantics from live transport semantics.

### Permission/classifier system
Interesting, but likely not the best early investment for a personal single-user tool.

### LLM-based tool-use summarization
Useful later. Not a foundational first slice.

## Recommended first implementation slice

If only one slice is implemented now, it should be:

### Tool output manager slice

Scope:
- per-tool max budget
- aggregate visible-output budget
- persisted-output references with preview text
- tail-preserving formatter for shell/build/test output
- deterministic replacement memoization by tool-call ID

Why this slice first:
- highest leverage from the analysis
- low dependence on UI changes
- improves long sessions, large logs, and cache behavior immediately
- produces infrastructure that later token-budget and compression work can build upon

## Recommended order after that

1. Tool output manager
2. File-edit hardening
3. Cancellation cleanup
4. Prompt-cache latching
5. Budget tracker refinement

## Explicit anti-goals

Do not:
- transliterate TypeScript modules one-for-one into Go packages
- replicate Claude Code's internal naming unless it fits SirTopham naturally
- build model-based summarization before deterministic budgeting and persistence are solid
- overfit the design to Anthropic-specific features that SirTopham may not need immediately

## Key direct takeaways from `cc-analysis.md`

The specific findings most worth carrying forward are:
- budget tool results individually and in aggregate
- persist oversized outputs with previews instead of only truncating
- make edit tools conservative with read-before-write and stale-write checks
- preserve transcript invariants on cancellation and fallback
- stabilize prompt bytes for cache reuse
- reserve response budget and use hybrid estimation + actual-usage accounting

## Bottom line

The right next step is not "port Claude Code to Go."

The right next step is "implement a few Go-native infrastructure seams that embody the most proven Claude Code patterns while staying faithful to SirTopham's architecture."
