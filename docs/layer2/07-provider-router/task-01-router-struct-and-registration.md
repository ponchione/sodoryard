# Task 01: Router Struct, Constructor, and Provider Registration

**Epic:** 07 — Provider Router
**Status:** ⬚ Not started
**Dependencies:** L2-E01 (provider types — Provider interface, ProviderError, Request, Response), L2-E06 (sub-call tracking — TrackedProvider), L0-E02 (logging — slog.Logger), L0-E03 (config — YAML routing section)

---

## Description

Define the core Router struct, its configuration types (RouterConfig, RouteTarget, ProviderHealth), and the constructor function. The constructor accepts a RouterConfig parsed from the `routing:` section of sirtopham.yaml, an optional TrackedProvider for sub-call tracking, and a logger. The RegisterProvider method allows individual provider instances (Anthropic, OpenAI-compatible, Codex) to be added to the router's internal map, keyed by their Name(). This task establishes the foundational data structures that all subsequent routing, fallback, and health-tracking tasks build upon.

## Acceptance Criteria

- [ ] Define the following types in `internal/provider/router/router.go`:

```go
type RouteTarget struct {
    Provider string `yaml:"provider"`
    Model    string `yaml:"model"`
}

type RouterConfig struct {
    Default  RouteTarget  `yaml:"default"`
    Fallback *RouteTarget `yaml:"fallback"` // nil when no fallback configured
}

type ProviderHealth struct {
    Healthy       bool
    LastError     error
    LastErrorAt   time.Time
    LastSuccessAt time.Time
}

type Router struct {
    providers map[string]provider.Provider  // keyed by provider Name()
    config    RouterConfig
    health    map[string]*ProviderHealth
    mu        sync.RWMutex
    logger    *slog.Logger
}
```

- [ ] Implement the constructor with the following signature:

```go
func NewRouter(config RouterConfig, tracker *tracking.TrackedProvider, logger *slog.Logger) (*Router, error)
```

  The constructor must:
  - Return an error if `config.Default.Provider` is empty, with message: `"routing.default.provider is required in configuration"`
  - Return an error if `config.Default.Model` is empty, with message: `"routing.default.model is required in configuration"`
  - Initialize `providers` as an empty `map[string]provider.Provider`
  - Initialize `health` as an empty `map[string]*ProviderHealth`
  - Store the logger; if logger is nil, create a no-op logger via `slog.New(slog.NewTextHandler(io.Discard, nil))`
  - Store the tracker reference (may be nil if sub-call tracking is not enabled)
  - Return the initialized `*Router` and nil error on success

- [ ] Implement the RegisterProvider method:

```go
func (r *Router) RegisterProvider(p provider.Provider) error
```

  The method must:
  - Acquire a write lock (`r.mu.Lock()`) and release it on return
  - Return an error if `p` is nil, with message: `"cannot register nil provider"`
  - Return an error if a provider with the same `p.Name()` is already registered, with message: `"provider already registered: <name>"`
  - Store the provider in `r.providers[p.Name()]`
  - Initialize a `ProviderHealth` entry in `r.health[p.Name()]` with `Healthy: true` (assume healthy until proven otherwise)
  - Log at INFO level: `"provider registered"` with attrs `provider=<name>`
  - Return nil on success

- [ ] Implement the Name method:

```go
func (r *Router) Name() string
```

  Returns the literal string `"router"`.

- [ ] Implement a read-only accessor for health data:

```go
func (r *Router) ProviderHealth() map[string]*ProviderHealth
```

  The method must:
  - Acquire a read lock (`r.mu.RLock()`) and release it on return
  - Return a shallow copy of `r.health` (new map with same pointer values) to avoid callers mutating the internal map's key set

- [ ] All exported types and functions have GoDoc comments
- [ ] The `router` package compiles with no errors (`go build ./internal/provider/router/...`)
