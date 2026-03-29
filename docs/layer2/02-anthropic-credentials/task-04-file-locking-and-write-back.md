# Task 04: File Locking and Credential Write-Back

**Epic:** 02 — Anthropic Credential Manager
**Status:** Not started
**Dependencies:** Task 03 (refresh flow that calls write-back after obtaining new tokens)

---

## Description

Implement the credential write-back function that persists refreshed OAuth tokens to `~/.claude/.credentials.json` after a successful token refresh. The write uses atomic file replacement (write to temp file, then rename) under an exclusive advisory file lock to prevent corruption if Claude Code reads the file concurrently. The lock acquisition has a timeout to avoid blocking API calls indefinitely.

## Acceptance Criteria

- [ ] An unexported function is defined with this signature:
```go
func (cm *CredentialManager) writeCredentialFile() error
```
- [ ] `writeCredentialFile` reads `cm.cached` to build the output JSON (the caller holds `cm.mu.Lock()` so no additional locking is needed to read `cm.cached`)
- [ ] The output JSON is marshalled using `json.MarshalIndent` with prefix `""` and indent `"    "` (4 spaces) to produce this exact format:
```json
{
    "accessToken": "<access_token>",
    "refreshToken": "<refresh_token>",
    "expiresAt": "<ISO8601_timestamp>"
}
```
- [ ] The `expiresAt` field is formatted using `cm.cached.ExpiresAt.Format(time.RFC3339)`
- [ ] The marshalled JSON has a trailing newline appended (`append(data, '\n')`)
- [ ] `writeCredentialFile` creates a temporary file in the same directory as `cm.credPath` using `os.CreateTemp(filepath.Dir(cm.credPath), ".credentials-*.tmp")`
- [ ] The temporary file is written with permissions `0600` (set via `os.Chmod` after creation, or by opening with `os.OpenFile` with mode `0600`)
- [ ] `writeCredentialFile` opens the target credential file (or creates a lock file at `cm.credPath + ".lock"`) to acquire the exclusive lock
- [ ] The exclusive advisory lock is acquired by calling `syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)` in a retry loop
- [ ] The retry loop attempts to acquire the lock at 100ms intervals for a maximum of 5 seconds (50 attempts). This avoids blocking indefinitely if another process holds the lock
- [ ] If the lock cannot be acquired within 5 seconds, `writeCredentialFile` logs a warning message `"failed to acquire lock on credential file after 5s, skipping write-back"` using the project logger, removes the temp file, and returns `nil` (not an error, since stale on-disk credentials are acceptable when the in-memory token is already updated)
- [ ] After acquiring the exclusive lock, `writeCredentialFile` writes the JSON bytes to the temporary file and calls `file.Sync()` to flush to disk
- [ ] After syncing, `writeCredentialFile` calls `os.Rename(tempPath, cm.credPath)` to atomically replace the credential file
- [ ] The exclusive lock is released via `syscall.Flock(fd, syscall.LOCK_UN)` in a `defer` after successful acquisition
- [ ] The temporary file is cleaned up in all error paths: a `defer` removes it via `os.Remove(tempPath)` if the rename has not yet succeeded (use a boolean flag `renamed` to track)
- [ ] If `os.Rename` fails, `writeCredentialFile` returns: `fmt.Errorf("failed to update credential file: %w", err)`
- [ ] If writing to the temporary file fails, `writeCredentialFile` returns: `fmt.Errorf("failed to write temporary credential file: %w", err)`
- [ ] `writeCredentialFile` is called from `refreshToken` (Task 03) after updating `cm.cached`; errors from `writeCredentialFile` are logged as warnings but do not propagate to the caller of `refreshToken`
