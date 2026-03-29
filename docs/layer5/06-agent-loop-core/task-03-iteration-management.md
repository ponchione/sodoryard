# Task 03: Iteration Management

**Epic:** 06 — Agent Loop Core
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Implement loop detection and iteration limit enforcement within the turn state machine. Loop detection catches cases where the LLM is stuck calling the same tool with the same arguments repeatedly (a common failure mode). The iteration limit prevents runaway turns from consuming unlimited tokens. When the iteration limit is reached, the final LLM call has tools disabled, forcing a text-only response that ends the turn.

## Acceptance Criteria

- [ ] **Loop detection:** after each tool dispatch, compare the current tool call(s) against the previous N iterations (configurable, default threshold 3). If the same tool is called with identical arguments for `LoopDetectionThreshold` consecutive iterations:
  - Inject a nudge message before the next LLM call: a system-injected user message like `"You appear to be repeating the same action. Please try a different approach or explain what you're trying to accomplish."`
  - The nudge does NOT stop the loop — it redirects the LLM. The iteration continues normally
  - Emit a warning-level structured log
- [ ] **Loop detection comparison:** tool call identity is determined by `(tool_name, arguments_json)` tuple. JSON arguments are compared after canonicalization (sorted keys) to avoid false negatives from key ordering differences
- [ ] **Iteration limit:** configurable maximum iterations per turn (default 50 from `AgentLoopConfig.MaxIterations`)
- [ ] **Final iteration behavior:** when `currentIteration == MaxIterations`, the next LLM call is made with `DisableTools = true` in the prompt config. This forces the LLM to produce a text-only response, ending the turn. A directive message is injected before the final call: `"You have reached the maximum number of tool calls for this turn. Please provide a text summary of your progress and any remaining work."`
- [ ] **Iteration count tracking:** the current iteration number is tracked throughout the turn and included in the `TurnCompleteEvent`
- [ ] If `MaxIterations` is set to 0 or negative, the limit is effectively unlimited (no cap). This is primarily for testing
- [ ] Package compiles with `go build ./internal/agent/...`
