package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ponchione/sirtopham/internal/provider"
)

// AuthMode indicates how the CredentialManager authenticates with Anthropic.
type AuthMode int

const (
	// AuthModeOAuth uses OAuth tokens from ~/.claude/.credentials.json.
	AuthModeOAuth AuthMode = iota
	// AuthModeAPIKey uses an API key from the ANTHROPIC_API_KEY env var.
	AuthModeAPIKey
)

// tokenEndpoint is the Anthropic OAuth token refresh endpoint.
// It is a var so tests can override it.
var tokenEndpoint = "https://console.anthropic.com/v1/oauth/token"

// oauthToken holds the cached OAuth credentials.
type oauthToken struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// CredentialManager handles Anthropic credential discovery, caching,
// and transparent OAuth token refresh.
type CredentialManager struct {
	credPath     string // default: ~/.claude/.credentials.json
	mu           sync.RWMutex
	cached       *oauthToken // cached OAuth credentials
	mode         AuthMode
	apiKey       string // only set in API key mode
	apiKeySource string
	httpClient   *http.Client // used for token refresh requests
}

// CredentialOption is a functional option for NewCredentialManager.
type CredentialOption func(*CredentialManager)

// WithCredentialPath overrides the default credential file path.
func WithCredentialPath(path string) CredentialOption {
	return func(cm *CredentialManager) {
		cm.credPath = path
	}
}

// WithAPIKey forces API key mode using the resolved provider secret source.
func WithAPIKey(apiKey string) CredentialOption {
	return func(cm *CredentialManager) {
		cm.apiKey = apiKey
		cm.apiKeySource = "config"
	}
}

// NewCredentialManager creates a CredentialManager with credential discovery.
// It prefers an explicitly supplied API key, then ANTHROPIC_API_KEY, then OAuth mode.
func NewCredentialManager(opts ...CredentialOption) (*CredentialManager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine home directory: %w", err)
	}

	cm := &CredentialManager{
		credPath:   filepath.Join(home, ".claude", ".credentials.json"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}

	for _, opt := range opts {
		opt(cm)
	}

	// Credential discovery.
	if cm.apiKey != "" {
		cm.mode = AuthModeAPIKey
	} else if val, ok := os.LookupEnv("ANTHROPIC_API_KEY"); ok {
		if val == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY is set but empty.")
		}
		cm.mode = AuthModeAPIKey
		cm.apiKey = val
		cm.apiKeySource = "env:ANTHROPIC_API_KEY"
	} else {
		cm.mode = AuthModeOAuth
	}

	return cm, nil
}

// GetAuthHeader returns the HTTP header name and value for authenticating
// with the Anthropic API. It is safe for concurrent use.
func (cm *CredentialManager) GetAuthHeader(ctx context.Context) (headerName string, headerValue string, err error) {
	if ctx.Err() != nil {
		return "", "", fmt.Errorf("credential request cancelled: %w", ctx.Err())
	}

	if cm.mode == AuthModeAPIKey {
		return "X-Api-Key", cm.apiKey, nil
	}

	// OAuth mode: try read lock first.
	cm.mu.RLock()
	if cm.cached != nil && !isTokenExpired(cm.cached) {
		token := cm.cached.AccessToken
		cm.mu.RUnlock()
		return "Authorization", "Bearer " + token, nil
	}
	cm.mu.RUnlock()

	// Need to refresh or load: acquire write lock.
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if ctx.Err() != nil {
		return "", "", fmt.Errorf("credential request cancelled: %w", ctx.Err())
	}

	// Double-check after acquiring write lock.
	if cm.cached != nil && !isTokenExpired(cm.cached) {
		return "Authorization", "Bearer " + cm.cached.AccessToken, nil
	}

	// If no cached token, load from file.
	if cm.cached == nil {
		token, err := readCredentialFile(cm.credPath)
		if err != nil {
			return "", "", err
		}
		cm.cached = token
	}

	// If still not expired after loading, return it.
	if !isTokenExpired(cm.cached) {
		return "Authorization", "Bearer " + cm.cached.AccessToken, nil
	}

	// Token is expired: refresh.
	if err := cm.refreshToken(ctx); err != nil {
		return "", "", err
	}

	return "Authorization", "Bearer " + cm.cached.AccessToken, nil
}

// isTokenExpired returns true if the token is nil or will expire within 2 minutes.
func isTokenExpired(token *oauthToken) bool {
	if token == nil {
		return true
	}
	return time.Now().Add(2 * time.Minute).After(token.ExpiresAt)
}

// tokenRefreshResponse is the JSON structure returned by the OAuth token endpoint.
type tokenRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// refreshToken performs an OAuth token refresh against the Anthropic token endpoint.
// The caller must hold cm.mu.Lock().
func (cm *CredentialManager) refreshToken(ctx context.Context) error {
	if ctx.Err() != nil {
		return fmt.Errorf("credential refresh cancelled: %w", ctx.Err())
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", cm.cached.RefreshToken)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenEndpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to refresh Claude credentials: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := cm.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("credential refresh cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("failed to refresh Claude credentials: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case 200:
		// Success - parse below.
	case 401:
		return fmt.Errorf("Claude credentials expired. Run `claude login` to re-authenticate.")
	case 400:
		return fmt.Errorf("Claude refresh token is invalid. Run `claude login` to re-authenticate.")
	default:
		bodyText := string(respBody)
		if len(bodyText) > 256 {
			bodyText = bodyText[:256]
		}
		return fmt.Errorf("Anthropic token refresh failed (HTTP %d): %s", resp.StatusCode, bodyText)
	}

	var tokenResp tokenRefreshResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return fmt.Errorf("failed to parse token refresh response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("Anthropic token refresh response missing access_token")
	}

	// Update cached token.
	cm.cached.AccessToken = tokenResp.AccessToken
	cm.cached.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// Retain existing refresh token if the response didn't provide one.
	if tokenResp.RefreshToken != "" {
		cm.cached.RefreshToken = tokenResp.RefreshToken
	}

	// Write back to disk; errors are logged but not propagated.
	if err := cm.writeCredentialFile(); err != nil {
		slog.Warn("failed to write back refreshed credentials", "error", err)
	}

	return nil
}

func (cm *CredentialManager) AuthStatus(ctx context.Context) (*provider.AuthStatus, error) {
	status := &provider.AuthStatus{
		Provider:    "anthropic",
		Remediation: "Configure ANTHROPIC_API_KEY or run `claude login`.",
	}

	switch cm.mode {
	case AuthModeAPIKey:
		status.Mode = "api_key"
		status.Source = cm.apiKeySource
		status.HasAccessToken = cm.apiKey != ""
		status.Detail = "Anthropic is configured for API key auth."
		return status, nil
	default:
		status.Mode = "oauth"
		status.Source = "credential_file"
		status.StorePath = cm.credPath
		status.SourcePath = cm.credPath
		status.ActiveProvider = "anthropic"
	}

	cm.mu.RLock()
	cached := cm.cached
	cm.mu.RUnlock()
	if cached != nil {
		status.HasAccessToken = cached.AccessToken != ""
		status.HasRefreshToken = cached.RefreshToken != ""
		status.ExpiresAt = cached.ExpiresAt
		status.Detail = "Anthropic OAuth credentials loaded from the cached credential manager state."
		return status, nil
	}

	token, err := readCredentialFile(cm.credPath)
	if err != nil {
		return nil, provider.NewAuthProviderError("anthropic", provider.AuthMissingCredentials, 0, err.Error(), status.Remediation, err)
	}
	status.HasAccessToken = token.AccessToken != ""
	status.HasRefreshToken = token.RefreshToken != ""
	status.ExpiresAt = token.ExpiresAt
	status.Detail = "Anthropic OAuth credentials loaded from ~/.claude/.credentials.json."
	return status, nil
}
