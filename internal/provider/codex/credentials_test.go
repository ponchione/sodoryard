package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sirtopham/internal/provider"
)

// overrideHomeDir temporarily overrides the homeDir function to return the
// given directory. Returns a cleanup function to restore the original.
func overrideHomeDir(t *testing.T, dir string) {
	t.Helper()
	orig := homeDir
	homeDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { homeDir = orig })
}

func overrideStdinIsTerminal(t *testing.T, v bool) {
	t.Helper()
	orig := stdinIsTerminal
	stdinIsTerminal = func() bool { return v }
	t.Cleanup(func() { stdinIsTerminal = orig })
}

func writeAuthFile(t *testing.T, dir, content string) {
	t.Helper()
	codexDir := filepath.Join(dir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("failed to create .codex dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write auth.json: %v", err)
	}
}

func testJWT(t *testing.T, exp time.Time) string {
	t.Helper()
	headerJSON, err := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payloadJSON, err := json.Marshal(map[string]any{"exp": exp.Unix()})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	encode := func(in []byte) string {
		return base64.RawURLEncoding.EncodeToString(in)
	}
	return encode(headerJSON) + "." + encode(payloadJSON) + ".sig"
}

func TestReadAuthFile_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	writeAuthFile(t, tmpDir, `{"access_token": "eyJ_test_token", "expires_at": "2026-03-28T18:00:00Z"}`)

	p := &CodexProvider{}
	token, expiry, err := p.readAuthFile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "eyJ_test_token" {
		t.Errorf("expected token %q, got %q", "eyJ_test_token", token)
	}
	expected := time.Date(2026, 3, 28, 18, 0, 0, 0, time.UTC)
	if !expiry.Equal(expected) {
		t.Errorf("expected expiry %v, got %v", expected, expiry)
	}
}

func TestReadAuthFile_NestedTokensJWTExpiry(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	expected := time.Date(2026, 4, 3, 17, 45, 59, 0, time.UTC)
	token := testJWT(t, expected)
	writeAuthFile(t, tmpDir, `{"auth_mode":"chatgpt","last_refresh":"2026-03-24T17:45:59Z","tokens":{"access_token":"`+token+`","refresh_token":"redacted"}}`)

	p := &CodexProvider{}
	gotToken, expiry, err := p.readAuthFile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotToken != token {
		t.Fatalf("expected nested token to be returned")
	}
	if !expiry.Equal(expected) {
		t.Fatalf("expected expiry %v, got %v", expected, expiry)
	}
}

func TestReadAuthFile_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)

	p := &CodexProvider{}
	_, _, err := p.readAuthFile()
	if err == nil {
		t.Fatal("expected error for missing auth file")
	}
	if !strings.Contains(err.Error(), "auth file not found at") {
		t.Errorf("expected error containing %q, got %q", "auth file not found at", err.Error())
	}
}

func TestReadAuthFile_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	writeAuthFile(t, tmpDir, `{invalid json}`)

	p := &CodexProvider{}
	_, _, err := p.readAuthFile()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid auth file format") {
		t.Errorf("expected error containing %q, got %q", "invalid auth file format", err.Error())
	}
}

func TestReadAuthFile_EmptyAccessToken(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	writeAuthFile(t, tmpDir, `{"access_token": "", "expires_at": "2026-03-28T18:00:00Z"}`)

	p := &CodexProvider{}
	_, _, err := p.readAuthFile()
	if err == nil {
		t.Fatal("expected error for empty access token")
	}
	if !strings.Contains(err.Error(), "empty access_token") {
		t.Errorf("expected error containing %q, got %q", "empty access_token", err.Error())
	}
}

func TestReadAuthFile_InvalidTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	writeAuthFile(t, tmpDir, `{"access_token": "tok", "expires_at": "not-a-date"}`)

	p := &CodexProvider{}
	_, _, err := p.readAuthFile()
	if err == nil {
		t.Fatal("expected error for invalid timestamp")
	}
	if !strings.Contains(err.Error(), "invalid expires_at timestamp") {
		t.Errorf("expected error containing %q, got %q", "invalid expires_at timestamp", err.Error())
	}
}

func TestGetAccessToken_CachedTokenReturnedWithoutIO(t *testing.T) {
	p := &CodexProvider{
		cachedToken: "cached_tok",
		tokenExpiry: time.Now().Add(10 * time.Minute),
	}

	token, err := p.getAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "cached_tok" {
		t.Errorf("expected token %q, got %q", "cached_tok", token)
	}
}

func TestGetAccessToken_ExpiredCachedTokenUsesStillValidAuthFileWithoutRefresh(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	writeAuthFile(t, tmpDir, `{"access_token": "fresh_from_file", "expires_at": "2027-01-01T00:00:00Z"}`)

	mockBin := createMockScript(t, tmpDir, "#!/bin/sh\necho 'should not refresh' >&2\nexit 1\n")
	p := &CodexProvider{
		cachedToken:  "old_tok",
		tokenExpiry:  time.Now().Add(60 * time.Second),
		codexBinPath: mockBin,
	}

	token, err := p.getAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "fresh_from_file" {
		t.Errorf("expected token %q, got %q", "fresh_from_file", token)
	}
}

func TestGetAccessToken_ExpiredTokenTriggersRefresh(t *testing.T) {
	// Set up a token that is within the 120-second buffer (60s left)
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	overrideStdinIsTerminal(t, true)

	// Write a valid auth file so readAuthFile succeeds after "refresh"
	writeAuthFile(t, tmpDir, `{"access_token": "new_token", "expires_at": "2027-01-01T00:00:00Z"}`)

	// Create a mock script that exits 0 (simulating successful refresh)
	mockBin := createMockScript(t, tmpDir, "#!/bin/sh\nexit 0\n")

	p := &CodexProvider{
		cachedToken:  "old_tok",
		tokenExpiry:  time.Now().Add(60 * time.Second), // within 120s buffer
		codexBinPath: mockBin,
	}

	token, err := p.getAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "new_token" {
		t.Errorf("expected token %q, got %q", "new_token", token)
	}
}

func TestGetAccessToken_EmptyTokenTriggersRead(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)

	// Write a valid auth file with a far-future expiry
	writeAuthFile(t, tmpDir, `{"access_token": "fresh_token", "expires_at": "2027-01-01T00:00:00Z"}`)

	p := &CodexProvider{
		cachedToken: "", // empty
	}

	token, err := p.getAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "fresh_token" {
		t.Errorf("expected token %q, got %q", "fresh_token", token)
	}
}

func TestGetAccessToken_ExpiredTokenInNonInteractiveRuntimeDoesNotShellOut(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	overrideStdinIsTerminal(t, false)
	expired := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	writeAuthFile(t, tmpDir, `{"access_token": "expired_token", "expires_at": "`+expired+`"}`)
	marker := filepath.Join(tmpDir, "refresh-ran")
	mockBin := createMockScript(t, tmpDir, "#!/bin/sh\ntouch \""+marker+"\"\nexit 0\n")

	p := &CodexProvider{codexBinPath: mockBin}
	_, err := p.getAccessToken(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	pe, ok := err.(*provider.ProviderError)
	if !ok {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if !strings.Contains(pe.Message, "no TTY") {
		t.Fatalf("expected actionable message about non-interactive renewal, got %q", pe.Message)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("expected refresh command not to run, stat err = %v", statErr)
	}
}

func createMockScript(t *testing.T, dir, content string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("mock scripts not supported on Windows")
	}
	path := filepath.Join(dir, "mock-codex")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("failed to write mock script: %v", err)
	}
	return path
}

func TestRefreshToken_Success(t *testing.T) {
	tmpDir := t.TempDir()
	overrideStdinIsTerminal(t, true)
	mockBin := createMockScript(t, tmpDir, "#!/bin/sh\nexit 0\n")

	p := &CodexProvider{codexBinPath: mockBin}
	err := p.refreshToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRefreshToken_FailedWithExitCode(t *testing.T) {
	tmpDir := t.TempDir()
	overrideStdinIsTerminal(t, true)
	mockBin := createMockScript(t, tmpDir, "#!/bin/sh\necho 'token expired' >&2\nexit 1\n")

	p := &CodexProvider{codexBinPath: mockBin}
	err := p.refreshToken(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}

	pe, ok := err.(*provider.ProviderError)
	if !ok {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if !strings.Contains(pe.Message, "Codex credential refresh failed (exit 1)") {
		t.Errorf("expected message containing exit code, got %q", pe.Message)
	}
	if !strings.Contains(pe.Message, "token expired") {
		t.Errorf("expected message containing stderr, got %q", pe.Message)
	}
}

func TestRefreshToken_Timeout(t *testing.T) {
	tmpDir := t.TempDir()
	overrideStdinIsTerminal(t, true)
	mockBin := createMockScript(t, tmpDir, "#!/bin/sh\nsleep 60\n")

	p := &CodexProvider{codexBinPath: mockBin}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := p.refreshToken(ctx)
	if err == nil {
		t.Fatal("expected error")
	}

	pe, ok := err.(*provider.ProviderError)
	if !ok {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	// The error should indicate a timeout
	if !strings.Contains(pe.Message, "timed out") && !strings.Contains(pe.Message, "refresh failed") {
		t.Errorf("expected timeout-related message, got %q", pe.Message)
	}
}

func TestRefreshToken_NonInteractiveReturnsActionableErrorWithoutShellingOut(t *testing.T) {
	tmpDir := t.TempDir()
	overrideStdinIsTerminal(t, false)
	marker := filepath.Join(tmpDir, "refresh-ran")
	mockBin := createMockScript(t, tmpDir, "#!/bin/sh\ntouch \""+marker+"\"\nexit 0\n")

	p := &CodexProvider{codexBinPath: mockBin}
	err := p.refreshToken(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}

	pe, ok := err.(*provider.ProviderError)
	if !ok {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if !strings.Contains(pe.Message, "interactive renewal") {
		t.Fatalf("expected actionable non-interactive message, got %q", pe.Message)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("expected refresh command not to run, stat err = %v", statErr)
	}
}
