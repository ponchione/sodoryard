package codex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/ponchione/sirtopham/internal/provider"
)

// codexAuthFile represents the JSON structure of ~/.codex/auth.json.
type codexAuthFile struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   string `json:"expires_at"` // RFC3339 format, e.g. "2026-03-28T16:00:00Z"
	LastRefresh string `json:"last_refresh"`
	Tokens      struct {
		AccessToken string `json:"access_token"`
	} `json:"tokens"`
}

type jwtClaims struct {
	Exp int64 `json:"exp"`
}

// getAccessToken obtains a valid access token, refreshing if needed.
// It uses a read-lock fast path when the cached token is still valid,
// and a write-lock slow path with double-check to avoid redundant refreshes.
func (p *CodexProvider) getAccessToken(ctx context.Context) (string, error) {
	// Fast path: read lock
	p.mu.RLock()
	if p.cachedToken != "" && time.Until(p.tokenExpiry) > 120*time.Second {
		token := p.cachedToken
		p.mu.RUnlock()
		return token, nil
	}
	p.mu.RUnlock()

	// Slow path: write lock
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check: another goroutine may have refreshed while we waited
	if p.cachedToken != "" && time.Until(p.tokenExpiry) > 120*time.Second {
		return p.cachedToken, nil
	}

	// Try reading the auth file first (it may already have a valid token).
	token, expiry, err := p.readAuthFile()
	if err == nil && token != "" && time.Until(expiry) > 30*time.Second {
		p.cachedToken = token
		p.tokenExpiry = expiry
		return token, nil
	}

	// Fall back to CLI refresh only when the auth file is missing/expired.
	if refreshErr := p.refreshToken(ctx); refreshErr != nil {
		return "", refreshErr
	}

	// Read the updated auth file.
	token, expiry, err = p.readAuthFile()
	if err != nil {
		return "", err
	}
	p.cachedToken = token
	p.tokenExpiry = expiry
	return token, nil
}


// authFilePath is a package-level variable to allow tests to override the home directory.
var homeDir = os.UserHomeDir

var stdinIsTerminal = func() bool {
	fd := os.Stdin.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// readAuthFile reads and parses ~/.codex/auth.json.
func (p *CodexProvider) readAuthFile() (string, time.Time, error) {
	home, err := homeDir()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("codex: cannot determine home directory: %w", err)
	}

	path := home + "/.codex/auth.json"
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", time.Time{}, fmt.Errorf("codex: auth file not found at %s. Run `codex auth` to authenticate.", path)
		}
		return "", time.Time{}, fmt.Errorf("codex: cannot read auth file: %w", err)
	}

	var auth codexAuthFile
	if err := json.Unmarshal(data, &auth); err != nil {
		return "", time.Time{}, fmt.Errorf("codex: invalid auth file format: %w", err)
	}

	token := auth.AccessToken
	if token == "" {
		token = auth.Tokens.AccessToken
	}
	if token == "" {
		return "", time.Time{}, fmt.Errorf("codex: auth file contains empty access_token. Run `codex auth` to re-authenticate.")
	}

	if auth.ExpiresAt != "" {
		expiry, err := time.Parse(time.RFC3339, auth.ExpiresAt)
		if err != nil {
			return "", time.Time{}, fmt.Errorf("codex: invalid expires_at timestamp in auth file: %w", err)
		}
		return token, expiry, nil
	}

	expiry, err := jwtExpiry(token)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("codex: auth file missing expires_at and token exp claim: %w", err)
	}
	return token, expiry, nil
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

// refreshToken shells out to `codex refresh` to obtain fresh credentials.
func (p *CodexProvider) refreshToken(ctx context.Context) error {
	if !stdinIsTerminal() {
		return &provider.ProviderError{
			Provider:   "codex",
			StatusCode: 0,
			Message:    "Codex credentials need interactive renewal, but this runtime has no TTY. Run `codex auth` or `codex refresh` in a terminal, then retry.",
			Retriable:  false,
		}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, p.codexBinPath, "refresh")
	cmd.Cancel = func() error {
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = 2 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr

	err := cmd.Run()
	if err != nil {
		if timeoutCtx.Err() != nil {
			return &provider.ProviderError{
				Provider:   "codex",
				StatusCode: 0,
				Message:    "Codex credential refresh timed out after 30s",
				Retriable:  true,
			}
		}

		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}

		errOutput := stderr.String()
		if len(errOutput) > 512 {
			errOutput = errOutput[:512]
		}
		errOutput = strings.TrimSpace(errOutput)

		return &provider.ProviderError{
			Provider:   "codex",
			StatusCode: 0,
			Message:    fmt.Sprintf("Codex credential refresh failed (exit %d): %s", exitCode, errOutput),
			Retriable:  false,
		}
	}

	return nil
}
