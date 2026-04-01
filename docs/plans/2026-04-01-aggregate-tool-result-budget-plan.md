# Aggregate Tool Result Budget Implementation Plan

> For Hermes: execute this plan with strict TDD. Write the failing test first, run it to confirm failure, then implement the smallest code to pass.

Goal: Add an aggregate budget pass over fresh tool results in the agent loop so multiple individually-acceptable tool outputs cannot collectively blow up the next request.

Architecture: Keep the first slice narrow and deterministic. Reuse the existing per-result normalization/truncation pipeline in internal/tool, then add a second-pass aggregate budget in the agent loop after tool execution and before persistence/current-turn message assembly. Do not add persisted-output artifacts in this slice.

Tech Stack: Go, internal/agent, internal/tool, existing loop tests, existing prompt builder/request path.

---

## Scope

In scope:
- Aggregate size budget for fresh tool results within one iteration
- Deterministic largest-first shrinking policy
- File-read-aware policy: deprioritize aggressive shrinking relative to other tools when possible
- Tests proving the next iteration sees budgeted outputs

Out of scope:
- Persisted-output refs / artifact store
- Prompt-cache memoization by tool_use_id
- Schema changes
- Full iteration-atomic tool_execution/sub_call persistence fix

---

## Task 1: Add failing loop tests for aggregate budgeting

Objective: Prove the current loop allows multiple tool results to exceed an aggregate budget and define the desired behavior.

Files:
- Modify: internal/agent/loop_compression_test.go

Steps:
1. Add a test where two tool calls each return medium-sized output, both individually under per-tool limits, but together exceed a low aggregate budget.
2. Assert that after the first tool-using iteration, the persisted/current-turn tool results are reduced enough that the next request would fit the aggregate budget.
3. Assert deterministic behavior: largest non-file_read result is shrunk first.
4. Run the focused test and confirm failure.

## Task 2: Add aggregate budget config and helper

Objective: Introduce a narrow config surface and pure helper for the budget pass.

Files:
- Modify: internal/agent/loop.go
- Create or modify: internal/agent/toolresult_budget.go
- Add tests: internal/agent/toolresult_budget_test.go

Steps:
1. Add config field for aggregate tool-result budget.
2. Write helper tests for largest-first shrinking and file_read deprioritization.
3. Implement a minimal pure helper that accepts fresh provider.ToolResult values plus tool names and returns budgeted results.
4. Run focused helper tests and confirm pass.

## Task 3: Wire the helper into RunTurn

Objective: Apply the budget pass at the real insertion point.

Files:
- Modify: internal/agent/loop.go

Steps:
1. Invoke the budget helper after toolResults collection and before persistMessages/currentTurnMessages are built.
2. Preserve tool_use_id linkage and error flags.
3. Keep the implementation deterministic and side-effect free beyond replacing content.
4. Run the targeted loop test and confirm pass.

## Task 4: Run regression coverage

Objective: Ensure no existing loop or compression behavior regresses.

Files:
- No code changes required unless tests reveal issues.

Steps:
1. Run aggregate-budget-specific tests.
2. Run internal/agent tests.
3. If failures appear, fix with smallest change and re-run.

## Task 5: Inspect persistence seam impact

Objective: Confirm this slice does not worsen the known iteration persistence gap.

Files:
- Read-only unless a tiny fix becomes necessary: internal/tool/adapter.go, internal/tool/executor.go, internal/conversation/history.go

Steps:
1. Confirm aggregate budgeting only changes message content, not persistence order.
2. Note separately that tool_execution persistence is still non-atomic / adapter-bypassed and should be a follow-up slice.

---

## Success criteria

- Multiple fresh tool results are budgeted in aggregate, not only individually.
- The budget pass runs in internal/agent/loop.go after tool execution and before persistence/current-turn message assembly.
- Behavior is deterministic.
- Existing agent tests still pass.
- No artifact store or schema churn is introduced in this slice.
