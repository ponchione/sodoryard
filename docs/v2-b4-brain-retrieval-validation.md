# V2-B4 proactive brain retrieval validation

This is the maintained live validation package for the current v0.2 brain-retrieval contract.

What it proves:
- a fresh turn against the running app can answer a brain-only fact from proactive context assembly
- the stored context report shows the actual semantic queries, included brain hits, and non-zero brain budget
- the ordered `/api/metrics/conversation/:id/context/:turn/signals` endpoint exposes the signal/query flow used to make the retrieval decision

What it does not claim:
- this is not proof of semantic/index-backed brain retrieval
- this is not a guarantee that the model will never choose a reactive brain tool detour for other prompts
- this package is intentionally scoped to the current operator-facing truth: MCP/vault-backed keyword retrieval is live today

## Runtime assumptions

Primary validated runtime:
- app: `http://localhost:8092`
- config: `/tmp/my-website-runtime-8092.yaml`
- target project: `~/source/my-website`
- brain vault: `~/source/my-website/.brain/`
- expected note: `notes/runtime-brain-proof-apr-07.md`
- expected fact: `ORBIT LANTERN 642`

The note exists only in the brain vault, not in the repo code. That makes it a useful operator-facing proof that the answer came from brain retrieval rather than ordinary code RAG.

## Exact prompt

Use this as the default first-turn prompt:

`What is the runtime brain proof canary phrase?`

Expected answer shape:
- contains `ORBIT LANTERN 642`
- completes from assembled context without explicit tool detours for the current validated runtime
- the matching note path is then corroborated from `brain_results` in the stored context report

## Repeatable command

From the repo root:

`python3 scripts/validate_brain_retrieval.py --base-url http://localhost:8092 --expected-note notes/runtime-brain-proof-apr-07.md`

Optional looser mode for exploratory prompts that may still choose reactive note reads:

`python3 scripts/validate_brain_retrieval.py --base-url http://localhost:8092 --prompt "What is the runtime brain proof canary phrase for this project? Answer in one sentence and cite the source note path." --expected-note notes/runtime-brain-proof-apr-07.md --allow-tool-calls`

The default command is the real V2-B4 proof because it requires the canary prompt to complete without explicit tool detours while still proving the proactive retrieval path through the persisted context report and signal stream.

## Passing conditions

The script must exit 0 and print JSON with `"status": "passed"`.

Required evidence in the output:
- `assistant_text` includes `ORBIT LANTERN 642`
- `semantic_queries` is non-empty
- `brain_results` contains `notes/runtime-brain-proof-apr-07.md`
- `budget_breakdown.brain > 0`
- `signal_stream` is non-empty and includes at least:
  - a `semantic_query` entry
  - a `flag` entry with `value: "prefer_brain_context"`

Useful corroboration:
- `tool_calls` is empty for the canary prompt, or at least does not need to carry the proof by itself
- `event_counts.context_debug == 1`
- `turn_number == 1` for the fresh conversation

## Manual spot checks

If you want to inspect the same run manually, use the printed `conversation_id` and `turn_number`:

`curl -s "http://localhost:8092/api/metrics/conversation/<conversation_id>/context/<turn_number>"`

`curl -s "http://localhost:8092/api/metrics/conversation/<conversation_id>/context/<turn_number>/signals"`

Things to confirm in the full context report:
- `needs.semantic_queries` is populated
- `brain_results` includes the runtime proof note and not just `_log.md`
- `budget_breakdown.brain` is non-zero
- code RAG is absent or clearly secondary for this prompt family

Things to confirm in the signal stream:
- the ordered stream shows the same retrieval intent the analyzer produced
- the semantic queries visible there match the queries persisted on the report
- `prefer_brain_context` appears when the prompt is clearly brain-seeking

## Failure interpretation

If the answer is right only after explicit `brain_*` tool use, but the report still shows empty `brain_results` or `budget_breakdown.brain == 0`, treat that as a proactive retrieval regression, not a pass.

If the report shows the expected note in `brain_results` and non-zero brain budget, but the answer is wrong, treat that as an answer-quality or prompt-shaping issue rather than proof that retrieval is absent.

If the signal stream is empty while the full context report looks right, treat that as an observability regression in the narrow `/signals` endpoint or its persistence path.

## Why this package exists

Earlier work proved the individual pieces separately:
- reactive brain tools worked
- proactive brain retrieval could appear in context reports
- the inspector consumed the dedicated `/signals` endpoint

V2-B4 closes the packaging gap by keeping one durable, repeatable command that proves those pieces together against the live app.
