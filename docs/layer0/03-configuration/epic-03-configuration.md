# Epic 03: Configuration Loading

**Phase:** Build Phase 1 — Layer 0
**Status:** ⬚ Not started
**Dependencies:** [[01-project-scaffolding]]
**Blocks:** [[04-sqlite-connection]] (needs database file path from config)

---

## Description

Define the full `sirtopham.yaml` configuration schema as Go structs, covering all sections that appear across the architecture docs: provider routing (doc 03), index settings (doc 04), agent loop config (doc 05), context assembly config (doc 06), brain config (doc 09), plus top-level settings (project root, log level, server port). Implement YAML parsing with sensible defaults for every field, validation (port ranges, path existence checks, enum values), and environment variable overrides for sensitive fields (API keys).

---

## Definition of Done

- [ ] `internal/config/` package loads a YAML file into a typed Config struct
- [ ] Every config section from docs 03-06 and 09 is represented with correct types and default values
- [ ] Missing file produces a valid default config; malformed file produces a clear error with line context
- [ ] Validation rejects invalid values (unknown provider types, out-of-range thresholds, negative token budgets) with specific error messages
- [ ] Environment variable overrides work for at least: `ANTHROPIC_API_KEY`, `OPENROUTER_API_KEY`, `SIRTOPHAM_LOG_LEVEL`
- [ ] Unit tests cover: default generation, partial override, invalid field rejection, env var precedence

---

## Config Sections to Cover

These config blocks appear across the architecture docs and must be represented:

**Top-level:** project root, log level, log format, server port/host

**Provider routing (doc 03):**
```yaml
routing:
  default:
    provider: anthropic
    model: claude-sonnet-4-6
  fallback:
    provider: local
    model: qwen2.5-coder-7b
providers:
  local:
    type: openai-compatible
    base_url: http://localhost:8080/v1
    model: qwen2.5-coder-7b
```

**Index settings (doc 04):**
```yaml
index:
  include: ["**/*.go", "**/*.sql", "**/*.md"]
  exclude: ["**/.git/**", "**/vendor/**"]
  max_rag_results: 30
  max_tree_lines: 200
  auto_reindex: true
  max_file_size_bytes: 51200
  max_total_file_size_bytes: 524288
```

**Agent loop (doc 05):**
```yaml
agent:
  max_iterations_per_turn: 50
  loop_detection_threshold: 3
  tool_output_max_tokens: 50000
  shell_timeout_seconds: 120
  shell_denylist: ["rm -rf /", "git push --force"]
  extended_thinking: true
  cache_system_prompt: true
  cache_assembled_context: true
  cache_conversation_history: true
```

**Context assembly (doc 06):**
```yaml
context:
  max_assembled_tokens: 30000
  max_chunks: 25
  max_explicit_files: 5
  convention_budget_tokens: 3000
  git_context_budget_tokens: 2000
  relevance_threshold: 0.35
  structural_hop_depth: 1
  structural_hop_budget: 10
  momentum_lookback_turns: 2
  compression_threshold: 0.50
  compression_head_preserve: 3
  compression_tail_preserve: 4
  compression_model: "local"
  emit_context_debug: true
  store_assembly_reports: true
```

**Brain (doc 09):**
```yaml
brain:
  enabled: true
  vault_path: ~/obsidian-vaults/sirtopham-brain
  obsidian_api_url: http://localhost:27124
  obsidian_api_key: ""
  # v0.2+ smart-retrieval fields (keyword-only reactive tools are the v0.1 scope)
  embedding_model: "nomic-embed-code"
  chunk_at_headings: true
  reindex_on_startup: true
  max_brain_tokens: 8000
  brain_relevance_threshold: 0.30
  include_graph_hops: true
  graph_hop_depth: 1
  log_brain_queries: true
```

---

## Architecture References

- [[03-provider-architecture]] — Provider routing config, credential paths
- [[04-code-intelligence-and-rag]] — Indexing config, glob patterns, RAG parameters
- [[05-agent-loop]] — Agent loop config, tool limits, caching flags
- [[06-context-assembly]] — Context budget, thresholds, compression config
- [[09-project-brain]] — Brain/Obsidian config
