# Task 02: Default Model Selection and Per-Conversation Override

**Epic:** 10 — Settings & Metrics
**Status:** ⬚ Not started
**Dependencies:** Task 01, Epic 07 Task 01 (WebSocket client hook)

---

## Description

Implement the default model selection in the settings panel and the per-conversation model override in the conversation view. The settings panel allows changing the global default provider/model via a dropdown that calls `PUT /api/config`. The conversation view includes a model selector that sends a `model_override` WebSocket event to change the model for the current conversation. The UI visually indicates when a conversation is using a non-default model.

## Acceptance Criteria

- [ ] **Default model selector:** Dropdown/select in the settings panel showing the current default provider and model
- [ ] Dropdown populated with available models from all configured providers (from `GET /api/providers`)
- [ ] Models grouped by provider in the dropdown for clarity
- [ ] Changing the selection calls `PUT /api/config` with the new `{default_provider, default_model}`
- [ ] Success feedback: display a brief "Default model updated" notification or inline confirmation
- [ ] Error feedback: if the PUT fails (invalid model, server error), display an error message and revert the selector to the previous value
- [ ] The current default is displayed prominently: "Default: claude-sonnet-4-20250514 (anthropic)"
- [ ] **Per-conversation model override:** A compact model selector in the conversation view header (e.g., a dropdown or button that opens a selector)
- [ ] Changing the model sends a `model_override` WebSocket event: `{type: "model_override", data: {provider, model}}`
- [ ] The override takes effect for the next turn — does not affect a currently running turn
- [ ] **Non-default indicator:** When a conversation is using a model different from the global default, display a visual indicator (e.g., badge, icon, or text note: "Using claude-sonnet-4-20250514 (override)")
- [ ] If no override is set, the selector shows the global default with a label like "Default"
- [ ] The model selector is disabled while a turn is running (to prevent confusing mid-turn changes)
