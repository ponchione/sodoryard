# Spec 19: Tool Result Details

**Phase:** 9 - Tool observability and UI metadata
**Status:** proposed design, ready for narrow implementation
**Date:** 2026-04-28

---

## 1. Problem

sodoryard currently treats a tool result as one primary string. That string is used for three different jobs:

- model-visible tool content sent back to the LLM
- WebSocket/UI display content
- analytics and execution bookkeeping

That simplicity has been useful, but it forces the UI and analytics code to infer structured facts by parsing text. Examples include shell exit codes, file paths changed by `file_edit`, whether a diff was truncated, and whether aggregate budgeting persisted a large result to a temp file.

The Pi coding-agent harness has a useful distinction here: tools can return model-facing content plus structured `details`. We should adopt that narrow idea without adopting Pi's extension system, custom renderer system, or broader session architecture.

## 2. Goal

Add optional structured details to tool results while keeping model-visible behavior stable.

The model should continue to receive the same `Content` string it receives today. The new details payload exists for first-party UI, observability, and analytics. It must not change tool schemas, prompts, provider request construction, or the agent's reasoning contract.

## 3. Non-Goals

- Do not build a plugin or extension renderer system.
- Do not add user-configurable tool renderers.
- Do not change the LLM-visible tool result format.
- Do not require every existing tool to emit details in the first implementation.
- Do not parse structured details back into prompts or context assembly.
- Do not store large duplicate payloads, such as a second copy of a full shell log or diff, in details.

## 4. Current System

Current relevant contracts:

- `internal/tool/types.go`
  - `ToolResult` contains `CallID`, `Content`, `Success`, `Error`, `DurationMs`, `OutputSize`, and `NormalizedSize`.
  - `Content` is the model-visible result text.
  - `ToProvider()` drops everything except `ToolUseID`, `Content`, and `IsError`.
- `internal/provider/types.go`
  - `provider.ToolResult` contains `ToolUseID`, `Content`, and `IsError`.
- `internal/provider/message.go`
  - `NewToolResultMessage(...)` serializes only the content string into conversation history.
- `internal/agent/events.go`
  - `ToolCallOutputEvent.Output` and `ToolCallEndEvent.Result` are strings.
- `web/src/types/events.ts`
  - `ToolCallEndEvent` mirrors the string-only backend event shape.
- `internal/db/schema.sql`
  - `messages.content` stores model-visible text.
  - `tool_executions` stores execution analytics, but no structured metadata.

The implementation must preserve this string path as the authoritative model path.

## 5. Target Contract

### 5.1 Terms

`content`
: The plain string returned to the LLM as the tool result. This remains the durable, model-visible truth.

`details`
: Optional structured JSON attached to the tool result for UI and analytics. Details are not sent to providers and are not injected into model history.

### 5.2 Backend Types

Add an optional raw JSON field to the internal and provider-level result structs:

```go
type ToolResult struct {
    CallID         string          `json:"call_id"`
    Content        string          `json:"content"`
    Success        bool            `json:"success"`
    Error          string          `json:"error,omitempty"`
    DurationMs     int64           `json:"duration_ms"`
    OutputSize     int             `json:"output_size,omitempty"`
    NormalizedSize int             `json:"normalized_size,omitempty"`
    Details        json.RawMessage `json:"details,omitempty"`
}

type provider.ToolResult struct {
    ToolUseID string          `json:"tool_use_id"`
    Content   string          `json:"content"`
    IsError   bool            `json:"is_error,omitempty"`
    Details   json.RawMessage `json:"details,omitempty"`
}
```

`ToolResult.ToProvider()` copies `Details`. Provider message construction ignores `Details`.

`provider.NewToolResultMessage(...)` should remain unchanged unless a later persistence phase explicitly adds a separate details path. The serialized LLM message must continue to be content-only.

### 5.3 WebSocket Events

Add details only to the final tool event:

```go
type ToolCallEndEvent struct {
    Type       string          `json:"type"`
    ToolCallID string          `json:"tool_call_id"`
    Result     string          `json:"result,omitempty"`
    Details    json.RawMessage `json:"details,omitempty"`
    Duration   time.Duration   `json:"duration,omitempty"`
    Success    bool            `json:"success,omitempty"`
    Time       time.Time       `json:"time"`
}
```

Keep `ToolCallOutputEvent.Output` string-only. Streaming output is an output channel, not a metadata channel.

### 5.4 Frontend Types

Add a generic details field to the event and block model:

```ts
export interface ToolCallEndEvent {
  type: "tool_call_end";
  tool_call_id: string;
  result?: string;
  details?: Record<string, unknown>;
  duration?: number;
  success?: boolean;
  time: string;
}

export interface ToolCallBlock {
  kind: "tool_call";
  toolCallId: string;
  toolName: string;
  args?: Record<string, unknown>;
  output: string;
  result?: string;
  details?: Record<string, unknown>;
  duration?: number;
  success?: boolean;
  done: boolean;
}
```

The UI should render details opportunistically and fall back to the existing result text when details are absent.

## 6. Details Envelope

Every details payload should be a JSON object with a small common envelope:

```json
{
  "version": 1,
  "kind": "file_mutation",
  "summary": "Edited internal/tool/file_edit.go"
}
```

Required fields:

- `version`: integer schema version, starting at `1`
- `kind`: stable category string used by first-party UI renderers

Optional common fields:

- `summary`: short human-readable label for compact UI
- `truncated`: whether the model-visible content was truncated
- `original_size`: byte length before normalization/truncation
- `normalized_size`: byte length after write-time normalization
- `returned_size`: byte length finally returned to the model
- `persisted_path`: path to a persisted full result, when aggregate budgeting replaced model content with a persisted-result reference

Details should stay small. Initial implementation should cap details at 32 KiB. If a payload exceeds the cap, drop tool-specific detail fields and keep only the common envelope plus a `details_truncated: true` marker.

Details must not contain secrets or sensitive data that are not already present in the tool result, tool input, or analytics row. This feature should not create a new leakage channel.

## 7. Initial Tool Payloads

Do not block the framework on every tool. Start with the tools that currently force the most UI string parsing.

### 7.1 `shell`

Kind: `shell`

```json
{
  "version": 1,
  "kind": "shell",
  "command": "make test",
  "working_dir": ".",
  "exit_code": 0,
  "timed_out": false,
  "cancelled": false,
  "timeout_ms": 120000,
  "stdout_bytes": 1234,
  "stderr_bytes": 0,
  "output_bytes": 1248,
  "original_size": 1248,
  "normalized_size": 1210,
  "returned_size": 1210,
  "truncated": false
}
```

Notes:

- Non-zero command exit codes remain `Success: true`, matching current shell semantics.
- `exit_code` must come from process execution, not by parsing the formatted content string.
- If the executor or aggregate budget later truncates/persists the result, it should update the common size/truncation fields.

### 7.2 `file_edit`

Kind: `file_mutation`

```json
{
  "version": 1,
  "kind": "file_mutation",
  "operation": "edit",
  "path": "internal/tool/file_edit.go",
  "created": false,
  "changed": true,
  "diff_format": "unified",
  "diff_line_count": 18,
  "diff_truncated": false,
  "first_changed_line": 42,
  "bytes_before": 3901,
  "bytes_after": 4048
}
```

Notes:

- Do not duplicate the full diff in details. The model-visible `Content` string is already the diff.
- `first_changed_line` is optional. Add it only if it is cheap and reliable.
- Error cases may include `error_code`, `candidate_lines_count`, or `candidate_snippets_count`, but should not duplicate long snippets already in `Content`.

### 7.3 `file_write`

Kind: `file_mutation`

```json
{
  "version": 1,
  "kind": "file_mutation",
  "operation": "write",
  "path": "docs/specs/19-tool-result-details.md",
  "created": true,
  "changed": true,
  "diff_format": "unified",
  "diff_line_count": 0,
  "diff_truncated": false,
  "bytes_before": 0,
  "bytes_after": 8120
}
```

Notes:

- For a new file, `diff_line_count` may be `0` if the result content is the existing `[new file created]` message.
- For overwrites, use the same diff metadata as `file_edit`.

### 7.4 Later Candidates

Good follow-up candidates after the initial slice:

- `file_read`: path, requested line range, returned line range, total lines, binary/image marker
- `search_text` and `search_semantic`: query, result count, path count, truncation state
- `git_diff`: refs/path filter, file count, diff truncation state
- `test_run`: command, test status, package/test counts when available
- brain tools: document path, title, action, tag count

## 8. Pipeline

### 8.1 Tool Execution

Tools may populate `ToolResult.Details` directly. The result content remains the primary behavior contract.

### 8.2 Executor Normalization and Truncation

The executor already owns write-time normalization and per-result truncation. It should also enrich details with common size fields:

1. capture raw content size before normalization
2. normalize successful result content
3. capture normalized size
4. apply per-result truncation
5. capture returned size and truncation state
6. merge those fields into `Details`

This avoids making every tool understand executor-level normalization and truncation policy.

### 8.3 Agent Aggregate Budgeting

`ToolOutputManager.ApplyAggregateBudget(...)` may shrink or persist model-visible result content after provider results have been produced. It should preserve `Details` and update common fields when it replaces `Content`.

When a full result is persisted, details may include `persisted_path`. The model-visible persisted-result reference remains in `Content`, exactly as today.

### 8.4 Provider Boundary

Provider request construction must ignore `Details`.

This is the main invariant:

```go
provider.NewToolResultMessage(result.ToolUseID, call.Name, result.Content)
```

continues to send only `Content` to the model.

### 8.5 Event Emission

`finalizeExecutedToolResults(...)` should emit:

- `ToolCallOutputEvent.Output = toolResult.Content`
- `ToolCallEndEvent.Result = toolResult.Content`
- `ToolCallEndEvent.Details = toolResult.Details`

This keeps existing clients compatible while allowing newer clients to render richer cards.

## 9. Persistence

Initial implementation should not require a database migration.

Phase 1:

- carry details through tool execution, provider result structs, agent finalization, WebSocket events, and frontend in-memory state
- keep `messages.content` content-only
- keep `tool_executions` unchanged
- accept that reloaded history will not have structured details yet

Phase 2, only if reloadable metadata proves useful:

- add nullable `details_json` to `tool_executions`
- store details from `ToolExecutionRecorder.Record(...)`
- expose details in conversation-history APIs by joining on `(conversation_id, tool_use_id)`
- keep `messages` free of details so model-visible history remains clean

Do not add details to `messages.content`.

## 10. Frontend Rendering

The UI should use `details.kind` for small, first-party renderers:

- `shell`: show exit code, timeout/cancelled state, output size, and persisted-path link if present
- `file_mutation`: show path, operation, changed/created state, byte delta, and diff stats
- unknown kind: render the current result text only

Rendering rules:

- result text remains available and expandable
- details render above the raw result as compact facts, not as a replacement for the result
- do not parse `result` text to recover facts that details should supply
- do not add tool-specific UI until the backend emits that details kind

## 11. Testing Plan

Backend tests:

- `internal/tool`: `shell` emits shell details without parsing formatted content
- `internal/tool`: `file_edit` emits `file_mutation` details on a successful edit
- `internal/tool`: `file_write` emits `file_mutation` details for create and overwrite paths
- `internal/tool`: executor enriches details with size/truncation fields
- `internal/tool`: `ToolResult.ToProvider()` preserves details
- `internal/provider`: `NewToolResultMessage(...)` remains content-only
- `internal/agent`: `ToolCallEndEvent` includes details, while persisted tool messages remain content-only
- `internal/agent`: aggregate budgeting preserves details and updates truncation/persistence fields when content is replaced

Frontend tests:

- `web/src/hooks/use-conversation.ts`: reducer stores `details` from `tool_call_end`
- `web/src/components/chat/tool-call-card.tsx`: renders known detail facts when present and still renders raw result fallback
- existing tool transcript behavior remains unchanged when details are absent

Suggested verification commands:

```bash
rtk go test -tags sqlite_fts5 ./internal/tool ./internal/provider ./internal/agent
rtk proxy sh -lc 'cd web && npm test -- tool-transcript'
rtk make test
rtk make build
```

## 12. Rollout Plan

1. Add the optional result/detail fields and copy them through existing structs.
2. Add backend event propagation with no tool emitters yet.
3. Add frontend type/reducer support with no visible UI change.
4. Add `shell` details and focused tests.
5. Add `file_edit` and `file_write` details and focused tests.
6. Add compact frontend renderers for `shell` and `file_mutation`.
7. Re-evaluate durable persistence after using live details in the UI.

Each step should be independently shippable and backward compatible.

## 13. Acceptance Criteria

- Existing tool result content sent to providers is unchanged.
- Existing WebSocket clients continue to work because `result` and `output` remain strings.
- New WebSocket clients can read `tool_call_end.details`.
- At least `shell`, `file_edit`, and `file_write` emit useful structured details.
- Details are small, optional, and omitted when a tool has nothing structured to report.
- The UI never has to parse shell result text to get exit code or parse diffs to get the changed path.
- `make test` and `make build` pass after implementation.

## 14. Open Questions

- Should durable details become part of `tool_executions` once the UI proves useful on live turns?
- Should details payloads eventually get typed Go structs per kind, or is `json.RawMessage` sufficient with tests?
- Should persisted full-result paths become clickable/downloadable in the UI, or remain diagnostic text only?
- Should `test_run` get a first-class details schema after `shell`, since it likely matters more to chain audit UX?
