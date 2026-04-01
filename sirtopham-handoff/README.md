# SirTopham handoff bundle from Claude Code reconnaissance

This directory is a compact package intended to be handed to the SirTopham agent.

Goal: convert the findings in `../cc-analysis.md` into Go-native implementation inputs instead of encouraging a wasteful TypeScript-to-Go port.

Contents:

- `01-priority-recommendations.md`
  - What is worth implementing now vs later
  - Concrete recommendations derived from the Claude Code analysis
- `02-package-sketch.md`
  - Proposed Go package boundaries, responsibilities, and interfaces
- `03-implementation-plan.md`
  - A staged implementation plan with a narrow first slice and explicit deferrals
- `04-agent-brief.md`
  - Copy-pasteable briefing for the SirTopham agent
- `stubs/tooloutput/tooloutput.go`
  - Go-oriented interfaces and types for per-result and aggregate tool-output budgeting
- `stubs/fileedit/fileedit.go`
  - Go-oriented interfaces and types for read-before-edit and stale-write protection
- `stubs/turnstate/turnstate.go`
  - Go-oriented interfaces and types for cancellation cleanup and transcript invariants
- `stubs/promptcache/promptcache.go`
  - Go-oriented types for prompt block stability and cache-relevant latching
- `stubs/tokenbudget/tokenbudget.go`
  - Go-oriented types for response reserve, estimation, and overflow decisions

How to use this bundle:

1. Read `01-priority-recommendations.md` first.
2. Use `02-package-sketch.md` as the architectural map.
3. Use `03-implementation-plan.md` if the agent should plan before coding.
4. Use `04-agent-brief.md` as the initial prompt or handoff text.
5. Treat the `.go` files as design stubs, not production-ready code.

Guiding principle:

Do not port Claude Code. Port only the durable architecture patterns:
- output persistence + preview references
- aggregate result budgeting
- read-before-edit + stale-write invariants
- cancellation cleanup preserving transcript structure
- prompt-cache stability and latching

The most important idea from `cc-analysis.md` is that Claude Code's maturity comes from edge-management at system seams, not from one magical algorithm.
