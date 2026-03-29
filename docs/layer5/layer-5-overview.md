# Layer 5: Agent Loop — Epic Overview

**Build Phase:** 5 (Core Orchestration)
**Architecture Docs:** [[05-agent-loop]], [[06-context-assembly]]
**Layer Dependencies:** [[layer-0-overview]] (Epics 01-06), [[layer-1-overview]] (Epics 01-09), [[layer-2-overview]] (Epics 01-07), [[layer-3-overview]] (Epics 01, 06-07), [[layer-4-overview]] (Epics 01-06)
**Layer Consumers:** Layer 6/7 (Web Interface & Streaming)
**Last Updated:** 2026-03-29

---

## Summary

Layer 5 is the orchestration engine — the turn state machine that receives a user message, assembles context, calls the LLM, dispatches tools, iterates until the turn is complete, and persists everything. It owns the agent-loop runtime and the prompt-building machinery around it.

Context assembly is no longer decomposed here. Turn Analyzer, Context Assembly Pipeline, and Compression Engine are canonically owned by Layer 3 and should not be duplicated in Layer 5. Layer 5 consumes those Layer 3 outputs.

To preserve existing cross-layer references, the remaining Layer 5 epics keep their original numbering after the Layer 3 extraction. Layer 5 therefore retains only Epics 01, 02, 05, and 06.

---

## Dependency Graph

```
Layers 0-4 (foundations complete) + Layer 3 (context assembly)
                    │
        Epic 01 (Event System & Session Types)
                    │
        Epic 02 (Conversation Manager)
                    │
        Epic 05 (System Prompt Builder)
                    │
        Epic 06 (Agent Loop Core)
```

### External Layer 3 Dependencies

- **Layer 3 Epic 01** provides the context-assembly types consumed by Layer 5 (`FullContextPackage`, `ContextAssemblyReport`).
- **Layer 3 Epic 06** provides the assembled context pipeline invoked at turn start.
- **Layer 3 Epic 07** provides conversation compression, which the agent loop invokes when history exceeds budget.

### Internal Dependency Order

1. **Epic 01** (Event System & Session Types) — foundational agent-loop contracts.
2. **Epic 02** (Conversation Manager) — persistence and reconstruction used by later epics.
3. **Epic 05** (System Prompt Builder) — consumes conversation history plus Layer 3 context output.
4. **Epic 06** (Agent Loop Core) — capstone orchestration over Layers 2, 3, and 4.

---

## Epic Summary

| # | Epic | Layer | Status | Dependencies |
|---|------|-------|--------|--------------|
| 01 | Event System & Session Types | 5 | ⬚ | Layer 0 Epic 01; Layer 3 Epic 01 (report/type reuse) |
| 02 | Conversation Manager | 5 | ⬚ | Epic 01, Layer 0 (04-06), Layer 2 (01, 06-07) |
| 05 | System Prompt Builder | 5 | ⬚ | Epic 02, Layer 3 (01, 06), Layer 2 Epic 01, Layer 4 Epic 01 |
| 06 | Agent Loop Core | 5 | ⬚ | Epics 01-02, 05, Layer 3 (06-07), Layer 2 (01, 06-07), Layer 4 Epic 01 |

---

## Cross-Layer Dependencies

### What This Layer Consumes

| From Layer | Epic(s) | What's Used |
|------------|---------|-------------|
| Layer 0 Epic 01 | Package layout (`internal/agent/`, `internal/conversation/`) |
| Layer 0 Epic 02 | Structured logging for loop execution and event emission |
| Layer 0 Epic 03 | Agent config, prompt config, compression thresholds |
| Layer 0 Epic 04 | SQLite connection manager |
| Layer 0 Epic 05 | UUIDv7 generation — conversation IDs |
| Layer 0 Epic 06 | Schema & sqlc — conversations, messages, sub_calls, tool_executions, context_reports |
| Layer 2 Epic 01 | Provider interface (Stream, Complete), StreamEvent types, Request/Response types |
| Layer 2 Epic 06 | Sub-call tracking middleware |
| Layer 2 Epic 07 | Provider router — model selection |
| Layer 3 Epic 01 | `FullContextPackage`, `ContextAssemblyReport`, related context types |
| Layer 3 Epic 06 | Context assembly pipeline |
| Layer 3 Epic 07 | Compression engine |
| Layer 4 Epic 01 | Tool interface, Registry, Executor |

### What Consumes This Layer

| Consumer | What It Needs |
|----------|---------------|
| Layer 6/7 (Web UI) | EventSink subscription, WebSocket bridge, REST API access to conversations, RunTurn/Cancel APIs |

---

## Architecture Doc References

- [[05-agent-loop]] — Primary. Entire document defines Layer 5.
- [[06-context-assembly]] — Consumed by Layer 5; Layer 3 owns the implementation.
- [[03-provider-architecture]] — Provider interface and router consumed by the loop.
- [[04-code-intelligence-and-rag]] — Indirectly consumed through Layer 3 context assembly.
- [[07-web-interface-and-streaming]] — WebSocket and REST consumers of the loop.
- [[08-data-model]] — Conversations, messages, tool executions, sub_calls, context_reports schemas.

---

## Status Legend

- ⬚ Not started
- 🔨 In progress
- ✅ Complete
