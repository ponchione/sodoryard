package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// writeTestCredFile writes a credential JSON file to the given path.
func writeTestCredFile(t *testing.T, dir string, accessToken, refreshToken string, expiresAt time.Time) string {
	t.Helper()
	path := filepath.Join(dir, ".credentials.json")
	cred := credentialFileJSON{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt.Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(cred, "", "    ")
	if err != nil {
		t.Fatalf("failed to marshal test credentials: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("failed to write test credentials: %v", err)
	}
	return path
}

// Test 1 — API key mode returns X-Api-Key header.
func TestAPIKeyModeReturnsXApiKeyHeader(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key-123")

	cm, err := NewCredentialManager()
	if err != nil {
		t.Fatalf("NewCredentialManager() error: %v", err)
	}

	headerName, headerValue, err := cm.GetAuthHeader(context.Background())
	if err != nil {
		t.Fatalf("GetAuthHeader() error: %v", err)
	}
	if headerName != "X-Api-Key" {
		t.Errorf("headerName = %q, want %q", headerName, "X-Api-Key")
	}
	if headerValue != "test-key-123" {
		t.Errorf("headerValue = %q, want %q", headerValue, "test-key-123")
	}
}

// Test 2 — API key mode rejects empty env var.
func TestAPIKeyModeRejectsEmptyEnvVar(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	_, err := NewCredentialManager()
	if err == nil {
		t.Fatal("NewCredentialManager() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "ANTHROPIC_API_KEY is set but empty.") {
		t.Errorf("error = %q, want substring %q", err.Error(), "ANTHROPIC_API_KEY is set but empty.")
	}
}

func TestWithAPIKeyOverridesEnvAndOAuth(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-key")
	dir := t.TempDir()
	credPath := writeTestCredFile(t, dir, "sk-ant-valid", "rt-ant-valid", time.Now().Add(30*time.Minute))

	cm, err := NewCredentialManager(WithCredentialPath(credPath), WithAPIKey("config-key"))
	if err != nil {
		t.Fatalf("NewCredentialManager() error: %v", err)
	}

	headerName, headerValue, err := cm.GetAuthHeader(context.Background())
	if err != nil {
		t.Fatalf("GetAuthHeader() error: %v", err)
	}
	if headerName != "X-Api-Key" || headerValue != "config-key" {
		t.Fatalf("expected explicit API key to win, got %s=%q", headerName, headerValue)
	}
	status, err := cm.AuthStatus(context.Background())
	if err != nil {
		t.Fatalf("AuthStatus() error: %v", err)
	}
	if status.Mode != "api_key" || status.Source != "config" {
		t.Fatalf("unexpected auth status: %+v", status)
	}
	if status.SourcePath != "" {
		t.Fatalf("expected SourcePath to be empty for api_key mode, got %+v", status)
	}
}

// Test 3 — OAuth mode with valid non-expired credential file.
func TestOAuthModeValidCredentialFile(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_API_KEY") })

	dir := t.TempDir()
	expiresAt := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
	credPath := writeTestCredFile(t, dir, "sk-ant-valid", "rt-ant-valid", expiresAt)

	cm, err := NewCredentialManager(WithCredentialPath(credPath))
	if err != nil {
		t.Fatalf("NewCredentialManager() error: %v", err)
	}

	headerName, headerValue, err := cm.GetAuthHeader(context.Background())
	if err != nil {
		t.Fatalf("GetAuthHeader() error: %v", err)
	}
	if headerName != "Authorization" {
		t.Errorf("headerName = %q, want %q", headerName, "Authorization")
	}
	if headerValue != "Bearer sk-ant-valid" {
		t.Errorf("headerValue = %q, want %q", headerValue, "Bearer sk-ant-valid")
	}
	status, err := cm.AuthStatus(context.Background())
	if err != nil {
		t.Fatalf("AuthStatus() error: %v", err)
	}
	if status.Mode != "oauth" || status.StorePath != credPath || !status.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("unexpected auth status: %+v", status)
	}
}

// Test 4 — OAuth mode with expired token triggers refresh.
func TestOAuthModeExpiredTokenTriggersRefresh(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_API_KEY") })

	dir := t.TempDir()
	credPath := writeTestCredFile(t, dir, "sk-ant-old", "rt-ant-old",
		time.Now().Add(-1*time.Minute))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"sk-ant-new","refresh_token":"rt-ant-new","expires_in":3600}`)
	}))
	defer srv.Close()

	origEndpoint := tokenEndpoint
	tokenEndpoint = srv.URL + "/v1/oauth/token"
	t.Cleanup(func() { tokenEndpoint = origEndpoint })

	cm, err := NewCredentialManager(WithCredentialPath(credPath))
	if err != nil {
		t.Fatalf("NewCredentialManager() error: %v", err)
	}

	headerName, headerValue, err := cm.GetAuthHeader(context.Background())
	if err != nil {
		t.Fatalf("GetAuthHeader() error: %v", err)
	}
	if headerName != "Authorization" {
		t.Errorf("headerName = %q, want %q", headerName, "Authorization")
	}
	if headerValue != "Bearer sk-ant-new" {
		t.Errorf("headerValue = %q, want %q", headerValue, "Bearer sk-ant-new")
	}

	// Verify credential file was updated on disk.
	data, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatalf("failed to read credential file: %v", err)
	}
	if !strings.Contains(string(data), `"accessToken":"sk-ant-new"`) {
		// Try with pretty-printed format.
		if !strings.Contains(string(data), `"accessToken": "sk-ant-new"`) {
			t.Errorf("credential file does not contain updated accessToken, got: %s", string(data))
		}
	}
}

// Test 5 — OAuth mode with credential file not found.
func TestOAuthModeCredentialFileNotFound(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_API_KEY") })

	cm, err := NewCredentialManager(WithCredentialPath("/nonexistent/path/.credentials.json"))
	if err != nil {
		t.Fatalf("NewCredentialManager() error: %v", err)
	}

	_, _, err = cm.GetAuthHeader(context.Background())
	if err == nil {
		t.Fatal("GetAuthHeader() expected error, got nil")
	}
	expected := "~/.claude/.credentials.json not found. Install Claude Code and run `claude login`."
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("error = %q, want substring %q", err.Error(), expected)
	}
}

// Test 6 — OAuth mode with malformed JSON.
func TestOAuthModeMalformedJSON(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_API_KEY") })

	dir := t.TempDir()
	credPath := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(credPath, []byte("{not valid json}"), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cm, err := NewCredentialManager(WithCredentialPath(credPath))
	if err != nil {
		t.Fatalf("NewCredentialManager() error: %v", err)
	}

	_, _, err = cm.GetAuthHeader(context.Background())
	if err == nil {
		t.Fatal("GetAuthHeader() expected error, got nil")
	}
	expected := "Failed to parse Claude credentials at ~/.claude/.credentials.json:"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("error = %q, want substring %q", err.Error(), expected)
	}
}

// Test 7 — OAuth mode refresh returns 401.
func TestOAuthModeRefreshReturns401(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_API_KEY") })

	dir := t.TempDir()
	credPath := writeTestCredFile(t, dir, "sk-ant-expired", "rt-ant-expired",
		time.Now().Add(-1*time.Minute))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	origEndpoint := tokenEndpoint
	tokenEndpoint = srv.URL + "/v1/oauth/token"
	t.Cleanup(func() { tokenEndpoint = origEndpoint })

	cm, err := NewCredentialManager(WithCredentialPath(credPath))
	if err != nil {
		t.Fatalf("NewCredentialManager() error: %v", err)
	}

	_, _, err = cm.GetAuthHeader(context.Background())
	if err == nil {
		t.Fatal("GetAuthHeader() expected error, got nil")
	}
	expected := "Claude credentials expired. Run `claude login` to re-authenticate."
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("error = %q, want substring %q", err.Error(), expected)
	}
}

// Test 8 — OAuth mode refresh network error.
func TestOAuthModeRefreshNetworkError(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_API_KEY") })

	dir := t.TempDir()
	credPath := writeTestCredFile(t, dir, "sk-ant-expired", "rt-ant-expired",
		time.Now().Add(-1*time.Minute))

	origEndpoint := tokenEndpoint
	tokenEndpoint = "http://127.0.0.1:1"
	t.Cleanup(func() { tokenEndpoint = origEndpoint })

	cm, err := NewCredentialManager(WithCredentialPath(credPath))
	if err != nil {
		t.Fatalf("NewCredentialManager() error: %v", err)
	}

	_, _, err = cm.GetAuthHeader(context.Background())
	if err == nil {
		t.Fatal("GetAuthHeader() expected error, got nil")
	}
	expected := "failed to refresh Claude credentials:"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("error = %q, want substring %q", err.Error(), expected)
	}
}

// Test 9 — Concurrent GetAuthHeader calls coalesce to one refresh.
func TestConcurrentGetAuthHeaderCoalesces(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_API_KEY") })

	dir := t.TempDir()
	credPath := writeTestCredFile(t, dir, "sk-ant-expired", "rt-ant-expired",
		time.Now().Add(-1*time.Minute))

	var callCount atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"sk-ant-concurrent","refresh_token":"rt-ant-concurrent","expires_in":3600}`)
	}))
	defer srv.Close()

	origEndpoint := tokenEndpoint
	tokenEndpoint = srv.URL + "/v1/oauth/token"
	t.Cleanup(func() { tokenEndpoint = origEndpoint })

	cm, err := NewCredentialManager(WithCredentialPath(credPath))
	if err != nil {
		t.Fatalf("NewCredentialManager() error: %v", err)
	}

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	type result struct {
		headerName  string
		headerValue string
		err         error
	}
	results := make([]result, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			name, value, err := cm.GetAuthHeader(context.Background())
			results[idx] = result{name, value, err}
		}(i)
	}
	wg.Wait()

	count := callCount.Load()
	if count != 1 {
		t.Errorf("refresh HTTP call count = %d, want 1", count)
	}

	for i, r := range results {
		if r.err != nil {
			t.Errorf("goroutine %d: GetAuthHeader() error: %v", i, r.err)
			continue
		}
		if r.headerName != "Authorization" {
			t.Errorf("goroutine %d: headerName = %q, want %q", i, r.headerName, "Authorization")
		}
		if r.headerValue != "Bearer sk-ant-concurrent" {
			t.Errorf("goroutine %d: headerValue = %q, want %q", i, r.headerValue, "Bearer sk-ant-concurrent")
		}
	}
}

// Test 10 — Write-back persists refreshed credentials to disk.
func TestWriteBackPersistsCredentials(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_API_KEY") })

	dir := t.TempDir()
	credPath := writeTestCredFile(t, dir, "sk-ant-old", "rt-ant-old",
		time.Now().Add(-1*time.Minute))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"sk-ant-refreshed","refresh_token":"rt-ant-refreshed","expires_in":7200}`)
	}))
	defer srv.Close()

	origEndpoint := tokenEndpoint
	tokenEndpoint = srv.URL + "/v1/oauth/token"
	t.Cleanup(func() { tokenEndpoint = origEndpoint })

	cm, err := NewCredentialManager(WithCredentialPath(credPath))
	if err != nil {
		t.Fatalf("NewCredentialManager() error: %v", err)
	}

	_, _, err = cm.GetAuthHeader(context.Background())
	if err != nil {
		t.Fatalf("GetAuthHeader() error: %v", err)
	}

	// Read the credential file from disk and verify.
	data, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatalf("failed to read credential file: %v", err)
	}

	var cred credentialFileJSON
	if err := json.Unmarshal(data, &cred); err != nil {
		t.Fatalf("failed to parse credential file: %v", err)
	}

	if cred.AccessToken != "sk-ant-refreshed" {
		t.Errorf("accessToken = %q, want %q", cred.AccessToken, "sk-ant-refreshed")
	}
	if cred.RefreshToken != "rt-ant-refreshed" {
		t.Errorf("refreshToken = %q, want %q", cred.RefreshToken, "rt-ant-refreshed")
	}

	expiresAt, err := time.Parse(time.RFC3339, cred.ExpiresAt)
	if err != nil {
		t.Fatalf("failed to parse expiresAt %q: %v", cred.ExpiresAt, err)
	}

	expected := time.Now().Add(7200 * time.Second)
	diff := math.Abs(expected.Sub(expiresAt).Seconds())
	if diff > 10 {
		t.Errorf("expiresAt off by %.0f seconds (want within 10s), got %v", diff, expiresAt)
	}
}
