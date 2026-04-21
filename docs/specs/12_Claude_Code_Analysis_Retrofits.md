**Status:** Draft v0.1 **Last Updated:** 2026-03-31 **Author:** Mitchell **Source:** Exploratory analysis of Claude Code (TypeScript) codebase, March 2026

---

## Overview

This document captures findings from an architectural reconnaissance of the Claude Code (CC) codebase — Anthropic's shipping AI coding agent — and translates them into actionable retrofit items for sodoryard. These are not speculative improvements. Every item here reflects a battle-tested pattern from a production agent operating at scale.

The findings are organized into two tiers:

- **Design Changes** — items that modify or extend sodoryard's existing architecture. These require implementation work.
- **Design Validations** — items where CC's approach confirms sodoryard's existing design. No work required, but documented here so future sessions don't re-litigate settled questions.

Each design change is scoped as a discrete work item with enough context for an agent session to implement it without re-reading the full CC analysis.

**Prerequisite:** All items in this document assume a code audit has been completed against the current implementation. The audit determines the actual delta between what's built and what this document specifies. See [[#Audit-First Rationale]] at the end.

---

## Design Changes

### DC-01: Aggregate Tool-Result Budget

**Affects:** [[10___Tool_System]], [[05-agent-loop]] **Priority:** High **Category:** Tool output management

**The problem:** sodoryard budgets tool output per-tool (default 50k tokens per result). But when the model fires multiple tools in parallel — say, five `file_read` calls each returning 40k — the merged user message containing all five results hits 200k. The per-tool budget passed every individual check, but the aggregate blew out the context.

**What CC does:** A second budget layer (`MAX_TOOL_RESULTS_PER_MESSAGE_CHARS = 200_000`) enforces a cap on the total size of all tool results within a single merged API message. When the aggregate exceeds this, CC persists the largest fresh results to disk and replaces them with previews + references. Replacement decisions are memoized by `tool_use_id` to preserve prompt cache prefix identity across replay/resume paths.

**What sodoryard should do:**

1. Add a configurable `max_tool_results_per_message_tokens` (default: a sensible fraction of the context budget — CC uses 200k chars which is roughly 50k tokens).
2. After all tool results for a batch are collected but before they're appended to conversation history, measure the aggregate size.
3. If over budget, apply the existing per-tool truncation/compression pipeline to the largest results first, working down until the aggregate fits.
4. The budget check runs in the agent loop's tool dispatch path, after `Execute` returns but before message assembly.

**Implementation notes:**

- This is a new function in the tool dispatch flow, not a change to individual tool implementations.
- The priority ordering for which results to compress first: largest results first, with `file_read` results deprioritized for compression (compressing a file the model just asked to read is counterproductive — CC explicitly opts `file_read` out of its persistence-replacement path by setting `maxResultSizeChars: Infinity`).
- Memoization of replacement decisions by `tool_use_id` is a cache stability optimization. Worth implementing if prompt caching is working well; safe to defer if not.

---

### DC-02: Hybrid Token Counting

**Affects:** [[06-context-assembly]], [[05-agent-loop]] **Priority:** High **Category:** Context window management

**The problem:** sodoryard uses `chars / 4` as a rough estimate for compression preflight checks. This is cheap but imprecise, and the imprecision matters most at the compression threshold boundary — where a bad estimate means either unnecessary compression (quality loss) or a context overflow error (wasted iteration + retry).

**What CC does:** A hybrid approach. After every API response, CC records the actual `prompt_tokens` from the usage object. For the next preflight check, it uses that real count as the baseline and only estimates the delta (messages added since the last response). The estimation is still rough for the delta, but the baseline — which is the vast majority of the token count — is exact.

**What sodoryard should do:**

1. After each API response, store `prompt_tokens` from the usage object alongside a marker identifying which messages were included in that count (e.g., the message sequence number of the last message in the request).
2. For compression preflight, compute: `last_known_prompt_tokens + estimate_tokens(messages_since_last_response)`.
3. Keep `chars / 4` as the estimator for the delta portion — it only needs to be accurate for a small number of new messages, not the entire history.
4. On the first iteration of a turn (no prior API response in this turn), fall back to pure `chars / 4` for the full history. This is the least accurate case but also the rarest case where compression fires (history hasn't grown within the turn yet).

**Implementation notes:**

- The `sub_calls` table already records `tokens_in` per API call. The new requirement is making the most recent value easily accessible to the compression preflight check — likely a field on the turn state or conversation manager, not a database query.
- The post-response exact check (using actual `prompt_tokens` from the response that just came back) remains unchanged. This hybrid approach only improves the _preflight_ estimate.
- CC does not ship a local tokenizer as its primary counting mechanism. The hybrid reuse-real-usage approach is sufficient.

---

### DC-03: Full-Read-Before-Edit Invariant

**Affects:** [[10___Tool_System]] (`file_edit` tool) **Priority:** High **Category:** File edit safety

**The problem:** Without a read gate, the model can confidently issue a `file_edit` on a file it has never read in the current session. It "knows" what the file contains from training data or from assembled context snippets, but those may be stale, incomplete, or hallucinated. The `old_str` match might succeed by coincidence while the surrounding context the model assumed is wrong.

**What CC does:** `file_edit` requires a prior full read of the target file via `readFileState`. Partial reads (line ranges) are explicitly insufficient. If the model hasn't read the file, the edit is rejected with: "Read it first before writing to it."

**What sodoryard should do:**

1. Track a `fullReadsThisSession` set (or map of `filePath → lastReadSequence`) on the session/conversation state.
2. When `file_read` executes without a line range (full file read), record the file path in this set.
3. When `file_edit` executes, check that the target file is in the set. If not, return an error: `"Error: file '{path}' has not been fully read in this session. Read it first with file_read before editing.\nThis prevents edits based on stale or assumed content."`
4. Historical note: the landed runtime is stricter than this draft. `file_write` may create a new file without a prior read, but overwriting a non-empty existing file requires a fresh prior full read in the current session.

**Implementation notes:**

- This is a validation check in `file_edit`, not a change to the tool interface or schema.
- The tracking state lives on the session/conversation, not persisted to SQLite — it resets on session start, which is correct (a new session should require fresh reads).
- Assembled context (RAG chunks) do NOT count as a "read." Only explicit `file_read` tool results count. The model needs to have seen the actual current file content, not a cached snippet.

---

### DC-04: Stale-Write Detection

**Affects:** [[10___Tool_System]] (`file_edit`, `file_write`) **Priority:** Medium **Category:** File edit safety

**The problem:** The model issues a `shell` command that modifies a file (e.g., `sed`, a build tool that generates code, a formatter). In a later iteration, it issues `file_edit` on the same file. The edit tool's knowledge of the file (from an earlier `file_read`) is now stale. The `old_string` might still match, but the edit is based on an outdated view of the file.

**What CC does:** Two-phase stale-write detection. At validation time, compare the file's current mtime against the mtime recorded when the model last read it. If they differ, the file was modified externally. CC checks this again immediately before the actual write (inside the critical section) to catch races.

**What sodoryard should do:**

1. When `file_read` executes, record `filePath → mtime` in the session state (extend the tracking from DC-03).
2. When `file_edit` or `file_write` (overwrite case) executes: a. **Preflight check:** Compare current mtime to recorded mtime. If different, return an error: `"Error: file '{path}' has been modified since you last read it (likely by a shell command or external process). Read it again with file_read before editing."` b. **Pre-write check:** Immediately before the atomic write, check mtime again. If it changed between preflight and write, return the same error.
3. Historical note: the landed runtime does not keep treating a just-written file as freshly trusted. After a successful `file_edit` or overwrite-style `file_write`, the stored snapshot/read state is cleared so a later mutation must re-read the file first.

**Implementation notes:**

- The double-check (preflight + pre-write) matters because the model might issue a `shell` command and a `file_edit` in the same batch. Mutating tools execute sequentially (per doc 05), so the shell runs first and changes the file. The preflight check on the subsequent `file_edit` catches this.
- Go's `os.Stat().ModTime()` is the mtime source. File systems that don't update mtime reliably (some networked mounts) could cause false positives, but this is a local development tool targeting local filesystems.

---

### DC-05: Synthesized Tool Results on Cancellation

**Affects:** [[05-agent-loop]] **Priority:** Medium **Category:** Cancellation safety

**The problem:** sodoryard's current cancellation design deletes the incomplete iteration's messages from SQLite. This is clean for persistence but leaves a gap: if you ever need to resume, fork, or replay from a turn where cancellation happened mid-tool-execution, orphaned `tool_use` blocks (from the assistant message) without matching `tool_result` blocks will cause API rejections.

**What CC does:** On cancellation, CC synthesizes missing `tool_result` blocks for any `tool_use` that was in-flight. The synthesized results contain an error/cancellation marker. This keeps the transcript's tool_use/tool_result pairing structurally valid at all times.

**What sodoryard should do:**

1. Before deleting an incomplete iteration's messages, check if the assistant message for that iteration contained `tool_use` blocks.
2. If so, for each `tool_use` that lacks a corresponding `tool_result`, synthesize a result: `"[Tool execution cancelled by user]"` with `is_error: true`.
3. Persist the assistant message AND the synthesized tool results, then mark the iteration as cancelled (not deleted).
4. The existing `DELETE FROM messages WHERE conversation_id = ? AND turn_number = ? AND iteration = ?` query changes to an update that marks messages as cancelled rather than removing them — or the synthesized results are inserted and the iteration is kept.

**Implementation notes:**

- This is a change to the cancellation path in the agent loop, not to individual tools.
- The simplest implementation: instead of deleting the iteration, persist it with synthesized results and a `cancelled = 1` flag. The history reconstruction query (`WHERE is_compressed = 0`) adds `AND cancelled = 0` to exclude cancelled iterations from normal replay, but they remain available for debugging and structural integrity.
- If conversation branching/forking is implemented later, this becomes critical — forks from a cancelled point need valid transcript structure.
- Consider adding a `cancelled` column to the messages table schema. Alternatively, use `is_compressed = 1` to hide cancelled iterations and accept the semantic overload.

---

### DC-06: Explicit Behavioral Instructions in System Prompt

**Affects:** System prompt (Block 1) **Priority:** Medium **Category:** Prompt engineering

**The problem:** sodoryard's system prompt content hasn't been fully specified yet (it's part of Block 1, which is "base personality + tool instructions"). CC's exploration reveals that the specific behavioral instructions in the system prompt have an outsized effect on agent convergence and quality.

**What CC encodes in its system prompt that sodoryard should consider:**

1. **Verification requirement:** "Before reporting a task complete, run the test / execute the script / check the output. If verification is impossible, say so explicitly." This is a convergence accelerator — the model doesn't declare victory prematurely.
2. **Read-before-modify:** "Do not propose code changes to files that have not been read first." Reinforces DC-03 at the prompt level (belt and suspenders).
3. **Tool preference hierarchy:** "Use dedicated read/edit/write/search tools instead of shell equivalents. Reserve shell for true system operations." Prevents the model from doing `cat file.go` instead of `file_read` or `sed -i` instead of `file_edit`.
4. **Parallel call encouragement:** "Parallelize independent tool calls aggressively." The model won't parallelize unless told to.
5. **Communication cadence:** "Before your first tool call, briefly state what you're about to do. Give short updates at key moments." This improves UX — the user sees intent before action.
6. **Anti-pattern bans:** Explicit instructions not to retry denied tool calls, not to blindly trust tool output (treat as potentially prompt-injection-capable).

**What sodoryard should do:**

- When writing the Block 1 system prompt, include concrete behavioral rules rather than vague role framing. "You are a senior engineer" is less useful than "Verify your work before reporting completion."
- These don't need their own architecture doc — they're prompt content, not system design. But they should be written deliberately, not improvised during implementation.

**Implementation notes:**

- This is a writing task, not a coding task. The system prompt is a string constant (or template) in the codebase.
- CC's prompt is not public, so these are principles extracted from the exploration, not text to copy.

---

### DC-07: Error Messages as Teaching Mechanisms

**Affects:** [[10___Tool_System]] (all tools) **Priority:** Low (mostly already in sodoryard's design) **Category:** Tool output quality

**What CC does:** CC puts heavy teaching pressure on deterministic error messages rather than elaborate tool description prose. The tool description for `file_edit` is terse ("A tool for editing files"), but validation errors include concrete self-correction hints:

- File not found → current working directory + "Did you mean ...?" suggestions
- File not yet read → "Read it first before writing to it."
- Multiple matches → "Either set replace_all=true or provide more context to uniquely identify the instance."

**Relevance:** sodoryard already designs enriched error messages (doc 10 specifies directory listings on file-not-found, match locations on ambiguous edits). This validates that approach and suggests keeping tool _descriptions_ short while investing in error _messages_. The model learns more from a failed attempt with a good error than from a long tool description it skims.

**What sodoryard should do:**

- During implementation, audit that every tool error path includes actionable next-step guidance, not just a description of what went wrong.
- Keep tool schema descriptions concise. Move behavioral guidance to the system prompt (DC-06) and self-correction guidance to error messages.

---

## Design Validations

These findings confirm sodoryard's existing design. No changes needed. Documented here to prevent future re-litigation.

### DV-01: Per-Tool Truncation with Tail Preservation

CC's shell/task output formatter keeps the tail of output where errors cluster, matching sodoryard's design for shell tool output truncation (doc 10). Confirmed as correct.

### DV-02: Two-Phase Compression Architecture

CC uses model-based summarization for tool-use compaction, but not as the first-line response to every large output. Truncation/persistence comes first; summarization is a later-stage cleanup. This matches sodoryard's doc 11 two-phase approach (truncate first, LLM-summarize if still too big). Confirmed as correct.

### DV-03: Stable/Volatile Prompt Boundary

CC's `SYSTEM_PROMPT_DYNAMIC_BOUNDARY` is functionally identical to sodoryard's Block 1 (stable) / Block 2 (per-turn) split. CC explicitly categorizes prompt sections as cacheable vs. uncached. Confirmed as correct.

### DV-04: Typed Streaming Events Separate from Durable State

CC streams `stream_event` records for UX but stores normal messages for durability. VCR/test recording skips stream events, confirming they're ephemeral. This matches sodoryard's WebSocket events vs. SQLite messages separation. Confirmed as correct.

### DV-05: No Semantic Loop Detector

CC does not have a sophisticated semantic loop detector (similarity-based, progress-analysis-based). It relies on hard iteration caps, denial-count thresholds, token budget exhaustion, and stop hooks. sodoryard's 3-repeated-failures threshold plus hard cap of 50 is in line with shipping practice. Confirmed as adequate for v0.1.

### DV-06: No Filesystem Rollback on Cancellation

CC does not roll back completed filesystem writes during cancellation. Cleanup focuses on transcript/tool-state coherence, not undoing file mutations. This confirms sodoryard's approach is industry-standard. The problem remains open everywhere.

### DV-07: Exact-Match File Editing

CC uses exact string matching for `file_edit`, with limited normalization (quote style variants). Zero matches and multiple matches are errors with enriched feedback. This matches sodoryard's exactly-one-match requirement with enriched error messages. CC adds `replace_all` as an escape hatch for intentional multi-match replacements — worth noting but not urgent for sodoryard v0.1.

### DV-08: Context-Based Cancellation with Process Cleanup

CC uses abort controller hierarchies with parent→child propagation. sodoryard uses Go's `context.Context` with the same propagation semantics. Both clean up running processes (shell commands) on cancellation. Architecturally equivalent.

### DV-09: Cache Breakpoint Strategy

CC uses a small number of explicit cache breakpoints with aggressive byte stability. Tool definitions are treated as cache-key-critical and stabilized at session start. This validates sodoryard's three-breakpoint layout and tool-definition stability rule.

---

## Deferred / Watch Items

These findings are interesting but not actionable until sodoryard is running and can be evaluated empirically.

### W-01: Cache Editing

CC can delete stale tool-result content from cached prefixes without mutating the local transcript, using Anthropic's `cache_edits` API feature. This is a sophisticated optimization for long-lived sessions where old tool results waste cached prefix space. Requires testing whether this API feature is available via OAuth subscription access. Revisit after v0.1 when cache behavior can be measured.

### W-02: Orthogonal Soft Stops

CC distributes convergence signals across multiple systems: token budgets, denial-count caps, structured-output retry limits, stop hooks. sodoryard currently has the hard cap and repeated-failure detection. Adding orthogonal soft stops (especially a denial-count cap and a token budget exhaustion signal) could improve convergence without a heavyweight loop detector. Revisit after initial usage reveals whether the current heuristics are sufficient.

### W-03: Multiple Streaming Transports

CC supports both SSE and WebSocket transports, normalized into the same event stream. sodoryard's WebSocket-only design is correct for self-hosted use. If sodoryard ever needs a headless/CLI mode, SSE is the natural second transport. Not needed for v0.1.

### W-04: Permission Decision Logging

CC logs every permission approval/rejection to analytics and OpenTelemetry. sodoryard's `tool_executions` table already records tool dispatch outcomes. If debugging reveals cases where understanding _why_ a tool was invoked (not just _that_ it was) matters, consider adding a lightweight decision log. Low priority — the existing table likely covers the debugging need.

---

## Audit-First Rationale

All items in this document assume the current implementation has been audited. The audit is a prerequisite because:

1. **Implementation may already handle some of these.** Build agents working from the original docs may have anticipated edge cases (stale writes, aggregate budgets) even though the docs didn't specify them. Building "retrofits" for things that already exist wastes sessions.
    
2. **The actual delta is unknown.** DC-02 (hybrid token counting) changes the compression trigger. But what does the current compression trigger actually look like in code? If the agent session that built it already used API usage data, DC-02 is a no-op. If it used `chars / 4` exactly as specified, DC-02 is real work.
    
3. **Invariants may conflict with existing behavior.** DC-03 (full-read-before-edit) adds a validation gate. If the current `file_edit` implementation has a different validation flow, the gate needs to integrate with it, not replace it. The audit reveals the integration surface.
    
4. **Ordering matters.** Some retrofits depend on others (DC-04 stale-write detection extends DC-03's tracking state). The audit reveals which foundational pieces exist and which need to be built first.
    

**Recommended audit scope:** Focus on the code paths touched by the Design Changes (DC-01 through DC-07). Specifically:

- Tool dispatch flow in the agent loop (DC-01 aggregate budget)
- Compression trigger in context assembly (DC-02 hybrid counting)
- `file_edit` validation path (DC-03 read gate, DC-04 stale-write)
- Cancellation path in the agent loop (DC-05 synthesized results)
- System prompt content (DC-06 behavioral instructions)
- Tool error message quality across all tools (DC-07 teaching errors)

The audit produces a delta report: for each DC item, "already handled / partially handled / not present." That delta report becomes the actual work queue.

---

## Dependencies

- [[05-agent-loop]] — DC-01, DC-02, DC-05
- [[06-context-assembly]] — DC-02
- [[08-data-model]] — DC-05 (potential schema change for cancelled iteration tracking)
- [[10___Tool_System]] — DC-01, DC-03, DC-04, DC-07
- [[11-tool-result-normalization]] — DC-01 (extends the existing two-phase model)

---

## References

- Claude Code codebase exploration, March 2026 (source analysis document)
- [[05-agent-loop]] — Agent loop, tool dispatch, cancellation, caching
- [[06-context-assembly]] — Compression triggers, context budget
- [[08-data-model]] — Message schema, cancellation safety
- [[10___Tool_System]] — Tool implementations, purity classification, error handling
- [[11-tool-result-normalization]] — Two-phase compression architecture