# Layer 2: Provider Architecture — Epic Overview

**Build Phase:** 3 (Model Routing)
**Architecture Doc:** [[03-provider-architecture]]
**Layer Dependencies:** [[layer-0-overview]] (Epics 01-06 complete)
**Layer Consumers:** Layer 3 (Context Assembly), Layer 5 (Agent Loop)
**Last Updated:** 2026-03-28

---

## Summary

Layer 2 implements the provider abstraction that sits between sirtopham's agent loop and all LLM inference services. It covers the unified provider interface, three backend implementations (Anthropic OAuth, Codex CLI delegation, OpenAI-compatible), credential lifecycle management, sub-call tracking, and the routing layer that selects which provider handles each request.

This layer has **no dependency on Layer 1** (Code Intelligence). They are parallel tracks. Layer 2's consumers are Layer 3 (Context Assembly — for compression calls and context window metadata) and Layer 5 (Agent Loop — for all chat inference).

---

## Epic Index

| #   | Epic                                                            | Status | Tasks | Dependencies                              |
| --- | --------------------------------------------------------------- | ------ | ----- | ----------------------------------------- |
| 01  | [Provider Types & Interface](01-provider-types/)                | ⬜     | 6     | Layer 0: Epics 01, 03                     |
| 02  | [Anthropic Credential Manager](02-anthropic-credentials/)       | ⬜     | 5     | Epic 01; Layer 0: Epics 02, 03            |
| 03  | [Anthropic Provider](03-anthropic-provider/)                    | ⬜     | 7     | Epics 01, 02; Layer 0: Epic 02            |
| 04  | [OpenAI-Compatible Provider](04-openai-compatible/)             | ⬜     | 6     | Epic 01; Layer 0: Epics 02, 03            |
| 05  | [Codex Provider](05-codex-provider/)                            | ⬜     | 7     | Epic 01; Layer 0: Epics 02, 03            |
| 06  | [Sub-Call Tracking](06-sub-call-tracking/)                      | ⬜     | 4     | Epic 01; Layer 0: Epics 02, 04, 06        |
| 07  | [Provider Router](07-provider-router/)                          | ⬜     | 6     | Epics 03, 04, 05, 06; Layer 0: Epics 02, 03 |

---

## Dependency Graph

```
Layer 0 Epics 01-06 (complete)
         │
    Epic 01 (Provider Types & Interface)
    ┌────┼──────────────┬──────────────┐
    │    │              │              │
    │  Epic 02          │              │
    │  (Anthropic       │              │
    │   Credentials)    │              │
    │    │              │              │
    │  Epic 03        Epic 04        Epic 05
    │  (Anthropic     (OpenAI-       (Codex
    │   Provider)      Compatible)    Provider)
    │    │              │              │
    │    │              │              │
  Epic 06              │              │
  (Sub-Call            │              │
   Tracking)           │              │
    │    │              │              │
    │    └──────────────┴──────────────┘
    │                   │
    └────────┬──────────┘
             │
           Epic 07
           (Provider Router)
```

### Parallelization

- **Sequential gate:** Epic 01 must complete first — everything depends on the types.
- **Parallel track A:** Epics 02 → 03 (Anthropic credential manager, then Anthropic provider)
- **Parallel track B:** Epic 04 (OpenAI-compatible provider)
- **Parallel track C:** Epic 05 (Codex provider)
- **Parallel track D:** Epic 06 (Sub-call tracking)
- **Sequential gate:** Epic 07 (Router) requires all providers + tracking to be complete.

Tracks A, B, C, and D can execute simultaneously after Epic 01. Epic 07 is the capstone.

### Implementation Priority

Per [[03-provider-architecture]], recommended build order within the parallel tracks:

1. **Anthropic provider** (Epics 02 → 03) — Primary inference path, most complex, highest quality.
2. **OpenAI-compatible provider** (Epic 04) — Simplest, enables local models immediately.
3. **Codex provider** (Epic 05) — Requires external CLI dependency. Build last.

---

## Layer 0 Dependencies

| Layer 0 Epic | What Layer 2 Uses                                                           |
| ------------ | --------------------------------------------------------------------------- |
| Epic 01      | Package layout (`internal/provider/`)                                       |
| Epic 02      | Structured logging for credential discovery, API calls, errors              |
| Epic 03      | Config loading — provider config section (default/fallback provider, model, base URLs) |
| Epic 04      | SQLite connection manager — for sub_call persistence                        |
| Epic 06      | Schema & sqlc — `sub_calls` table definition and generated Go code          |

---

## Architecture Doc References

- [[03-provider-architecture]] — Primary. Entire document defines Layer 2.
- [[02-tech-stack-decisions]] — Frontier LLM access via subscription credential reuse, local LLM via Docker.
- [[05-agent-loop]] — Provider consumer. Steps 4-5 (LLM request, response handling), prompt caching strategy, extended thinking, streaming events.
- [[06-context-assembly]] — Provider consumer. Context window limits per model for budget management, compression summarization calls.
- [[08-data-model]] — `sub_calls` table schema with cache token columns.

---

## Status Legend

- ⬜ Not started
- 🔨 In progress
- ✅ Complete
