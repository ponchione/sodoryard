# Task 03: Token Expiry Check and OAuth Refresh

**Epic:** 02 — Anthropic Credential Manager
**Status:** Not started
**Dependencies:** Task 02 (credential file parsing and oauthToken struct)

---

## Description

Implement the token expiry check and the OAuth token refresh flow. When `GetAuthHeader` detects that the cached OAuth token is expired or within 2 minutes of expiry, it calls the refresh function to obtain a new access token from Anthropic's OAuth token endpoint. The refresh function makes an HTTP POST, parses the response, updates the in-memory cached token, and delegates to the write-back logic (Task 04) to persist the new credentials to disk. All HTTP calls respect context cancellation.

## Acceptance Criteria

- [ ] An unexported function is defined with this signature:
```go
func (cm *CredentialManager) refreshToken(ctx context.Context) error
```
- [ ] An unexported helper is defined with this signature:
```go
func isTokenExpired(token *oauthToken) bool
```
- [ ] `isTokenExpired` returns `true` when `time.Now().Add(2 * time.Minute).After(token.ExpiresAt)`, meaning the token is expired or will expire within 120 seconds
- [ ] `isTokenExpired` returns `true` when `token` is `nil`
- [ ] The OAuth token refresh endpoint URL is defined as a package-level variable (to allow test overrides):
```go
var tokenEndpoint = "https://console.anthropic.com/v1/oauth/token"
```
- [ ] `refreshToken` constructs an HTTP POST request to `tokenEndpoint` using `http.NewRequestWithContext(ctx, "POST", tokenEndpoint, body)`
- [ ] The request body is URL-encoded form data with exactly these fields: `grant_type=refresh_token&refresh_token=<cm.cached.RefreshToken>` (constructed via `url.Values` and encoded with `.Encode()`)
- [ ] The request `Content-Type` header is set to `application/x-www-form-urlencoded`
- [ ] The HTTP request is sent using `cm.httpClient.Do(req)`
- [ ] The response body is always closed via `defer resp.Body.Close()`
- [ ] The response body is read with a limit of 1 MB (`io.LimitReader(resp.Body, 1<<20)`) to prevent unbounded memory use
- [ ] The response JSON is represented by this unexported struct:
```go
type tokenRefreshResponse struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
    ExpiresIn    int    `json:"expires_in"`
}
```
- [ ] If the HTTP response status is 401, `refreshToken` returns an error with exact message: `"Claude credentials expired. Run ` + "`claude login`" + ` to re-authenticate."`
- [ ] If the HTTP response status is 400, `refreshToken` returns an error with exact message: `"Claude refresh token is invalid. Run ` + "`claude login`" + ` to re-authenticate."`
- [ ] If the HTTP response status is any non-200 value other than 400 or 401, `refreshToken` reads the body and returns an error with exact format: `"Anthropic token refresh failed (HTTP <status_code>): <body_text>"` where `<status_code>` is the integer status code and `<body_text>` is the response body truncated to 256 bytes
- [ ] If `cm.httpClient.Do(req)` returns a non-nil error (network failure), `refreshToken` returns: `fmt.Errorf("failed to refresh Claude credentials: %w", err)`
- [ ] If JSON decoding of a 200 response fails, `refreshToken` returns: `fmt.Errorf("failed to parse token refresh response: %w", jsonErr)`
- [ ] If the decoded `access_token` field is empty, `refreshToken` returns an error with exact message: `"Anthropic token refresh response missing access_token"`
- [ ] On a successful 200 response, `refreshToken` computes the new expiry as `time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)`
- [ ] On success, `refreshToken` updates `cm.cached` with the new `AccessToken`, `RefreshToken`, and `ExpiresAt` values (this write happens while the caller already holds `cm.mu.Lock()`)
- [ ] If the response `refresh_token` field is empty, `refreshToken` retains the existing `cm.cached.RefreshToken` instead of overwriting it with an empty string
- [ ] After updating `cm.cached`, `refreshToken` calls the write-back function (Task 04) to persist the new credentials to disk; write-back errors are logged but do not cause `refreshToken` to return an error (the in-memory token is still valid)
- [ ] If `ctx` is cancelled before or during the HTTP call, the function returns a context error: `fmt.Errorf("credential refresh cancelled: %w", ctx.Err())`
