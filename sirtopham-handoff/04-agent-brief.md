# Copy-paste brief for the SirTopham agent

You are being given a handoff bundle derived from a focused architectural reconnaissance of Claude Code.

Primary source documents:
- `cc-analysis.md`
- `sirtopham-handoff/01-priority-recommendations.md`
- `sirtopham-handoff/02-package-sketch.md`
- `sirtopham-handoff/03-implementation-plan.md`
- `sirtopham-handoff/stubs/...`

Your job is not to port Claude Code from TypeScript to Go.
Your job is to extract and incorporate the highest-value architecture patterns into SirTopham in a Go-native way.

Important framing:
- SirTopham already has a strong architecture: session -> turn -> iteration, per-turn frozen RAG context, typed WebSocket events, SQLite persistence, exact-match file editing, and a two-phase compression concept.
- The Claude Code analysis mainly suggests refinements at system seams, not a replacement architecture.

Highest-priority findings to incorporate:
1. Tool results should be budgeted both per-result and in aggregate across the next model-visible message.
2. Oversized tool outputs should usually be persisted outside the prompt and replaced with preview + stable reference, not just truncated.
3. File editing should enforce full-read-before-edit and stale-write checks immediately before write.
4. Cancellation/interrupt paths should preserve transcript invariants by tombstoning or finalizing partial assistant/tool state deterministically.
5. Prompt-cache stability should be treated as a first-class invariant, with stable vs dynamic prompt blocks explicit in code.
6. Token budgeting should reserve response space and reconcile estimates with actual usage.

Strong cautions:
- Do not transliterate Claude Code modules or naming blindly.
- Do not overfit to Anthropic-specific cache-edit tricks before simpler mechanisms exist.
- Do not start with classifier/safety machinery or transport redesign.

Recommended working mode:
- First, map the proposed handoff packages to the actual SirTopham codebase.
- Second, decide whether to stay in planning mode or implement the first slice.
- If implementing, start with the `tooloutput` slice only.

Concrete planning questions to answer:
1. Where in the current codebase should tool-output normalization live?
2. Where should persisted oversized outputs be stored?
3. Should file-read outputs be exempt from persisted-output substitution?
4. How should tombstones/synthesized tool results appear in the durable transcript?
5. Which request fields must latch to preserve prompt cache stability?
6. What is the minimal first slice that adds clear value with low churn?

Preferred deliverables from you:
- a package/path mapping into the real SirTopham repo
- a short decision memo for persisted-output storage
- a first-slice implementation plan with exact files and tests
- or, if straightforward, a narrow implementation of the tool-output manager slice

Bottom line:
Use Claude Code as a source of proven architecture patterns around edge-management, not as a codebase to port.
