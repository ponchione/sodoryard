# Shunter Project Memory Implementation Plan

Status: implementation-ready plan
Date: 2026-05-05

This plan replaces the earlier "project memory" sketch. The target is not a soft
MCP integration. The target is a full swap: Shunter becomes Sodoryard's canonical
project brain and operational state plane. The existing RAG stores stay, especially
the code retrieval stores, but they become rebuildable indexes over Shunter and
the workspace, not the source of truth.

## Objective

Sodoryard should have one local project memory runtime that owns:

- brain documents, conventions, receipts, and document history
- conversation history, summaries, tool executions, subcalls, and context reports
- chain state, steps, events, launches, and launch presets
- index freshness metadata for code, brain, and retrieval pipelines

The following should remain outside Shunter as derived or external state:

- source files in the workspace
- LanceDB vector stores for code and brain retrieval
- the code graph database
- exported Markdown backups
- provider APIs and embeddings calls

The final runtime path should not depend on the Obsidian MCP server, the `.brain`
filesystem vault, or `.yard/yard.db` as canonical memory.

## Current Ground Truth

### Shunter

The live Shunter codebase at `~/source/shunter` exposes a root Go API intended for
application embedding:

```go
mod := shunter.NewModule("yard_project_memory")
// mod.TableDef(...), mod.Reducer(...), mod.Query(...), mod.View(...)

rt, err := shunter.Build(mod, shunter.Config{
    DataDir: ".yard/shunter/project-memory",
})
if err != nil {
    return err
}
if err := rt.Start(ctx); err != nil {
    return err
}
defer rt.Close()
```

Useful public runtime methods include:

- `CallReducer(ctx, name, []byte, opts...)`
- `CallQuery(ctx, name, opts...)`
- `SubscribeView(ctx, name, queryID, opts...)`
- `Read(ctx, func(shunter.LocalReadView) error)`
- `Health()`
- `CreateSnapshot()`
- `CompactCommitLog(snapshotTxID)`
- `HTTPHandler()`
- `ListenAndServe(ctx)`
- `ExportSchema()` and `ExportContract()`

Reducer calls return `ReducerResult` values with a transaction id and status:

```go
res, err := rt.CallReducer(ctx, "write_document", payload)
if err != nil {
    return err
}
if res.Status != shunter.StatusCommitted {
    return fmt.Errorf("write_document failed: %s", res.Error)
}
```

Important constraints from current Shunter:

- Reducers execute synchronously on the executor path. They must not perform file
  I/O, network calls, embeddings, provider calls, or long blocking work.
- Reducer arguments and returns are raw `[]byte`; Sodoryard should use JSON for
  readability and traceability.
- `CallQuery` executes named declared SQL reads, but the root app API does not
  expose dynamic query arguments.
- `LocalReadView` currently exposes table scans, internal row-id lookups, and
  row counts, not public index seek/range helpers. `GetRow` takes Shunter's
  internal `RowID`, not an application primary-key value.
- The reducer-facing `ReducerDB` also lacks public index seek/range helpers.
  Reducers should not reach through `ReducerDB.Underlying()` to use lower-level
  store APIs.
- Shunter has internal index seek support, but it is not yet exposed through the
  root app and reducer APIs needed here.
- Nullable columns are rejected in v1. Schema must use non-nullable columns with
  sentinel values such as `""`, `0`, `false`, `"[]"`, and `"{}"`.
- Shunter timestamp values are microsecond precision. Sodoryard column names
  should use `*_unix_us` or `*_at_us`, not nanoseconds.
- Table ids are zero-based and assigned by module declaration order before
  Shunter appends system tables. Sodoryard must keep table declaration order
  stable and define explicit table id constants on its side.
- Shunter supports at most one primary-key column per table. Composite
  application identities must use deterministic single-column ids and/or unique
  secondary indexes.
- Online backup is not exposed as an app-level primitive. Yard should quiesce
  writes, snapshot or compact if useful, close the runtime, then copy the full
  Shunter data directory.

Two Shunter changes should land before making this the default:

1. Add a public durable wait helper.

```go
func (r *Runtime) WaitUntilDurable(ctx context.Context, txID types.TxID) error
```

Until that exists, Sodoryard can poll `Runtime.Health().Durability.DurableTxID`
after committed reducer calls, but that should be temporary.

2. Add public indexed reads for local reads and reducers.

For v1, table scans are acceptable only for low-cardinality surfaces and tests.
The real brain needs index lookup for paths, ids, conversations, chains, and turn
ranges. The exact Shunter shape can vary, but Sodoryard needs this capability
without using `ReducerDB.Underlying()` or lower-level store packages:

```go
type IndexedReadView interface {
    SeekIndex(tableID schema.TableID, indexID schema.IndexID, key ...types.Value) iter.Seq2[types.RowID, types.ProductValue]
    SeekIndexRange(tableID schema.TableID, indexID schema.IndexID, start, end IndexBound) iter.Seq2[types.RowID, types.ProductValue]
}

type IndexedReducerDB interface {
    SeekIndex(tableID uint32, indexID uint32, key ...types.Value) ([]types.RowID, error)
    SeekIndexRange(tableID uint32, indexID uint32, start, end IndexBound) ([]types.RowID, error)
}
```

### Sodoryard

The current memory seams are uneven:

- Brain operations already have an interface at `internal/brain.Backend`.
- The live brain backend is still MCP backed, but the MCP server is just a local
  wrapper around `internal/brain/vault.Client`.
- `BuildConventionSource` bypasses `brain.Backend` and reads `.brain/conventions`
  directly.
- Brain indexing already consumes `brain.Backend`, so a Shunter backend can feed
  the existing LanceDB path during migration. The current SQLite
  `brain_documents`/`brain_links` materialization must also become Shunter-backed
  before the default flip.
- Conversations, chains, context reports, tool execution logging, and several
  server/operator paths are still coupled to concrete SQLite stores.
- Provider subcall tracking already has an interface and is easier to move.
- `buildRuntimeBase` always opens `.yard/yard.db`; that must stop being the
  canonical runtime dependency.
- Spawned agents and reindex subprocesses currently start their own runtime.
  Under Shunter, child `tidmouth run`, `tidmouth index`, and `yard brain index`
  processes must connect to the parent memory service instead of opening the
  same Shunter data directory.

## Target Architecture

### One project memory owner

Create a new package, tentatively `internal/projectmemory`, that owns the embedded
Shunter runtime and exposes Sodoryard-shaped stores.

```text
internal/projectmemory/
  module.go        Shunter module/table/reducer/view declarations
  runtime.go       Build, Start, Close, Health, durable write helper
  rows.go          row encoding and decoding helpers
  reducers.go      JSON reducer payloads and result handling
  read.go          table scan and indexed read helpers
  service.go       local RPC server for child processes
  client.go        local RPC client for child processes
  brain.go         brain.Backend implementation
  conversation.go  conversation/history store implementation
  chain.go         chain store implementation
  context.go       context report store implementation
  tracking.go      provider subcall store implementation
  tools.go         tool execution recorder implementation
  indexstate.go    code/brain index metadata stores
```

Top-level commands own the memory runtime. This includes `yard`, `yard serve`,
`yard chain start`, `yard index`, `yard brain index`, and `yard memory ...`
commands when they run without `SODORYARD_MEMORY_ENDPOINT`. Spawned child
processes use a local RPC endpoint:

```text
yard chain start
  owns Shunter runtime
  listens on .yard/run/memory.sock
  spawns tidmouth run / tidmouth index / yard brain index with SODORYARD_MEMORY_ENDPOINT

tidmouth run / tidmouth index / yard brain index
  sees SODORYARD_MEMORY_ENDPOINT
  builds remote projectmemory.Client
  never opens .yard/shunter/project-memory directly
```

Use app-owned RPC first, not Shunter protocol, because the internal API needs
domain methods such as `PersistIteration`, `CompleteStepWithReceipt`, and
`PatchDocument`. The Shunter protocol can later power inspection tools or live UI
views.

### Configuration

Add memory configuration separate from the old brain vault setting:

```yaml
memory:
  backend: shunter
  shunter_data_dir: .yard/shunter/project-memory # project-relative unless absolute
  durable_ack: true
  rpc:
    transport: unix
    path: .yard/run/memory.sock # project-relative unless absolute

brain:
  enabled: true
  backend: shunter
  vault_path: .brain # import/export only when backend is shunter
  embedding_model: ...
```

Validation changes:

- `brain.vault_path` is required only for `brain.backend: vault` or explicit
  import/export commands.
- `memory.shunter_data_dir` is required for `memory.backend: shunter`.
- `memory.shunter_data_dir` and `memory.rpc.path` resolve relative to
  `project_root` when not absolute.
- If `SODORYARD_MEMORY_ENDPOINT` is set, runtime construction must create a remote
  memory client and must not open local Shunter state.
- Direct double-open of the Shunter data dir should fail fast through an
  app-owned lock or endpoint ownership check. Current Shunter does not provide a
  cross-process data-dir lock for a standalone `Runtime`.

## Canonical And Derived State

Canonical in Shunter:

- project identity and memory schema metadata
- brain documents, document chunks, links, revisions, and write operations
- conventions and receipts as normal brain documents with typed metadata
- conversations and messages
- compression summaries and message visibility metadata
- tool executions
- provider subcalls
- context reports and quality updates
- chains, steps, events, metrics, controls, launches, and launch presets
- code index freshness metadata, file hashes, and indexed commit state
- brain index freshness metadata and chunk fingerprints

Derived and rebuildable:

- LanceDB code embeddings
- LanceDB brain/document embeddings
- code graph database
- lexical search materializations
- exported Markdown vaults
- exported JSON snapshots

Transitional only:

- `.yard/yard.db`
- `.brain`
- MCP brain server

The transitional stores may exist during migration, but the target runtime path
must not consult them for live brain or harness state.

## Shunter Module Shape

The module name should be stable:

```go
const ModuleName = "yard_project_memory"
```

Declare table ids explicitly in Sodoryard code and never reorder table
declarations without a schema migration. The first application table is id `0`:

```go
const (
    tableProjectState schema.TableID = iota
    tableDocuments
    tableDocumentChunks
    tableDocumentLinks
    tableDocumentRevisions
    tableMemoryOperations
    tableConversations
    tableMessages
    tableToolExecutions
    tableSubCalls
    tableContextReports
    tableChains
    tableSteps
    tableEvents
    tableLaunches
    tableLaunchPresets
    tableCodeIndexState
    tableCodeIndexFiles
    tableBrainIndexState
    tableBrainIndexChunks
)
```

Reducer code can cast these constants with `uint32(tableDocuments)` at the
`ReducerDB` boundary. Index ids are assigned per table: a synthesized primary-key
index is id `0`, and explicit secondary indexes follow in declaration order.
Define index id constants next to each table declaration.

Use non-nullable columns only. Prefer typed scalar columns for common predicates
and JSON strings for irregular payloads.

Example table style:

```go
func declareDocuments(mod *shunter.Module) {
    mod.TableDef(schema.TableDefinition{
        Name: "documents",
        Columns: []schema.ColumnDefinition{
            {Name: "path", Type: types.KindString, PrimaryKey: true},
            {Name: "kind", Type: types.KindString},
            {Name: "title", Type: types.KindString},
            {Name: "content_hash", Type: types.KindString},
            {Name: "created_at_us", Type: types.KindUint64},
            {Name: "updated_at_us", Type: types.KindUint64},
            {Name: "deleted", Type: types.KindBool},
            {Name: "tags_json", Type: types.KindString},
            {Name: "metadata_json", Type: types.KindString},
        },
        Indexes: []schema.IndexDefinition{
            {Name: "documents_kind", Columns: []string{"kind"}},
            {Name: "documents_updated", Columns: []string{"updated_at_us"}},
        },
    })
}
```

Document content should be chunked for Shunter row size and revision management,
but the Shunter chunks are canonical storage chunks, not semantic RAG chunks.
Because Shunter has single-column primary keys, composite rows should use stable
single-column ids plus secondary indexes. Reconstruction joins exact chunks by a
unique `(path, chunk_index)` index.

```text
documents
  path, kind, title, content_hash, created_at_us, updated_at_us, deleted,
  tags_json, metadata_json

document_chunks
  chunk_id, path, chunk_index, body, body_hash

document_links
  link_id, source_path, target_path, link_text, created_at_us

document_revisions
  revision_id, path, revision, content_hash, operation_id, created_at_us,
  summary, actor

memory_operations
  operation_id, operation_type, path, actor, created_at_us, before_hash,
  after_hash, payload_json
```

Suggested operational tables:

```text
conversations
  id, project_id, title, created_at_us, updated_at_us, provider, model,
  settings_json, deleted

messages
  id, conversation_id, turn_number, iteration, sequence_index, role, content,
  tool_use_id, tool_name, created_at_us, visible, compressed, is_summary,
  summary_of_json, metadata_json

tool_executions
  id, conversation_id, turn_number, iteration, tool_use_id, tool_name, status,
  started_at_us, completed_at_us, input_json, output_size, normalized_size,
  error

sub_calls
  id, conversation_id, message_id, turn_number, iteration, provider, model,
  purpose, status, started_at_us, completed_at_us, tokens_in, tokens_out,
  cache_read_tokens, cache_creation_tokens, latency_ms, metadata_json

context_reports
  id, conversation_id, turn_number, created_at_us, updated_at_us, request_json,
  report_json, quality_json

chains
  id, source_specs_json, source_task, status, summary, created_at_us,
  updated_at_us, started_at_us, completed_at_us, metrics_json, limits_json,
  control_json

steps
  id, chain_id, sequence, role, task, task_context, status, verdict,
  created_at_us, started_at_us, completed_at_us, receipt_path, tokens_used,
  turns_used, duration_secs, exit_code, error

events
  id, chain_id, step_id, event_type, created_at_us, payload_json

launches
  id, project_id, status, mode, role, allowed_roles_json, roster_json,
  source_task, source_specs_json, created_at_us, updated_at_us

launch_presets
  id, project_id, name, mode, role, allowed_roles_json, roster_json,
  created_at_us, updated_at_us

code_index_state
  project_id, last_indexed_commit, last_indexed_at_us, dirty, metadata_json

code_index_files
  path, content_hash, indexed_at_us, language, symbols_hash, metadata_json

brain_index_state
  project_id, last_indexed_at_us, dirty, metadata_json

brain_index_chunks
  chunk_id, document_path, document_hash, chunk_hash, indexed_at_us,
  embedding_model, metadata_json
```

Conversation search currently benefits from SQLite FTS. The Shunter replacement
should start with deterministic lexical scanning over messages and documents, then
add a materialized `message_terms` or `document_terms` table if scan performance
is not acceptable. Those term tables are Shunter-owned derived materializations,
not canonical content. Do not keep SQLite FTS as a hidden runtime dependency.

## Reducer Design

Reducers should represent atomic application operations, not row-level CRUD. This
is where Shunter should become more valuable than the old mix of SQLite plus vault
files.

Core brain reducers:

- `write_document`
- `patch_document`
- `delete_document`
- `import_documents_batch`
- `record_memory_operation`
- `mark_brain_index_dirty`
- `mark_brain_index_clean`

Core conversation reducers:

- `create_conversation`
- `set_conversation_title`
- `set_runtime_defaults`
- `append_user_message`
- `persist_iteration`
- `cancel_iteration`
- `discard_turn`
- `compress_messages`
- `record_tool_execution`
- `record_sub_call`
- `store_context_report`
- `update_context_report_quality`

Core chain reducers:

- `create_chain`
- `start_step`
- `mark_step_running`
- `complete_step_with_receipt`
- `fail_step`
- `complete_chain`
- `update_chain_metrics`
- `set_chain_status`
- `append_chain_event`
- `request_chain_control`
- `upsert_launch`
- `delete_launch`
- `upsert_launch_preset`

Core index reducers:

- `mark_code_index_dirty`
- `mark_code_index_clean`
- `upsert_code_index_file`
- `remove_code_index_file`
- `upsert_brain_index_chunk`
- `remove_brain_index_chunk`

Important atomic reducers:

1. `complete_step_with_receipt`

   This should complete the chain step, write the receipt document, append the
   chain event, update chain metrics, and mark the brain index dirty in one
   transaction.

2. `persist_iteration`

   This should append assistant messages, record tool executions, attach provider
   subcall ids, and update conversation metadata in one transaction.

3. `write_document` and `patch_document`

   These should replace document chunks, write a revision row, append a memory
   operation, update link rows, and mark the brain index dirty in one transaction.

4. `compress_messages`

   This should mark old messages as compressed or hidden, insert the summary
   message, and preserve enough metadata for reconstruction in one transaction.

Patch operations must be conflict-aware. The adapter should read current content,
compute the patched document and exact storage chunks outside the reducer, then
call the reducer with the expected old hash:

```go
type PatchDocumentArgs struct {
    Path            string `json:"path"`
    Operation       string `json:"operation"`
    ExpectedOldHash string `json:"expected_old_hash"`
    NewContent      string `json:"new_content"`
    Actor           string `json:"actor"`
}
```

The reducer verifies `ExpectedOldHash` against the current document row before
replacing chunks. That prevents read-modify-write races between agents.

## Store Interfaces To Introduce

`brain.Backend` already exists and should get a Shunter implementation.

The other concrete SQLite stores need interfaces before the swap can be clean.
Keep these close to the current caller-facing method shapes instead of inventing
a new API during the storage swap:

```go
type ConversationStore interface {
    Create(ctx context.Context, projectID string, opts ...CreateOption) (*Conversation, error)
    Get(ctx context.Context, id string) (*Conversation, error)
    List(ctx context.Context, projectID string, limit, offset int) ([]ConversationSummary, error)
    Delete(ctx context.Context, id string) error
    SetTitle(ctx context.Context, conversationID, title string) error
    SetRuntimeDefaults(ctx context.Context, conversationID string, provider, model *string) error
    Count(ctx context.Context, projectID string) (int64, error)
    NextTurnNumber(ctx context.Context, conversationID string) (int, error)
    GetMessages(ctx context.Context, conversationID string) ([]MessageView, error)
    GetMessagePage(ctx context.Context, conversationID string, limit, offset int) ([]MessageView, error)
    Search(ctx context.Context, projectID string, query string) ([]SearchResult, error)
}

type HistoryStore interface {
    PersistUserMessage(ctx context.Context, conversationID string, turnNumber int, message string) error
    PersistIteration(ctx context.Context, conversationID string, turnNumber, iteration int, messages []conversation.IterationMessage) error
    CancelIteration(ctx context.Context, conversationID string, turnNumber, iteration int) error
    DiscardTurn(ctx context.Context, conversationID string, turnNumber int) error
    ReconstructHistory(ctx context.Context, conversationID string) ([]db.Message, error)
    SeenFiles(conversationID string) contextpkg.SeenFileLookup
}
```

The existing chain store should likewise become an interface matching current
callers:

```go
type ChainStore interface {
    StartChain(ctx context.Context, spec ChainSpec) (string, error)
    StartStep(ctx context.Context, spec StepSpec) (string, error)
    StepRunning(ctx context.Context, stepID string) error
    CompleteStep(ctx context.Context, params CompleteStepParams) error
    FailStep(ctx context.Context, params CompleteStepParams) error
    CompleteChain(ctx context.Context, chainID, status, summary string) error
    UpdateChainMetrics(ctx context.Context, chainID string, metrics ChainMetrics) error
    GetChain(ctx context.Context, chainID string) (*Chain, error)
    ListChains(ctx context.Context, limit int) ([]Chain, error)
    GetStep(ctx context.Context, stepID string) (*Step, error)
    ListSteps(ctx context.Context, chainID string) ([]Step, error)
    SetChainStatus(ctx context.Context, chainID, status string) error
    CountResolverStepsForContext(ctx context.Context, chainID, taskContext string) (int, error)
    CheckLimits(ctx context.Context, chainID string, in LimitCheckInput) error
    RemainingDuration(ctx context.Context, chainID string) (time.Duration, error)
    ListEvents(ctx context.Context, chainID string) ([]Event, error)
    ListEventsSince(ctx context.Context, chainID string, afterID int64) ([]Event, error)
    LogEvent(ctx context.Context, chainID string, stepID string, eventType EventType, eventData any) error
}
```

Additional stores:

- `context.ReportStore`
- `tracking.SubCallStore` already exists
- a `tool` execution-recorder interface around the current
  `ToolExecutionRecorder.Record` behavior
- launch and launch preset store
- code index metadata store
- brain index metadata store

The SQLite implementations can stay temporarily behind the same interfaces to
make the refactor reviewable, but the default target is Shunter.

## Brain Replacement

The current path:

```text
runtime -> brain/mcpclient -> local MCP server -> vault.Client -> .brain files
```

Target path:

```text
runtime -> projectmemory.BrainBackend -> Shunter reducers/read views
```

Required changes:

- Replace `BuildBrainBackend` so `brain.backend: shunter` returns
  `projectmemory.BrainBackend`.
- Replace `buildOrchestratorBrainBackend` the same way.
- Replace `BuildConventionSource` so conventions are read through the memory
  backend, not `.brain/conventions`.
- Keep `yard brain serve --vault` only as a legacy/export compatibility command,
  not as the live runtime brain.
- Make `yard brain index` read Shunter documents through `brain.Backend`, then
  rebuild LanceDB and any lexical derived tables.
- Make receipt writes go through `write_document` or `complete_step_with_receipt`,
  not filesystem writes.

`SearchKeyword` can initially scan document titles, paths, tags, and content
chunks. If that is too slow, add a Shunter-owned term table:

```text
document_terms
  term, path, field, frequency, updated_at_us
```

That table is not canonical content. It is a Shunter-owned derived
materialization that can be rebuilt from `documents` and `document_chunks`.
If maintained by reducers, tokenization must stay cheap and deterministic; if
that is not true, maintain it from the brain indexer after document commits.

## RAG And Code Retrieval

Keep the RAG databases, but demote their authority.

Code retrieval:

- Source files remain canonical in the workspace.
- LanceDB code embeddings remain in `.yard` as derived retrieval data.
- Code graph remains derived.
- Shunter owns code index state: last indexed commit, per-file hash, indexed time,
  language, symbol hash, and dirty flags.
- `yard index` and runtime retrieval should consult Shunter index state to decide
  freshness.

Brain retrieval:

- Shunter documents are canonical.
- LanceDB brain embeddings are derived from Shunter document chunks.
- `brain_index_chunks` records which Shunter document hash and chunk hash were
  embedded with which model.
- On document write, reducers mark the brain index dirty.
- `yard brain index` rebuilds stale chunks and then marks Shunter brain index
  state clean.

No reducer should call the embedder. Indexers should read committed Shunter state,
perform embedding outside Shunter, write LanceDB, then call a Shunter reducer to
record the successful derived index state.

## Local RPC For Child Processes

The parent memory owner should expose a small local RPC service over a Unix socket:

```text
.yard/run/memory.sock
```

Environment:

```text
SODORYARD_MEMORY_ENDPOINT=unix:.yard/run/memory.sock
SODORYARD_MEMORY_TOKEN=<optional local token>
```

When this endpoint is set, all runtime builders must use the RPC client for
memory state. This applies to spawned `tidmouth run`, spawned `tidmouth index`,
and spawned `yard brain index`, not only agent execution. A top-level command
with no endpoint owns the embedded Shunter runtime, creates `.yard/run`, removes
any stale socket it owns, and passes the endpoint through subprocess env.

Initial RPC surface should mirror store methods, not raw Shunter internals:

- brain read/write/patch/list/search
- conversation create/list/get/search/history persistence
- tool execution recording
- provider subcall recording
- context report insert/update
- chain start/update/event/list
- launch and preset operations
- code and brain index state operations

This avoids the dangerous model where each spawned agent opens the same Shunter
data dir. It also keeps reducer names and row schemas private to the parent
process.

## Migration Commands

Add explicit one-way migration commands:

```text
yard memory migrate \
  --from-vault .brain \
  --from-sqlite .yard/yard.db \
  --to .yard/shunter/project-memory

yard memory verify
yard memory backup --to ./backup/project-memory-YYYYMMDD
yard brain export --to ./backup/brain-markdown
```

Migration rules:

- Sort source documents, messages, chains, and events before import for
  deterministic transactions.
- Preserve existing ids where possible.
- Preserve message turn numbers and timestamps.
- Import receipts as normal documents with `kind = "receipt"`.
- Import conventions as normal documents with `kind = "convention"`.
- Import `.brain` links and tags into Shunter document metadata.
- Import SQLite conversations, messages, tool executions, subcalls, context
  reports, chains, steps, events, launches, and presets.
- Do not modify `.brain` during migration.
- Make migration idempotent by checking ids, hashes, and source fingerprints.

Backup rules:

- Pause or reject new writes.
- Wait for committed transactions to become durable.
- Create a Shunter snapshot if useful; pass the returned tx id to
  `CompactCommitLog(snapshotTxID)` if compacting.
- Close the runtime.
- Copy the full Shunter data directory, preferably through Shunter's offline
  `BackupDataDir` helper after `Close`.
- Restart the runtime.

## Implementation Phases

This is phased for reviewability, not because the target is partial.

### Phase 1: Shunter Foundation

Deliverables:

- add Shunter dependency
- add `internal/projectmemory` module declaration
- add memory config and validation
- build/start/close embedded Shunter runtime
- add durable write helper using public Shunter durable wait when available
- resolve or explicitly gate the Shunter API blockers: durable wait plus indexed
  read access from both `LocalReadView` and reducer code
- add local RPC server/client skeleton
- add tests for reducer commit, restart recovery, and schema compatibility

Exit criteria:

- `make test`
- `make build`
- a test can write a document, close/reopen Shunter, and read it back

### Phase 2: Brain, Conventions, And Receipts

Deliverables:

- implement `projectmemory.BrainBackend`
- route `BuildBrainBackend` and orchestrator brain construction to Shunter
- route convention loading through brain backend
- write receipts through Shunter
- make `yard brain index` read Shunter documents
- leave `yard brain serve --vault` as legacy compatibility only

Exit criteria:

- runtime can operate with no live `.brain` reads
- receipt documents survive restart
- LanceDB brain index rebuilds from Shunter documents

### Phase 3: Conversations And Agent History

Deliverables:

- introduce conversation/history store interfaces
- implement Shunter conversation and history stores
- move compression engine to the history store abstraction
- implement tool execution recorder on Shunter
- implement context report store on Shunter
- implement provider subcall store on Shunter

Exit criteria:

- agent loop persists and reconstructs history from Shunter
- tool calls and provider subcalls are visible after restart
- conversation search works without SQLite FTS

### Phase 4: Chains And Multi-Process Memory

Deliverables:

- introduce chain store interface
- implement Shunter chain/step/event store
- implement `complete_step_with_receipt`
- pass `SODORYARD_MEMORY_ENDPOINT` to spawned `tidmouth run`, `tidmouth index`,
  and `yard brain index`
- make child runtimes and spawned reindex commands use the RPC client instead of
  opening Shunter
- move launches and launch presets to Shunter

Exit criteria:

- parent chain runner owns the only local Shunter runtime
- spawned agents write through the parent memory service
- chain state, events, and receipts recover after restart

### Phase 5: Index State And SQLite Removal From Runtime

Deliverables:

- move code index state from SQLite to Shunter
- move brain index state from SQLite to Shunter
- update `yard index` and retrieval freshness checks
- stop opening `.yard/yard.db` in `buildRuntimeBase` for Shunter mode
- keep `.yard/yard.db` only for explicit migration or legacy mode. The derived
  code graph SQLite database (`.yard/graph.db`) may remain because it is not
  canonical memory.

Exit criteria:

- a normal Shunter-mode runtime starts without `.yard/yard.db`
- code and brain RAG stores remain usable and rebuildable
- deleting derived LanceDB data and rebuilding works from Shunter/workspace state

### Phase 6: Default Flip And Cleanup

Deliverables:

- default new projects to `memory.backend: shunter`
- add migration documentation
- add failure-mode tests for crash/restart/durable ack
- remove MCP brain from the live runtime path
- remove `.brain` as an active write target

Exit criteria:

- `make test`
- `make build`
- runtime smoke with confirmed provider/model
- no live runtime path reads or writes `.brain` in Shunter mode
- no canonical runtime path uses `.yard/yard.db` in Shunter mode

## Validation Matrix

Required tests:

- Shunter reducer tests for each atomic operation
- restart recovery after committed document, message, and chain writes
- durable ack behavior for writes that must survive process exit
- migration import from `.brain` and `.yard/yard.db`
- export round trip from Shunter to Markdown
- brain index rebuild from Shunter documents
- code index freshness update after workspace changes
- parent/child chain run where only the parent owns Shunter
- spawned reindex commands in a chain use the parent memory endpoint
- convention lookup with `.brain` absent or renamed
- conversation reconstruction after compression
- operator/server list views for conversations, chains, events, launches

Required commands:

```text
make test
make build
```

Runtime smoke tests still need the configured provider and model confirmed before
websocket or agent execution tests.

## Main Risks

1. Public Shunter API gaps

   Durable wait and indexed reads are important enough to fix in Shunter rather
   than working around them permanently in Sodoryard. The indexed-read gap is
   both external (`LocalReadView`) and reducer-facing (`ReducerDB`).

2. SQLite FTS replacement

   Message and document search cannot silently remain SQLite-backed if the goal
   is a full brain swap. Start with deterministic scans, then add Shunter-owned
   term tables if needed.

3. Schema evolution

   Shunter table declaration order, zero-based table ids, single-column primary
   keys, and non-nullable columns require discipline. Add schema compatibility
   checks before opening existing project memory.

4. Multi-process ownership

   Child agents and spawned reindex commands must not open the Shunter data
   directory. Parent-owned local RPC is mandatory for chain mode.

5. Reducer purity

   Reducers must only mutate Shunter state. Embeddings, provider calls, file
   reads, and graph construction stay outside reducers.

6. Backup expectations

   Yard must own backup semantics by quiescing, waiting for durability, closing,
   and copying the Shunter data directory.

## Practical First Slice

The first implementation slice should be small but should prove the architecture:

1. Add `memory.backend: shunter` config and `internal/projectmemory`.
2. Declare `documents`, `document_chunks`, `document_revisions`, and
   `memory_operations` with zero-based table id constants and deterministic
   single-column ids for chunk/revision rows.
3. Implement `write_document`, `patch_document`, `list_documents`,
   `read_document`, and keyword scan. Do not flip this on by default until the
   Shunter indexed-read blocker is resolved or the scan-only path is explicitly
   limited to development/test use.
4. Implement `brain.Backend` on Shunter.
5. Route `BuildBrainBackend` and `BuildConventionSource` through that backend.
6. Add `yard memory migrate --from-vault .brain` for documents only.
7. Rebuild the existing LanceDB brain index from Shunter documents.
8. Prove restart recovery and absence of live `.brain` reads in Shunter mode.

That slice removes the MCP/vault brain from the live path and establishes the
pattern for moving conversations, chains, and index state next.
