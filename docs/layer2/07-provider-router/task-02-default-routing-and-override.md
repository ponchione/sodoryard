# Task 02: Default Routing and Per-Request Override

**Epic:** 07 — Provider Router
**Status:** ⬚ Not started
**Dependencies:** Task 01 (Router struct, constructor, RegisterProvider)

---

## Description

Implement the Router's Complete and Stream methods so the router satisfies the provider.Provider interface. These methods handle two routing decisions: (1) if the request carries a per-request model override (`req.Model` is non-empty), find which registered provider offers that model and route there; (2) otherwise, route to the configured default provider and model. This task does NOT implement fallback logic — that is added in Task 03. Both Complete and Stream return errors directly if the selected provider fails.

## Acceptance Criteria

- [ ] Implement the Complete method:

```go
func (r *Router) Complete(ctx context.Context, req *provider.Request) (*provider.Response, error)
```

  Routing logic (applied in order):
  1. **Per-request override:** If `req.Model` is non-empty, call `r.resolveOverride(ctx, req.Model)` to find the target provider. If a provider is found, set `req.Model` to the override model and call `targetProvider.Complete(ctx, req)`. If no provider is found for that model, log a WARN message `"requested model not found in any registered provider, falling through to default"` with attr `model=<req.Model>`, then fall through to step 2.
  2. **Default routing:** Look up `r.config.Default.Provider` in `r.providers`. If found, set `req.Model` to `r.config.Default.Model` and call `defaultProvider.Complete(ctx, req)`. If the default provider name is not in `r.providers`, return an error with message `"default provider not available: <name>"`.
  3. Return the response and error from the selected provider's Complete call (no fallback in this task).

- [ ] Implement the Stream method:

```go
func (r *Router) Stream(ctx context.Context, req *provider.Request) (<-chan provider.StreamEvent, error)
```

  The routing logic is identical to Complete (same override check, same default fallback), but calls `targetProvider.Stream(ctx, req)` instead. Return the channel and error from the selected provider's Stream call.

- [ ] Implement the internal override resolution helper:

```go
func (r *Router) resolveOverride(ctx context.Context, modelID string) (provider.Provider, error)
```

  The method must:
  - Acquire a read lock on `r.mu`
  - Iterate all entries in `r.providers`
  - For each provider, call `p.Models(ctx)` to get its model list
  - If any model's ID matches `modelID` exactly (case-sensitive), return that provider and nil error
  - If `Models()` returns an error for a provider, log at WARN level `"failed to list models for provider"` with attrs `provider=<name>`, `error=<err>`, and continue to the next provider
  - If no provider offers the requested model, return `nil, nil` (no error, just not found)

- [ ] When routing to a provider, the router must acquire a read lock to access `r.providers` and release it before calling the provider method (do not hold the lock during the provider call, which may block on network I/O)

- [ ] Implement the Models method:

```go
func (r *Router) Models(ctx context.Context) ([]provider.Model, error)
```

  The method must:
  - Iterate all registered providers (under read lock for the map access)
  - Call `p.Models(ctx)` on each provider
  - Collect all models into a single combined slice
  - If a provider's `Models()` call fails, log at WARN level `"failed to list models from provider"` with attrs `provider=<name>`, `error=<err>`, and skip that provider (do not fail the entire call)
  - Return the combined slice and nil error (partial results are acceptable)

- [ ] The router preserves the original `req.Model` value if it was empty before routing — specifically, the router sets `req.Model` to the resolved model only for the duration of the provider call. Use a local copy or restore the original value after the call to avoid mutating the caller's request object.

- [ ] All new exported methods have GoDoc comments
- [ ] The package compiles and the Router type satisfies the `provider.Provider` interface at compile time (verified by an interface compliance check: `var _ provider.Provider = (*Router)(nil)`)
