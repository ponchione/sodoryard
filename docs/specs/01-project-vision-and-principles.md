# 01 — Project Vision & Principles

**Status:** Draft v0.1
**Last Updated:** 2026-05-01
**Author:** Mitchell

---

## What sodoryard Is

sodoryard is a self-hosted AI coding agent with RAG-powered codebase awareness, built in Go. It runs as a local web application — a single CLI command launches a browser-based interface where a developer has a conversation with an AI agent that deeply understands their codebase.

The core differentiator: every conversation turn is backed by programmatic, task-specific context assembly. Instead of relying on static context files (AGENTS.md, CLAUDE.md) — which research has shown to decrease agent success rates and increase costs — sodoryard dynamically assembles relevant code, conventions, and project structure via RAG and tree-sitter code intelligence.

The agent doesn't get a generic project overview dumped into its system prompt. It gets exactly the code, types, and conventions relevant to what the developer just asked about.

## What sodoryard Is Not

- **Not only a rigid pipeline tool.** Its predecessor (topham) was a batch pipeline (scope → build → verify → approve). sodoryard keeps interactive conversation as a primary mode while also exposing agent-judged chain orchestration for non-interactive work.
- **Not a wrapper around Claude Code or Codex.** It is its own agent with its own tool system. It *borrows credentials* from those tools but runs its own agent loop.
- **Not a TUI.** The interface is a web application served locally in the browser.
- **Not a multi-user system.** Single developer, single machine. No auth, no tenancy, no sharing.
- **Not built for mass adoption.** This is a personal tool, purpose-built for the developer who created it. Other users are welcome to run it, but they're self-selecting into a tool that requires conscious setup and has opinions.

## Who It's For

Mitchell. That's it. Other people can use it, and reasonable accommodation will be made to keep it approachable, but no architectural decision will be compromised for hypothetical users. If someone else wants to run it, they follow the README and meet the prerequisites.

## The Research Basis

Two papers inform the core architecture:

**ETH Zurich / LogicStar.ai (arXiv:2602.11988v1):** Static context files (AGENTS.md, etc.) decrease agent success rates by ~3% for LLM-generated files while increasing costs 20%+. Developer-written files marginally help (+4%) but still increase cost. Conclusion: programmatic, minimal, task-specific context is the correct approach.

**MIT CSAIL RLM (arXiv:2512.24601v1):** Recursive Language Models treat long prompts as external environments the LLM can programmatically examine and decompose. Relevant for how sodoryard handles large codebases — decompose via code intelligence rather than context stuffing.

## Design Principles

1. **Programmatic context, never static files.** No AGENTS.md, no CLAUDE.md. Context is assembled per-turn from RAG, code intelligence, and project conventions.

2. **Single binary.** The Go binary embeds the frontend. One build artifact. The only external dependencies are Docker containers for local models/embeddings (optional if using only subscription-based cloud providers).

3. **Local-first.** No external services required for core functionality. SQLite, LanceDB (or alternative), and local models all run on the developer's machine. Cloud LLM providers are accessed via existing subscriptions, not per-token API costs.

4. **Observable by default.** Every LLM call, tool execution, and RAG query is logged and metriced. The web UI exposes this — understanding what the agent is doing builds trust and improves the system over time.

5. **Model-agnostic.** The provider interface abstracts over Anthropic, OpenAI/Codex, local models, and others. Switching models is config, not code.

6. **Tools are first-class.** The tool system is generic and extensible. Adding a new tool is: implement the interface, register it, done.

7. **Web-first command center with a unified operator CLI.** The browser is the primary interactive and operational interface: chat, project readiness, chain monitoring/control, context inspection, and metrics live there. The `yard` CLI remains the scriptable/operator fallback for init, serve, index, config, auth, doctor, brain, llm, and chain orchestration. Autonomous single-agent work is a one-step chain, not a separate run surface.

8. **Conversations and chains as the units of work.** Interactive chat is conversation-shaped. Autonomous harness work is chain-shaped, including one-step chains. Context assembly, metrics, persistence, receipts, and cost tracking attach to the appropriate unit instead of inventing a third execution model.

9. **Zero API cost for inference.** LLM access piggybacks on existing Claude and Codex subscriptions via OAuth credential reuse. No separate API billing.

## Prior Art

### Hermes Agent (NousResearch)
MIT-licensed Python agent harness. Key ideas adopted or studied:
- Self-improving skills system (future consideration)
- Agent-curated persistent memory (future consideration)
- OAuth credential discovery and reuse pattern (adopted — reimplemented in Go)
- Provider abstraction with fallback routing (adopted)
- Context compression with lineage-aware persistence (adopted concept)

Hermes's weaknesses that sodoryard addresses:
- Relies on static context files rather than programmatic assembly
- No tree-sitter code intelligence or vector-based code search
- Python — sodoryard is Go for single-binary distribution and lower resource footprint

### topham (predecessor)
Go-based pipeline orchestrator. Components being carried forward:
- Tree-sitter parsers (Go, TypeScript/TSX, Python)
- LanceDB vector store with nomic-embed-code embeddings
- Context assembly concepts
- Shell git execution (not go-git)
- SQLite persistence patterns
- Sub-call tracking and metrics

## What Success Looks Like

**v0.1 — Walking skeleton:** Index a Go project, open a browser, have a multi-turn conversation where the agent reads files, runs commands, and edits code — with RAG-assembled context visible in a debug panel.

**v0.5 — Daily driver:** Replace Hermes as the primary interactive coding agent. Conversations feel informed by the codebase. Context assembly noticeably reduces the need to manually paste code.

**v1.0 — Resume piece:** A polished, self-contained AI coding agent with a compelling command-center UI, demonstrably superior context assembly backed by research, and clean Go architecture.

## Open Questions

Tracked in individual architecture documents. High-level items still unresolved:
- Context assembly trigger heuristics and quality tuning (see [[06-context-assembly]])
- Skills/memory system design beyond the project brain (future — v0.5+)

Resolved since the original draft:
- LanceDB is the selected vector store for current code and brain semantic indexes.
- React/Vite/TypeScript is the selected frontend stack.
- Brain access uses the project brain MCP/vault-backed runtime path.
- UI-driven chain execution is now active command-center scope in [[20-command-center-ui]] and uses the same internal chain start path as `yard chain start`.
