# 23 - Genkit Patterns Worth Stealing

**Status:** Working plan **Last Updated:** 2026-05-08 **Author:** Mitchell / Codex

---

## Purpose

This document captures the useful ideas from the Genkit Go SDK that are worth adapting into Sodoryard without adopting Genkit as the core harness runtime.

The conclusion from the SDK vetting pass is:

- Do not replace Yard's provider router, agent loop, tool executor, context assembly, chain runner, or Shunter-backed state model with Genkit.
- Do use Genkit as a reference implementation for observability, typed contracts, middleware shape, tool approval, evaluations, prompt ergonomics, model capability metadata, and workflow inspection.
- Keep the implementation local to Yard's existing architecture: `yard` remains the operator surface, `tidmouth` remains the internal chain-step engine until the spawn contract is redesigned, Shunter remains the chain/brain state plane, and SQLite remains acceptable for high-volume runtime metrics.

Reviewed source context:

- Genkit repository: <https://github.com/genkit-ai/genkit>
- Go module: `github.com/firebase/genkit/go`, latest observed `v1.8.0` on 2026-05-08
- Go SDK README, `go/go.mod`, `go/ai/generate.go`, `go/ai/tools.go`, `go/ai/middleware.go`, OpenAI-compatible, Anthropic, Ollama, and middleware plugin sources
- Current Yard docs and runtime code: provider router, agent loop, tool executor, chain orchestrator, TUI/web inspector specs

Out of scope for this document:

- Typed session state as a standalone feature
- HTTP flow handlers as a public Yard API surface
- Vector-store plugin templates

Those are lower-priority ideas and should not distract from the higher-impact runtime work below.

---

## Relationship To Spec 22

[[22-sequential-agent-guardrails]] is authoritative for the overlapping chain-safety contracts:

- sequential mutating-agent invariants
- role mutation classes and writer locks
- typed receipt validity
- audit finding IDs
- changed-file manifests
- pre-step briefings
- post-step chain health warnings

This document should extend those contracts, not redefine them. In particular:

- The schema-backed receipt and finding work below should use the receipt/finding semantics from spec 22 as the canonical contract.
- Chain templates should respect spec 22 role classes, transition rules, and writer-lock constraints.
- Tool approval interrupts should build on spec 22 mutating-tool and mutating-role classification.
- Eval suites should include spec 22 invariants as deterministic test cases once those slices land.
- Tracing and middleware hooks may proceed independently because they are observability/plumbing work and can help implement or debug spec 22.

If this document conflicts with spec 22 on chain safety, receipt semantics, findings, changed-file manifests, or flow health, spec 22 wins.

---

## Selection Criteria

An idea is worth stealing only if it improves one of Yard's live differentiators:

- Better operator understanding of why an agent or chain behaved a certain way
- Stronger machine-readable contracts between agents, receipts, metrics, and UI
- Less duplicated cross-cutting code around providers/tools/retry/tracking/policy
- Better dogfooding and regression testing of agent quality
- Cleaner preflight warnings before expensive or long-running chains
- More robust reconnect/resume behavior for terminal and browser inspection

An idea is not worth stealing if it requires Yard to surrender control of:

- Codex subscription OAuth and Yard-owned private auth storage
- Anthropic OAuth/API-key dual path
- Role-scoped tool registration and brain write guards
- Purity-aware tool execution
- Per-turn context assembly and compression
- Chain receipts, event logs, pause/resume/cancel, and Shunter project memory

---

## Recommended Backlog Order

1. Trace spans and timeline data
2. Schema-backed receipts and findings
3. Provider/tool middleware hooks
4. Evaluation harness
5. Model capability registry
6. Chain templates as typed launch units
7. Tool approval interrupts
8. Prompt files with metadata frontmatter
9. Provider factory registry
10. Common retriever interface
11. Web inspector timeline
12. Durable/reconnectable streaming

The order is intentionally biased toward observability and contracts first. Those make every later feature easier to validate.

---

## 1. Trace Spans And Timeline Data

### Problem

Yard already records many useful facts: chain events, step receipts, sub-calls, tool executions, context reports, and TUI/web summaries. The missing piece is a unified timeline that explains the causal path of a turn or chain:

- Context assembly started, queried these sources, and consumed this budget
- Provider call started with this provider/model and these prompt/cache settings
- Tool batch started with N pure and M mutating calls
- Individual tool calls ran, failed, truncated, or required retry
- Compression triggered before or after a provider response
- A chain step spawned, emitted output, wrote a receipt, and updated metrics

Genkit's trace/span model is useful because it treats AI calls, tools, and sub-steps as inspectable units instead of loose log lines.

### Yard Adaptation

Add a Yard-native tracing layer with spans around major runtime actions. This should not replace existing chain events or metrics. It should provide a common timeline key that links those records.

Suggested package:

```text
internal/trace
```

Suggested core type:

```go
type Span struct {
    ID             string
    ParentID       string
    TraceID        string
    ConversationID string
    ChainID        string
    StepID         string
    TurnNumber     int
    Iteration      int
    Name           string
    Kind           string // context, provider, tool_batch, tool, compression, chain, receipt, reindex
    Status         string // running, ok, error, cancelled, approval_required
    StartedAt      time.Time
    EndedAt        time.Time
    DurationMs     int64
    Attributes     map[string]any
    Error          string
}
```

High-volume span storage should start in SQLite because it is query-oriented and similar to existing sub-call/tool/context report data. Chain-level summary facts can be mirrored into Shunter chain metrics or receipts when needed.

### Initial Span Points

- `ContextAssembler.Assemble`
- `ProviderRouter.Stream` and `ProviderRouter.Complete`
- Individual provider implementation calls after routing resolution
- `AgentLoop` iteration loop
- `tool.Executor.Execute` for the batch
- `tool.Executor.executeSingle` for individual tools
- Compression preflight, post-response, and emergency compression
- `spawn_agent` subprocess lifecycle
- Receipt write/parse/update
- Reindex before chain steps

### Code Touchpoints

- `internal/agent/loop.go`
- `internal/agent/runturn_iteration.go`
- `internal/context/assembler.go`
- `internal/provider/router/router.go`
- `internal/provider/tracking`
- `internal/tool/executor.go`
- `internal/spawn/spawn_agent.go`
- `internal/chain`
- `internal/server` chain and metrics endpoints
- `internal/tui` chain health/detail rendering
- `web/src/pages/chain-detail.tsx`

### MVP

1. Add span recorder interface and no-op implementation.
2. Wire recorder through runtime builders.
3. Record provider-call, tool-batch, individual-tool, compression, and chain-step spans.
4. Add a read API for a chain timeline.
5. Render a simple ordered timeline in the web inspector.

### Acceptance Criteria

- A chain detail view can show a single ordered timeline across orchestrator and spawned steps.
- Each tool execution and provider call has a span with duration and status.
- Existing sub-call and tool-execution rows can be linked to a span ID.
- Tests cover span parent/child nesting and no-op behavior when tracing is disabled.

---

## 2. Schema-Backed Receipts And Findings

### Problem

Receipts are the durable contract between agents and the orchestrator, but markdown alone forces downstream code to parse prose. That makes chain health, flow analysis, and audit resolution harder than necessary.

Genkit's typed structured output pattern is worth adapting: keep human-readable text, but pair it with a machine-readable schema.

### Yard Adaptation

Keep markdown receipts as the human contract. Add a required machine-readable contract in frontmatter or a fenced JSON block.

Recommended shape:

```yaml
---
schema_version: yard.receipt.v1
chain_id: CHAIN-123
step_id: step-001
role: coder
verdict: completed
changed_files:
  - internal/foo.go
findings:
  - id: FIND-correctness-001
    status: open
    severity: high
    category: correctness
    summary: "..."
followups:
  - "..."
metrics:
  turns: 7
  input_tokens: 12345
  output_tokens: 678
---
```

Structured finding fields should support:

- Stable `id`
- `status`: open, addressed, closed, reopened, invalid
- `severity`: critical, high, medium, low, info
- `category`: correctness, quality, performance, security, integration, docs, test
- `file`
- `line`
- `summary`
- `evidence`
- `recommendation`
- `addressed_by_step`

### Code Touchpoints

- Receipt writing in `cmd/tidmouth/run.go`
- Receipt parsing in `internal/chain` and `internal/operator`
- Chain metrics and flow analysis in `internal/chain/flow_analysis.go`
- TUI chain health rendering
- Web inspector chain detail types/rendering
- Agent role prompts in `agents/` and embedded prompt assets

### MVP

1. Define Go structs for receipt metadata and findings.
2. Add a parser that tolerates old receipts but flags missing schema metadata as a warning.
3. Update generated receipts to include `schema_version`.
4. Update chain metrics to prefer structured findings over prose parsing.
5. Add tests with coder, auditor, resolver, and docs-arbiter receipts.

### Acceptance Criteria

- New receipts include schema-backed metadata.
- Old receipts still render, but chain health reports warn that structured receipt metadata is missing.
- Flow analysis can identify open/addressed/reopened findings without regexing body prose.
- `yard chain metrics <chain-id>` reports malformed receipt metadata clearly.

---

## 3. Provider And Tool Middleware Hooks

### Problem

Yard already has cross-cutting behavior spread across several places:

- Provider tracking
- Router retry/fallback
- Provider health marking
- Tool output normalization/truncation
- Tool persistence
- Future policy/approval
- Future tracing spans

Genkit's middleware hooks are useful because they create explicit extension points around generate/model/tool execution.

### Yard Adaptation

Add small internal hook chains. This should be a Yard-native mechanism, not a public plugin API initially.

Suggested interfaces:

```go
type ProviderHook interface {
    BeforeProviderCall(ctx context.Context, req *provider.Request) (context.Context, error)
    AfterProviderCall(ctx context.Context, req *provider.Request, usage provider.Usage, err error) error
}

type ToolHook interface {
    BeforeTool(ctx context.Context, call tool.ToolCall, def tool.Tool) (context.Context, error)
    AfterTool(ctx context.Context, call tool.ToolCall, result tool.ToolResult) (tool.ToolResult, error)
}
```

Do not move all existing behavior immediately. Start by adding hooks where they reduce new tracing/approval/eval work.

### Candidate Built-In Hooks

- Trace hook: creates spans for provider and tool calls
- Tracking hook: links provider/tool records to span IDs
- Approval hook: blocks or pauses risky tools
- Budget hook: checks aggregate token/tool-result budgets
- Retry/fallback hook: eventually absorbs router retry/fallback policy
- Policy hook: role-specific constraints and future deny lists

### Code Touchpoints

- `internal/runtime` builders for dependency injection
- `internal/provider/router`
- `internal/provider/tracking`
- `internal/tool/executor.go`
- `internal/tool/adapter.go`
- `internal/agent/loop.go`

### MVP

1. Add hook-chain types with no-op defaults.
2. Add tool hooks to `tool.Executor`.
3. Add provider hooks around router calls or tracked provider wrapper.
4. Implement tracing as the first hook user.
5. Keep existing behavior unchanged when no hooks are configured.

### Acceptance Criteria

- Hook order is deterministic and covered by tests.
- Hook errors fail closed where appropriate.
- Existing provider/router/tool tests pass without hook configuration.
- Trace spans can be implemented without provider/tool-specific edits.

---

## 4. Evaluation Harness

### Problem

Yard needs a way to know whether context assembly, tool design, receipts, and chain sequencing are getting better or worse. One-off dogfooding runs are useful, but they are hard to compare over time.

Genkit treats evaluation as part of the development loop. Yard should do the same.

### Yard Adaptation

Add an evaluation harness for deterministic and live agent-quality checks.

Possible command surface:

```bash
yard eval list
yard eval run <suite>
yard eval run <suite> --live
yard chain eval <chain-id>
```

Suggested fixture layout:

```text
eval/
  suites/
    retrieval-smoke.yaml
    receipt-contract.yaml
    chain-flow.yaml
  fixtures/
    small-go-project/
```

Evaluation dimensions:

- Retrieval relevance: expected files/symbols appear in assembled context
- Receipt validity: schema present, required fields valid, findings parse
- Chain flow: planner before coder, auditor after coder, resolver only for findings
- Tool behavior: no repeated failing call loops, no mutating calls from read-only roles
- Runtime health: provider/model readiness, auth clarity, index freshness warnings
- Cost/runtime: token totals, iterations, wall time, compression events
- Output quality: live optional checks using evaluator prompts or deterministic assertions

### Code Touchpoints

- New `internal/eval`
- `cmd/yard/eval.go`
- `internal/context` report store
- `internal/chain` metrics/flow analysis
- Receipt parser from this plan
- Test fixtures under `testdata/` or `eval/`

### MVP

1. Add deterministic evaluation for receipt contracts and chain flow against fixture data.
2. Add retrieval evaluation that runs context assembly without making LLM calls.
3. Add JSON and human-readable output.
4. Keep live model evaluation opt-in.

### Implemented First Slice

Implemented on 2026-05-08:

- `yard eval list`
- `yard eval run receipt-contract`
- `yard eval run chain-flow`
- deterministic embedded fixtures for receipt schema/section/finding checks
- deterministic embedded fixtures for chain-flow warnings, finding lifecycle state, and running source-writer conflict detection
- human-readable and JSON output

Not implemented in this slice:

- live model evaluation
- retrieval evaluation that runs live context assembly against fixture projects
- baseline comparison

### Implemented Chain Eval Slice

Implemented on 2026-05-08:

- `yard chain eval <chain-id>`
- read-only evaluation of persisted chain rows, steps, and events
- deterministic assertions for successful chain status, flow warning codes, source-writer conflicts, and unresolved findings on completed chains
- human-readable and JSON output using the same eval report shape as `yard eval run`

Not implemented in this slice:

- live model evaluation
- retrieval evaluation that runs live context assembly against fixture projects
- baseline comparison

### Implemented Baseline Comparison Slice

Implemented on 2026-05-08:

- `yard eval run <suite> --baseline <report.json>`
- baseline comparison against saved JSON eval reports
- machine-readable baseline status/diffs in JSON output
- human-readable baseline status and per-field diffs in normal output

Not implemented in this slice:

- baseline writing/management commands
- historical trend storage
- tolerance policies for live or noisy evals

### Implemented Retrieval Contract Slice

Implemented on 2026-05-08:

- `yard eval run retrieval-contract`
- deterministic embedded context-report fixtures for source-specific and stored normalized retrieval rows
- assertions for normalized retrieval result count, source kinds, included paths, excluded paths, and symbols
- CI-safe coverage for the common retrieval-result contract without provider credentials or live indexing

Not implemented in this slice:

- live context assembly against fixture projects
- model-judged retrieval quality
- retrieval ranking or orchestration changes

### Implemented Tool Contract Slice

Implemented on 2026-05-08:

- `yard eval run tool-contract`
- deterministic executor-level assertions for approval-required shell calls
- deterministic assertion that allowed shell calls still execute when approval patterns do not match
- provider-free and shell-free fake tool coverage for CI

Not implemented in this slice:

- repeated failing-call loop evals
- read-only role mutating-tool evals
- live tool behavior evals

### Acceptance Criteria

- `yard eval run receipt-contract` can run in CI without provider credentials.
- Eval output includes pass/fail, score, warnings, and exact failed assertions.
- A chain ID can be evaluated after a dogfooding run.
- The harness can compare current run metrics to a saved baseline.

---

## 5. Model Capability Registry

### Problem

Yard knows provider/model names and context windows, but chains need richer preflight information:

- Does the model support native tools?
- Does it support extended thinking or reasoning-effort controls?
- Does it support JSON/schema-constrained output?
- Does it support prompt cache controls?
- Does it support images?
- What quirks or warnings apply?

Genkit's model metadata is worth adapting because it catches bad runtime combinations before an expensive chain starts.

### Yard Adaptation

Extend `provider.Model` and config-derived metadata with explicit capabilities.

Suggested fields:

```go
type ModelCapabilities struct {
    SupportsTools            bool
    SupportsThinking         bool
    SupportsReasoningEffort  bool
    SupportsStructuredOutput bool
    SupportsPromptCache      bool
    SupportsImages           bool
    SupportsToolChoice       bool
    MaxOutputTokens          int
    KnownQuirks              []string
}
```

Use this for:

- `yard config`
- `yard doctor`
- TUI `/status`
- Launch preview warnings
- Chain start preflight
- Web inspector runtime status

### Code Touchpoints

- `internal/provider/types.go`
- Provider implementations under `internal/provider/*`
- `internal/runtime/provider.go`
- Config validation in `internal/config`
- Operator runtime/readiness reporting
- TUI status and launch preview
- Web runtime status endpoint/types

### MVP

1. Extend metadata shape without changing provider request behavior.
2. Populate conservative defaults for existing providers.
3. Add warnings when a role needs tools but selected model reports no tool support.
4. Add warnings when structured receipts are enabled but selected model has no structured-output support.

### Implemented First Slice

Implemented on 2026-05-08:

- extended provider model metadata with reasoning-effort, structured-output, prompt-cache, image, tool-choice, max-output-token, and known-quirk fields
- added a config-derived capability resolver for the default provider/model
- surfaced context window and capability labels in `yard config`, operator runtime status, runtime status API, and TUI `/model` / `/status`
- added launch-preview warnings when configured launch roles require tools but the selected model is configured as not supporting tools
- added optional provider config overrides such as `supports_tools`, `supports_structured_output`, `max_output_tokens`, and `known_quirks`

Not implemented in this slice:

- changing provider request behavior based on capabilities
- hard launch rejection for capability mismatches
- structured-output preflight warnings, pending a dedicated structured-output runtime mode/flag
- richer web rendering of model capabilities

### Acceptance Criteria

- Runtime status displays provider, model, context window, and key capabilities.
- Chain launch preview warns about incompatible role/model choices.
- Provider aliases preserve capability metadata.
- Tests cover default/fallback provider capability resolution.

---

## 6. Chain Templates As Typed Launch Units

### Problem

Yard already has launch modes and presets in the TUI, plus chain start flags. The shape is useful but still partly UI-driven. Genkit's flow idea is worth stealing only as a concept: named, typed, inspectable workflow units.

### Yard Adaptation

Formalize chain templates as a typed model that all operator surfaces use.

Candidate templates:

- `one_step`
- `manual_roster`
- `constrained_orchestration`
- `sir_topham_decides`
- Future: `bug_fix`, `feature_slice`, `docs_only`, `audit_only`

Template fields:

```go
type ChainTemplate struct {
    ID              string
    Label           string
    Description     string
    InputSchema      json.RawMessage
    DefaultRoles     []string
    AllowedRoles     []string
    RequiresTask      bool
    RequiresSpecs     bool
    SafetyDefaults    ChainLimits
    ReceiptSchema     string
    PreflightChecks   []string
}
```

### Code Touchpoints

- `internal/operator` launch compilation
- `cmd/yard/chain.go`
- `internal/tui` launch model
- `internal/server` launch endpoints if browser intake survives
- Config/preset persistence
- Chain start tests

### MVP

1. Move existing launch modes into a shared template registry.
2. Make CLI, TUI, and API compile launch requests through the same template path.
3. Add launch preview output based on template metadata and model capabilities.
4. Keep custom presets as saved parameter sets over templates.

### Implemented First Slice

Implemented on 2026-05-08:

- typed `LaunchTemplate` metadata for current launch modes
- shared template registry for one-step, manual-roster, constrained-orchestration, and Sir Topham-managed launches
- launch previews now carry template metadata, including ID, label, receipt schema, default roles, and preflight checks
- TUI launch preview and slash-command preview render the template label when available

Not implemented in this slice:

- changing launch compile semantics
- custom presets explicitly binding to template IDs beyond their existing launch mode
- CLI/API listing commands for launch templates
- JSON input schemas or template-specific hard validation beyond existing launch validation

### Implemented CLI Visibility Slice

Implemented on 2026-05-08:

- `yard chain templates`
- `yard chain templates --json`
- human-readable and machine-readable launch template listing from the same operator registry used by launch previews

Not implemented in this slice:

- API listing command for browser intake
- template selection by ID beyond existing launch mode flags
- JSON input schemas or template-specific hard validation beyond existing launch validation

### Acceptance Criteria

- One-step, manual-roster, constrained-orchestration, and full-orchestration launches use one shared compile path.
- TUI launch preview and CLI dry-run agree.
- Template metadata is visible in operator surfaces.
- Invalid role/template combinations fail before chain creation.

---

## 7. Tool Approval Interrupts

### Problem

Today tools mostly either run or return a failure result. Some operations need a third state: "this is valid, but a human must approve it before it executes."

Genkit's interrupt/resume pattern is a useful reference. Yard needs a version that fits long-running chains, TUI-first operation, and durable state.

### Yard Adaptation

Add a tool approval state for risky mutating operations.

Initial candidates:

- `shell` commands matching destructive patterns
- `file_write` overwriting large or sensitive files
- `file_edit` touching files outside task scope
- Future git tools that mutate state
- Future external network tools
- `spawn_agent` when orchestrator wants to run a mutating role after warnings

Suggested pending action shape:

```go
type PendingApproval struct {
    ID             string
    ChainID        string
    StepID         string
    ConversationID string
    TurnNumber     int
    Iteration      int
    ToolName       string
    ToolInput      json.RawMessage
    Reason         string
    RiskLevel      string
    CreatedAt      time.Time
    Status         string // pending, approved, denied, expired
}
```

### Execution Semantics

For interactive TUI/web sessions:

1. Tool hook detects approval is required.
2. Agent turn pauses and emits `approval_required`.
3. Operator approves, denies, or edits input.
4. Runtime resumes with a tool result or re-executes the approved call.

For headless chains:

- Default should fail closed unless `--allow-approval-wait` or an equivalent chain mode is set.
- In fail-closed mode, write a receipt or tool result that escalates to the human.
- In wait mode, chain status becomes `waiting_approval`; TUI/web can resume it.

### Code Touchpoints

- Tool middleware hook from this plan
- `internal/tool/shell.go`
- `internal/tool/file_write.go`
- `internal/tool/file_edit.go`
- `internal/agent` turn state and events
- `internal/chain` statuses/events
- TUI chain controls
- Web inspector chain controls

### MVP

1. Implement approval as a hook for `shell` only.
2. Add chain status/event support for `waiting_approval`.
3. Add CLI/TUI approval path for pending approvals.
4. Add tests that approval-required tools do not execute before approval.

### Implemented First Slice

Implemented on 2026-05-08:

- `PendingApproval` and `ApprovalRequiredError` tool contracts
- configurable `shell_approval_patterns` for shell commands that require operator approval
- shell approval detection runs through the existing tool hook chain before execution
- headless/default behavior fails closed with structured `approval_required` tool-result metadata
- tests cover approval-required shell calls not executing before approval

Not implemented in this slice:

- durable approval storage
- `waiting_approval` chain status/events
- CLI/TUI approve/deny/resume controls
- approval support for file mutation tools or spawned agents

### Implemented Trace Context Slice

Implemented on 2026-05-08:

- approval-required tool results now include trace-scope context when available
- pending approval metadata carries chain ID, step ID, conversation ID, turn number, and iteration
- exported trace scope read helper for middleware that needs to enrich structured runtime events

Not implemented in this slice:

- durable approval storage
- `waiting_approval` chain status/events
- approval resume controls

### Implemented Chain Event Contract Slice

Implemented on 2026-05-08:

- `approval_required` chain event type
- `waiting_approval` chain status constant
- chain scheduling treats `waiting_approval` as a stop state
- chain cancel controls can request cancellation while waiting for approval
- chain log rendering shows approval ID, tool name, risk, status, and reason

Not implemented in this slice:

- runtime emission of approval events
- durable approval storage
- approve/deny/resume controls

### Acceptance Criteria

- A risky shell command can be blocked before execution.
- The operator can approve or deny from the TUI or CLI.
- Denial is visible to the model as a tool result.
- Headless chain behavior is deterministic and documented.

---

## 8. Prompt Files With Metadata Frontmatter

### Problem

Role prompts and `yard.yaml` role config are related but separate. Genkit's prompt-file frontmatter is useful for keeping prompt-local intent near the text while preserving runtime config as the enforcement source.

### Yard Adaptation

Allow optional YAML frontmatter in role prompt files and embedded prompt sources.

Important rule: `yard.yaml` remains authoritative for security-sensitive behavior. Prompt frontmatter can validate, document, and suggest, but it must not silently grant tools or write paths.

Candidate fields:

```yaml
---
role_key: coder
persona: Thomas
expected_tools:
  - brain
  - file
  - git
  - shell
receipt_schema: yard.receipt.v1
recommended_max_turns: 100
requires_structured_findings: false
---
```

Use cases:

- Warn if prompt expected tools differ from `yard.yaml`
- Document receipt schema expectations
- Help sync embedded prompts and editable `agents/` prompts
- Feed docs generation or `yard config validate`

### Code Touchpoints

- Role prompt loading in `internal/role` or config runtime helpers
- Embedded prompt generation/sync path
- `yard config validate`
- Tests for built-in prompt loading

### MVP

1. Add a frontmatter parser that gracefully handles prompts without frontmatter.
2. Expose parsed metadata in role validation.
3. Warn on mismatches between prompt metadata and configured role tools.
4. Do not change actual tool registration from frontmatter.

### Implemented First Slice

Implemented on 2026-05-08:

- optional role prompt frontmatter parser for fields such as `role_key`, `expected_tools`, `receipt_schema`, and `recommended_max_turns`
- prompt loading strips valid frontmatter before sending role prompts to models
- malformed prompt frontmatter is warning-only and leaves prompt content unchanged
- `yard config` prints warning-only prompt metadata mismatches for role key, expected tools, and receipt schema
- frontmatter remains documentation/validation metadata only; it does not grant tools or change role registration

Not implemented in this slice:

- syncing embedded prompt metadata across all built-in prompt files
- docs generation from prompt metadata
- hard rejection of prompt metadata mismatches

### Acceptance Criteria

- Existing prompts continue to load unchanged.
- New frontmatter can be parsed from both filesystem and embedded prompts.
- Mismatches produce warnings, not silent behavior changes.
- Tests cover missing, valid, and malformed frontmatter.

---

## 9. Provider Factory Registry

### Problem

Provider construction is centralized enough today, but adding more providers will keep growing switch statements and config conditionals.

Genkit's plugin model is too broad for Yard core. The useful part is a small provider factory boundary.

### Yard Adaptation

Add a registry of provider factories keyed by config type.

Suggested package:

```text
internal/provider/factory
```

Suggested shape:

```go
type Factory interface {
    Type() string
    Build(ctx context.Context, cfg appconfig.ProviderConfig, deps Deps) (provider.Provider, error)
}
```

Initial registered factories:

- `codex`
- `anthropic`
- `openai-compatible`

Future factories:

- `gemini`
- `ollama-native`
- `bedrock`
- `openrouter` as explicit wrapper around OpenAI-compatible defaults

### Code Touchpoints

- `internal/runtime/provider.go`
- Provider config validation
- Tests in `internal/runtime/provider_test.go`
- Optional provider docs

### MVP

1. Introduce factory registry with current providers.
2. Preserve all existing config behavior.
3. Make provider aliases and optional auth reporter/pinger delegation work exactly as today.

### Implemented First Slice

Implemented on 2026-05-08:

- `internal/provider/factory` registry with factories for `codex`, `anthropic`, and `openai-compatible`
- runtime provider construction now delegates through the default registry while preserving the existing `runtime.BuildProvider` helper
- provider alias wrapping moved behind the factory boundary with `Pinger` and `AuthStatusReporter` delegation preserved
- registry tests for default types, duplicate/unknown provider types, factory error propagation, Codex aliasing, and optional interface delegation

Not implemented in this slice:

- new provider types
- dynamic plugin loading
- changing provider config validation behavior

### Acceptance Criteria

- Provider construction tests pass unchanged or with narrow updates.
- Adding a new provider type requires a factory, validation entry, and docs, not edits across unrelated runtime code.
- Auth status and ping behavior remain available through wrappers.

---

## 10. Common Retriever Interface

### Problem

Yard has a strong context assembly system, but code search, brain search, graph search, conventions, and future sources each have their own shape. A common interface would make ranking experiments, trace spans, evals, and reports easier.

Genkit's retriever/indexer vocabulary is useful, but Yard's implementation should stay specialized to code and project brain behavior.

### Yard Adaptation

Introduce a common result model around existing retrieval sources.

Suggested shape:

```go
type Retriever interface {
    Name() string
    Retrieve(ctx context.Context, req RetrievalRequest) ([]RetrievalResult, error)
}

type RetrievalResult struct {
    Source       string
    Kind         string // code_chunk, brain_doc, symbol, convention, git
    Path         string
    Symbol       string
    Score        float64
    Content      string
    TokenEstimate int
    Metadata     map[string]any
}
```

This does not require replacing the current retrieval orchestrator at once. Start by wrapping sources so the context report and eval harness can consume a common shape.

### Code Touchpoints

- `internal/context/retrieval.go`
- `internal/context/assembler.go`
- `internal/context/report_store.go`
- `internal/codeintel/searcher`
- `internal/context/brain_search.go`
- Context inspector endpoints and frontend types

### MVP

1. Define common result types.
2. Convert context report serialization to include common result metadata.
3. Wrap code and brain search first.
4. Leave ranking policy unchanged.

### Implemented First Slice

Implemented on 2026-05-08:

- normalized `RetrievalResult` shape in `internal/context`
- context assembly reports now include source-agnostic `retrieval_results`
- normalized report results are derived from existing code, brain, graph, and explicit-file result records
- SQLite/project-memory report reads rebuild normalized retrieval results for older reports when needed
- metrics context-report API includes `retrieval_results` alongside source-specific result arrays

Not implemented in this slice:

- replacing retrieval orchestration or ranking policy
- separate retriever interface wrappers for each source
- live retrieval eval suite assertions over fixture projects

### Implemented Source Wrapper Slice

Implemented on 2026-05-08:

- source-level `RetrievalRequest` and `SourceRetriever` contract
- code-search source retriever wrapper that returns normalized `RetrievalResult` metadata
- brain-search source retriever wrapper that returns normalized `RetrievalResult` metadata
- tests verify wrappers preserve current context config and brain graph-hop settings

### Implemented Additional Source Wrapper Slice

Implemented on 2026-05-09:

- structural graph source retriever wrapper that returns normalized symbol results
- explicit-file source retriever wrapper that returns normalized file results
- convention source retriever wrapper that returns normalized convention text
- git-context source retriever wrapper that returns normalized git context with depth metadata
- focused tests cover graph structural config, explicit file content, convention loading, and git depth/workdir forwarding

Not implemented in these slices:

- replacing the current retrieval orchestrator with source wrappers
- ranking experiments across normalized source results

### Acceptance Criteria

- Context reports show consistent metadata across code and brain hits.
- Eval harness can assert "expected file appeared in retrieval results" without source-specific parsing.
- Existing context assembly output remains stable except for added metadata.

---

## 11. Web Inspector Timeline

### Problem

The browser inspector should not become a second command center, but it is the best place to inspect detailed timelines, traces, prompts, tool outputs, diffs, and charts.

Genkit's local developer UI reinforces the product lesson: runnable units need a visual trace view.

### Yard Adaptation

Build a chain/turn timeline view on top of trace spans and existing chain events.

Timeline lanes:

- Chain lifecycle
- Orchestrator provider calls
- Spawned step subprocesses
- Agent iterations
- Tool batches and tools
- Context assembly
- Compression
- Receipt writes
- Warnings/findings

Each timeline item should link to its detail panel:

- Provider request summary
- Context package/report
- Tool input/output/truncation details
- Receipt frontmatter/body
- Chain event payload
- Related files/diff

### Code Touchpoints

- Trace API from item 1
- `internal/server/chains.go`
- `web/src/pages/chain-detail.tsx`
- `web/src/types/chains.ts`
- Existing chain detail tests

### MVP

1. Add timeline API shape.
2. Render ordered list before attempting complex visual charts.
3. Link timeline rows to existing receipt/events/tool/context sections.
4. Add tests for rendering warnings and span statuses.

### Implemented Timeline Link Slice

Implemented on 2026-05-09:

- web inspector timeline rows now expose action links into existing step, event, receipt, source, and guardrail sections where matching data is available
- provider/tool/context span rows can reveal trace IDs, parent span IDs, conversation IDs, and span attributes inline
- timeline status rendering distinguishes warning-like events from normal event rows and preserves error span rendering
- chain detail page tests cover linked timeline rows, warning events, trace attributes, and receipt selection from a timeline row

Not implemented in this slice:

- dedicated provider request, tool output, context report, or diff detail panels beyond the chain detail sections that already exist
- complex timeline chart lanes
- browser approval controls from timeline rows

### Acceptance Criteria

- Web inspector can answer "what happened before this failure?" without reading raw logs.
- Timeline survives missing optional data.
- TUI remains the primary control surface; browser timeline is read-oriented unless approval controls are explicitly added.

---

## 12. Durable And Reconnectable Streaming

### Problem

Long chains outlive terminals, browser tabs, and WebSocket sessions. Yard already has chain event logs, but reconnect behavior should be treated as a core runtime guarantee.

Genkit's durable streaming idea is worth adapting in Yard's event-log terms.

### Yard Adaptation

Make persisted events the source of truth for chain and turn streaming.

For chains:

- Every streamable event has a monotonic event sequence or cursor.
- TUI/web can request events after cursor N.
- Follow mode replays missing events before tailing live events.

For interactive conversations:

- Persist enough turn events to reconstruct visible output after reconnect.
- Token-level persistence may be optional; chunk or message-level persistence is enough for MVP.
- Tool/provider/context events should be reconstructable from trace spans and persisted iteration data.

### Code Touchpoints

- `internal/chain/events.go`
- Chain store event queries
- `yard chain logs --follow`
- TUI follow mode
- WebSocket handler and browser reconnect logic
- Trace spans if item 1 lands first

### MVP

1. Add cursor-based chain event follow if not already complete.
2. Make TUI/web reconnect from last seen event ID.
3. Add tests for replay-then-tail behavior.
4. Defer token-level durable streaming unless browser refresh loses important state.

### Implemented CLI Cursor Slice

Implemented on 2026-05-08:

- `yard chain logs --after-id <event-id>`
- non-follow log reads can resume from a persisted event cursor using existing chain event IDs
- follow mode can replay missing events after a cursor before tailing new persisted events
- CLI tests cover duplicate suppression for already-seen event IDs

Not implemented in this slice:

- TUI/web reconnect from the last seen event ID
- interactive conversation replay
- token-level durable streaming

### Implemented Web Event Cursor Slice

Implemented on 2026-05-08:

- `GET /api/chains/{id}/events?after_id=<event-id>`
- browser/API consumers can replay only persisted chain events after their last seen cursor
- server tests cover full event reads and after-cursor duplicate suppression

### Implemented Browser Event Replay Slice

Implemented on 2026-05-09:

- chain detail view tracks the latest seen persisted event ID from its initial detail load
- active chain detail pages poll `GET /api/chains/{id}/events?after_id=<event-id>`
- newly replayed events are duplicate-suppressed and merged into both recent events and the ordered timeline
- browser tests cover after-cursor polling and timeline/recent-event replay from persisted chain events

Not implemented in this slice:

- WebSocket replay from a cursor
- interactive conversation replay

### Acceptance Criteria

- Restarting the TUI or refreshing the browser does not lose chain progress context.
- Follow mode catches up from persisted events before streaming live events.
- Duplicate events are suppressed by event ID/cursor.
- Chain logs remain useful even if the orchestrator process exits unexpectedly.

---

## Cross-Cutting Implementation Notes

### Storage Choice

Use SQLite for high-volume trace/eval/runtime detail that needs query speed and is already local runtime data. Use Shunter project memory for durable project knowledge, receipts, chain state, and human-readable summaries.

Avoid writing noisy trace internals into the brain. Summaries and receipts belong there; raw spans do not.

### Compatibility

All new contracts should tolerate old data:

- Old receipts render with warnings.
- Missing spans do not break chain detail views.
- Providers without capability metadata get conservative defaults.
- Prompts without frontmatter continue to load.
- Eval harness has deterministic suites that do not require provider credentials.

### Testing Strategy

Prefer narrow tests around each contract:

- Span nesting and persistence
- Receipt parser and schema validation
- Hook order and error behavior
- Capability preflight warnings
- Template compile validation
- Approval-required shell calls
- Retriever result metadata
- Timeline API serialization
- Replay-then-tail event behavior

Run `make test` after each narrow implementation slice.

---

## First Implementation Slice

Recommended first slice:

1. Add `internal/trace` with no-op and SQLite recorder.
2. Add spans around provider calls and tool execution.
3. Link span IDs into sub-call/tool-execution details where practical.
4. Add a chain timeline read endpoint backed by spans plus existing events.
5. Render a simple ordered timeline in the web inspector.

Why first:

- It improves debugging immediately.
- It gives later receipt/eval/approval work a shared observation layer.
- It can be implemented without changing agent behavior.
- It is easy to verify against existing tests plus a small chain smoke run.

---

## Open Decisions

1. Should trace spans be stored only in SQLite, or should a compact chain-level span summary also be written to Shunter?
2. Should receipt structured metadata live only in frontmatter, or should large sections such as findings use a fenced JSON block to avoid oversized frontmatter?
3. Should provider/tool hooks be internal only for v0.1, or should config eventually expose policy hooks?
4. Should approval waits be allowed in default chain mode, or only when the operator opts in?
5. How much token streaming needs durable persistence versus reconstructing final messages from conversation history?
6. Should `yard eval` live under the public operator surface immediately, or start as an internal/dev command until fixtures stabilize?
