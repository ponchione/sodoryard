# Task 06: Unit Tests (Translation and Token Caching)

**Epic:** 05 — Codex Provider
**Status:** ⬚ Not started
**Dependencies:** Task 02 (credential delegation), Task 03 (request translation), Task 04 (response translation), Task 05 (streaming parser)

---

## Description

Write comprehensive unit tests for the Codex provider's request translation, response translation, token caching, and credential delegation logic. These tests exercise pure functions and in-process logic only (no HTTP servers, no real CLI binaries). The credential tests use temporary files and mock executables to validate the auth file reading, expiry checking, and refresh shell-out behavior.

## Acceptance Criteria

- [ ] File `internal/provider/codex/translate_test.go` exists and declares `package codex`

- [ ] **Request translation tests** — each test calls `buildResponsesRequest` directly and validates the JSON output:

  - [ ] Test: system prompt concatenation. Given `req.SystemBlocks` with two entries `{Text: "You are a coding assistant."}` and `{Text: "Project context: Go backend"}`, the first input item is `{"role": "system", "content": "You are a coding assistant.\n\nProject context: Go backend"}`. Assert the content is a plain string (not an array)

  - [ ] Test: empty system blocks. Given `req.SystemBlocks` is nil, no system input item is emitted. The first input item should be the first message

  - [ ] Test: user message. Given a unified user message with text `"Fix the auth bug"`, the output contains `{"role": "user", "content": "Fix the auth bug"}`

  - [ ] Test: assistant message with text only. Given a unified assistant message with a single text content block `{Type: "text", Text: "I'll check the code."}`, the output contains `{"role": "assistant", "content": [{"type": "text", "text": "I'll check the code."}]}`

  - [ ] Test: assistant message with tool use. Given a unified assistant message with a text block and a tool_use block `{Type: "tool_use", ID: "tc_1", Name: "file_read", Input: json.RawMessage(`{"path":"auth.go"}`)}`, the output contains an assistant input item with content array including `{"type": "function_call", "id": "fc_tc_1", "call_id": "tc_1", "name": "file_read", "arguments": "{\"path\":\"auth.go\"}"}`

  - [ ] Test: tool result message. Given a unified tool message with `Role: "tool"`, `ToolUseID: "tc_1"`, and content `"package auth..."`, the output contains a user input item with content `[{"type": "function_call_output", "call_id": "tc_1", "output": "package auth..."}]`

  - [ ] Test: tool definitions. Given `req.Tools` with one entry `{Name: "file_read", Description: "Read a file", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)}`, the output tools array contains `{"type": "function", "name": "file_read", "description": "Read a file", "parameters": {"type":"object","properties":{"path":{"type":"string"}}}}`

  - [ ] Test: reasoning configuration for o3. Given model `"o3"`, the output includes `"reasoning": {"effort": "high", "encrypted_content": "retain"}`

  - [ ] Test: reasoning configuration for gpt-4.1. Given model `"gpt-4.1"`, the output does NOT include a `reasoning` field (it should be nil/omitted)

  - [ ] Test: stream flag. Given `stream=true`, the output includes `"stream": true`. Given `stream=false`, the output includes `"stream": false`

- [ ] File `internal/provider/codex/response_test.go` exists and declares `package codex`

- [ ] **Response translation tests** — each test constructs a `responsesResponse` struct and validates the unified `provider.Response` output:

  - [ ] Test: text-only response. Given a response with one output item `{Type: "message", Content: [{Type: "output_text", Text: "I found the issue."}]}`, the unified response contains one `provider.ContentBlock{Type: "text", Text: "I found the issue."}`

  - [ ] Test: function call response. Given a response with one output item `{Type: "function_call", CallID: "call_1", Name: "file_read", Arguments: "{\"path\":\"auth.go\"}"}`, the unified response contains one `provider.ContentBlock{Type: "tool_use", ID: "call_1", Name: "file_read", Input: json.RawMessage("{\"path\":\"auth.go\"}")}` and `StopReason` is `provider.StopReasonToolUse`

  - [ ] Test: mixed response with reasoning. Given a response with three output items (reasoning, message, function_call), the unified response contains three content blocks in order: reasoning block with encrypted content, text block, tool_use block. `StopReason` is `provider.StopReasonToolUse` because a function_call is present

  - [ ] Test: usage mapping. Given `responsesUsage{InputTokens: 500, OutputTokens: 150, InputTokensDetails: {CachedTokens: 100}, OutputTokensDetails: {ReasoningTokens: 80}}`, the unified usage has `InputTokens: 500`, `OutputTokens: 150`, `CacheReadTokens: 100`, `CacheCreationTokens: 0`

  - [ ] Test: stop reason with no tool calls. Given a response with only a message output item (no function_call), `StopReason` is `provider.StopReasonEndTurn`

- [ ] File `internal/provider/codex/credentials_test.go` exists and declares `package codex`

- [ ] **Auth file parsing tests** — each test creates a temporary `auth.json` file:

  - [ ] Test: valid auth file. Write `{"access_token": "eyJ_test_token", "expires_at": "2026-03-28T18:00:00Z"}` to a temp file. Call `readAuthFile` (with home dir overridden to the temp dir). Assert token is `"eyJ_test_token"` and expiry parses to the expected time

  - [ ] Test: missing auth file. Point home dir to an empty temp directory. Assert error contains `"auth file not found at"`

  - [ ] Test: invalid JSON. Write `{invalid json}` to the auth file path. Assert error contains `"invalid auth file format"`

  - [ ] Test: empty access token. Write `{"access_token": "", "expires_at": "2026-03-28T18:00:00Z"}`. Assert error contains `"empty access_token"`

  - [ ] Test: invalid timestamp. Write `{"access_token": "tok", "expires_at": "not-a-date"}`. Assert error contains `"invalid expires_at timestamp"`

- [ ] **Token caching tests** — exercise `getAccessToken` with controlled clock/state:

  - [ ] Test: cached token returned without I/O. Manually set `p.cachedToken` to `"cached_tok"` and `p.tokenExpiry` to 10 minutes in the future. Call `getAccessToken`. Assert it returns `"cached_tok"` without touching the filesystem or running any subprocess

  - [ ] Test: expired token triggers refresh. Set `p.cachedToken` to `"old_tok"` and `p.tokenExpiry` to 60 seconds in the future (within the 120-second buffer). Assert `getAccessToken` attempts to refresh (this test may use a mock executable or verify the refresh path is taken)

  - [ ] Test: empty cached token triggers refresh. Leave `p.cachedToken` empty. Assert `getAccessToken` reads the auth file

- [ ] **Refresh shell-out tests** — use a mock executable script:

  - [ ] Test: successful refresh. Create a temporary shell script that exits 0 and writes a valid auth file. Set `p.codexBinPath` to the script path. Assert `refreshToken` returns nil

  - [ ] Test: failed refresh with exit code. Create a temporary shell script that writes `"token expired"` to stderr and exits with code 1. Assert error contains `"Codex credential refresh failed (exit 1): token expired"`

  - [ ] Test: refresh timeout. Create a temporary shell script that sleeps for 60 seconds. Call `refreshToken` with a context that has a 1-second timeout. Assert error contains `"Codex credential refresh timed out after 30s"` or context deadline exceeded

- [ ] All test files compile and pass with `go test ./internal/provider/codex/...`

- [ ] Tests do not depend on the real `codex` binary being installed. All CLI interactions use temporary mock scripts

- [ ] Tests do not make real HTTP requests. All response parsing tests use constructed structs
