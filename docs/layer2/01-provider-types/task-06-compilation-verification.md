# Task 06: Compilation Verification and Package Layout

**Epic:** 01 — Provider Types & Interface
**Status:** ⬚ Not started
**Dependencies:** Task 01 (Provider Interface and Request Struct), Task 02 (Response, Usage, and StopReason Types), Task 03 (Message and ContentBlock Types), Task 04 (StreamEvent Type Hierarchy), Task 05 (Model, ToolCall, ToolResult, and ProviderError Types)

---

## Description

Verify that all types defined in Tasks 01-05 compile together, that the package layout supports provider sub-packages without circular imports, and that the public API surface is correct. Create placeholder sub-package directories for the three provider implementations (Anthropic, OpenAI, Codex) with minimal Go files that import the parent `provider` package, proving the import graph is acyclic. Add a compilation test file that uses compile-time interface assertions to lock down the type system.

## Acceptance Criteria

- [ ] Running `go build ./internal/provider/...` succeeds with zero errors and zero warnings
- [ ] Running `go vet ./internal/provider/...` succeeds with zero findings
- [ ] The following files exist in `internal/provider/`:
  - `provider.go` (Provider interface, Request, SystemBlock, CacheControl, ToolDefinition)
  - `response.go` (Response, Usage, StopReason)
  - `message.go` (Message, Role, ContentBlock, helper constructors)
  - `stream.go` (StreamEvent interface, TokenDelta, ThinkingDelta, ToolCallStart, ToolCallDelta, ToolCallEnd, StreamUsage, StreamError, StreamDone)
  - `types.go` (Model, ToolCall, ToolResult, ProviderError)
- [ ] Placeholder directory `internal/provider/anthropic/` exists with a file `anthropic.go` containing `package anthropic` and an import of the parent provider package: `import _ "github.com/<module>/internal/provider"` (using the correct module path from `go.mod`)
- [ ] Placeholder directory `internal/provider/openai/` exists with a file `openai.go` containing `package openai` and an import of the parent provider package
- [ ] Placeholder directory `internal/provider/codex/` exists with a file `codex.go` containing `package codex` and an import of the parent provider package
- [ ] Running `go build ./internal/provider/anthropic/` succeeds, confirming no circular import between `provider` and `provider/anthropic`
- [ ] Running `go build ./internal/provider/openai/` succeeds, confirming no circular import between `provider` and `provider/openai`
- [ ] Running `go build ./internal/provider/codex/` succeeds, confirming no circular import between `provider` and `provider/codex`
- [ ] A test file `internal/provider/provider_test.go` exists with `package provider_test` (external test package) containing compile-time interface satisfaction checks:
  ```go
  // Compile-time assertions that all StreamEvent variants satisfy the interface
  var _ provider.StreamEvent = provider.TokenDelta{}
  var _ provider.StreamEvent = provider.ThinkingDelta{}
  var _ provider.StreamEvent = provider.ToolCallStart{}
  var _ provider.StreamEvent = provider.ToolCallDelta{}
  var _ provider.StreamEvent = provider.ToolCallEnd{}
  var _ provider.StreamEvent = provider.StreamUsage{}
  var _ provider.StreamEvent = provider.StreamError{}
  var _ provider.StreamEvent = provider.StreamDone{}

  // Compile-time assertion that ProviderError satisfies the error interface
  var _ error = (*provider.ProviderError)(nil)
  ```
- [ ] The test file includes a basic `TestNewUserMessage` that creates a user message with `NewUserMessage("hello")`, asserts `Role == RoleUser`, and asserts the content unmarshals to the string `"hello"`
- [ ] The test file includes a basic `TestContentBlocksFromRaw` that marshals a `[]ContentBlock` containing one `NewTextBlock("test")` and one `NewToolUseBlock("tc_1", "file_read", json.RawMessage(`{"path":"/tmp"}`))`, then round-trips through `ContentBlocksFromRaw` and asserts the two blocks are reconstructed correctly (Type, Text, ID, Name fields match)
- [ ] The test file includes a `TestProviderErrorRetriable` that creates `ProviderError` values with `NewProviderError` using status codes 429, 500, 502, 503 and asserts `Retriable == true` for each; creates errors with status codes 400, 401, 403 and asserts `Retriable == false` for each; creates an error with status code 0 and a non-nil `Err` and asserts `Retriable == true`
- [ ] The test file includes a `TestUsageAdd` that creates two `Usage` values, calls `Add`, and asserts all four fields (`InputTokens`, `OutputTokens`, `CacheReadTokens`, `CacheCreationTokens`) are correctly summed
- [ ] Running `go test ./internal/provider/...` passes all tests
- [ ] No file in `internal/provider/` imports any package from outside the standard library (the provider types package has zero external dependencies)
