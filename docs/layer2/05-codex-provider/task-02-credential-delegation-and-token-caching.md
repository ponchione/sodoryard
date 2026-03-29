# Task 02: Credential Delegation and Token Caching

**Epic:** 05 — Codex Provider
**Status:** ⬚ Not started
**Dependencies:** Task 01 (provider struct with `codexBinPath`, `cachedToken`, `tokenExpiry`, `mu` fields)

---

## Description

Implement the credential delegation flow that reads `~/.codex/auth.json` for cached tokens, checks expiry with a 120-second buffer, and shells out to `codex refresh` when the token is expired or near-expiry. The token is cached in memory on the `CodexProvider` struct and only refreshed when necessary, avoiding unnecessary shell-outs on every API call. This method is called by `Complete` and `Stream` before making HTTP requests.

## Acceptance Criteria

- [ ] File `internal/provider/codex/credentials.go` exists and declares `package codex`

- [ ] The auth file JSON structure is defined as an unexported type:
  ```go
  type codexAuthFile struct {
      AccessToken string `json:"access_token"`
      ExpiresAt   string `json:"expires_at"` // RFC3339 format, e.g. "2026-03-28T16:00:00Z"
  }
  ```

- [ ] An unexported method obtains a valid access token, refreshing if needed:
  ```go
  func (p *CodexProvider) getAccessToken(ctx context.Context) (string, error)
  ```

- [ ] `getAccessToken` first acquires a read lock (`p.mu.RLock()`) and checks if `p.cachedToken` is non-empty AND `p.tokenExpiry` is more than 120 seconds in the future (i.e., `time.Until(p.tokenExpiry) > 120*time.Second`). If both conditions are met, it returns `p.cachedToken` immediately (fast path, no I/O)

- [ ] If the cached token is empty or within 120 seconds of expiry, `getAccessToken` releases the read lock, acquires a write lock (`p.mu.Lock()`), and performs a double-check: re-checks if another goroutine already refreshed the token while this one was waiting for the write lock. If the token is now valid, returns it without refreshing

- [ ] If refresh is needed, `getAccessToken` calls `p.refreshToken(ctx)` to shell out to `codex refresh`, then calls `p.readAuthFile()` to read the updated token from disk

- [ ] An unexported method reads and parses the auth file:
  ```go
  func (p *CodexProvider) readAuthFile() (string, time.Time, error)
  ```

- [ ] `readAuthFile` reads the file at `<user_home_dir>/.codex/auth.json` using `os.UserHomeDir()` to resolve the home directory. If `os.UserHomeDir()` fails, returns error: `"codex: cannot determine home directory: <underlying error>"`

- [ ] If the auth file does not exist, `readAuthFile` returns error: `"codex: auth file not found at <path>. Run ` + "`codex auth`" + ` to authenticate."`

- [ ] If the auth file exists but cannot be read, `readAuthFile` returns error: `"codex: cannot read auth file: <underlying error>"`

- [ ] `readAuthFile` unmarshals the file contents into `codexAuthFile`. If JSON parsing fails, returns error: `"codex: invalid auth file format: <underlying error>"`

- [ ] `readAuthFile` parses `codexAuthFile.ExpiresAt` using `time.Parse(time.RFC3339, ...)`. If parsing fails, returns error: `"codex: invalid expires_at timestamp in auth file: <underlying error>"`

- [ ] If `codexAuthFile.AccessToken` is empty, `readAuthFile` returns error: `"codex: auth file contains empty access_token. Run ` + "`codex auth`" + ` to re-authenticate."`

- [ ] `readAuthFile` returns the access token string, the parsed expiry time, and nil error on success

- [ ] An unexported method shells out to `codex refresh`:
  ```go
  func (p *CodexProvider) refreshToken(ctx context.Context) error
  ```

- [ ] `refreshToken` creates a `context.WithTimeout` derived from `ctx` with a 30-second timeout

- [ ] `refreshToken` constructs the command using `exec.CommandContext(timeoutCtx, p.codexBinPath, "refresh")`

- [ ] `refreshToken` captures stderr using `cmd.CombinedOutput()` or by setting `cmd.Stderr` to a `bytes.Buffer`

- [ ] If the context deadline is exceeded (30-second timeout), `refreshToken` returns a `*provider.ProviderError` with:
  - `Provider: "codex"`
  - `StatusCode: 0`
  - `Message: "Codex credential refresh timed out after 30s"`
  - `Retriable: true`

- [ ] If the command exits with a non-zero exit code, `refreshToken` returns a `*provider.ProviderError` with:
  - `Provider: "codex"`
  - `StatusCode: 0`
  - `Message: "Codex credential refresh failed (exit <code>): <stderr trimmed to 512 bytes>"`
  - `Retriable: false`
  - The exit code is extracted from the `*exec.ExitError` type

- [ ] If the command succeeds (exit 0), `refreshToken` returns nil

- [ ] After a successful refresh, `getAccessToken` calls `readAuthFile()`, stores the returned token and expiry in `p.cachedToken` and `p.tokenExpiry` (under the write lock), and returns the token

- [ ] If `readAuthFile()` fails after a successful refresh, `getAccessToken` returns the error wrapped in a `*provider.ProviderError` with `Provider: "codex"`, `StatusCode: 0`, `Retriable: false`

- [ ] All errors returned by `getAccessToken` are `*provider.ProviderError` instances with `Provider: "codex"`

- [ ] The file imports: `bytes`, `context`, `encoding/json`, `fmt`, `os`, `os/exec`, `strings`, `time`, and `github.com/<module>/internal/provider`

- [ ] The file compiles with `go build ./internal/provider/codex/...` with no errors
