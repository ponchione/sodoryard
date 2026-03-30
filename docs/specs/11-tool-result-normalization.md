**Status:** Draft v0.1 **Last Updated:** 2026-03-29 **Author:** Mitchell **Inspired by:** [tamp](https://github.com/sliday/tamp) (sliday) — HTTP proxy for token compression

---

## Purpose

Tool results accumulate in conversation history and are re-sent as input tokens on every subsequent API call within a turn. A `file_read` of a pretty-printed `package.json` at iteration 2 is still consuming tokens at iteration 47. This document specifies how sirtopham normalizes tool result content at write time and compresses historical results at serialization time to reduce cumulative token waste — without any external dependencies.

tamp solves this problem as an external HTTP proxy because tools like Claude Code don't expose their request pipeline. sirtopham owns the entire pipeline from tool execution through context assembly through API request construction. Every optimization tamp applies externally, sirtopham can apply natively with better information about content age and relevance.

---

## Why Not Use tamp Directly

Running tamp as a localhost proxy between sirtopham and the Anthropic API would:

- Add a network hop (latency) for every API call
- Introduce a Node.js runtime dependency for no structural reason
- Operate blind — tamp compresses all tool results equally because it has no concept of which results the LLM is actively reasoning about vs. ancient history
- Duplicate work — tamp would re-compress content sirtopham already normalized
- Conflict with prompt caching — tamp mutates the request body, which changes the cache key, potentially invalidating Anthropic's prefix cache on every request

sirtopham has context that tamp cannot: tool names, result age (turn and iteration number), content type awareness from the tool that produced it, and the compression lifecycle from [[06-context-assembly]]. Native implementation makes smarter decisions.

---

## Two-Phase Architecture

Normalization happens at two distinct points in the pipeline:

### Phase 1: Write-Time Normalization

Applied immediately when a tool result is produced, before it enters conversation history. The normalized form is what gets stored in the `messages` table and what the LLM sees on the very first re-send.

**Scope:** Current-turn results. The LLM is actively working with this content.

**Principle:** Lossless. Remove only bytes that carry zero information for the LLM. The content must be semantically identical before and after normalization.

### Phase 2: History Compression

Applied when serializing conversation history for an API request. Historical tool results (from completed prior turns) are compressed more aggressively than current-turn results.

**Scope:** Tool results from turns older than the current turn.

**Principle:** Lossy but recoverable. The LLM can re-read files or re-run commands if it needs the full content. Compression trades token cost for a tool call the LLM probably won't make.

---

## Phase 1: Write-Time Normalization

These transforms apply to every tool result string before it is stored in conversation history. They are implemented as a normalization pipeline in the tool executor's result handling path — after `Execute()` returns, before the result is wrapped as a `role=tool` message.

### Transform: JSON Minification

**When:** The tool result content is valid JSON (or contains an embedded JSON block).

**What:** Remove all insignificant whitespace. Pretty-printed JSON with 2-space or 4-space indentation collapses dramatically. A typical `package.json` read via `file_read` drops 30-40% of its character count.

**How:** `json.Compact()` from the Go standard library. Zero dependencies.

**Detection:** Attempt `json.Valid()` on the content. If the entire result is valid JSON, compact it. If not, scan for JSON blocks embedded in structured output (e.g., a `file_read` result where the file content happens to be JSON). The `file_read` tool knows it read a `.json` file — it can signal this via metadata rather than requiring generic detection.

**Example:**

```
Before (file_read of config.json, 847 chars):
{
    "name": "my-project",
    "version": "1.0.0",
    "dependencies": {
        "express": "^4.18.2",
        "lodash": "^4.17.21"
    },
    "devDependencies": {
        "typescript": "^5.3.0",
        "jest": "^29.7.0"
    }
}

After (523 chars, -38%):
{"name":"my-project","version":"1.0.0","dependencies":{"express":"^4.18.2","lodash":"^4.17.21"},"devDependencies":{"typescript":"^5.3.0","jest":"^29.7.0"}}
```

### Transform: Line-Number Prefix Stripping

**When:** The content contains line-numbered output — specifically, the format produced by `file_read`.

**What:** Strip the line-number prefixes and pipe delimiters that `file_read` adds for human/LLM reference. These prefixes are useful on first read (the LLM uses them for `file_edit` targeting) but become pure waste when re-sent in history.

**Why not strip at write time:** This is the one transform that is **deferred to Phase 2**. The line numbers are actively useful during the current turn — the LLM references them in `file_edit` calls. Stripping them from historical results (Phase 2) is safe because the LLM won't be editing based on stale line numbers from a prior turn.

**Note:** This transform is Phase 2 only. Listed here for completeness because tamp applies it universally — sirtopham is smarter about when.

### Transform: Trailing Whitespace and Empty Lines

**When:** Always.

**What:** Strip trailing whitespace from every line. Collapse runs of 3+ empty lines down to 2. Trim trailing newlines to a single newline.

**How:** Simple string processing. No dependencies.

**Savings:** Modest (2-5%) but free and universal.

### Transform: Shell Output Normalization

**When:** The result is from the `shell` tool.

**What:** Two sub-transforms:

1. **ANSI escape code stripping.** Many CLI tools emit color codes even when stdout is a pipe. Strip all ANSI escape sequences (`\x1b\[...m` and related). The LLM gains nothing from color codes.

2. **Progress line collapsing.** Build tools, package managers, and test runners emit progress lines (downloading, compiling, etc.) that overwrite each other via `\r`. Collapse consecutive progress-style lines to a single summary. For example, 47 lines of `Compiling foo v0.1.0` become `[Compiled 47 crates]`.


**Detection:** ANSI codes are regex-detectable. Progress lines are heuristic — lines containing `\r`, lines matching common patterns (`Compiling`, `Downloading`, `Installing`, percentage indicators).

**Savings:** Highly variable. A clean `go test` output: minimal. A `cargo build` from scratch: 80%+ reduction from progress line collapsing alone.

### Transform Pipeline

The transforms apply in order:

```
raw tool result
  → ANSI stripping (shell only)
  → progress line collapsing (shell only)
  → trailing whitespace cleanup (all)
  → JSON minification (when content is JSON)
  → store in conversation history
```

Order matters: ANSI stripping must happen before JSON detection (color codes inside JSON output would break `json.Valid()`). Whitespace cleanup before JSON minification avoids double-processing.

---

## Phase 2: History Compression

Applied during conversation history serialization — the step where stored messages are assembled into the API request body. This runs on every API call, so it must be fast.

### Eligibility

A tool result is eligible for Phase 2 compression when:

- It is from a **completed prior turn** (not the current turn). The current turn's results are actively in use.
- It has **not already been compressed** by the head-tail compression engine from [[06-context-assembly]]. If the message is inside the compressed summary region, it's already gone — Phase 2 doesn't apply.
- It is a `role=tool` message (not user or assistant content).

### Transform: Line-Number Stripping

Deferred from Phase 1. Strip `file_read` line-number prefixes from historical results. The LLM won't be issuing `file_edit` calls against line numbers from 5 turns ago.

**Detection:** Match the `file_read` header format (`File: {path} ({N} lines)`) and the line-number pattern (`{N} │`).

**Savings:** ~15-20% on file read results. Line numbers plus pipe delimiters plus padding add 8-10 characters per line.

### Transform: Historical JSON Re-Minification

Tool results stored before Phase 1 was implemented (migration path) or results from tools that produce pretty-printed JSON in their output format may not have been minified at write time. Re-run `json.Compact()` on historical tool results that contain valid JSON.

This is idempotent — already-minified JSON passes through unchanged.

### Transform: Duplicate Result Elision

If the same file was read multiple times across turns (the LLM re-reads files it read earlier), historical results for the same file path can be elided entirely, replaced with a pointer:

```
[file_read result elided — same file was read again in turn {N}. Content from the later read is in history.]
```

**Detection:** Track `(tool_name, path_argument)` pairs across the conversation. When a duplicate is found, keep only the most recent result and elide older ones.

**Scope:** `file_read` and `git_diff` only. Shell results are never deduplicated — the same command can produce different output at different times. Search results are never deduplicated — the codebase may have changed.

**Savings:** Potentially massive. An LLM that re-reads a 200-line file 4 times across a conversation is wasting 3x the tokens. This is the single highest-impact Phase 2 transform.

### Transform: Stale Result Summarization

For tool results older than a configurable threshold (default: 10 turns ago), replace verbose results with a one-line summary:

```
[Historical: file_read of internal/auth/middleware.go returned 89 lines at turn 3]
```

The LLM knows the file exists, knows approximately what it contained, and can re-read it if needed. The 89 lines of content are no longer consuming tokens.

**Scope:** `file_read` and `search_text` results only. Shell results are already handled by the head-tail compression engine at this age. Git results are small enough to not matter.

**Threshold:** Configurable via `agent.history_summarize_after_turns` (default: 10). Set to 0 to disable.

**Interaction with head-tail compression:** This fires before the head-tail compression engine from [[06-context-assembly]]. If a result is old enough to be summarized here, it's probably in the "middle" region that head-tail compression would summarize anyway. The two mechanisms are complementary — Phase 2 summarization reduces the input to the head-tail compressor, making its summary more focused.

---

## What We Explicitly Do NOT Port from tamp

### TOON Columnar Encoding

tamp's most novel technique: converting homogeneous JSON arrays into a columnar text format. For example, a file listing array becomes a tab-separated table.

**Why not:** TOON is a non-standard format that LLMs haven't been trained on extensively. The compression ratio is good (~40% on arrays), but the risk of confusing the LLM or degrading response quality is not worth the savings. sirtopham's tools don't produce homogeneous arrays as a common case anyway — `file_read` returns text, `search_text` returns formatted matches, `shell` returns raw output. The array case is narrow.

### LLMLingua Neural Compression

tamp optionally uses Microsoft's LLMLingua sidecar for neural text compression — a language model that identifies and removes low-information tokens from natural language.

**Why not:** Adds a Python dependency and a model inference step. The latency cost on every API call is non-trivial. The compression targets natural language text, which is a small fraction of tool results (most are code, JSON, or structured output). The complexity-to-savings ratio is poor for sirtopham's use case.

### Proxy Architecture

tamp's core pattern — intercepting HTTP requests and mutating the body before forwarding.

**Why not:** sirtopham owns the request construction. There is no request to intercept. The transforms are applied directly in the serialization path.

---

## Integration Points

### Tool Executor (Phase 1)

The tool executor's result handling path in [[10___Tool_System]] gains a normalization step between `Execute()` and message storage:

```go
// Pseudocode — tool executor result path
result := tool.Execute(ctx, input)
result = NormalizeToolResult(tool.Name(), result)  // NEW
// ... wrap as role=tool message, store in history
```

`NormalizeToolResult` applies the Phase 1 pipeline. It receives the tool name so it can apply tool-specific transforms (shell ANSI stripping, JSON-aware file_read handling).

### Conversation History Serializer (Phase 2)

The history serialization step in [[05-agent-loop]] (Step 4: Build LLM Request) gains a compression pass over historical tool results:

```go
// Pseudocode — history serialization for API request
messages := loadConversationHistory(conversationID)
for i, msg := range messages {
    if msg.Role == "tool" && msg.TurnNumber < currentTurn {
        messages[i].Content = CompressHistoricalResult(msg, conversationState)  // NEW
    }
}
// ... build API request with messages
```

`CompressHistoricalResult` applies Phase 2 transforms. It receives the full conversation state so it can detect duplicate file reads and determine result age.

### Prompt Cache Interaction

**Phase 1 does not affect caching.** Normalization happens at write time — the stored content is already normalized, so serialization produces a stable prefix.

**Phase 2 requires care.** Compression of historical results changes the message content, which changes the cache key for block 3 (history prefix). However, Phase 2 is deterministic — the same conversation state always produces the same compressed output. As long as the compression is applied consistently (not conditionally or randomly), the cache prefix remains stable across iterations within a turn. The prefix only changes between turns (when new results enter history), which is when it would change anyway due to new messages.

### Configuration

```yaml
agent:
  # Phase 1: Write-time normalization
  normalize_tool_results: true          # Master toggle (default: true)
  strip_ansi_codes: true                # Shell output ANSI stripping
  collapse_progress_lines: true         # Shell progress line collapsing
  minify_json_results: true             # JSON whitespace removal

  # Phase 2: History compression
  compress_historical_results: true     # Master toggle (default: true)
  strip_historical_line_numbers: true   # Remove file_read line prefixes from old turns
  elide_duplicate_reads: true           # Replace duplicate file reads with pointers
  history_summarize_after_turns: 10     # Summarize verbose results older than N turns (0 = disable)
```

All toggles default to `true`. Individual transforms can be disabled for debugging or if they cause issues with specific models.

---

## Estimated Savings

Based on tamp's published benchmarks and sirtopham's tool output characteristics:

|Transform|Phase|Applies To|Estimated Savings|
|---|---|---|---|
|JSON minification|1|file_read of JSON files|30-40% per result|
|Trailing whitespace cleanup|1|All results|2-5% per result|
|ANSI stripping|1|Shell results|5-15% per result|
|Progress line collapsing|1|Shell build/test output|40-80% per result|
|Line-number stripping|2|Historical file_read results|15-20% per result|
|Duplicate read elision|2|Re-read files across turns|Up to 100% per duplicate|
|Stale result summarization|2|Old file_read/search results|Up to 95% per result|

The cumulative effect compounds across a session. In a 50-iteration turn where the LLM reads 20 files and runs 10 shell commands, Phase 1 saves tokens on every re-send of those 30 results. Phase 2 kicks in on subsequent turns, further reducing the historical payload.

The highest-impact single transform is **duplicate read elision** — it's common for an LLM to re-read the same file 3-5 times across a multi-turn conversation, and each duplicate wastes the full file's token count on every subsequent API call.

---

## Implementation Sequencing

Phase 1 normalization is straightforward and should be implemented alongside the tool executor in **Layer 4 (Tool System), Epic 05: Shell Tool** — the shell tool benefits most from ANSI stripping and progress collapsing. JSON minification can be added to the shared tool result path at the same time.

Phase 2 history compression belongs in **Layer 5 (Agent Loop + Context Assembly)** alongside the existing head-tail compression engine. It shares the same serialization hook and the same conversation state awareness.

Both phases are independently valuable. Phase 1 alone provides meaningful savings with zero architectural complexity.

---

## Observability

The existing `tool_executions` table from [[08-data-model]] already tracks `output_size`. Add a `normalized_size` column to capture the post-Phase-1 size, enabling per-tool normalization effectiveness tracking:

```sql
ALTER TABLE tool_executions ADD COLUMN normalized_size INTEGER;
```

The `sub_calls` table's `tokens_in` field captures the total input tokens per API call. Comparing `tokens_in` trends before and after enabling Phase 2 compression provides session-level savings measurement.

The context assembly report from [[06-context-assembly]] can include a `history_compression_savings` field — bytes saved by Phase 2 transforms on the current API call.

---

## References

- [[05-agent-loop]] — History serialization (Step 4), iteration loop, output handling
- [[06-context-assembly]] — Head-tail compression engine, prompt cache layout, context budget
- [[08-data-model]] — Messages table, tool_executions table, compression flags
- [[10___Tool_System]] — Tool executor result path, output truncation, shell tool implementation
- tamp: https://github.com/sliday/tamp (external reference — compression patterns only, not a dependency)
- RTK: https://github.com/rtk-ai/rtk (external reference — complementary command-output compression, system prompt integration only)