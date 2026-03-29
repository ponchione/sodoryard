# Task 03: Context Assembly and Brain Config Structs

**Epic:** 03 — Configuration Loading
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Define the `Context` and `Brain` config section structs with YAML tags, covering the current v0.1 fields plus any explicitly retained future-facing brain settings from docs 06 and 09.

## Acceptance Criteria

- [ ] `Context` struct with the current v0.1 fields: `max_assembled_tokens`, `max_chunks`, `max_explicit_files`, `convention_budget_tokens`, `git_context_budget_tokens`, `relevance_threshold`, `structural_hop_depth`, `structural_hop_budget`, `momentum_lookback_turns`, `compression_threshold`, `compression_head_preserve`, `compression_tail_preserve`, `compression_model`, `emit_context_debug`, `store_assembly_reports`
- [ ] `Brain` struct with v0.1 reactive-tool fields: `enabled`, `vault_path`, `obsidian_api_url`, `obsidian_api_key`
- [ ] If future-facing brain retrieval fields are retained in config loading, they are documented as reserved for v0.2+: `embedding_model`, `chunk_at_headings`, `reindex_on_startup`, `max_brain_tokens`, `brain_relevance_threshold`, `include_graph_hops`, `graph_hop_depth`, `log_brain_queries`
- [ ] Both structs wired into the root `Config`
