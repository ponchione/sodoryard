# Task 01: CredentialManager Struct and Discovery Logic

**Epic:** 02 — Anthropic Credential Manager
**Status:** Not started
**Dependencies:** L2-E01 (provider types package), L0-E02 (logging package)

---

## Description

Define the `CredentialManager` struct and credential discovery logic in `internal/provider/anthropic/`. The constructor `NewCredentialManager` accepts functional options and performs credential discovery: it checks the `ANTHROPIC_API_KEY` environment variable first (API key mode), then falls back to reading `~/.claude/.credentials.json` (OAuth mode). The `GetAuthHeader` method returns the appropriate HTTP header name and value for whichever auth mode was discovered. This task covers the struct definitions, constructor, option functions, and the `GetAuthHeader` method shell that delegates to the credential file parser (Task 02) and refresh logic (Task 03) for OAuth mode.

## Acceptance Criteria

- [ ] File `internal/provider/anthropic/credentials.go` exists and compiles with no errors
- [ ] The following `AuthMode` type and constants are defined:
```go
type AuthMode int

const (
    AuthModeOAuth  AuthMode = iota
    AuthModeAPIKey
)
```
- [ ] The following `oauthToken` struct is defined (unexported):
```go
type oauthToken struct {
    AccessToken  string
    RefreshToken string
    ExpiresAt    time.Time
}
```
- [ ] The following `CredentialManager` struct is defined:
```go
type CredentialManager struct {
    credPath   string       // default: ~/.claude/.credentials.json
    mu         sync.RWMutex
    cached     *oauthToken  // cached OAuth credentials
    mode       AuthMode
    apiKey     string       // only set in API key mode
    httpClient *http.Client // used for token refresh requests
}
```
- [ ] The following functional option type and option function are defined:
```go
type CredentialOption func(*CredentialManager)

func WithCredentialPath(path string) CredentialOption
```
- [ ] `WithCredentialPath` sets `cm.credPath` to the provided path, overriding the default
- [ ] The constructor is defined with this signature:
```go
func NewCredentialManager(opts ...CredentialOption) (*CredentialManager, error)
```
- [ ] `NewCredentialManager` sets the default credential path to `~/.claude/.credentials.json` by joining `os.UserHomeDir()` with `.claude/.credentials.json`
- [ ] If `os.UserHomeDir()` fails, `NewCredentialManager` returns a wrapped error: `"failed to determine home directory: <underlying error>"`
- [ ] `NewCredentialManager` applies all provided `CredentialOption` functions after setting defaults
- [ ] `NewCredentialManager` performs credential discovery in this exact order:
  1. Check `os.Getenv("ANTHROPIC_API_KEY")`. If the env var is set (checked via `os.LookupEnv`) and non-empty, set `mode = AuthModeAPIKey` and store the value in `apiKey`
  2. If `ANTHROPIC_API_KEY` is set but empty (present in environment, value is `""`), return error with exact message: `"ANTHROPIC_API_KEY is set but empty."`
  3. If `ANTHROPIC_API_KEY` is not set, set `mode = AuthModeOAuth` (credential file will be read lazily on first `GetAuthHeader` call)
- [ ] The `GetAuthHeader` method is defined with this signature:
```go
func (cm *CredentialManager) GetAuthHeader(ctx context.Context) (headerName string, headerValue string, err error)
```
- [ ] When `cm.mode == AuthModeAPIKey`, `GetAuthHeader` returns `("X-Api-Key", cm.apiKey, nil)` without any file I/O or network calls
- [ ] When `cm.mode == AuthModeOAuth`, `GetAuthHeader` acquires `cm.mu.RLock()` to check if `cm.cached` is non-nil and not expired (expiry check: `time.Now().Add(2 * time.Minute).After(cm.cached.ExpiresAt)`). If the cached token is valid, it returns `("Authorization", "Bearer " + cm.cached.AccessToken, nil)` and releases the read lock
- [ ] When `cm.mode == AuthModeOAuth` and the cached token is nil or expired, `GetAuthHeader` upgrades to a write lock (`cm.mu.Lock()`), re-checks expiry (double-check locking pattern), and if still expired, calls the refresh flow (Task 03) or initial file parse (Task 02) before returning the header
- [ ] `GetAuthHeader` respects `ctx` cancellation: if the context is cancelled, it returns `ctx.Err()` wrapped in a descriptive error
- [ ] The `httpClient` field defaults to `&http.Client{Timeout: 30 * time.Second}` if not provided
