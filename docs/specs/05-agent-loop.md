# 05 — Agent Loop

**Status:** Draft v0.1 **Last Updated:** 2026-03-28 **Author:** Mitchell

---

## Overview

The agent loop is the core orchestration engine. It receives a user message, assembles context, makes LLM calls, dispatches tools, and iterates until the turn is complete. Every other layer feeds into or is driven by this loop.

This is entirely net-new for sirtopham. topham's pipeline model (scope → build → verify → approve) has no equivalent — topham made single-shot LLM calls per phase with no tool calling and no iteration. The agent loop is the fundamental architectural shift from pipeline to conversation.

---

## Core Concepts

### Session

A session is a conversation. It has a unique ID, a sequence of turns, and an accumulated message history. Sessions are short and focused — typically one task per session. The user starts a fresh session for each work order or task.

### Turn

A turn begins when the user sends a message and ends when the assistant produces a final text response. Between those two points, the agent may execute many iterations of LLM calls and tool executions. A turn is the atomic unit of user-visible work.

### Iteration

A single LLM roundtrip within a turn. The agent sends the full message array to the LLM, receives a response (text, tool calls, or both), and processes it. If the response contains tool calls, the agent executes them, appends the results, and starts another iteration. If the response is text-only, the turn is complete.

### Hierarchy

```
Session (conversation)
  └─ Turn 1 (user message → final response)
       ├─ Context assembly (fires once at turn start)
       ├─ Iteration 1: LLM returns tool_calls [file_read, search_text]
       │    ├─ Execute file_read (parallel)
       │    └─ Execute search_text (parallel)
       ├─ Iteration 2: LLM returns tool_calls [file_edit]
       │    └─ Execute file_edit (sequential — mutating)
       ├─ Iteration 3: LLM returns tool_calls [shell] 
       │    └─ Execute shell (sequential — mutating)
       └─ Iteration 4: LLM returns text only → turn complete
  └─ Turn 2 ...
```

---

## The Loop: Step by Step

### 1. User Message Received

The web UI sends a message via WebSocket. The agent loop receives it with the conversation ID (or creates a new session).

### 2. Context Assembly

The turn analyzer examines the user's message and recent conversation history to determine what codebase context is needed. If relevant, RAG queries run against the code intelligence layer. The assembled context is frozen for the duration of this turn — it does not re-run on subsequent iterations. See [[06-context-assembly]] for details.

If the agent needs additional codebase information mid-turn, it uses the `search_semantic` tool (reactive retrieval) rather than re-triggering context assembly.

### 3. System Prompt Construction

The system prompt is composed of three blocks, ordered for Anthropic prompt caching:

```
[CACHE BLOCK 1] Base system prompt
  Agent personality, behavioral guidelines, tool usage instructions.
  Thin — frontier models don't need verbose instructions.
  Identical across all calls in all sessions.

[CACHE BLOCK 2] Assembled context (per-turn)
  RAG results, project conventions, file tree excerpt.
  Frozen at turn start. Identical across all iterations within a turn.

[CACHE BLOCK 3] Conversation history (previous turns)
  All messages from prior turns in this session.
  Grows monotonically. Prefix is stable — new turns only append.
```

Followed by the fresh (uncached) content:

```
[FRESH] Current turn messages
  User message, in-progress tool calls and results from earlier iterations.
```

This ordering is critical — Anthropic's cache is keyed on exact prefix matching. Cached blocks must be a contiguous prefix of the message array.

### 4. LLM Request

Send to the configured provider: system prompt + conversation history + current turn messages + tool definitions. Stream the response.

**Extended thinking** is enabled by default. Claude's extended thinking uses tokens but significantly improves reasoning quality on complex coding tasks. With a 1M context window and subscription access, there is no reason to disable it. Thinking blocks are preserved in history and displayed (collapsible) in the web UI.

### 5. Response Handling

The LLM response is a stream of content blocks. Three possible content types:

**Text blocks:** Stream tokens to the UI in real-time via WebSocket. These are the agent's visible response.

**Thinking blocks:** Stream to the UI as collapsible "thinking" content. Preserved in conversation history for context continuity.

**Tool use blocks:** The LLM is requesting tool execution. Collect all tool calls in the response, then proceed to tool dispatch.

If the response contains only text (and possibly thinking), the turn is complete. Proceed to step 8.

If the response contains tool calls, proceed to step 6.

### 6. Tool Dispatch

#### Tool Purity Classification

Each tool is classified by its side effects:

**Pure (read-only):** `file_read`, `search_text`, `search_semantic`, `git_status`, `git_diff`

- No side effects on the filesystem or environment
- Safe to execute in parallel
- Order does not matter

**Mutating:** `file_write`, `file_edit`, `shell`

- Modify the filesystem or execute arbitrary commands
- Must respect ordering — a file_write followed by a shell command (e.g., run tests) must execute sequentially
- The LLM's specified order is the execution order

#### Execution Strategy

When a batch of tool calls arrives from the LLM:

1. **Partition** the batch into pure and mutating calls.
2. **Execute all pure calls concurrently** using goroutines. Collect results.
3. **Execute mutating calls sequentially** in the order specified by the LLM. Collect results.
4. **Assemble tool result messages** in the original order (matching tool call IDs).
5. **Append** all tool results to the conversation history.

If the LLM returns a mix (e.g., file_read + file_write + shell in one batch), the file_read runs concurrently with nothing (it's the only pure call), then file_write runs, then shell runs.

#### Output Handling

Tool results can be large (a 5000-line file, a verbose build log). Each tool has a configurable output size limit. Results exceeding the limit are truncated with a notice:

```
[Output truncated — showing first 200 lines of 5847. Use file_read with line_start/line_end for specific sections.]
```

The truncation limit is generous by default (the 1M context window gives us room) but configurable per tool and globally.

### 7. Iteration Check

After tool dispatch, check:

**Iteration count:** Has the turn exceeded the maximum iteration limit (default: 50)?

- If **under the limit:** Append tool results to history, go to step 4 (next LLM request).
- If **at the limit:** Inject a directive message and make one final LLM call with tools disabled:

```
"You have reached the maximum number of tool call iterations for this turn. 
Do not request any more tool calls. Summarize your progress so far and list 
any remaining work that still needs to be completed."
```

Disabling tools on the final call forces the LLM to produce a text response. The user can then start a new turn to continue the work.

**Loop detection:** If the same tool has been called with identical arguments 3 consecutive times and keeps failing, inject a nudge before the next LLM call:

```
"You have attempted [tool_name] with the same arguments 3 times without success. 
Consider a different approach."
```

This does not stop the loop — it nudges the model. The iteration limit is the hard stop.

### 8. Turn Complete

When the LLM produces a final text response:

1. **Persist** the final text-only assistant iteration to SQLite. Earlier completed iterations have already been committed atomically as they finished, and the user message was persisted at turn start. See [[08-data-model]].
2. **Log** the turn's sub-calls: every LLM invocation during the turn gets a row in the sub_calls table with provider, model, tokens, latency, and purpose.
3. **Emit** `turn_complete` event via WebSocket with usage summary.
4. **Return to idle** — await next user message or session end.

---

## Cancellation

The user can cancel a turn in progress via the web UI (cancel button sends a `cancel` event over WebSocket).

### Cancellation During LLM Call

Cancel the HTTP request via Go context cancellation (`ctx.Cancel()`). The provider's streaming connection is closed. Any tokens already streamed to the UI remain visible. The partially completed assistant message is discarded (not persisted).

### Cancellation During Tool Execution

For **pure tools** (file reads, searches): cancel the context. Operations abort cleanly.

For **shell commands**: send SIGTERM to the process group. If the process doesn't exit within 5 seconds, send SIGKILL. Capture whatever output was produced.

### Post-Cancellation State

After cancellation:

- Messages already committed to history from completed iterations remain.
- The in-flight iteration (partial LLM response, in-progress tool calls) is discarded.
- The conversation is in a consistent state — the user can send a new message immediately.
- A `turn_cancelled` event is emitted to the UI.

---

## Error Recovery

Three layers, from lightest to heaviest:

### Layer 1: Feed It Back (Default)

Tool execution fails → the error message becomes the tool result sent back to the LLM. The LLM sees the error and self-corrects.

```json
{
  "tool_call_id": "tc_123",
  "content": "Error: file not found: internal/auth/handler.go\nAvailable files in internal/auth/: middleware.go, service.go, types.go"
}
```

Enriching error messages with helpful context (like listing available files on a "not found" error) improves self-correction rates significantly.

### Layer 2: Loop Detection (Nudge)

Same tool, same arguments, 3 consecutive failures → inject a nudge message. The LLM is not stopped, just redirected. This catches the case where the model is stuck in a pattern but might succeed with a different approach.

### Layer 3: Iteration Limit (Hard Stop)

50 iterations per turn (configurable). On the final iteration, tools are disabled and the LLM must summarize. This prevents runaway sessions from burning through the context window.

### LLM API Errors

- **Rate limiting (429):** Retry with exponential backoff. 3 attempts, then surface the error to the user. Optionally fall back to the configured fallback provider.
- **Server error (500/502/503):** Retry with backoff, 3 attempts. Fall back if exhausted.
- **Auth failure (401/403):** Do not retry. Surface a clear message: "Claude credentials expired. Run `claude login` to re-authenticate."
- **Context overflow (400 — context length exceeded):** This shouldn't happen with proper budget management, but if it does: trigger emergency compression (summarize old turns), retry once.
- **Malformed tool calls:** If the LLM produces invalid JSON in a tool call, feed the parse error back as a tool result with correction guidance. The LLM almost always fixes it on the next attempt.

---

## Tool Set

Eight tools for v0.1. Each exists because it provides something bash alone cannot — structured output for the UI, safety guardrails, or token efficiency.

### file_read

Read file contents, optionally with a line range. Returns content with line numbers. Structured output enables the UI to show syntax-highlighted code with the exact file and line range.

**Purity:** Pure

### file_write

Write or overwrite a file. Returns confirmation and a diff preview (first N lines of the diff). Creates parent directories if needed. Refuses writes outside the project root.

**Purity:** Mutating

### file_edit

Apply a targeted edit — search for a unique string in a file and replace it. Returns the diff. Dramatically more token-efficient than the LLM rewriting an entire file — the tool call contains only the search string and replacement, not the full file content.

**Purity:** Mutating

### search_text

Ripgrep-based text search across the project. Returns matches with surrounding context lines. Structured output enables the UI to show clickable file:line results.

**Purity:** Pure

### search_semantic

RAG-based semantic search against the code intelligence layer. "Find the authentication logic" → returns relevant code chunks with file paths, descriptions, and similarity scores. This is the agent's on-demand access to the full codebase knowledge.

**Purity:** Pure

### git_status

Current branch, dirty files, recent commits. Clean structured output.

**Purity:** Pure

### git_diff

Diff of working tree or between refs. Returns the diff output.

**Purity:** Pure

### shell

Execute an arbitrary shell command. Captures stdout, stderr, exit code. Configurable timeout (default: 120 seconds). Working directory is always the project root.

Safety: optional denylist for destructive patterns (e.g., `rm -rf /`, `git push --force`). Since this is a personal tool, the denylist is minimal — just catastrophic mistakes, not general safety theater.

**Purity:** Mutating

### Future Tools (Not v0.1)

- **search_web:** Web search for documentation, Stack Overflow, etc.
- **mcp_call:** Invoke an MCP server tool (MCP integration)
- **delegate:** Spawn a sub-agent for a focused sub-task (Hermes-style delegation)

---

## Prompt Caching Strategy

### Why It Matters

Even with a 1M context window and subscription access (no per-token cost), prompt caching provides significant **latency** benefits. A cache hit on 50k tokens of conversation history means those tokens are processed nearly instantly rather than being re-read and re-attended-to.

### Anthropic Cache Mechanics

Anthropic's prompt caching is keyed on exact prefix matching. If the first N tokens of the current request match the first N tokens of a previous request, those N tokens are served from cache.

### sirtopham's Cache Layout

**Block 1 — System prompt (stable across all calls):** Base personality, tool instructions. Thin — under 5k tokens. Cache hit rate: ~100% across an entire session.

**Block 2 — Assembled context (stable within a turn):** RAG results, conventions, file tree. Assembled once at the start of each turn, frozen for all iterations. Cache hit rate: 100% within a turn, 0% across turns (context changes per turn).

**Block 3 — Conversation history prefix (grows monotonically):** All completed turns. Each new iteration within a turn extends the prefix but the existing prefix is unchanged. Cache hit rate: high — each iteration within a turn gets a longer prefix match. Across turns, the previous turns' history is still a valid prefix.

**Block 4 — Fresh content (never cached):** Current iteration's new messages (latest tool results, user message on first iteration).

### Implementation

Use Anthropic's `cache_control` markers on the configured prompt blocks. In the current implementation, `agent.cache_system_prompt`, `agent.cache_assembled_context`, and `agent.cache_conversation_history` are real per-block controls passed through `internal/agent/prompt.go`. When enabled, the corresponding base-prompt, assembled-context, and history-prefix regions get explicit `cache_control` breakpoints; current-iteration content remains unmarked.

For the Codex/OpenAI provider: OpenAI has automatic prompt caching on recent models. No explicit marking needed — the prefix matching happens server-side.

For local models: no caching. Local inference doesn't support it. Latency is controlled by proximity (localhost) instead.

---

## Streaming to the Web UI

The agent loop emits events as they occur. The WebSocket handler forwards them to the frontend. Events are typed:

```
token            — text delta from the LLM
thinking_start   — extended thinking block begins
thinking_delta   — thinking text delta
thinking_end     — thinking block ends
tool_call_start  — tool dispatch beginning (name, args)
tool_call_output — incremental output from a tool (e.g., streaming shell stdout)
tool_call_end    — tool execution complete (result, duration, success/failure)
turn_complete    — turn finished (usage summary, iteration count)
turn_cancelled   — turn was cancelled by user
error            — recoverable or non-recoverable error
status           — agent state change (assembling_context, waiting_for_llm, executing_tools, compressing, idle)
context_debug    — ContextAssemblyReport emitted after context assembly (frontend may ignore unless the debug panel is open)
```

Current shipped status values are more explicit than the older three-state shorthand:

```
assembling_context — Layer 3 context assembly is running
waiting_for_llm    — the provider request is in flight
executing_tools    — one or more tool calls are running
compressing        — history compression is running
idle               — no turn is currently active
```

The web UI maps those states to operator-facing labels like "Assembling context…", "Waiting for model…", "Running tools…", and "Compressing history…".

Tool call events include the tool call ID so the UI can match starts to ends and display concurrent tool executions correctly.

See [[07-web-interface-and-streaming]] for the full WebSocket protocol specification.

---

## Persistence

Every turn persists the following to SQLite:

**Messages table:**

- User message (role=user, turn_number=N, sequence=0)
- Assistant responses (role=assistant, with tool_calls JSON, thinking content)
- Tool results (role=tool, with tool_call_id, tool_name, result content)
- Each message records: tokens_in, tokens_out, model, provider, latency_ms

**Sub_calls table:**

- Every LLM invocation: provider, model, tokens, latency, purpose ("chat"), success/failure
- Context assembly LLM calls (if any): purpose="context_classification"
- Enables per-session metrics: total tokens, total iterations, total latency, model breakdown

**When to persist:**

- Each completed iteration's messages are persisted immediately (not batched to end of turn). This ensures that if the process crashes mid-turn, completed work is not lost.
- On cancellation, only completed iterations are persisted. In-flight work is discarded.

See [[08-data-model]] for the full schema.

---

## Configuration

```yaml
agent:
  max_iterations_per_turn: 50       # Hard stop for runaway loops
  loop_detection_threshold: 3       # Same tool+args failures before nudge
  
  tool_output_max_tokens: 50000     # Default truncation limit per tool result
  shell_timeout_seconds: 120        # Default shell command timeout
  shell_denylist:                   # Patterns to reject in shell commands
    - "rm -rf /"
    - "git push --force"
  
  extended_thinking: true           # Enable Claude's extended thinking
  
  # Prompt caching (Anthropic explicit cache_control markers only)
  # Non-Anthropic providers ignore these toggles because they do not use
  # explicit cache breakpoints in the request shape.
  cache_system_prompt: true
  cache_assembled_context: true
  cache_conversation_history: true
```

---

## Dependencies

- [[03-provider-architecture]] — LLM calls, streaming, caching API
- [[04-code-intelligence-and-rag]] — search_semantic tool, context assembly's data source
- [[06-context-assembly]] — fires at turn start, provides assembled context
- [[07-web-interface-and-streaming]] — WebSocket event protocol
- [[08-data-model]] — message and sub_call persistence

---

## What Ports from topham

Very little directly:

- **Event sink pattern:** topham's `EventSink` interface (Emit/ChannelSink) is a clean pattern for the agent loop to emit events. Reusable concept, different event types.
- **Streaming parsing:** SSE line parsing from `llm/client.go` for OpenAI-compatible providers. Anthropic streaming uses a different format that's net-new.
- **RoleResolver concept:** mapping roles to providers. Simplified in sirtopham to per-conversation model selection rather than per-pipeline-phase.

Everything else — the turn loop, tool dispatch, iteration management, cancellation, caching strategy — is net-new.

---

## Open Questions

- **Conversation branching:** The web UI makes it feasible to "fork" a conversation at any turn — try a different approach without losing the original path. Architecturally, this is a new session seeded with the message history up to the fork point. Worth building? It's a compelling UI feature but adds complexity to the conversation model.
- **Streaming tool output:** Should long-running shell commands stream their stdout to the UI in real-time (via `tool_call_output` events), or should we wait for completion? Real-time is better UX but adds complexity. Probably worth it for build/test commands that take 30+ seconds.
- **Tool result summarization:** For very large tool results (full build logs, large file reads), should the agent loop summarize them before feeding back to the LLM? Or just truncate? Summarization preserves information but costs an extra LLM call. Truncation is simple but lossy.
- **Anthropic cache_control API specifics:** Need to verify the exact mechanism for marking cache breakpoints with OAuth-based access. The API docs describe it for API-key users — confirm it works identically with subscription OAuth tokens.

---

## References

- Anthropic tool use: https://docs.anthropic.com/en/docs/build-with-claude/tool-use
- Anthropic prompt caching: https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching
- Anthropic extended thinking: https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking
- Hermes agent loop: `agent/` directory in hermes-agent repo (reference for tool dispatch patterns)
- topham event system: `internal/pipeline/events.go` (reusable EventSink pattern)
- topham LLM client: `internal/llm/client.go` (streaming parsing patterns)