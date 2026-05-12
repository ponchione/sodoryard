package factory

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/provider"
	"github.com/ponchione/sodoryard/internal/provider/anthropic"
	"github.com/ponchione/sodoryard/internal/provider/codex"
	"github.com/ponchione/sodoryard/internal/provider/openai"
)

// Factory builds one provider type from Yard provider configuration.
type Factory interface {
	Type() string
	Build(ctx context.Context, name string, cfg appconfig.ProviderConfig, deps Deps) (provider.Provider, error)
}

// Deps is intentionally empty for the built-in factories today. It gives future
// providers a narrow extension point without widening runtime.BuildProvider.
type Deps struct{}

// Registry maps provider config types to provider builders.
type Registry struct {
	factories map[string]Factory
}

// NewRegistry constructs a provider factory registry.
func NewRegistry(factories ...Factory) (Registry, error) {
	r := Registry{factories: map[string]Factory{}}
	for _, f := range factories {
		if f == nil {
			return Registry{}, fmt.Errorf("provider factory is nil")
		}
		key := strings.TrimSpace(f.Type())
		if key == "" {
			return Registry{}, fmt.Errorf("provider factory type is empty")
		}
		if _, exists := r.factories[key]; exists {
			return Registry{}, fmt.Errorf("duplicate provider factory type %q", key)
		}
		r.factories[key] = f
	}
	return r, nil
}

// DefaultRegistry returns factories for all provider types supported by core.
func DefaultRegistry() Registry {
	r, err := NewRegistry(AnthropicFactory{}, CodexFactory{}, OpenAICompatibleFactory{})
	if err != nil {
		panic(err)
	}
	return r
}

// Types returns the registered provider config types in stable order.
func (r Registry) Types() []string {
	out := make([]string, 0, len(r.factories))
	for key := range r.factories {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// Build constructs a provider using the factory registered for cfg.Type.
func (r Registry) Build(ctx context.Context, name string, cfg appconfig.ProviderConfig, deps Deps) (provider.Provider, error) {
	key := strings.TrimSpace(cfg.Type)
	if key == "" {
		return nil, fmt.Errorf("unsupported provider type: %q", cfg.Type)
	}
	f, ok := r.factories[key]
	if !ok {
		return nil, fmt.Errorf("unsupported provider type: %q", cfg.Type)
	}
	return f.Build(ctx, name, cfg, deps)
}

// ResolveProviderAPIKey returns the API key for a provider config, checking
// the direct APIKey field first, then the APIKeyEnv environment variable.
func ResolveProviderAPIKey(cfg appconfig.ProviderConfig) string {
	if cfg.APIKey != "" {
		return cfg.APIKey
	}
	if cfg.APIKeyEnv != "" {
		return os.Getenv(cfg.APIKeyEnv)
	}
	return ""
}

// AnthropicFactory builds Anthropic Messages API providers.
type AnthropicFactory struct{}

func (AnthropicFactory) Type() string { return "anthropic" }

func (AnthropicFactory) Build(_ context.Context, name string, cfg appconfig.ProviderConfig, _ Deps) (provider.Provider, error) {
	apiKey := ResolveProviderAPIKey(cfg)
	var credOpts []anthropic.CredentialOption
	if apiKey != "" {
		credOpts = append(credOpts, anthropic.WithAPIKey(apiKey))
	}
	creds, err := anthropic.NewCredentialManager(credOpts...)
	if err != nil {
		return nil, err
	}
	return WithProviderAlias(name, anthropic.NewAnthropicProvider(creds)), nil
}

// CodexFactory builds Codex Responses API providers.
type CodexFactory struct{}

func (CodexFactory) Type() string { return "codex" }

func (CodexFactory) Build(_ context.Context, name string, cfg appconfig.ProviderConfig, _ Deps) (provider.Provider, error) {
	var opts []codex.ProviderOption
	if cfg.BaseURL != "" {
		opts = append(opts, codex.WithBaseURL(cfg.BaseURL))
	}
	if cfg.ReasoningEffort != "" {
		opts = append(opts, codex.WithReasoningEffort(cfg.ReasoningEffort))
	}
	p, err := codex.NewCodexProvider(opts...)
	if err != nil {
		return nil, err
	}
	return WithProviderAlias(name, p), nil
}

// OpenAICompatibleFactory builds providers for OpenAI-compatible endpoints.
type OpenAICompatibleFactory struct{}

func (OpenAICompatibleFactory) Type() string { return "openai-compatible" }

func (OpenAICompatibleFactory) Build(_ context.Context, name string, cfg appconfig.ProviderConfig, _ Deps) (provider.Provider, error) {
	return openai.NewOpenAIProvider(openai.OpenAIConfig{
		Name:          name,
		BaseURL:       cfg.BaseURL,
		APIKey:        cfg.APIKey,
		APIKeyEnv:     cfg.APIKeyEnv,
		Model:         cfg.Model,
		ContextLength: cfg.ContextLength,
	})
}

// WithProviderAlias wraps a provider to override its Name() if the
// config-level name differs from the provider's built-in name.
func WithProviderAlias(name string, inner provider.Provider) provider.Provider {
	if inner == nil || name == "" || inner.Name() == name {
		return inner
	}
	return AliasedProvider{Name_: name, Inner: inner}
}

// AliasedProvider wraps a provider.Provider, overriding Name() with a
// config-level alias while delegating all other methods to the inner provider.
type AliasedProvider struct {
	Name_ string
	Inner provider.Provider
}

func (p AliasedProvider) Name() string {
	return p.Name_
}

func (p AliasedProvider) Complete(ctx context.Context, req *provider.Request) (*provider.Response, error) {
	return p.Inner.Complete(ctx, req)
}

func (p AliasedProvider) Stream(ctx context.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	return p.Inner.Stream(ctx, req)
}

func (p AliasedProvider) Models(ctx context.Context) ([]provider.Model, error) {
	return p.Inner.Models(ctx)
}

func (p AliasedProvider) Ping(ctx context.Context) error {
	pinger, ok := p.Inner.(provider.Pinger)
	if !ok {
		return nil
	}
	return pinger.Ping(ctx)
}

func (p AliasedProvider) AuthStatus(ctx context.Context) (*provider.AuthStatus, error) {
	reporter, ok := p.Inner.(provider.AuthStatusReporter)
	if !ok {
		return nil, nil
	}
	status, err := reporter.AuthStatus(ctx)
	if err != nil || status == nil {
		return status, err
	}
	cloned := *status
	cloned.Provider = p.Name_
	return &cloned, nil
}
