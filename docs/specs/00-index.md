# sirtopham — Architecture Documents

**Project:** sirtopham
**Version:** Pre-implementation
**Last Updated:** 2026-03-27

---

## Document Index

| #   | Document                             | Status  | Summary                                                                |
| --- | ------------------------------------ | ------- | ---------------------------------------------------------------------- |
| 01  | [[01-project-vision-and-principles]] | ✅ Draft | What sirtopham is, design principles, success criteria                 |
| 02  | [[02-tech-stack-decisions]]          | ✅ Draft | Every technology choice with rationale and alternatives                |
| 03  | [[03-provider-architecture]]         | ✅ Draft | LLM provider interface, OAuth credential reuse, routing                |
| 04  | [[04-code-intelligence-and-rag]]     | ✅ Draft | Tree-sitter, embeddings, vector store — blocked on LanceDB eval        |
| 05  | [[05-agent-loop]]                    | ✅ Draft | Core orchestration, turn state machine, tool dispatch                  |
| 06  | [[06-context-assembly]]              | ✅ Draft | Per-turn context retrieval, budget management, the differentiator      |
| 07  | [[07-web-interface-and-streaming]]   | ✅ Draft | Frontend stack, WebSocket protocol, UI components                      |
| 08  | [[08-data-model]]                    | ✅ Draft | SQLite schema for conversations, messages, metrics, indexing           |
| 09  | [[09-project-brain]]                 | ✅ Draft | Obsidian-backed project knowledge base, retrieval, agent co-authorship |
## Status Legend

- ✅ **Draft** — Substantive content based on completed discussions. Ready for review and refinement.
- ⚠️ **Skeleton** — Structure and key questions defined. Needs dedicated deep-dive conversation to fill in.
- 🔴 **Blocked** — Cannot proceed without resolving a dependency.

## Next Actions

1. **Evaluate LanceDB** — Audit topham's indexing pipeline, run test queries, assess retrieval quality. Unblocks [[04-code-intelligence-and-rag]].
2. **Deep dive: Agent Loop** — Design the turn state machine, tool dispatch, iteration management. Fills in [[05-agent-loop]].
3. **Deep dive: Context Assembly** — Design the turn analyzer, trigger heuristics, budget management. Fills in [[06-context-assembly]].
4. **Deep dive: Frontend Stack** — Finalize React vs alternatives, design the streaming protocol. Fills in [[07-web-interface-and-streaming]].

## Architecture Diagram (Layers)

```
┌─────────────────────────────────────────────┐
│  Layer 6: Web Interface                     │  [[07-web-interface-and-streaming]]
│  React + WebSocket + REST                   │
├─────────────────────────────────────────────┤
│  Layer 5: Agent Loop                        │  [[05-agent-loop]]
│  Turn orchestration, tool dispatch          │
├─────────────────────────────────────────────┤
│  Layer 4: Tool System                       │  (document TBD)
│  Shell, file, search, git tools             │
├──────────────────────┬──────────────────────┤
│  Layer 3: Context    │  Layer 2: Model      │  [[06-context-assembly]]
│  Assembly            │  Routing             │  [[03-provider-architecture]]
├──────────────────────┴──────────────────────┤
│  Layer 1: Code Intelligence                 │  [[04-code-intelligence-and-rag]]
│  Tree-sitter, embeddings, vector store      │
├─────────────────────────────────────────────┤
│  Layer 0: Foundation                        │  [[08-data-model]]
│  Config, SQLite, structured logging         │  [[02-tech-stack-decisions]]
└─────────────────────────────────────────────┘
```
