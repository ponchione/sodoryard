# Layer 3: Context Assembly — Epic Overview

**Build Phase:** 5 (combined with Layer 5 — Agent Loop)
**Architecture Doc:** [[06-context-assembly]]
**Layer Dependencies:** [[layer-0-overview]] (Epics 01-06), [[layer-1-overview]] (Epics 01-09), [[layer-2-overview]] (Epics 01-07)
**Layer Consumers:** Layer 5 (Agent Loop — consumes FullContextPackage)
**Last Updated:** 2026-03-29

---

## Summary

Layer 3 implements sirtopham's core differentiator: per-turn, RAG-driven context assembly. Before every LLM call in a turn, the system analyzes the user's message, retrieves relevant code and project knowledge from multiple sources in parallel, fits results into a token budget by priority, and serializes the package as a markdown block in the system prompt.

This layer is entirely net-new — no prior art exists for per-turn RAG-driven context assembly in a conversational coding agent. Hermes Agent uses static context files. Claude Code uses CLAUDE.md. sirtopham dynamically assembles exactly the context the current turn needs. Heavy instrumentation (ContextAssemblyReport, quality metrics) is built in from day one because there is no existing system to benchmark against.

This layer depends on **Layer 0** (config, SQLite, logging), **Layer 1** (RAG searcher, structural graph, embedding client, convention cache), and **Layer 2** (provider for compression summarization calls, context window metadata per model). It does NOT depend on Layer 4 (Tool System) or Layer 5 (Agent Loop). Layer 5 consumes Layer 3's `FullContextPackage` output.

### Relationship to Layer 5

The earlier combined Layer 5+3 decomposition placed Turn Analyzer, Context Assembly Pipeline, and Compression Engine as Layer 5 epics. With Layer 3 now having its own directory, those three epics belong here and should not be duplicated in Layer 5. Compression Engine ownership is resolved here: Layer 3 owns the canonical implementation in `internal/context/`, and Layer 5 consumes it. Layer 5 retains: Event System & Session Types, Conversation Manager, System Prompt Builder, Agent Loop Core.

---

## Epic Index

| #   | Epic                                                    | Status | Dependencies                                    |
| --- | ------------------------------------------------------- | ------ | ----------------------------------------------- |
| 01  | [[layer-3-epic-01-context-assembly-types]]              | ⬚     | Layer 0: Epics 01, 03                           |
| 02  | [[layer-3-epic-02-turn-analyzer]]                       | ⬚     | Epic 01                                         |
| 03  | [[layer-3-epic-03-query-extraction-momentum]]           | ⬚     | Epic 01                                         |
| 04  | [[layer-3-epic-04-retrieval-orchestrator]]              | ⬚     | Epic 01; Layer 1: Epics 01, 07, 08; Layer 0: 03 |
| 05  | [[layer-3-epic-05-budget-manager-serialization]]        | ⬚     | Epic 01; Layer 2: Epic 01 (context window meta) |
| 06  | [[layer-3-epic-06-context-assembly-pipeline]]           | ⬚     | Epics 02, 03, 04, 05, 07; Layer 0: Epics 04, 06 |
| 07  | [[layer-3-epic-07-compression-engine]]                  | ⬚     | Epic 01; Layer 2: Epics 01, 07 (provider calls) |

---

## Dependency Graph

```
Layer 0 (Foundation, complete)
Layer 1 (Code Intelligence, complete)
Layer 2 (Provider Architecture, complete)
                    │
           E01 (Context Assembly Types & Interfaces)
     ┌──────┼──────────┬──────────┬──────────┐
     │      │          │          │          │
   E02    E03        E04        E05       E07
  (Turn   (Query    (Retrieval  (Budget   (Compression
  Analyzer) Extract   Orch)     Manager    Engine)
     │    & Momentum)  │       & Serial)     │
     │      │          │          │          │
     └──────┴──────────┴──────────┘          │
                    │                        │
                  E06 ───────────────────────┘
              (Context Assembly Pipeline)
```

### Parallelization

- **Sequential gate:** Epic 01 must complete first — all types and interfaces.
- **Parallel tracks:** Epics 02, 03, 04, 05, and 07 can all execute simultaneously after Epic 01. Each has its own testability surface and no compile-time dependency on the others.
- **Sequential gate:** Epic 06 (Pipeline) requires all other epics — it wires them together and produces the `FullContextPackage` that Layer 5 consumes.

### Critical Path

E01 → E04 → E06 is the longest chain (types → retrieval orchestrator → pipeline), because E04 has the most complex I/O coordination and the most dependencies on Layer 1.

### Recommended Build Order

1. **E01** (types — everything depends on it)
2. **E02 + E03 + E05 + E07** in parallel (pure computation, no external I/O, easy to test)
3. **E04** (retrieval orchestrator — needs Layer 1 searcher, structural graph, convention cache, and git-context retrieval wired up; proactive project-brain retrieval joins in v0.2)
4. **E06** (pipeline — capstone, wires everything together)

---

## Status Legend

- ⬚ Not started
- 🟡 In progress
- ✅ Complete
