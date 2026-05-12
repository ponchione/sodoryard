package runtime

import (
	"context"
	"errors"
	"log/slog"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/provider"
	providerfactory "github.com/ponchione/sodoryard/internal/provider/factory"
)

// ResolveProviderAPIKey returns the API key for a provider config, checking
// the direct APIKey field first, then the APIKeyEnv environment variable.
func ResolveProviderAPIKey(cfg appconfig.ProviderConfig) string {
	return providerfactory.ResolveProviderAPIKey(cfg)
}

// BuildProvider constructs a provider.Provider from config. It applies
// name aliasing when the constructed provider's internal name differs
// from the config key.
func BuildProvider(name string, cfg appconfig.ProviderConfig) (provider.Provider, error) {
	return providerfactory.DefaultRegistry().Build(context.Background(), name, cfg, providerfactory.Deps{})
}

// WithProviderAlias wraps a provider to override its Name() if the
// config-level name differs from the provider's built-in name.
func WithProviderAlias(name string, inner provider.Provider) provider.Provider {
	return providerfactory.WithProviderAlias(name, inner)
}

// AliasedProvider wraps a provider.Provider, overriding Name() with a
// config-level alias while delegating all other methods to the inner
// provider.
type AliasedProvider = providerfactory.AliasedProvider

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
