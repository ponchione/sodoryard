# Task 02: Credential File Parsing

**Epic:** 02 — Anthropic Credential Manager
**Status:** Not started
**Dependencies:** Task 01 (CredentialManager struct and oauthToken type)

---

## Description

Implement the credential file reader that loads and parses `~/.claude/.credentials.json` into an `oauthToken` struct. The reader uses advisory file locking (shared lock via `syscall.Flock` with `LOCK_SH`) to avoid races with Claude Code, which may be writing to the same file concurrently. Every error case produces a specific, actionable error message that tells the user how to fix the problem.

## Acceptance Criteria

- [ ] An unexported function is defined in `internal/provider/anthropic/credentials.go` (or a new file `internal/provider/anthropic/credfile.go`) with this signature:
```go
func readCredentialFile(path string) (*oauthToken, error)
```
- [ ] The on-disk JSON structure is represented by an unexported struct used only for deserialization:
```go
type credentialFileJSON struct {
    AccessToken  string `json:"accessToken"`
    RefreshToken string `json:"refreshToken"`
    ExpiresAt    string `json:"expiresAt"`
}
```
- [ ] `readCredentialFile` opens the file at `path` using `os.Open` (read-only)
- [ ] After opening, `readCredentialFile` acquires a shared advisory lock by calling `syscall.Flock(int(file.Fd()), syscall.LOCK_SH)` before reading any bytes
- [ ] The shared lock is released by calling `syscall.Flock(int(file.Fd()), syscall.LOCK_UN)` in a `defer` immediately after acquisition succeeds
- [ ] The file descriptor is closed via `defer file.Close()` before attempting the lock
- [ ] If `os.Open` returns `os.ErrNotExist` (checked via `errors.Is`), `readCredentialFile` returns an error with exact message: `"~/.claude/.credentials.json not found. Install Claude Code and run ` + "`claude login`" + `."` (the path in the error message is always the literal `~/.claude/.credentials.json`, not the resolved absolute path)
- [ ] If `os.Open` returns `os.ErrPermission` (checked via `errors.Is`), `readCredentialFile` returns an error wrapping the original error: `fmt.Errorf("permission denied reading %s: %w", path, err)`
- [ ] For any other `os.Open` error, `readCredentialFile` returns: `fmt.Errorf("failed to open credential file %s: %w", path, err)`
- [ ] File contents are decoded using `json.NewDecoder(file).Decode(&credentialFileJSON{})` (not `json.Unmarshal` on the full bytes, to avoid double-buffering)
- [ ] If JSON decoding fails, `readCredentialFile` returns an error with exact format: `"Failed to parse Claude credentials at ~/.claude/.credentials.json: <json_error>"` where `<json_error>` is the string from the `json.Decoder` error
- [ ] If the decoded `accessToken` field is empty string, `readCredentialFile` returns an error with exact message: `"Claude credentials file missing accessToken field"`
- [ ] If the decoded `refreshToken` field is empty string, `readCredentialFile` returns an error with exact message: `"Claude credentials file missing refreshToken field"`
- [ ] The `expiresAt` string is parsed using `time.Parse(time.RFC3339, expiresAt)`
- [ ] If `expiresAt` is empty or `time.Parse` returns an error, `readCredentialFile` returns an error with exact format: `"Claude credentials file has invalid expiresAt: <parse_error>"` where `<parse_error>` is either `"empty value"` (if the string was empty) or the error string from `time.Parse`
- [ ] On success, `readCredentialFile` returns a populated `*oauthToken` with `AccessToken`, `RefreshToken`, and `ExpiresAt` fields set
- [ ] `readCredentialFile` is called from `GetAuthHeader` (Task 01) when `cm.cached` is nil and `cm.mode == AuthModeOAuth`, under the write lock
