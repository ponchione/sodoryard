# Task 08: Body Truncation Enforcement

**Epic:** 02 — Tree-sitter Parsers
**Status:** ⬚ Not started
**Dependencies:** Task 02, L1-E01 (RawChunk type, MaxBodyLength constant)

---

## Description

Implement body truncation as a post-processing step in the parser dispatcher. After any parser returns its `[]RawChunk`, the dispatcher must enforce the `MaxBodyLength` constraint (2000 characters) on every chunk's `Body` field before returning results. This prevents oversized chunks from consuming excessive embedding and storage resources.

## Acceptance Criteria

- [ ] `truncateBody(body string, maxLen int) string` function in `internal/codeintel/treesitter/truncate.go` that:
  - Returns the body unchanged if `len(body) <= maxLen`
  - Truncates to `maxLen` characters if `len(body) > maxLen`
  - Truncation is byte-safe: if `maxLen` falls in the middle of a multi-byte UTF-8 character, the truncation point is moved backward to the last complete rune boundary
- [ ] The `MaxBodyLength` constant from L1-E01 (`internal/codeintel/`) is used as the truncation limit (value: 2000)
- [ ] The dispatcher's `Parse` method applies `truncateBody` to every `RawChunk.Body` in the result slice before returning. truncateBody is applied after the fallback-on-error path (Task 02 AC7/AC8) — if the Go parser errors and the dispatcher falls to the fallback chunker, truncation still applies to the fallback results. Truncation always runs as the final step before returning, regardless of which parser produced the chunks.
- [ ] Truncation does NOT modify `Signature`, `Name`, or any other `RawChunk` field -- only `Body`
- [ ] Truncation is applied uniformly regardless of which parser produced the chunks (Go, TypeScript, Python, Markdown, Fallback)
- [ ] Unit test: a body of exactly 2000 characters is not truncated
- [ ] Unit test: a body of 2001 characters is truncated to 2000 characters
- [ ] Unit test: a body containing multi-byte UTF-8 characters near the 2000-character boundary is truncated to a valid UTF-8 string
- [ ] Unit test: an empty body is returned unchanged

## Sizing Note

Estimated ~30-45 minutes. While the core function is simple, UTF-8 rune boundary handling (AC4) and the 4 unit tests (AC5-AC8) with edge cases provide enough substance. The wiring into the dispatcher (AC3) also requires understanding the dispatcher's flow.
