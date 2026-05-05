# Remaining Legacy Items

Last updated: 2026-05-05

This is the short list of legacy-looking items intentionally left in place after the Phase 0-6 Shunter audit because removing them still has non-Shunter fallback, test, derived-index, or operator impact.

## Not Safe To Remove Yet

- SQLite/appdb stores and generated queries: Shunter-mode runtime paths use project memory for conversations, chains, provider call tracking, tool execution records, context reports, and index state. The SQLite adapters remain for non-Shunter fallback branches, sqlc/schema tests, metrics tests, and historical compatibility tests. Removing them is a separate deletion pass, not a Phase 0-6 blocker.
- `.yard/graph.db`: the structural code graph is still a derived SQLite store used by code indexing and retrieval. It is not canonical brain memory, but deleting it would remove current graph functionality.
- Internal `tidmouth` engine binary: chain execution still spawns `tidmouth run`/`tidmouth index` through the internal subprocess contract. Removing it requires a spawn-contract redesign, not just command cleanup.
- Codex private auth store at `~/.sirtopham/auth.json`: provider auth docs and tests still treat this as the authoritative private store after bootstrap import. Removing or renaming it needs an auth-store migration plan to avoid breaking existing logins.
- Sir Topham Hatt orchestrator persona and prompt asset: the `orchestrator` built-in role, launch presets, README role table, and embedded prompt names still expose this persona. Renaming it is a product/persona migration, not a Shunter brain cleanup.
- Legacy backend test fixtures: a few tests still construct `memory.backend = "legacy"` or `brain.backend = "vault"` to exercise the remaining SQLite fallback paths or validation boundaries. They should disappear with the fallback stores.
- Historical specs under `docs/specs/`: several superseded drafts still describe the old vault/SQLite design. They are marked as non-current where relevant and should be curated in a documentation-history pass instead of deleted piecemeal.
- `.brain` rejection/exclusion references: tests and path filters still mention `.brain` to prove Shunter-native behavior rejects old brain paths and ignores old state directories. These are guardrails, not compatibility support.

## Removed In This Cleanup

- Obsidian/vault initializer helpers and tests.
- Initializer `.brain` template placeholders and template-listing helpers.
- `brain.vault_path` config field, default, helper, and test fixtures.
- `SIRTOPHAM_LOG_LEVEL` environment compatibility alias.
- Stale `.brain` setup in tests that did not need legacy brain state.
