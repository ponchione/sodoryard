# Shunter Project Memory Runtime Spec

Status: proposed
Date: 2026-05-03

## Objective

Replace Sodoryard's Obsidian-backed `.brain/` source of truth with a Shunter
runtime that owns durable project memory.

The goal is broader than replacing markdown document storage. Shunter should
become the project-local state runtime for the brain and, over time, adjacent
operator state currently stored in SQLite when that state is part of agent
memory or orchestration:

- brain documents, revisions, links, tags, frontmatter, and operation logs
- conversation records, messages, tool executions, subcalls, and context reports
- chain, step, event, receipt, launch, and launch-preset state
- brain index state and derived-index metadata

LanceDB remains the vector index for code and brain semantic retrieval. Shunter
owns the canonical structured state and document content. LanceDB stores
rebuildable embeddings and nearest-neighbor search data.

## Desired End State

The live runtime path no longer reads or writes `.brain/` during normal use.
Agents interact with project memory through Sodoryard tools and runtime
interfaces backed by Shunter.

The human-facing affordance is Sodoryard itself: `yard tui`, `yard serve`,
chain views, receipt views, brain search/read/write tools, and explicit export
commands. Obsidian compatibility is not a requirement.

Markdown remains a useful document body format because it carries headings,
frontmatter, tags, links, and plain-text readability. It is no longer the
storage backend.

## Non-Goals

- Do not make LanceDB durable source of truth.
- Do not require Obsidian or an Obsidian-compatible vault.
- Do not let multiple independent processes open and mutate the same Shunter
  data directory directly.
- Do not push embedding generation, file IO, network calls, or model calls into
  Shunter reducers. Reducers must stay synchronous and deterministic.
- Do not require the first migration slice to move every SQLite table at once.

## How Shunter Persists State

Sodoryard defines a Shunter module with tables, reducers, declared reads, and
views. `shunter.Build` opens or bootstraps a runtime from `Config.DataDir`.
`Runtime.Start` starts the executor, scheduler, subscription fanout, and
durability worker.

All mutations enter through reducers via `Runtime.CallReducer`.

Reducer flow:

1. The caller supplies JSON args.
2. Shunter invokes the reducer on its serialized executor goroutine.
3. The reducer mutates only the transaction-scoped `ctx.DB` surface.
4. If the reducer returns an error or panics, Shunter rolls back.
5. If it succeeds, Shunter commits the transaction to in-memory committed state.
6. Shunter assigns a monotonic transaction ID and produces a changeset.
7. The durability worker writes the changeset to append-only segment logs.
8. Recovery restores the latest valid snapshot plus log replay.

On disk, the Shunter data directory contains binary log segments named by
starting transaction ID, sparse offset-index sidecars, and snapshots under
`snapshots/<txid>/snapshot`.

For Sodoryard memory writes, the adapter should not report success until the
commit is durable. Current Shunter health exposes durable transaction progress;
the long-term API should add a public `Runtime.WaitUntilDurable(txID)` helper so
Sodoryard does not have to poll health.

## Runtime Ownership Model

Sodoryard should treat Shunter as a single owner runtime per project.

Default data directory:

```yaml
memory:
  backend: shunter
  shunter_data_dir: .yard/shunter/project-memory
```

The parent Yard process owns this runtime:

- `yard serve`
- `yard tui`
- `yard chain start`
- any long-lived operator process that spawns step engines

Child `tidmouth run` subprocesses must not open the same Shunter data directory
directly. They should talk to the parent-owned memory runtime through a local
Sodoryard memory service.

Initial transport should be a narrow internal RPC surface, not Shunter protocol
exposure everywhere. The service can run in-process for `yard serve` / `yard
tui` and expose a loopback HTTP or Unix-socket endpoint to step subprocesses.
Later, Shunter's WebSocket protocol and subscriptions can power richer live UI
surfaces.

## Module Boundary

Use one Shunter module for project memory:

```text
yard_project_memory
```

A single module gives atomic reducers across related state. For example, a
chain step can append a message, record a tool execution, update step state,
write a receipt document, append a chain event, and mark brain index state
stale in one transaction.

Splitting into multiple Shunter modules should be reserved for hard isolation
requirements, because cross-module transactions are not the desired first
problem.

## Table Model

Use stable table IDs and explicit schema versioning. Prefer internal `uint64`
primary keys plus stable public string IDs where needed.

### Documents

`documents`

- `id uint64` primary key
- `path string` unique
- `kind string`
- `title string`
- `status string`
- `content_hash string`
- `frontmatter_json string`
- `tags_json string`
- `created_at_ns int64`
- `updated_at_ns int64`
- `created_by string`
- `source_conversation_id string`
- `source_chain_id string`

`document_chunks`

- `id uint64` primary key
- `document_id uint64`
- `chunk_index uint64`
- `heading string`
- `body string`
- `body_hash string`
- `token_count uint64`

Document bodies should be chunked rather than stored as one very large row.
This respects Shunter's row-size bounds and aligns with LanceDB brain chunking.
`ReadDocument(path)` reconstructs markdown by ordering `document_chunks`.

`document_links`

- `id uint64` primary key
- `source_document_id uint64`
- `source_path string`
- `target_path string`
- `link_text string`

`document_revisions`

- `id uint64` primary key
- `document_id uint64`
- `path string`
- `operation string`
- `old_hash string`
- `new_hash string`
- `actor string`
- `created_at_ns int64`
- `summary string`

`memory_operations`

- `id uint64` primary key
- `operation string`
- `actor string`
- `role string`
- `target_path string`
- `request_json string`
- `result_json string`
- `created_at_ns int64`

### Conversation State

These replace or mirror current SQLite tables before the SQLite dependency is
removed for this state class.

`conversations`

- `id string` primary/public ID
- `title string`
- `model string`
- `provider string`
- `created_at_ns int64`
- `updated_at_ns int64`

`messages`

- `id uint64` primary key
- `conversation_id string`
- `role string`
- `content string`
- `tool_use_id string`
- `tool_name string`
- `turn_number uint64`
- `iteration uint64`
- `sequence string`
- `is_compressed bool`
- `is_summary bool`
- `compressed_turn_start uint64`
- `compressed_turn_end uint64`
- `created_at_ns int64`

`tool_executions`

- `id uint64` primary key
- `conversation_id string`
- `turn_number uint64`
- `iteration uint64`
- `tool_use_id string`
- `tool_name string`
- `input_json string`
- `output_size uint64`
- `normalized_size uint64`
- `error string`
- `success bool`
- `duration_ms uint64`
- `created_at_ns int64`

`sub_calls`

- `id uint64` primary key
- `conversation_id string`
- `message_id uint64`
- `turn_number uint64`
- `iteration uint64`
- `provider string`
- `model string`
- `purpose string`
- `tokens_in uint64`
- `tokens_out uint64`
- `cache_read_tokens uint64`
- `cache_creation_tokens uint64`
- `latency_ms uint64`
- `success bool`
- `error_message string`
- `created_at_ns int64`

`context_reports`

- `id uint64` primary key
- `conversation_id string`
- `turn_number uint64`
- `analysis_latency_ms uint64`
- `retrieval_latency_ms uint64`
- `total_latency_ms uint64`
- `needs_json string`
- `signals_json string`
- `rag_results_json string`
- `brain_results_json string`
- `graph_results_json string`
- `explicit_files_json string`
- `budget_total uint64`
- `budget_used uint64`
- `budget_breakdown_json string`
- `token_budget_json string`
- `agent_used_search_tool bool`
- `agent_read_files_json string`
- `context_hit_rate string`
- `created_at_ns int64`

### Chain State

`chains`

- `id string` primary/public ID
- `source_specs string`
- `source_task string`
- `status string`
- `summary string`
- `total_steps uint64`
- `total_tokens uint64`
- `total_duration_secs uint64`
- `resolver_loops uint64`
- `max_steps uint64`
- `max_resolver_loops uint64`
- `max_duration_secs uint64`
- `token_budget uint64`
- `started_at_ns int64`
- `completed_at_ns int64`
- `created_at_ns int64`
- `updated_at_ns int64`

`steps`

- `id string` primary/public ID
- `chain_id string`
- `sequence_num uint64`
- `role string`
- `task string`
- `task_context string`
- `status string`
- `verdict string`
- `receipt_path string`
- `tokens_used uint64`
- `turns_used uint64`
- `duration_secs uint64`
- `exit_code int64`
- `error_message string`
- `started_at_ns int64`
- `completed_at_ns int64`
- `created_at_ns int64`

`events`

- `id uint64` primary key
- `chain_id string`
- `step_id string`
- `event_type string`
- `event_data_json string`
- `created_at_ns int64`

`launches`

- `id string` primary/public ID
- `status string`
- `mode string`
- `role string`
- `allowed_roles_json string`
- `roster_json string`
- `source_task string`
- `source_specs string`
- `created_at_ns int64`
- `updated_at_ns int64`

`launch_presets`

- `id string` primary/public ID
- `name string`
- `mode string`
- `role string`
- `allowed_roles_json string`
- `roster_json string`
- `created_at_ns int64`
- `updated_at_ns int64`

### Derived Index Metadata

LanceDB vectors remain outside Shunter, but Shunter should own the metadata that
determines whether those vectors are fresh.

`brain_index_state`

- `id uint64` primary key
- `status string`
- `last_indexed_at_ns int64`
- `stale_since_ns int64`
- `stale_reason string`
- `updated_at_ns int64`

`brain_index_chunks`

- `id string` primary/public ID
- `document_id uint64`
- `document_path string`
- `chunk_index uint64`
- `chunk_hash string`
- `lancedb_key string`
- `indexed_at_ns int64`

Code index state and graph state can move later. The first Shunter memory
runtime should not block on replacing code RAG storage.

## Reducer Surface

Reducers are the write API. They accept JSON arguments so traces and failure
artifacts stay readable.

Brain reducers:

- `import_documents_batch`
- `write_document`
- `patch_document`
- `delete_document`
- `record_brain_search`
- `mark_brain_index_stale`
- `mark_brain_index_clean`
- `upsert_brain_index_chunk`
- `delete_brain_index_chunks_for_document`

Conversation reducers:

- `create_conversation`
- `update_conversation`
- `append_message`
- `compress_messages`
- `record_tool_execution`
- `record_sub_call`
- `store_context_report`

Chain reducers:

- `create_chain`
- `update_chain_status`
- `upsert_step`
- `append_chain_event`
- `record_step_receipt`
- `upsert_launch`
- `delete_launch`
- `upsert_launch_preset`
- `delete_launch_preset`

Reducer rules:

- Parse and validate JSON args at the reducer boundary.
- Perform markdown parsing, chunk planning, token counting, and link extraction
  before calling reducers when possible.
- Never call embeddings, LanceDB, filesystem, providers, or network services
  from reducers.
- Store enough operation metadata to reproduce failures and audit writes.
- Return user errors before mutating when validation fails.
- Use one transaction for related changes that must be observed atomically.

## Read Surface

Sodoryard should expose application-level interfaces over Shunter, not leak
Shunter row shapes through the agent tools.

Initial local interfaces:

- `brain.Backend`
- conversation store
- chain store
- context report store
- launch store

Dynamic reads such as `ReadDocument(path)` can use `Runtime.Read` and scan or
use adapter-maintained indexes. Fixed UI and protocol views can use declared
queries/views.

Declared query/view examples:

- `recent_documents`
- `recent_conversations`
- `conversation_messages`
- `active_chains`
- `chain_events`
- `latest_context_reports`
- `brain_index_status`

Shunter subscriptions should eventually drive live UI updates for active
chains, events, launches, and memory writes.

## Keyword And Semantic Search

Semantic search stays LanceDB-backed.

Keyword search should initially be implemented in the Shunter adapter by
reading document metadata/chunks and applying Sodoryard's deterministic lexical
ranking. If this becomes too slow, add an inverted-index table:

`document_terms`

- `id uint64` primary key
- `term string`
- `document_id uint64`
- `chunk_id uint64`
- `count uint64`

Reducers that write documents can replace term rows for the affected document
inside the same transaction.

`yard brain index` changes from vault scanning to Shunter scanning:

1. list Shunter documents
2. read chunks and metadata
3. embed chunks into LanceDB
4. write `brain_index_chunks`
5. mark `brain_index_state` clean

## Agent Harness Interaction

Agent tools should not call Shunter directly. They should call Sodoryard
runtime interfaces that are backed by Shunter.

Example write path:

```text
agent brain_write tool
  -> tool path permission validation
  -> markdown parse/chunk/link extraction
  -> ShunterMemoryBackend.WriteDocument
  -> Runtime.CallReducer("write_document", args)
  -> wait until tx durable
  -> return tool result
```

Example chain receipt path:

```text
step engine completes
  -> parent chain runner receives receipt payload
  -> Shunter reducer writes/updates document under receipts/...
  -> same reducer updates step status and appends chain event
  -> live UI subscription receives event/update
```

Example context assembly path:

```text
turn analyzer emits brain intent
  -> Shunter-backed brain search
  -> optional LanceDB semantic search
  -> Shunter-backed brain reads for selected chunks/documents
  -> context report stored back into Shunter
```

For child engine subprocesses:

```text
tidmouth run
  -> receives SODORYARD_MEMORY_ENDPOINT
  -> uses remote Sodoryard memory client
  -> parent process owns Shunter Runtime
```

This preserves single-writer ownership of the Shunter data directory.

## Configuration

Proposed config shape:

```yaml
memory:
  backend: shunter
  shunter_data_dir: .yard/shunter/project-memory
  durable_ack: true
  rpc:
    transport: unix
    path: .yard/run/memory.sock

brain:
  enabled: true
  backend: shunter
  log_brain_queries: true
  max_brain_tokens: 8000
  brain_relevance_threshold: 0.30
```

During migration, `brain.vault_path` may remain only for import/export commands.
It should not be consulted by live brain tools when `brain.backend: shunter`.

## Migration Plan

### Phase 0: Contract And Spike

- Add this spec.
- Build a small Shunter module in Sodoryard tests or a new package.
- Prove document write/read/patch/list/search through `brain.Backend`.
- Prove restart recovery from `.yard/shunter/project-memory`.
- Add durability wait behavior before returning successful writes.

Exit criteria:

- Shunter-backed `brain.Backend` passes current vault-backend parity tests.
- No live code path reads `.brain/` when configured for Shunter backend.

### Phase 1: Brain Backend Replacement

- Add `internal/brain/shunterbackend`.
- Add memory runtime construction to `internal/runtime`.
- Change brain tools to use Shunter backend through the existing interface.
- Change `yard brain index` to read from Shunter.
- Add `yard brain import --from .brain`.
- Add `yard brain export --to <dir>` for backup and inspection.

Exit criteria:

- Brain read/write/update/search works without Obsidian or `.brain/`.
- LanceDB brain index rebuilds from Shunter documents.
- Import/export is deterministic enough for backup validation.

### Phase 2: Conversations And Context Reports

- Add Shunter-backed conversation store.
- Move `conversations`, `messages`, `tool_executions`, `sub_calls`, and
  `context_reports` to Shunter.
- Keep SQLite reads only behind compatibility/adaptation if needed during the
  transition.

Exit criteria:

- `yard serve`, `yard tui`, and agent turn persistence work from Shunter.
- Context inspector reads reports from Shunter.
- Existing conversation tests pass against Shunter-backed store.

### Phase 3: Chains, Events, Receipts, Launches

- Add Shunter-backed chain store.
- Store receipts as Shunter documents and chain/step records atomically.
- Move `chains`, `steps`, `events`, `launches`, and `launch_presets`.
- Use Shunter subscriptions for live event/chain UI where practical.

Exit criteria:

- `yard chain start/status/logs/receipt/pause/cancel/resume` works with
  Shunter state.
- Step subprocesses use parent memory RPC instead of opening storage directly.
- Chain receipt writes and status updates are atomic from the operator view.

### Phase 4: SQLite Reduction

- Remove migrated tables from the required SQLite runtime path.
- Keep SQLite only for surfaces that are deliberately not project memory yet,
  such as transitional code graph storage, if any remain.
- Reassess whether code graph metadata should move to Shunter as well.

Exit criteria:

- Project memory and orchestration state survive restart through Shunter alone.
- `.brain/` is optional export/import material, not live runtime state.

## Validation

Required validation before using Shunter memory by default:

- Shunter full suite, pinned Staticcheck, targeted race suite.
- OpsBoard canary full and stress gates.
- Sodoryard Shunter brain parity tests.
- Import/export round trip from existing `.brain/`.
- Crash/restart tests after document writes, conversation writes, and chain
  event writes.
- Multi-process harness test: parent owns runtime, child step writes through
  memory RPC, parent restarts and state recovers.
- LanceDB rebuild from Shunter documents.
- Role path permission tests for brain writes.
- UI/API tests for recent chains, receipts, brain hits, and context reports.

## Open Questions

- Shunter should expose a public durable-wait API. Health polling is acceptable
  for a spike but should not be the long-term adapter contract.
- Shunter's local dynamic read ergonomics are lower-level than a store API.
  The adapter may need small helper indexes or typed row decoding helpers.
- Large document chunking needs a stable reconstruction contract.
- Export format should be human-readable markdown directories, JSONL, or both.
- Code graph state may also belong in Shunter eventually, but moving it is not
  required for removing Obsidian.
- Shunter schema migration tooling must be exercised against real Sodoryard
  memory schema evolution before the backend becomes default.
