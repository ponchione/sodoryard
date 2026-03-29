# Task 01: Settings Panel and Provider List Display

**Epic:** 10 — Settings & Metrics
**Status:** ⬚ Not started
**Dependencies:** Epic 06 Task 04 (app shell), Epic 06 Task 05 (API client)

---

## Description

Create the settings panel accessible from the app header or sidebar, and populate it with the list of configured providers and project information. The panel opens as a modal, drawer, or dedicated route and displays all configured providers with their types, availability status, and available models. It also shows basic project information (name, root path, language, last indexed time).

## Acceptance Criteria

- [ ] **Settings panel:** Accessible via a settings icon/gear button in the app header or sidebar
- [ ] Panel opens as a modal, slide-out drawer, or dedicated route (agent's choice for the pattern)
- [ ] Panel has a clear title ("Settings") and a close/dismiss mechanism
- [ ] **Provider list:** Fetched from `GET /api/providers` and displayed as a list or card layout
- [ ] Each provider shows: name (e.g., "anthropic"), type (e.g., "anthropic", "codex", "openai-compatible", "local"), and status
- [ ] Provider status displayed as a badge: "Available" (green) or "Unavailable" (red/grey)
- [ ] Each provider expands or lists its available models with context window sizes (e.g., "claude-sonnet-4-20250514 — 200K tokens")
- [ ] **Project info section:** Displays project name, root path, primary language, last indexed timestamp (human-readable), and last indexed commit (short hash)
- [ ] Project info fetched from `GET /api/project`
- [ ] If the project info endpoint returns an error, display a warning rather than breaking the entire settings panel
- [ ] Settings panel is read-only except for the model selection (Task 02) — other config knobs are not editable in the UI for v0.1
- [ ] Panel renders correctly with the dark theme
