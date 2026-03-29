# Task 01: Provider Struct, Constructor, and CLI Startup Check

**Epic:** 05 — Codex Provider
**Status:** ⬚ Not started
**Dependencies:** L2-E01 (all provider types: `Provider` interface, `Request`, `Response`, `StreamEvent`, `Message`, `ContentBlock`, `SystemBlock`, `CacheControl`, `ToolDefinition`, `Model`, `ProviderError`, `Usage`, `StopReason`), L0-E02 (structured logging), L0-E03 (config loading)

---

## Description

Define the `CodexProvider` struct in `internal/provider/codex/` and its constructor `NewCodexProvider`. The constructor verifies that the `codex` CLI binary is available on `PATH` using `exec.LookPath("codex")`, returning a clear actionable error if missing. The struct holds an `*http.Client`, token cache fields (access token string and expiry time), and a mutex for thread-safe token access. The `Name()` and `Models()` methods return static information about the Codex provider and its supported models.

## Acceptance Criteria

- [ ] File `internal/provider/codex/provider.go` exists and declares `package codex`

- [ ] The provider struct is defined exactly:
  ```go
  // CodexProvider implements the unified Provider interface for OpenAI's
  // Responses API, using credentials delegated to the codex CLI binary.
  type CodexProvider struct {
      httpClient    *http.Client
      baseURL       string       // default: "https://api.openai.com"
      mu            sync.RWMutex // guards cachedToken and tokenExpiry
      cachedToken   string
      tokenExpiry   time.Time
      codexBinPath  string       // resolved path from exec.LookPath
  }
  ```

- [ ] A functional option type and option functions are defined:
  ```go
  type ProviderOption func(*CodexProvider)

  func WithHTTPClient(c *http.Client) ProviderOption
  func WithBaseURL(url string) ProviderOption
  ```

- [ ] `WithHTTPClient` sets `p.httpClient` to the provided client, overriding the default

- [ ] `WithBaseURL` sets `p.baseURL` to the provided URL (with any trailing `/` stripped), overriding the default `"https://api.openai.com"`

- [ ] The constructor is defined:
  ```go
  func NewCodexProvider(opts ...ProviderOption) (*CodexProvider, error)
  ```

- [ ] `NewCodexProvider` sets defaults before applying options: `baseURL` to `"https://api.openai.com"`, `httpClient` to `&http.Client{Timeout: 120 * time.Second}`

- [ ] `NewCodexProvider` calls `exec.LookPath("codex")` to find the CLI binary. If `exec.LookPath` returns an error, the constructor returns `nil` and a `*provider.ProviderError` with these exact fields:
  - `Provider: "codex"`
  - `StatusCode: 0`
  - `Message: "Codex CLI not found on PATH. Install from https://openai.com/codex and run ` + "`codex auth`" + `."`
  - `Retriable: false`

- [ ] If `exec.LookPath("codex")` succeeds, `NewCodexProvider` stores the resolved binary path in `p.codexBinPath`

- [ ] `NewCodexProvider` applies all option functions after defaults are set and after the LookPath check succeeds

- [ ] The `Name` method is defined:
  ```go
  func (p *CodexProvider) Name() string
  ```
  Returns the string `"codex"`

- [ ] The `Models` method is defined:
  ```go
  func (p *CodexProvider) Models(ctx context.Context) ([]provider.Model, error)
  ```
  Returns a static slice of exactly three models:
  - `{ID: "o3", Name: "o3", ContextWindow: 200000, SupportsTools: true, SupportsThinking: false}`
  - `{ID: "o4-mini", Name: "o4-mini", ContextWindow: 200000, SupportsTools: true, SupportsThinking: false}`
  - `{ID: "gpt-4.1", Name: "GPT-4.1", ContextWindow: 1000000, SupportsTools: true, SupportsThinking: false}`

- [ ] `SupportsThinking` is `false` for all three models because Codex models use encrypted reasoning (opaque `encrypted_content`) rather than sirtopham's displayable thinking blocks

- [ ] The file imports: `context`, `net/http`, `os/exec`, `sync`, `time`, and `github.com/<module>/internal/provider`

- [ ] The file compiles with `go build ./internal/provider/codex/...` with no errors

- [ ] Unit tests in `internal/provider/codex/provider_test.go` cover:
  - `Name()` returns `"codex"`
  - `Models()` returns exactly three models with the IDs `"o3"`, `"o4-mini"`, `"gpt-4.1"` and the correct context windows (200000, 200000, 1000000)
  - `WithBaseURL` strips trailing slash (e.g., `"https://api.openai.com/"` becomes `"https://api.openai.com"`)
  - Constructor error when `codex` is not on PATH (test by setting `PATH` to an empty temp directory): error message contains `"Codex CLI not found on PATH"`
