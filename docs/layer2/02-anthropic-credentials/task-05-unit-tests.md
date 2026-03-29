# Task 05: Unit Tests for Credential Manager

**Epic:** 02 — Anthropic Credential Manager
**Status:** Not started
**Dependencies:** Task 04 (all credential manager functionality complete)

---

## Description

Write comprehensive unit tests for the full credential manager in `internal/provider/anthropic/credentials_test.go`. Tests use `t.TempDir()` for filesystem isolation and `httptest.NewServer` for mocking the OAuth token endpoint. The package-level `tokenEndpoint` variable is overridden in tests to point at the mock server. Each test case covers a single logical path through the credential manager with specific inputs and expected outputs.

## Acceptance Criteria

- [ ] File `internal/provider/anthropic/credentials_test.go` exists and all tests pass via `go test ./internal/provider/anthropic/...`
- [ ] **Test 1 — API key mode returns X-Api-Key header:** Set env var `ANTHROPIC_API_KEY=test-key-123` (using `t.Setenv`). Call `NewCredentialManager()` then `GetAuthHeader(context.Background())`. Assert return values are `headerName == "X-Api-Key"`, `headerValue == "test-key-123"`, `err == nil`
- [ ] **Test 2 — API key mode rejects empty env var:** Set env var `ANTHROPIC_API_KEY=""` (using `t.Setenv`). Call `NewCredentialManager()`. Assert the returned error contains the exact substring `"ANTHROPIC_API_KEY is set but empty."`
- [ ] **Test 3 — OAuth mode with valid non-expired credential file:** Unset `ANTHROPIC_API_KEY` (using `os.Unsetenv` in test setup). Write a credential file to `t.TempDir()` with contents `{"accessToken":"sk-ant-valid","refreshToken":"rt-ant-valid","expiresAt":"<30 minutes from now in RFC3339>"}`. Call `NewCredentialManager(WithCredentialPath(tempCredPath))` then `GetAuthHeader(context.Background())`. Assert return values are `headerName == "Authorization"`, `headerValue == "Bearer sk-ant-valid"`, `err == nil`
- [ ] **Test 4 — OAuth mode with expired token triggers refresh:** Write a credential file with `expiresAt` set to 1 minute ago (already expired). Start an `httptest.NewServer` that handles POST to `/v1/oauth/token` and returns `{"access_token":"sk-ant-new","refresh_token":"rt-ant-new","expires_in":3600}` with status 200. Override `tokenEndpoint` to the mock server URL + `/v1/oauth/token`. Call `GetAuthHeader(context.Background())`. Assert return values are `headerName == "Authorization"`, `headerValue == "Bearer sk-ant-new"`, `err == nil`. Read the credential file from disk and assert it contains `"accessToken":"sk-ant-new"`
- [ ] **Test 5 — OAuth mode with credential file not found:** Unset `ANTHROPIC_API_KEY`. Call `NewCredentialManager(WithCredentialPath("/nonexistent/path/.credentials.json"))` then `GetAuthHeader(context.Background())`. Assert the returned error contains the exact substring `"~/.claude/.credentials.json not found. Install Claude Code and run ` + "`claude login`" + `."`
- [ ] **Test 6 — OAuth mode with malformed JSON:** Write a credential file containing the bytes `{not valid json}`. Call `GetAuthHeader(context.Background())`. Assert the returned error contains the exact substring `"Failed to parse Claude credentials at ~/.claude/.credentials.json:"`
- [ ] **Test 7 — OAuth mode refresh returns 401:** Write a valid but expired credential file. Start an `httptest.NewServer` that returns HTTP 401 for all requests. Override `tokenEndpoint`. Call `GetAuthHeader(context.Background())`. Assert the returned error contains the exact substring `"Claude credentials expired. Run ` + "`claude login`" + ` to re-authenticate."`
- [ ] **Test 8 — OAuth mode refresh network error:** Write a valid but expired credential file. Set `tokenEndpoint` to `"http://127.0.0.1:1"` (a port that is not listening, producing a connection refused error). Call `GetAuthHeader(context.Background())`. Assert the returned error contains the exact substring `"failed to refresh Claude credentials:"`
- [ ] **Test 9 — Concurrent GetAuthHeader calls coalesce to one refresh:** Write a valid but expired credential file. Start an `httptest.NewServer` with an atomic counter that increments on each request and returns a valid refresh response. Override `tokenEndpoint`. Launch 10 goroutines that each call `GetAuthHeader(context.Background())` concurrently using a `sync.WaitGroup`. After all goroutines complete, assert the atomic counter equals exactly 1 (only one refresh HTTP call was made, not 10). Assert all 10 goroutines received `headerName == "Authorization"` and `headerValue == "Bearer <new_token>"` with no errors
- [ ] **Test 10 — Write-back persists refreshed credentials to disk:** Write a valid but expired credential file. Start an `httptest.NewServer` returning `{"access_token":"sk-ant-refreshed","refresh_token":"rt-ant-refreshed","expires_in":7200}`. Override `tokenEndpoint`. Call `GetAuthHeader(context.Background())`. Read the credential file bytes from disk using `os.ReadFile`. Parse the JSON and assert `accessToken == "sk-ant-refreshed"`, `refreshToken == "rt-ant-refreshed"`, and `expiresAt` is a valid RFC3339 timestamp approximately 7200 seconds in the future (within a 10-second tolerance)
- [ ] Every test uses `t.Setenv` or `os.Unsetenv` with `t.Cleanup` to restore the original `ANTHROPIC_API_KEY` value, ensuring test isolation
- [ ] Every test that overrides the package-level `tokenEndpoint` variable restores the original value in `t.Cleanup`
- [ ] No test makes real HTTP calls to `console.anthropic.com`
- [ ] All credential files created in tests use `t.TempDir()` for automatic cleanup
