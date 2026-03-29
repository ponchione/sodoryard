# L1-E06 — Description Generator

**Layer:** 1 — Code Intelligence
**Epic:** 06
**Status:** ⬜ Not Started
**Dependencies:** L1-E01 (types & interfaces), L0-E03 (config)

---

## Description

Implement the description generator that sends file content to a local LLM and receives back semantic descriptions for each function/type in the file. The describer makes one LLM call per file, sending truncated file content plus relationship context (calls, called_by, types_used, implements), and parses the response as a JSON array of `{name, description}` pairs. These descriptions become the primary embedding text — they're what makes "auth middleware" match a function that handles JWT validation.

This is the quality-critical component of the RAG pipeline. Description quality directly determines embedding quality, which determines retrieval quality. Ports from topham's `internal/rag/describer.go`.

---

## Package

`internal/rag/describer/` — local LLM description generation.

---

## Definition of Done

- [ ] Implements the `Describer` interface from [[L1-E01-types-and-interfaces]]
- [ ] `DescribeFile(ctx context.Context, fileContent string, relationshipContext string) ([]Description, error)` — sends file content to local LLM, returns a `[]Description` for each function/type
- [ ] File content is truncated to 6000 characters by the CALLER before passing to `DescribeFile` (the describer does not truncate internally)
- [ ] Relationship context is received as a pre-formatted string from the caller — the describer does not build it from chunks
- [ ] Prompt construction: instructs the LLM to return a JSON array of `[{"name": "FuncName", "description": "1-2 sentence semantic summary"}]`
- [ ] Response parsing: extracts JSON from LLM response (handles potential markdown code fences around JSON), validates structure, returns `[]Description`
- [ ] **Graceful failure:** if the LLM call fails (container down, timeout) or returns invalid JSON, the describer returns an empty slice — NOT an error. The file is still indexed, just without descriptions. Log the failure for debugging
- [ ] HTTP POST to configurable base URL (default `http://localhost:8080`) using the OpenAI-compatible `/v1/chat/completions` endpoint
- [ ] Configurable via `internal/config/`: LLM container URL, model name, temperature, max tokens, timeout
- [ ] Context cancellation support
- [ ] Unit tests with a mock HTTP server: successful description, invalid JSON response, timeout, empty file
- [ ] Unit tests for relationship context formatting
- [ ] Unit tests for response JSON extraction (with and without markdown code fences)

---

## Architecture References

- [[04-code-intelligence-and-rag]] — "Component: Description Generation" (file truncation, relationship context, quality risk)
- [[02-tech-stack-decisions]] — "Local LLM Inference: Docker Container" (port 8080, configurable model)
- topham source: `internal/rag/describer.go`

---

## Notes

- The description generator is the quality bottleneck of the entire RAG pipeline. Bad descriptions → bad embeddings → bad retrieval. This is explicitly called out in [[04-code-intelligence-and-rag]] as the most important variable to monitor.
- The prompt should be firm about JSON-only output. Local models sometimes include preamble text before JSON — the parser must handle this by extracting the JSON array from anywhere in the response.
- Relationship context helps the LLM write better descriptions. Without it, the LLM might describe `ValidateToken` as "validates a token" — useless. With context showing it's called by `AuthMiddleware` and uses `Claims` type, the description becomes "Validates a JWT token extracted from the Authorization header and returns the associated user claims for the authenticated request."
- The 6000-character file truncation is a pragmatic limit for local models with smaller context windows. Files larger than this are truncated, and functions appearing after the truncation point may get no description (or a less accurate one). This is acceptable — the function's signature still gets embedded even without a description.
- The graceful failure behavior is critical for the indexing pipeline. A single LLM failure should never block indexing of an entire project. Log it, skip the descriptions, move on.
