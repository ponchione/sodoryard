# Task 01: Compression Trigger Detection (Preflight and Exact)

**Epic:** 07 — Compression Engine
**Status:** ⬚ Not started
**Dependencies:** Epic 01; Layer 2: Epic 01 (provider types for context window metadata)

---

## Description

Implement the compression trigger detection functions that determine when conversation history compression should fire. Three trigger modes: preflight (rough estimate before LLM call using `total_chars / 4`), post-response (exact using `prompt_tokens` from API usage response), and reactive (when the API returns a context length exceeded error). The threshold is configurable via `CompressionThreshold` (default 0.50, meaning 50% of model context window). These functions are called by the agent loop at appropriate points — the compression engine provides the check functions, the agent loop decides when to invoke compression.

## Acceptance Criteria

- [ ] **Preflight check (rough estimate):** Estimates total message tokens as `total_chars / 4`. Returns `true` if this exceeds `CompressionThreshold` percentage of model context window. Called before each LLM API call.
- [ ] **Post-response check (exact):** Accepts `prompt_tokens` from the API usage response. Returns `true` if this exceeds `CompressionThreshold` percentage of model context window. Called after each LLM response.
- [ ] **Reactive trigger:** Returns `true` when the API returns HTTP 400 with `"context_length_exceeded"` or HTTP 413. Called from the agent loop's error handling path.
- [ ] Threshold is configurable: `CompressionThreshold` (default 0.50)
- [ ] All three check functions accept model context limit as a parameter
- [ ] Package compiles with no errors
