package codex

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/ponchione/sodoryard/internal/provider"
)

var _ provider.Pinger = (*CodexProvider)(nil)
var _ provider.AuthStatusReporter = (*CodexProvider)(nil)

// Ping performs an authenticated probe against the configured Codex endpoint.
// It intentionally sends a malformed/minimal body and treats any non-401/403
// HTTP response as proof that auth is accepted, avoiding normal generation work.
func (p *CodexProvider) Ping(ctx context.Context) error {
	token, err := p.getAccessToken(ctx)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.responsesEndpointURL(), strings.NewReader(`{}`))
	if err != nil {
		return provider.NewProviderError("codex", 0, "failed to build auth probe request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return provider.NewProviderError("codex", 0, "Codex auth probe failed", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return provider.NewAuthProviderError("codex", provider.AuthInvalidCredentials, resp.StatusCode, "Codex auth probe was rejected by the backend.", codexAuthRemediation(), nil)
	}
	return nil
}
