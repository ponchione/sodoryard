package anthropic

import (
	"context"
	"net/http"
	"time"

	"github.com/ponchione/sirtopham/internal/provider"
)

// credentialSource is the internal interface for obtaining auth headers.
// Both CredentialManager and test mocks satisfy this interface.
type credentialSource interface {
	GetAuthHeader(ctx context.Context) (headerName string, headerValue string, err error)
}

// AnthropicProvider implements provider.Provider for the Anthropic Messages API.
type AnthropicProvider struct {
	creds      credentialSource
	httpClient *http.Client
	baseURL    string
	sleep      func(context.Context, time.Duration) bool
}

// ProviderOption is a functional option for NewAnthropicProvider.
type ProviderOption func(*AnthropicProvider)

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(c *http.Client) ProviderOption {
	return func(p *AnthropicProvider) {
		p.httpClient = c
	}
}

// WithBaseURL overrides the default Anthropic API base URL.
func WithBaseURL(url string) ProviderOption {
	return func(p *AnthropicProvider) {
		p.baseURL = url
	}
}

// NewAnthropicProvider creates a new Anthropic provider with the given
// credential manager and optional configuration overrides.
func NewAnthropicProvider(creds *CredentialManager, opts ...ProviderOption) *AnthropicProvider {
	p := &AnthropicProvider{
		creds:      creds,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		baseURL:    "https://api.anthropic.com",
		sleep:      sleepWithContext,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// newAnthropicProviderInternal creates a provider with the credentialSource
// interface. Used by tests with mock credentials.
func newAnthropicProviderInternal(creds credentialSource, opts ...ProviderOption) *AnthropicProvider {
	p := &AnthropicProvider{
		creds:      creds,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		baseURL:    "https://api.anthropic.com",
		sleep:      sleepWithContext,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name returns the provider name.
func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

// Models returns the static list of supported Claude models.
func (p *AnthropicProvider) Models(ctx context.Context) ([]provider.Model, error) {
	return []provider.Model{
		{ID: "claude-sonnet-4-6-20250514", Name: "Claude Sonnet 4.6", ContextWindow: 200000, SupportsTools: true, SupportsThinking: true},
		{ID: "claude-opus-4-6-20250515", Name: "Claude Opus 4.6", ContextWindow: 200000, SupportsTools: true, SupportsThinking: true},
		{ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", ContextWindow: 200000, SupportsTools: true, SupportsThinking: true},
	}, nil
}

// Compile-time assertion that AnthropicProvider satisfies provider.Provider.
var _ provider.Provider = (*AnthropicProvider)(nil)
