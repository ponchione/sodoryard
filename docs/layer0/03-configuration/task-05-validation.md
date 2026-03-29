# Task 05: Configuration Validation

**Epic:** 03 — Configuration Loading
**Status:** ⬚ Not started
**Dependencies:** Task 04

---

## Description

Implement validation logic that runs after loading and default-merging. Reject invalid values with specific, actionable error messages.

## Acceptance Criteria

- [ ] Invalid provider types produce a clear error
- [ ] Out-of-range thresholds (e.g., relevance > 1.0 or < 0.0) are rejected
- [ ] Negative token budgets are rejected
- [ ] Invalid port numbers (< 1 or > 65535) are rejected
- [ ] Invalid log levels and formats are rejected
- [ ] Path fields (`project_root`, `brain.vault_path` when `brain.enabled` is true) must exist and be valid directories; validation returns a clear error if the path doesn't exist or isn't a directory
- [ ] Validation errors name the specific field and the invalid value
