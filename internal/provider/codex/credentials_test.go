package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
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

func overrideCodexRefreshEndpoint(t *testing.T, clientID, tokenURL string) {
	t.Helper()
	origClientID := codexOAuthClientID
	origTokenURL := codexOAuthTokenURL
	codexOAuthClientID = clientID
	codexOAuthTokenURL = tokenURL
	t.Cleanup(func() {
		codexOAuthClientID = origClientID
		codexOAuthTokenURL = origTokenURL
	})
}

func overrideCodexDeviceEndpoints(t *testing.T, clientID, issuer, tokenURL string) {
	t.Helper()
	origClientID := codexOAuthClientID
	origIssuer := codexOAuthIssuer
	origTokenURL := codexOAuthTokenURL
	origSleeper := codexAuthSleeper
	origMaxWait := codexDeviceAuthMaxWait
	codexOAuthClientID = clientID
	codexOAuthIssuer = issuer
	codexOAuthTokenURL = tokenURL
	codexAuthSleeper = func(context.Context, time.Duration) error { return nil }
	codexDeviceAuthMaxWait = time.Second
	t.Cleanup(func() {
		codexOAuthClientID = origClientID
		codexOAuthIssuer = origIssuer
		codexOAuthTokenURL = origTokenURL
		codexAuthSleeper = origSleeper
		codexDeviceAuthMaxWait = origMaxWait
	})
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

func writePrivateAuthStore(t *testing.T, dir string, auth codexAuthFile) {
	t.Helper()
	if err := writeCodexStore(sirtophamAuthStorePath(dir), auth); err != nil {
		t.Fatalf("failed to write private auth store: %v", err)
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
	writeAuthFile(t, tmpDir, `{"auth_mode":"chatgpt","last_refresh":"2026-03-24T17:45:59Z","tokens":{"access_token": "`+token+`","refresh_token": "refresh_token"}}`)

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

func TestReadAuthFile_ImportsSharedStoreFromCODEXHOME(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	codexHome := filepath.Join(tmpDir, "custom-codex-home")
	t.Setenv("CODEX_HOME", codexHome)
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("MkdirAll CODEX_HOME: %v", err)
	}
	token := testJWT(t, time.Now().Add(2*time.Hour).UTC())
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"`+token+`","refresh_token":"refresh_token"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile CODEX_HOME auth: %v", err)
	}

	p := &CodexProvider{}
	status, err := p.AuthStatus(context.Background())
	if err != nil {
		t.Fatalf("AuthStatus() error: %v", err)
	}
	if status.Source != "codex_cli_store" || status.SourcePath != filepath.Join(codexHome, "auth.json") {
		t.Fatalf("AuthStatus() = %+v, want CODEX_HOME source", status)
	}
	gotToken, _, err := p.readAuthFile()
	if err != nil {
		t.Fatalf("readAuthFile() error: %v", err)
	}
	if gotToken != token {
		t.Fatalf("token = %q, want CODEX_HOME token", gotToken)
	}
	state, err := readCodexStoreState(sirtophamAuthStorePath(tmpDir))
	if err != nil {
		t.Fatalf("readCodexStoreState() error: %v", err)
	}
	if state.token != token {
		t.Fatalf("private store token = %q, want CODEX_HOME token", state.token)
	}
}

func TestReadAuthFile_NonJWTWithoutExpiryStillLoads(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	writeAuthFile(t, tmpDir, `{"auth_mode":"chatgpt","tokens":{"access_token": "opaque-token","refresh_token": "refresh_token"}}`)

	p := &CodexProvider{}
	token, expiry, err := p.readAuthFile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "opaque-token" {
		t.Fatalf("expected opaque token, got %q", token)
	}
	if !expiry.IsZero() {
		t.Fatalf("expected zero expiry when token format is opaque, got %v", expiry)
	}
	status, err := p.AuthStatus(context.Background())
	if err != nil {
		t.Fatalf("AuthStatus() error: %v", err)
	}
	if status.StorePath == "" || status.Source != "sirtopham_store" || !status.HasRefreshToken {
		t.Fatalf("unexpected auth status: %+v", status)
	}
}

func TestAuthStatus_DoesNotImportSharedStoreOnInspection(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	writeAuthFile(t, tmpDir, `{"auth_mode":"chatgpt","tokens":{"access_token": "opaque-token","refresh_token": "refresh-token"}}`)

	p := &CodexProvider{}
	status, err := p.AuthStatus(context.Background())
	if err != nil {
		t.Fatalf("AuthStatus() error: %v", err)
	}
	if status.Source != "codex_cli_store" {
		t.Fatalf("expected shared-store source, got %+v", status)
	}
	if status.StorePath != "" {
		t.Fatalf("expected no private store path during inspection, got %+v", status)
	}
	if !strings.HasSuffix(status.SourcePath, filepath.Join(".codex", "auth.json")) {
		t.Fatalf("expected shared auth source path, got %+v", status)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".sirtopham", "auth.json")); !os.IsNotExist(err) {
		t.Fatalf("expected AuthStatus inspection not to create private auth store, stat err=%v", err)
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
	if !strings.Contains(err.Error(), "auth not found in") {
		t.Errorf("expected error containing %q, got %q", "auth not found in", err.Error())
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
	if !strings.Contains(err.Error(), "failed to import Codex auth") {
		t.Errorf("expected error containing %q, got %q", "failed to import Codex auth", err.Error())
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

func TestGetAccessToken_ExpiredTokenTriggersDirectRefreshWithoutShellingOut(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	overrideStdinIsTerminal(t, false)
	expired := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	writeAuthFile(t, tmpDir, `{"auth_mode":"chatgpt","expires_at":"`+expired+`","tokens":{"access_token":"expired_token","refresh_token":"refresh_token"}}`)
	marker := filepath.Join(tmpDir, "refresh-ran")
	mockBin := createMockScript(t, tmpDir, "#!/bin/sh\ntouch \""+marker+"\"\nexit 0\n")

	refreshedToken := testJWT(t, time.Now().Add(1*time.Hour).UTC())
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q", got)
		}
		if got := r.Form.Get("refresh_token"); got != "refresh_token" {
			t.Fatalf("refresh_token = %q", got)
		}
		if got := r.Form.Get("client_id"); got != "test-client-id" {
			t.Fatalf("client_id = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"` + refreshedToken + `","refresh_token":"new_refresh_token"}`))
	}))
	defer server.Close()
	overrideCodexRefreshEndpoint(t, "test-client-id", server.URL)

	p := &CodexProvider{codexBinPath: mockBin}
	token, err := p.getAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != refreshedToken {
		t.Fatalf("expected token %q, got %q", refreshedToken, token)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("expected no CLI shellout, stat err = %v", statErr)
	}
}

func TestGetAccessToken_PrivateStoreWinsWhenSharedStoreLooksNewer(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)

	privateToken := testJWT(t, time.Now().Add(2*time.Hour).UTC())
	sharedToken := testJWT(t, time.Now().Add(24*time.Hour).UTC())
	writeAuthFile(t, tmpDir, `{"auth_mode":"chatgpt","last_refresh":"2026-04-10T00:43:46Z","tokens":{"access_token": "`+sharedToken+`","refresh_token": "shared_refresh_token"}}`)
	writePrivateAuthStore(t, tmpDir, codexAuthFile{
		AuthMode:    "chatgpt",
		ExpiresAt:   time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		LastRefresh: "2026-04-03T17:56:16Z",
		Tokens: codexAuthTokens{
			AccessToken:  privateToken,
			RefreshToken: "private_refresh_token",
		},
	})

	p := &CodexProvider{}
	token, err := p.getAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != privateToken {
		t.Fatalf("expected private-store token, got %q", token)
	}

	status, err := p.AuthStatus(context.Background())
	if err != nil {
		t.Fatalf("AuthStatus() error: %v", err)
	}
	if status.Source != "sirtopham_store" || status.StorePath == "" {
		t.Fatalf("expected private auth state after runtime use, got %+v", status)
	}
}

func TestGetAccessToken_ImportsSharedStoreOnceThenUsesPrivateStore(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)

	sharedToken := testJWT(t, time.Now().Add(2*time.Hour).UTC())
	writeAuthFile(t, tmpDir, `{"auth_mode":"chatgpt","last_refresh":"2026-04-10T00:43:46Z","tokens":{"access_token": "`+sharedToken+`","refresh_token": "shared_refresh_token"}}`)

	p := &CodexProvider{}
	firstToken, err := p.getAccessToken(context.Background())
	if err != nil {
		t.Fatalf("first getAccessToken() error: %v", err)
	}
	if firstToken != sharedToken {
		t.Fatalf("expected imported shared token, got %q", firstToken)
	}

	privateState, err := readCodexStoreState(sirtophamAuthStorePath(tmpDir))
	if err != nil {
		t.Fatalf("readCodexStoreState() error: %v", err)
	}
	if privateState.token != sharedToken {
		t.Fatalf("expected imported private-store token %q, got %q", sharedToken, privateState.token)
	}

	newerSharedToken := testJWT(t, time.Now().Add(24*time.Hour).UTC())
	writeAuthFile(t, tmpDir, `{"auth_mode":"chatgpt","last_refresh":"2026-04-11T00:43:46Z","tokens":{"access_token": "`+newerSharedToken+`","refresh_token": "newer_shared_refresh_token"}}`)

	p = &CodexProvider{}
	secondToken, err := p.getAccessToken(context.Background())
	if err != nil {
		t.Fatalf("second getAccessToken() error: %v", err)
	}
	if secondToken != sharedToken {
		t.Fatalf("expected imported private-store token to remain authoritative, got %q", secondToken)
	}
}

func TestAuthStatus_ReportsPrivateStoreAfterRuntimeImport(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)

	sharedToken := testJWT(t, time.Now().Add(2*time.Hour).UTC())
	writeAuthFile(t, tmpDir, `{"auth_mode":"chatgpt","tokens":{"access_token": "`+sharedToken+`","refresh_token": "shared_refresh_token"}}`)

	p := &CodexProvider{}
	if _, err := p.getAccessToken(context.Background()); err != nil {
		t.Fatalf("getAccessToken() error: %v", err)
	}

	writeAuthFile(t, tmpDir, `{"auth_mode":"chatgpt","last_refresh":"2026-04-11T00:43:46Z","tokens":{"access_token": "`+testJWT(t, time.Now().Add(24*time.Hour).UTC())+`","refresh_token": "newer_shared_refresh_token"}}`)

	status, err := p.AuthStatus(context.Background())
	if err != nil {
		t.Fatalf("AuthStatus() error: %v", err)
	}
	if status.Source != "sirtopham_store" {
		t.Fatalf("expected private-store source after runtime import, got %+v", status)
	}
	if status.StorePath == "" {
		t.Fatalf("expected private store path after runtime import, got %+v", status)
	}
	if !strings.HasSuffix(status.StorePath, filepath.Join(".sirtopham", "auth.json")) {
		t.Fatalf("expected private auth store path, got %+v", status)
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

func TestGetAccessToken_ExpiredTokenInNonInteractiveRuntimeReturnsActionableErrorWhenNoRefreshToken(t *testing.T) {
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
	if !strings.Contains(pe.Message, "interactive renewal") && !strings.Contains(pe.Message, "refresh_token") {
		t.Fatalf("expected actionable message, got %q", pe.Message)
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
	overrideHomeDir(t, tmpDir)
	overrideStdinIsTerminal(t, false)
	writeAuthFile(t, tmpDir, `{"auth_mode":"chatgpt","tokens":{"access_token":"expired_token","refresh_token":"refresh_token"}}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh_access","refresh_token":"fresh_refresh"}`))
	}))
	defer server.Close()
	overrideCodexRefreshEndpoint(t, "test-client-id", server.URL)

	p := &CodexProvider{}
	err := p.refreshToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tmpDir, ".sirtopham", "auth.json"))
	if err != nil {
		t.Fatalf("read auth file: %v", err)
	}
	if !strings.Contains(string(data), "fresh_access") {
		t.Fatalf("expected refreshed access token to be persisted, got %s", string(data))
	}
	sharedData, err := os.ReadFile(filepath.Join(tmpDir, ".codex", "auth.json"))
	if err != nil {
		t.Fatalf("read shared auth file: %v", err)
	}
	if strings.Contains(string(sharedData), "fresh_access") {
		t.Fatalf("expected shared Codex auth file to remain unchanged, got %s", string(sharedData))
	}
}

func TestRefreshToken_HTTPFailureReturnsProviderError(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	writeAuthFile(t, tmpDir, `{"auth_mode":"chatgpt","tokens":{"access_token":"expired_token","refresh_token":"refresh_token"}}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh expired"}`))
	}))
	defer server.Close()
	overrideCodexRefreshEndpoint(t, "test-client-id", server.URL)

	p := &CodexProvider{}
	err := p.refreshToken(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}

	pe, ok := err.(*provider.ProviderError)
	if !ok {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if !strings.Contains(pe.Message, "refresh expired") {
		t.Errorf("expected message containing refresh error, got %q", pe.Message)
	}
}

func TestRefreshToken_Timeout(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	writeAuthFile(t, tmpDir, `{"auth_mode":"chatgpt","tokens":{"access_token":"expired_token","refresh_token":"refresh_token"}}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"late_token"}`))
	}))
	defer server.Close()
	overrideCodexRefreshEndpoint(t, "test-client-id", server.URL)

	p := &CodexProvider{}
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
	if !strings.Contains(pe.Message, "timed out") && !strings.Contains(pe.Message, "refresh") {
		t.Errorf("expected timeout-related message, got %q", pe.Message)
	}
}

func TestRefreshToken_MissingRefreshTokenReturnsActionableError(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	writeAuthFile(t, tmpDir, `{"auth_mode":"chatgpt","tokens":{"access_token":"expired_token"}}`)

	p := &CodexProvider{}
	err := p.refreshToken(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}

	pe, ok := err.(*provider.ProviderError)
	if !ok {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if !strings.Contains(pe.Message, "refresh_token") {
		t.Fatalf("expected actionable missing refresh token message, got %q", pe.Message)
	}
}

func TestLoginCodexDeviceCodeStoresPrivateAuthState(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeDir(t, tmpDir)
	accessToken := testJWT(t, time.Now().Add(2*time.Hour).UTC())

	var sawUserCodeRequest bool
	var sawPollRequest bool
	var sawTokenExchange bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			sawUserCodeRequest = true
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode usercode request: %v", err)
			}
			if payload["client_id"] != "test-client-id" {
				t.Fatalf("client_id = %q", payload["client_id"])
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"user_code":"ABCD-EFGH","device_auth_id":"device-1","interval":1}`))
		case "/api/accounts/deviceauth/token":
			sawPollRequest = true
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode token poll request: %v", err)
			}
			if payload["device_auth_id"] != "device-1" || payload["user_code"] != "ABCD-EFGH" {
				t.Fatalf("unexpected poll payload: %#v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"authorization_code":"auth-code","code_verifier":"verifier-1"}`))
		case "/oauth/token":
			sawTokenExchange = true
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			if got := r.Form.Get("grant_type"); got != "authorization_code" {
				t.Fatalf("grant_type = %q", got)
			}
			if got := r.Form.Get("code"); got != "auth-code" {
				t.Fatalf("code = %q", got)
			}
			if got := r.Form.Get("redirect_uri"); got != server.URL+"/deviceauth/callback" {
				t.Fatalf("redirect_uri = %q", got)
			}
			if got := r.Form.Get("client_id"); got != "test-client-id" {
				t.Fatalf("client_id = %q", got)
			}
			if got := r.Form.Get("code_verifier"); got != "verifier-1" {
				t.Fatalf("code_verifier = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"` + accessToken + `","refresh_token":"refresh-token"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	overrideCodexDeviceEndpoints(t, "test-client-id", server.URL, server.URL+"/oauth/token")

	var out strings.Builder
	if err := loginCodexDeviceCode(context.Background(), &out, server.Client()); err != nil {
		t.Fatalf("loginCodexDeviceCode() error: %v", err)
	}
	if !sawUserCodeRequest || !sawPollRequest || !sawTokenExchange {
		t.Fatalf("flow requests = usercode:%v poll:%v token:%v", sawUserCodeRequest, sawPollRequest, sawTokenExchange)
	}
	if !strings.Contains(out.String(), server.URL+"/codex/device") || !strings.Contains(out.String(), "ABCD-EFGH") {
		t.Fatalf("output did not include verification URL and code: %q", out.String())
	}

	state, err := readCodexStoreState(sirtophamAuthStorePath(tmpDir))
	if err != nil {
		t.Fatalf("readCodexStoreState() error: %v", err)
	}
	if state.auth.AuthMode != "chatgpt" {
		t.Fatalf("AuthMode = %q, want chatgpt", state.auth.AuthMode)
	}
	if state.auth.Tokens.AccessToken != accessToken || state.auth.AccessToken != accessToken {
		t.Fatalf("access token not persisted in both token fields: %+v", state.auth)
	}
	if state.auth.Tokens.RefreshToken != "refresh-token" || state.auth.RefreshToken != "refresh-token" {
		t.Fatalf("refresh token not persisted in both token fields: %+v", state.auth)
	}
	if state.auth.LastRefresh == "" {
		t.Fatalf("LastRefresh empty")
	}
	if state.auth.ExpiresAt == "" {
		t.Fatalf("ExpiresAt empty")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".codex", "auth.json")); !os.IsNotExist(err) {
		t.Fatalf("expected device login not to write shared Codex auth file, stat err=%v", err)
	}
}
