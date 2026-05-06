package codex

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

// CodexProvider implements the unified Provider interface for OpenAI's
// Responses API using Yard-owned Codex OAuth credentials.
type CodexProvider struct {
	httpClient      *http.Client
	baseURL         string       // default: "https://chatgpt.com/backend-api/codex"
	reasoningEffort string       // default: defaultCodexReasoningEffort
	mu              sync.RWMutex // guards cachedToken and tokenExpiry
	cachedToken     string
	tokenExpiry     time.Time
	codexBinPath    string // optional path used only by model discovery helpers/tests
}

// ProviderOption is a functional option for configuring CodexProvider.
type ProviderOption func(*CodexProvider)

// WithHTTPClient sets the HTTP client used for API requests.
func WithHTTPClient(c *http.Client) ProviderOption {
	return func(p *CodexProvider) {
		p.httpClient = c
	}
}

// WithBaseURL sets the base URL for the Responses API endpoint.
// Any trailing slash is stripped.
func WithBaseURL(url string) ProviderOption {
	return func(p *CodexProvider) {
		p.baseURL = strings.TrimRight(url, "/")
	}
}

// WithReasoningEffort sets the Responses API reasoning effort for Codex calls.
func WithReasoningEffort(effort string) ProviderOption {
	return func(p *CodexProvider) {
		p.reasoningEffort = normalizeCodexReasoningEffort(effort)
	}
}

// NewCodexProvider creates a new CodexProvider after verifying that the codex
// provider can reach the configured Responses endpoint at call time.
func NewCodexProvider(opts ...ProviderOption) (*CodexProvider, error) {
	p := &CodexProvider{
		baseURL:         "https://chatgpt.com/backend-api/codex",
		reasoningEffort: defaultCodexReasoningEffort,
		httpClient:      &http.Client{Timeout: 120 * time.Second},
	}

	for _, opt := range opts {
		opt(p)
	}

	return p, nil
}

func (p *CodexProvider) configuredReasoningEffort() string {
	if p == nil {
		return defaultCodexReasoningEffort
	}
	return normalizeCodexReasoningEffort(p.reasoningEffort)
}

// Name returns the provider name.
func (p *CodexProvider) Name() string {
	return "codex"
}

// Compile-time assertion that CodexProvider satisfies provider.Provider.
var _ provider.Provider = (*CodexProvider)(nil)

// Models returns the static visible model list for the Codex provider. Runtime
// requests are still pinned by codexRequestModel because ChatGPT Codex model
// availability changes independently of this UI/config surface.
func (p *CodexProvider) Models(ctx context.Context) ([]provider.Model, error) {
	_ = ctx
	return visibleModels(), nil
}

func (p *CodexProvider) responsesEndpointURL() string {
	base := strings.TrimRight(p.baseURL, "/")
	if strings.Contains(base, "chatgpt.com/backend-api/codex") || strings.HasSuffix(base, "/codex") {
		return base + "/responses"
	}
	return base + "/v1/responses"
}

func (p *CodexProvider) usesChatGPTCodexEndpoint() bool {
	base := strings.TrimRight(p.baseURL, "/")
	return strings.Contains(base, "chatgpt.com/backend-api/codex") || strings.HasSuffix(base, "/codex")
}
