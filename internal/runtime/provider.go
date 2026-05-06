package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/provider"
	"github.com/ponchione/sodoryard/internal/provider/anthropic"
	"github.com/ponchione/sodoryard/internal/provider/codex"
	"github.com/ponchione/sodoryard/internal/provider/openai"
)

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

// BuildProvider constructs a provider.Provider from config. It applies
// name aliasing when the constructed provider's internal name differs
// from the config key.
func BuildProvider(name string, cfg appconfig.ProviderConfig) (provider.Provider, error) {
	apiKey := ResolveProviderAPIKey(cfg)

	switch cfg.Type {
	case "anthropic":
		var credOpts []anthropic.CredentialOption
		if apiKey != "" {
			credOpts = append(credOpts, anthropic.WithAPIKey(apiKey))
		}
		creds, err := anthropic.NewCredentialManager(credOpts...)
		if err != nil {
			return nil, err
		}
		return WithProviderAlias(name, anthropic.NewAnthropicProvider(creds)), nil

	case "openai-compatible":
		return openai.NewOpenAIProvider(openai.OpenAIConfig{
			Name:          name,
			BaseURL:       cfg.BaseURL,
			APIKey:        cfg.APIKey,
			APIKeyEnv:     cfg.APIKeyEnv,
			Model:         cfg.Model,
			ContextLength: cfg.ContextLength,
		})

	case "codex":
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

	default:
		return nil, fmt.Errorf("unsupported provider type: %q", cfg.Type)
	}
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
// config-level alias while delegating all other methods to the inner
// provider.
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

// LogProviderAuthStatus logs the authentication status of a provider
// at registration time for operator diagnostics.
func LogProviderAuthStatus(ctx context.Context, logger *slog.Logger, name string, cfg appconfig.ProviderConfig, p provider.Provider) {
	attrs := []any{"name", name, "type", cfg.Type}
	reporter, ok := p.(provider.AuthStatusReporter)
	if !ok {
		logger.Info("registered provider", attrs...)
		return
	}
	status, err := reporter.AuthStatus(ctx)
	if err != nil {
		attrs = append(attrs, "auth_status_error", err.Error())
		var pe *provider.ProviderError
		if ErrorAsProviderError(err, &pe) && pe.Remediation != "" {
			attrs = append(attrs, "auth_remediation", pe.Remediation)
		}
		logger.Info("registered provider", attrs...)
		return
	}
	attrs = append(attrs,
		"auth_mode", status.Mode,
		"auth_source", status.Source,
		"auth_store", status.StorePath,
		"auth_source_path", status.SourcePath,
		"auth_has_refresh", status.HasRefreshToken,
	)
	if !status.ExpiresAt.IsZero() {
		attrs = append(attrs, "auth_expires_at", status.ExpiresAt)
	}
	if !status.LastRefresh.IsZero() {
		attrs = append(attrs, "auth_last_refresh", status.LastRefresh)
	}
	logger.Info("registered provider", attrs...)
}

// ErrorAsProviderError is a helper that wraps errors.As for provider.ProviderError.
func ErrorAsProviderError(err error, out **provider.ProviderError) bool {
	if err == nil {
		return false
	}
	return errors.As(err, out)
}
