package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/ponchione/sodoryard/internal/provider"
)

type codexAuthTokens struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// codexAuthFile represents the JSON structure persisted in Yard's compatibility
// auth store for the Codex provider.
type codexAuthFile struct {
	AuthMode     string          `json:"auth_mode,omitempty"`
	AccessToken  string          `json:"access_token,omitempty"`
	RefreshToken string          `json:"refresh_token,omitempty"`
	ExpiresAt    string          `json:"expires_at,omitempty"`
	LastRefresh  string          `json:"last_refresh,omitempty"`
	Tokens       codexAuthTokens `json:"tokens,omitempty"`
}

type codexRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Error        string `json:"error,omitempty"`
	Description  string `json:"error_description,omitempty"`
	Message      string `json:"message,omitempty"`
}

type codexDeviceCodeResponse struct {
	UserCode     string `json:"user_code"`
	DeviceAuthID string `json:"device_auth_id"`
	Interval     int    `json:"interval"`
}

type codexDeviceTokenResponse struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeVerifier      string `json:"code_verifier"`
	Error             string `json:"error,omitempty"`
	Description       string `json:"error_description,omitempty"`
	Message           string `json:"message,omitempty"`
}

type codexAuthState struct {
	path           string
	sourcePath     string
	version        int
	activeProvider string
	fromSharedCLI  bool
	auth           codexAuthFile
	token          string
	expiry         time.Time
	hasTokenExpiry bool
}

type jwtClaims struct {
	Exp int64 `json:"exp"`
}

var homeDir = os.UserHomeDir

var stdinIsTerminal = func() bool {
	fd := os.Stdin.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

var codexOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
var codexOAuthIssuer = "https://auth.openai.com"
var codexOAuthTokenURL = "https://auth.openai.com/oauth/token"
var codexDeviceAuthMaxWait = 15 * time.Minute
var codexAuthSleeper = func(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func codexAuthRemediation() string {
	return "Run `yard auth login codex` to refresh Codex login. Yard imports from CODEX_HOME/auth.json or ~/.codex/auth.json only when the compatibility store at ~/.sirtopham/auth.json is absent; once imported, that private store is authoritative."
}

func codexDeviceUserCodeURL() string {
	return strings.TrimRight(codexOAuthIssuer, "/") + "/api/accounts/deviceauth/usercode"
}

func codexDeviceTokenURL() string {
	return strings.TrimRight(codexOAuthIssuer, "/") + "/api/accounts/deviceauth/token"
}

func codexDeviceVerificationURL() string {
	return strings.TrimRight(codexOAuthIssuer, "/") + "/codex/device"
}

func codexDeviceRedirectURI() string {
	return strings.TrimRight(codexOAuthIssuer, "/") + "/deviceauth/callback"
}

// getAccessToken obtains a valid access token, refreshing Yard's private Codex
// auth store when needed.
func (p *CodexProvider) getAccessToken(ctx context.Context) (string, error) {
	p.mu.RLock()
	if p.cachedToken != "" && (p.tokenExpiry.IsZero() || time.Until(p.tokenExpiry) > 120*time.Second) {
		token := p.cachedToken
		p.mu.RUnlock()
		return token, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cachedToken != "" && (p.tokenExpiry.IsZero() || time.Until(p.tokenExpiry) > 120*time.Second) {
		return p.cachedToken, nil
	}

	state, err := p.readAuthState()
	if err == nil && state.token != "" {
		p.cachedToken = state.token
		p.tokenExpiry = state.expiry
		if !state.hasTokenExpiry || time.Until(state.expiry) > 30*time.Second {
			return state.token, nil
		}
	}

	if refreshErr := p.refreshToken(ctx); refreshErr != nil {
		return "", refreshErr
	}

	token, expiry, err := p.readAuthFile()
	if err != nil {
		return "", err
	}
	p.cachedToken = token
	p.tokenExpiry = expiry
	return token, nil
}

func (p *CodexProvider) readAuthState() (*codexAuthState, error) {
	return p.readAuthStateWithImport(true)
}

func (p *CodexProvider) inspectAuthState() (*codexAuthState, error) {
	return p.readAuthStateWithImport(false)
}

func (p *CodexProvider) readAuthStateWithImport(allowImport bool) (*codexAuthState, error) {
	home, err := homeDir()
	if err != nil {
		return nil, fmt.Errorf("codex: cannot determine home directory: %w", err)
	}

	storePath := sirtophamAuthStorePath(home)
	privateState, err := readCodexStoreState(storePath)
	if err == nil {
		return privateState, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, errCodexStoreStateNotFound) {
		return nil, fmt.Errorf("codex: invalid Yard Codex auth store %s: %w", storePath, err)
	}

	sharedState, err := p.readSharedCodexAuthState(home, storePath, allowImport)
	if err != nil {
		return nil, err
	}
	if !allowImport {
		return sharedState, nil
	}
	if err := writeCodexStore(storePath, sharedState.auth); err != nil {
		return nil, fmt.Errorf("codex: failed to persist imported auth state to %s: %w", storePath, err)
	}
	return sharedState, nil
}

func (p *CodexProvider) readSharedCodexAuthState(home, storePath string, allowImport bool) (*codexAuthState, error) {
	sharedPath := codexCLIAuthPath(home)
	var sharedAuth codexAuthFile
	if err := readJSONFileLocked(sharedPath, &sharedAuth); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("codex: auth not found in %s or %s. %s", storePath, sharedPath, codexAuthRemediation())
		}
		return nil, fmt.Errorf("codex: failed to import Codex auth from %s: %w", sharedPath, err)
	}

	path := sharedPath
	if allowImport {
		path = storePath
	}
	sharedState, err := buildCodexAuthState(path, sharedPath, sirtophamAuthStoreVersion, "codex", !allowImport, sharedAuth)
	if err != nil {
		return nil, err
	}
	return sharedState, nil
}

func buildCodexAuthState(path, sourcePath string, version int, activeProvider string, fromSharedCLI bool, auth codexAuthFile) (*codexAuthState, error) {
	token := strings.TrimSpace(auth.Tokens.AccessToken)
	if token == "" {
		token = strings.TrimSpace(auth.AccessToken)
	}
	if token == "" {
		return nil, fmt.Errorf("codex: auth state contains empty access_token. %s", codexAuthRemediation())
	}

	expiry, known, err := codexTokenExpiry(auth, token)
	if err != nil {
		return nil, err
	}

	return &codexAuthState{
		path:           path,
		sourcePath:     sourcePath,
		version:        version,
		activeProvider: activeProvider,
		fromSharedCLI:  fromSharedCLI,
		auth:           auth,
		token:          token,
		expiry:         expiry,
		hasTokenExpiry: known,
	}, nil
}

func codexTokenExpiry(auth codexAuthFile, token string) (time.Time, bool, error) {
	if strings.TrimSpace(auth.ExpiresAt) != "" {
		expiry, err := time.Parse(time.RFC3339, auth.ExpiresAt)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("codex: invalid expires_at timestamp in auth state: %w", err)
		}
		return expiry, true, nil
	}
	if expiry, err := jwtExpiry(token); err == nil {
		return expiry, true, nil
	}
	return time.Time{}, false, nil
}

// LoginCodexDeviceCode runs OpenAI's Codex device-code OAuth flow and persists
// the resulting tokens only to Yard's private auth store.
func LoginCodexDeviceCode(ctx context.Context, out io.Writer) error {
	return loginCodexDeviceCode(ctx, out, &http.Client{Timeout: 15 * time.Second})
}

func loginCodexDeviceCode(ctx context.Context, out io.Writer, client *http.Client) error {
	if out == nil {
		out = io.Discard
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	home, err := homeDir()
	if err != nil {
		return fmt.Errorf("codex: cannot determine home directory: %w", err)
	}
	storePath := sirtophamAuthStorePath(home)

	var device codexDeviceCodeResponse
	status, err := postCodexJSON(ctx, client, codexDeviceUserCodeURL(), map[string]string{
		"client_id": codexOAuthClientID,
	}, &device)
	if err != nil {
		return fmt.Errorf("codex device code request failed: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("codex device code request returned status %d", status)
	}
	if strings.TrimSpace(device.UserCode) == "" || strings.TrimSpace(device.DeviceAuthID) == "" {
		return fmt.Errorf("codex device code response missing required fields")
	}

	_, _ = fmt.Fprintln(out, "To continue, open this URL in your browser:")
	_, _ = fmt.Fprintf(out, "  %s\n\n", codexDeviceVerificationURL())
	_, _ = fmt.Fprintln(out, "Enter this code:")
	_, _ = fmt.Fprintf(out, "  %s\n\n", device.UserCode)
	_, _ = fmt.Fprintln(out, "Waiting for sign-in... (press Ctrl+C to cancel)")

	pollInterval := time.Duration(device.Interval) * time.Second
	if pollInterval < 3*time.Second {
		pollInterval = 3 * time.Second
	}
	deadline := time.Now().Add(codexDeviceAuthMaxWait)
	var codeResp codexDeviceTokenResponse
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("codex device login timed out after %s", codexDeviceAuthMaxWait)
		}
		if err := codexAuthSleeper(ctx, pollInterval); err != nil {
			return err
		}

		status, err = postCodexJSON(ctx, client, codexDeviceTokenURL(), map[string]string{
			"device_auth_id": device.DeviceAuthID,
			"user_code":      device.UserCode,
		}, &codeResp)
		if err != nil {
			return fmt.Errorf("codex device auth polling failed: %w", err)
		}
		if status == http.StatusOK {
			break
		}
		if status == http.StatusForbidden || status == http.StatusNotFound {
			continue
		}
		return fmt.Errorf("codex device auth polling returned status %d", status)
	}
	if strings.TrimSpace(codeResp.AuthorizationCode) == "" || strings.TrimSpace(codeResp.CodeVerifier) == "" {
		return fmt.Errorf("codex device auth response missing authorization_code or code_verifier")
	}

	var tokenResp codexRefreshResponse
	status, err = postCodexForm(ctx, client, codexOAuthTokenURL, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {codeResp.AuthorizationCode},
		"redirect_uri":  {codexDeviceRedirectURI()},
		"client_id":     {codexOAuthClientID},
		"code_verifier": {codeResp.CodeVerifier},
	}, &tokenResp)
	if err != nil {
		return fmt.Errorf("codex token exchange failed: %w", err)
	}
	if status != http.StatusOK {
		if tokenResp.Description != "" {
			return fmt.Errorf("codex token exchange failed: %s", tokenResp.Description)
		}
		if tokenResp.Message != "" {
			return fmt.Errorf("codex token exchange failed: %s", tokenResp.Message)
		}
		return fmt.Errorf("codex token exchange returned status %d", status)
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return fmt.Errorf("codex token exchange did not return an access_token")
	}

	auth := codexAuthFile{
		AuthMode:     "chatgpt",
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		LastRefresh:  time.Now().UTC().Format(time.RFC3339),
		Tokens: codexAuthTokens{
			AccessToken:  tokenResp.AccessToken,
			RefreshToken: tokenResp.RefreshToken,
		},
	}
	if expiry, err := jwtExpiry(tokenResp.AccessToken); err == nil {
		auth.ExpiresAt = expiry.Format(time.RFC3339)
	}
	if err := writeCodexStore(storePath, auth); err != nil {
		return fmt.Errorf("codex: failed to persist auth state to %s: %w", storePath, err)
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Login successful.")
	_, _ = fmt.Fprintf(out, "Auth state: %s\n", storePath)
	return nil
}

func postCodexJSON(ctx context.Context, client *http.Client, endpoint string, payload any, dst any) (int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if dst != nil {
		_ = json.NewDecoder(resp.Body).Decode(dst)
	}
	return resp.StatusCode, nil
}

func postCodexForm(ctx context.Context, client *http.Client, endpoint string, form url.Values, dst any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if dst != nil {
		_ = json.NewDecoder(resp.Body).Decode(dst)
	}
	return resp.StatusCode, nil
}

// readAuthFile reads and parses Yard's Codex auth store, importing the
// user's Codex CLI auth once when the local store is empty.
func (p *CodexProvider) readAuthFile() (string, time.Time, error) {
	state, err := p.readAuthState()
	if err != nil {
		return "", time.Time{}, err
	}
	return state.token, state.expiry, nil
}

func jwtExpiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, fmt.Errorf("token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("decode JWT payload: %w", err)
	}
	var claims jwtClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, fmt.Errorf("parse JWT payload: %w", err)
	}
	if claims.Exp <= 0 {
		return time.Time{}, fmt.Errorf("missing exp claim")
	}
	return time.Unix(claims.Exp, 0).UTC(), nil
}

// refreshToken refreshes Codex OAuth credentials via the token endpoint and
// persists them only to Yard's private auth store.
func (p *CodexProvider) refreshToken(ctx context.Context) error {
	state, err := p.readAuthState()
	if err != nil {
		return provider.NewAuthProviderError("codex", provider.AuthMissingCredentials, 0, err.Error(), codexAuthRemediation(), err)
	}

	refreshToken := strings.TrimSpace(state.auth.Tokens.RefreshToken)
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(state.auth.RefreshToken)
	}
	if refreshToken == "" {
		return provider.NewAuthProviderError("codex", provider.AuthMissingCredentials, 0, "Codex auth state is missing refresh_token.", codexAuthRemediation(), nil)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", codexOAuthClientID)

	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, codexOAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return provider.NewAuthProviderError("codex", provider.AuthMisconfigured, 0, fmt.Sprintf("Codex credential refresh request build failed: %v", err), codexAuthRemediation(), err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		message := fmt.Sprintf("Codex credential refresh failed: %v", err)
		retriable := timeoutCtx.Err() != nil
		if timeoutCtx.Err() != nil {
			message = "Codex credential refresh timed out after 30s"
		}
		return &provider.ProviderError{Provider: "codex", StatusCode: 0, Message: message, Retriable: retriable, Err: err, AuthKind: provider.AuthRefreshFailed, Remediation: codexAuthRemediation()}
	}
	defer resp.Body.Close()

	var payload codexRefreshResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return provider.NewAuthProviderError("codex", provider.AuthRefreshFailed, resp.StatusCode, fmt.Sprintf("Codex credential refresh returned invalid JSON: %v", err), codexAuthRemediation(), err)
	}

	if resp.StatusCode != http.StatusOK {
		message := fmt.Sprintf("Codex token refresh failed with status %d.", resp.StatusCode)
		if payload.Description != "" {
			message = fmt.Sprintf("Codex token refresh failed: %s", payload.Description)
		} else if payload.Message != "" {
			message = fmt.Sprintf("Codex token refresh failed: %s", payload.Message)
		}
		kind := provider.AuthRefreshFailed
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || payload.Error == "invalid_grant" {
			kind = provider.AuthExpiredCredentials
		}
		return provider.NewAuthProviderError("codex", kind, resp.StatusCode, message, codexAuthRemediation(), nil)
	}

	if strings.TrimSpace(payload.AccessToken) == "" {
		return provider.NewAuthProviderError("codex", provider.AuthRefreshFailed, resp.StatusCode, "Codex token refresh response was missing access_token.", codexAuthRemediation(), nil)
	}

	state.auth.AccessToken = payload.AccessToken
	state.auth.Tokens.AccessToken = payload.AccessToken
	if strings.TrimSpace(payload.RefreshToken) != "" {
		state.auth.RefreshToken = payload.RefreshToken
		state.auth.Tokens.RefreshToken = payload.RefreshToken
	}
	state.auth.AuthMode = "chatgpt"
	state.auth.LastRefresh = time.Now().UTC().Format(time.RFC3339)
	if expiry, err := jwtExpiry(payload.AccessToken); err == nil {
		state.auth.ExpiresAt = expiry.Format(time.RFC3339)
		state.expiry = expiry
		state.hasTokenExpiry = true
	} else {
		state.auth.ExpiresAt = ""
		state.expiry = time.Time{}
		state.hasTokenExpiry = false
	}
	state.token = payload.AccessToken

	if err := writeCodexStore(state.path, state.auth); err != nil {
		return provider.NewAuthProviderError("codex", provider.AuthMisconfigured, 0, fmt.Sprintf("Codex credential refresh could not persist auth state: %v", err), codexAuthRemediation(), err)
	}

	p.cachedToken = payload.AccessToken
	p.tokenExpiry = state.expiry
	return nil
}

func (p *CodexProvider) AuthStatus(ctx context.Context) (*provider.AuthStatus, error) {
	_ = ctx
	state, err := p.inspectAuthState()
	if err != nil {
		return nil, err
	}
	source := "sirtopham_store"
	storePath := state.path
	detail := ""
	if state.fromSharedCLI {
		source = "codex_cli_store"
		storePath = ""
		detail = "Available in the shared Codex CLI store and will be imported into ~/.sirtopham/auth.json on first use."
	}
	status := &provider.AuthStatus{
		Provider:        "codex",
		Mode:            state.auth.AuthMode,
		Source:          source,
		StorePath:       storePath,
		SourcePath:      state.sourcePath,
		ActiveProvider:  state.activeProvider,
		Version:         state.version,
		ExpiresAt:       state.expiry,
		HasAccessToken:  strings.TrimSpace(state.token) != "",
		HasRefreshToken: strings.TrimSpace(state.auth.Tokens.RefreshToken) != "" || strings.TrimSpace(state.auth.RefreshToken) != "",
		Detail:          detail,
		Remediation:     codexAuthRemediation(),
	}
	if state.auth.LastRefresh != "" {
		if t, err := time.Parse(time.RFC3339, state.auth.LastRefresh); err == nil {
			status.LastRefresh = t
		}
	}
	return status, nil
}
