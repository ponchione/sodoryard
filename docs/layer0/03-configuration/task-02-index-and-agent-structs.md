# Task 02: Index and Agent Config Structs

**Epic:** 03 — Configuration Loading
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Define the `Index` and `Agent` config section structs with YAML tags, covering all fields from docs 04 and 05.

## Acceptance Criteria

- [ ] `Index` struct: `include`, `exclude` (string slices), `max_rag_results`, `max_tree_lines`, `auto_reindex`, `max_file_size_bytes`, `max_total_file_size_bytes`
- [ ] `Agent` struct: `max_iterations_per_turn`, `loop_detection_threshold`, `tool_output_max_tokens`, `shell_timeout_seconds`, `shell_denylist` (string slice), `extended_thinking`, `cache_system_prompt`, `cache_assembled_context`, `cache_conversation_history`
- [ ] Both structs wired into the root `Config`
