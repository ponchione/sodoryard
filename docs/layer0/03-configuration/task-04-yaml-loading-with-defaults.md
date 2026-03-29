# Task 04: YAML File Loading with Default Values

**Epic:** 03 — Configuration Loading
**Status:** ⬚ Not started
**Dependencies:** Task 01, Task 02, Task 03

---

## Description

Implement the YAML loading function that reads `sirtopham.yaml` into the `Config` struct. If the file is missing, return a fully populated default config. If the file is malformed, return a clear error with line/context information. Every field must have a sensible default value.

## Acceptance Criteria

- [ ] Exported `Load(path string) (*Config, error)` function (or similar)
- [ ] Missing file returns a valid default config (no error)
- [ ] Malformed YAML returns an error with line number or context
- [ ] Partial YAML files merge with defaults (only specified fields override)
- [ ] All fields have sensible default values matching the architecture docs. Representative defaults (see `sirtopham.yaml.example` and specs for full list):
  - `server.port`: `8090`
  - `log_level`: `"info"`, `log_format`: `"text"`
  - `context.max_assembled_tokens`: `30000`
  - `context.relevance_threshold`: `0.35`
  - `context.compression_threshold`: `0.50`
  - `agent.max_iterations_per_turn`: `50`
  - `agent.shell_timeout_seconds`: `120`
  - `brain.enabled`: `true`
  - `brain.max_brain_tokens`: `8000` (reserved for v0.2 if these future-facing brain retrieval fields are kept in config)
