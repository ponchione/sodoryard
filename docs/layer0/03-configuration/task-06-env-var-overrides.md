# Task 06: Environment Variable Overrides

**Epic:** 03 — Configuration Loading
**Status:** ⬚ Not started
**Dependencies:** Task 04

---

## Description

After YAML loading and default-merging, apply environment variable overrides for sensitive or deployment-specific fields. Environment variables take precedence over file values.

## Acceptance Criteria

- [ ] `ANTHROPIC_API_KEY` overrides the Anthropic provider API key
- [ ] `OPENROUTER_API_KEY` overrides the OpenRouter provider API key
- [ ] `SIRTOPHAM_LOG_LEVEL` overrides the top-level log level
- [ ] Env vars only override when set (empty string vs unset should be distinguishable, or documented)
- [ ] Override happens after YAML load but before validation
