package codex

import (
	"context"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

// CodexProvider implements the unified Provider interface for OpenAI's
// Responses API, using credentials delegated to the codex CLI binary.
type CodexProvider struct {
	httpClient   *http.Client
	baseURL      string       // default: "https://chatgpt.com/backend-api/codex"
	mu           sync.RWMutex // guards cachedToken and tokenExpiry
	cachedToken  string
	tokenExpiry  time.Time
	codexBinPath string // resolved path from exec.LookPath
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

// NewCodexProvider creates a new CodexProvider after verifying that the codex
// CLI binary is available on PATH.
func NewCodexProvider(opts ...ProviderOption) (*CodexProvider, error) {
	p := &CodexProvider{
		baseURL:    "https://chatgpt.com/backend-api/codex",
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}

	binPath, err := exec.LookPath("codex")
	if err != nil {
		return nil, &provider.ProviderError{
			Provider:   "codex",
			StatusCode: 0,
			Message:    "Codex CLI not found on PATH. Install from https://openai.com/codex and run `codex auth`.",
			Retriable:  false,
		}
	}
	p.codexBinPath = binPath

	for _, opt := range opts {
		opt(p)
	}

	return p, nil
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
