# sodoryard — Architecture Documents

**Project:** sodoryard
**Version:** current
**Last Updated:** 2026-05-01

---

## Document Index

| #   | Document                             | Status  | Summary                                                                |
| --- | ------------------------------------ | ------- | ---------------------------------------------------------------------- |
| 01  | [[01-project-vision-and-principles]] | ✅ Draft | What sodoryard is, design principles, success criteria                 |
| 02  | [[02-tech-stack-decisions]]          | ✅ Draft | Every technology choice with rationale and alternatives                |
| 03  | [[03-provider-architecture]]         | ✅ Draft | LLM provider interface, OAuth credential reuse, routing                |
| 04  | [[04-code-intelligence-and-rag]]     | ✅ Draft | Tree-sitter, embeddings, LanceDB vector search                         |
| 05  | [[05-agent-loop]]                    | ✅ Draft | Core orchestration, turn state machine, tool dispatch                  |
| 06  | [[06-context-assembly]]              | ✅ Draft | Per-turn context retrieval, budget management, the differentiator      |
| 07  | [[07-web-interface-and-streaming]]   | ✅ Draft | Browser stack, WebSocket protocol, web inspector components            |
| 08  | [[08-data-model]]                    | ✅ Draft | SQLite schema for conversations, messages, metrics, indexing           |
| 09  | [[09-project-brain]]                 | ✅ Draft | Obsidian-backed project knowledge base, retrieval, agent co-authorship |
| 10  | [[10_Tool_System]]                   | ✅ Draft | Tool registry, execution contracts, path safety, and brain/file tools  |
| 11  | [[11-tool-result-normalization]]     | ✅ Draft | Tool-result cleanup, budgeting, persistence, and history compression   |
| 12  | [[12_Claude_Code_Analysis_Retrofits]] | ✅ Draft | Retrofitted lessons from Claude Code analysis                          |
| 13  | [[13_Headless_Run_Command]]          | ✅ Draft | Internal chain-step engine contract; one-step chains replace `yard run` |
| 14  | [[14_Agent_Roles_and_Brain_Conventions]] | ✅ Draft | Railway roles, brain ownership, safety limits, and receipts        |
| 15  | [[15-chain-orchestrator]]            | ✅ Draft | Chain execution, step spawning, control, and receipts                  |
| 16  | [[16-yard-init]]                     | ✅ Draft | `yard init` project bootstrap and seeded role config                   |
| 17  | [[17-yard-containerization]]         | ✅ Draft | Container packaging and no-legacy `yard` container UX                 |
| 18  | [[18-unified-yard-cli]]              | ✅ Draft | Unified operator CLI and retained internal `tidmouth` contract        |
| 19  | [[19-tool-result-details]]           | ✅ Draft | Structured tool-result metadata for UI and analytics, content unchanged |
| 20  | [[20-operator-console-tui]]          | ✅ Draft | Terminal-first operator console, launch, chain control, and receipts    |
| 21  | [[21-web-inspector]]                 | ✅ Draft | Browser inspector for transcripts, context, tools, diffs, and metrics   |

## Status Legend

- ✅ **Draft** — Substantive content based on completed discussions. Ready for review and refinement.
- ⚠️ **Skeleton** — Structure and key questions defined. Needs dedicated deep-dive conversation to fill in.
- 🔴 **Blocked** — Cannot proceed without resolving a dependency.

## Next Actions

1. Keep the specs aligned with the live `yard` / `tidmouth` / container/runtime contract.
2. Treat the TUI operator console as the active daily-driver UI target, specified in [[20-operator-console-tui]].
3. Treat the browser app as the rich inspector specified in [[21-web-inspector]], not as a second command center.
4. Remove stale planning residue when a slice is fully landed.
5. Prefer `NEXT_SESSION_HANDOFF.md` plus the current README over old implementation plans when resuming work.
6. Treat these specs as current-truth architecture docs, not historical migration notes.

## Architecture Diagram (Layers)

```
┌─────────────────────────────────────────────┐
│  Layer 6: Operator Interfaces               │  [[20-operator-console-tui]]
│  TUI + CLI + Web Inspector                  │  [[21-web-inspector]]
│  WebSocket + REST for browser inspection    │  [[07-web-interface-and-streaming]]
├─────────────────────────────────────────────┤
│  Layer 5: Agent Loop                        │  [[05-agent-loop]]
│  Turn orchestration, tool dispatch          │
├─────────────────────────────────────────────┤
│  Layer 4: Tool System                       │  [[10_Tool_System]]
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
