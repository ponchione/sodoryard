# Task 01: Embedding Config Struct and Defaults

**Epic:** 04 — Embedding Client
**Status:** ⬚ Not started
**Dependencies:** L0-E03 (configuration loading)

---

## Description

Add an `Embedding` config section to the root `Config` struct in `internal/config/`. This provides the embedding client with its configurable fields: container base URL, model name, batch size, HTTP timeout, and the query prefix used for asymmetric retrieval. All fields have sensible defaults so the embedding client works out of the box with a local nomic-embed-code container.

## Acceptance Criteria

- [ ] `Embedding` struct defined in `internal/config/` with YAML tags and the following fields:
  - `base_url` (string) — default `"http://localhost:8081"`
  - `model` (string) — default `"nomic-embed-code"`
  - `batch_size` (int) — default `32`
  - `timeout_seconds` (int) — default `30`
  - `query_prefix` (string) — default `"Represent this query for searching relevant code: "`
- [ ] `Embedding` struct wired into the root `Config` struct as `Embedding Embedding \`yaml:"embedding"\``
- [ ] Defaults applied during config loading (same pattern as other config sections)
- [ ] Corresponding YAML block documented in the example config file if one exists:
  ```yaml
  embedding:
    base_url: "http://localhost:8081"
    model: "nomic-embed-code"
    batch_size: 32
    timeout_seconds: 30
    query_prefix: "Represent this query for searching relevant code: "
  ```
- [ ] Validation: `batch_size` must be > 0, `timeout_seconds` must be > 0, `base_url` must not be empty
- [ ] Compiles cleanly: `go build ./internal/config/...`
