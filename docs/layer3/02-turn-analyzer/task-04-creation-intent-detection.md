# Task 04: Creation Intent Detection

**Epic:** 02 — Turn Analyzer
**Status:** ⬚ Not started
**Dependencies:** Task 01

---

## Description

Implement creation intent detection as the fourth signal rule. When a creation verb (write, create, add, implement, build, make) appears paired with a structural noun (test, endpoint, handler, middleware, migration, route, model, service), set `ContextNeeds.IncludeConventions = true` so that project conventions are retrieved to guide the new code. This ensures that when a user asks to create something, the assembled context includes relevant conventions and patterns.

## Acceptance Criteria

- [ ] Creation verb set defined: "write", "create", "add", "implement", "build", "make"
- [ ] Structural noun set defined: "test", "endpoint", "handler", "middleware", "migration", "route", "model", "service"
- [ ] When a creation verb is paired with a structural noun, `ContextNeeds.IncludeConventions` is set to `true`
- [ ] Each detection produces a `Signal{Type: "creation_intent", Source: <verb + noun>, Value: <noun>}`
- [ ] Package compiles with no errors
