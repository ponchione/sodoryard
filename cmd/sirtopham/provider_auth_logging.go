package main

import (
	"context"
	"log/slog"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/provider"
)

func logProviderAuthStatus(ctx context.Context, logger *slog.Logger, name string, cfg appconfig.ProviderConfig, p provider.Provider) {
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
		if errorAsProviderError(err, &pe) && pe.Remediation != "" {
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
