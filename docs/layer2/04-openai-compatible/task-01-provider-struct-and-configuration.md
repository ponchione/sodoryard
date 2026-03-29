# Task 01: Provider Struct, Constructor, and Configuration

**Epic:** 04 — OpenAI-Compatible Provider
**Status:** ⬚ Not started
**Dependencies:** L2-E01 (provider interface types), L0-E03 (config loading)

---

## Description

Define the `OpenAIProvider` struct in `internal/provider/openai/` that implements the unified provider interface. The struct holds a resolved configuration (base URL, API key, model, context length) and an `*http.Client`. The constructor `NewOpenAIProvider` accepts an `OpenAIConfig`, resolves the API key (from direct value or environment variable), validates all required fields, and returns a ready-to-use provider instance. Each config entry in `sirtopham.yaml` produces a separate `OpenAIProvider` instance; the `Name()` method returns the config-level name (e.g., `"local"`, `"openrouter"`) so the router can distinguish instances.

## Acceptance Criteria

- [ ] File `internal/provider/openai/provider.go` exists and declares `package openai`.

- [ ] The following config struct is defined exactly:
  ```go
  // OpenAIConfig holds provider-level configuration for one OpenAI-compatible endpoint.
  type OpenAIConfig struct {
      Name          string `yaml:"name"`          // provider instance name (e.g., "local", "openrouter")
      BaseURL       string `yaml:"base_url"`      // e.g., "http://localhost:8080/v1"
      APIKey        string `yaml:"api_key"`        // optional, direct API key value
      APIKeyEnv     string `yaml:"api_key_env"`    // optional, env var name containing the API key
      Model         string `yaml:"model"`          // default model name
      ContextLength int    `yaml:"context_length"` // context window size in tokens
  }
  ```

- [ ] The provider struct is defined exactly:
  ```go
  // OpenAIProvider implements the unified provider interface for any
  // OpenAI-compatible chat completions API.
  type OpenAIProvider struct {
      name          string
      baseURL       string
      apiKey        string       // resolved key (may be empty for keyless local servers)
      model         string
      contextLength int
      client        *http.Client
  }
  ```

- [ ] The constructor is defined with the following signature:
  ```go
  // NewOpenAIProvider creates a provider instance from config. It resolves
  // the API key, validates required fields, and configures the HTTP client.
  func NewOpenAIProvider(cfg OpenAIConfig) (*OpenAIProvider, error)
  ```

- [ ] API key resolution logic inside `NewOpenAIProvider` works as follows, in order:
  1. If `cfg.APIKey` is non-empty, use it directly.
  2. Else if `cfg.APIKeyEnv` is non-empty, read `os.Getenv(cfg.APIKeyEnv)`. If the env var is empty or unset, return error: `"openai provider '<name>': environment variable '<api_key_env>' is not set or empty"`.
  3. If both `cfg.APIKey` and `cfg.APIKeyEnv` are empty, the API key is left as the empty string (valid for keyless local servers).

- [ ] `NewOpenAIProvider` validates required fields and returns descriptive errors:
  - If `cfg.Name` is empty: `"openai provider config: name is required"`
  - If `cfg.BaseURL` is empty: `"openai provider '<name>': base_url is required"`
  - If `cfg.Model` is empty: `"openai provider '<name>': model is required"`
  - If `cfg.ContextLength` is <= 0: `"openai provider '<name>': context_length must be a positive integer"`

- [ ] `NewOpenAIProvider` strips any trailing `/` from `cfg.BaseURL` before storing it (so callers can append `/chat/completions` without double-slash issues).

- [ ] The HTTP client is created with a 120-second timeout: `&http.Client{Timeout: 120 * time.Second}`.

- [ ] The following methods are defined:
  ```go
  // Name returns the provider instance name from config (e.g., "local", "openrouter").
  func (p *OpenAIProvider) Name() string

  // Models returns a slice containing the single configured model name.
  func (p *OpenAIProvider) Models() []string

  // ContextLength returns the context window size in tokens.
  func (p *OpenAIProvider) ContextLength() int
  ```

- [ ] `Name()` returns `p.name`, `Models()` returns `[]string{p.model}`, `ContextLength()` returns `p.contextLength`.

- [ ] Unit tests in `internal/provider/openai/provider_test.go` cover:
  - Successful construction with direct API key (all fields populated correctly).
  - Successful construction with `APIKeyEnv` that is set in the environment.
  - Error when `APIKeyEnv` references an unset environment variable.
  - Successful construction with no API key and no `APIKeyEnv` (keyless local mode).
  - Validation errors for each missing required field (`Name`, `BaseURL`, `Model`, `ContextLength <= 0`).
  - Trailing slash stripping from `BaseURL` (e.g., `"http://localhost:8080/v1/"` becomes `"http://localhost:8080/v1"`).
  - `Name()`, `Models()`, and `ContextLength()` return expected values.
