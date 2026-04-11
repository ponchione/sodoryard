package openai

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

// OpenAIConfig holds provider-level configuration for one OpenAI-compatible endpoint.
type OpenAIConfig struct {
	Name          string `yaml:"name"`           // provider instance name (e.g., "local", "openrouter")
	BaseURL       string `yaml:"base_url"`       // e.g., "http://localhost:8080/v1"
	APIKey        string `yaml:"api_key"`         // optional, direct API key value
	APIKeyEnv     string `yaml:"api_key_env"`     // optional, env var name containing the API key
	Model         string `yaml:"model"`           // default model name
	ContextLength int    `yaml:"context_length"`  // context window size in tokens
}

// OpenAIProvider implements the unified provider interface for any
// OpenAI-compatible chat completions API.
type OpenAIProvider struct {
	name          string
	baseURL       string
	apiKey        string       // resolved key (may be empty for keyless local servers)
	model         string
	contextLength int
	client        *http.Client
}

// NewOpenAIProvider creates a provider instance from config. It resolves
// the API key, validates required fields, and configures the HTTP client.
func NewOpenAIProvider(cfg OpenAIConfig) (*OpenAIProvider, error) {
	// Validate required fields.
	if cfg.Name == "" {
		return nil, fmt.Errorf("openai provider config: name is required")
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("openai provider '%s': base_url is required", cfg.Name)
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("openai provider '%s': model is required", cfg.Name)
	}
	if cfg.ContextLength <= 0 {
		return nil, fmt.Errorf("openai provider '%s': context_length must be a positive integer", cfg.Name)
	}

	// Resolve API key.
	var apiKey string
	if cfg.APIKey != "" {
		apiKey = cfg.APIKey
	} else if cfg.APIKeyEnv != "" {
		apiKey = os.Getenv(cfg.APIKeyEnv)
		if apiKey == "" {
			return nil, fmt.Errorf("openai provider '%s': environment variable '%s' is not set or empty", cfg.Name, cfg.APIKeyEnv)
		}
	}
	// If both APIKey and APIKeyEnv are empty, apiKey stays empty (keyless local mode).

	// Strip trailing slashes from BaseURL.
	baseURL := strings.TrimRight(cfg.BaseURL, "/")

	return &OpenAIProvider{
		name:          cfg.Name,
		baseURL:       baseURL,
		apiKey:        apiKey,
		model:         cfg.Model,
		contextLength: cfg.ContextLength,
		client:        &http.Client{Timeout: 120 * time.Second},
	}, nil
}

// Name returns the provider instance name from config (e.g., "local", "openrouter").
func (p *OpenAIProvider) Name() string {
	return p.name
}

// Models returns a single-element list describing the configured model.
func (p *OpenAIProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{
		{
			ID:            p.model,
			Name:          p.model,
			ContextWindow: p.contextLength,
			SupportsTools: true,
		},
	}, nil
}

// ContextLength returns the context window size in tokens.
func (p *OpenAIProvider) ContextLength() int {
	return p.contextLength
}

// Compile-time assertion that OpenAIProvider satisfies provider.Provider.
var _ provider.Provider = (*OpenAIProvider)(nil)

// Compile-time assertion that OpenAIProvider satisfies provider.Pinger.
var _ provider.Pinger = (*OpenAIProvider)(nil)

// Ping performs a lightweight reachability check by sending an HTTP HEAD request
// to the provider's base URL. This is faster than a Models() call and sufficient
// to verify that a local or OpenAI-compatible server is reachable.
func (p *OpenAIProvider) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "HEAD", p.baseURL, nil)
	if err != nil {
		return &provider.ProviderError{
			Provider:   p.name,
			StatusCode: 0,
			Message:    fmt.Sprintf("failed to create ping request: %s", err),
			Retriable:  false,
			Err:        err,
		}
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return &provider.ProviderError{
			Provider:   p.name,
			StatusCode: 0,
			Message:    fmt.Sprintf("server unreachable: %s", err),
			Retriable:  false,
			Err:        err,
		}
	}
	resp.Body.Close()
	// Any HTTP response (even 404) means the server is reachable.
	return nil
}
