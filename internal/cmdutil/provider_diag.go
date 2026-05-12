package cmdutil

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/localservices"
	"github.com/ponchione/sodoryard/internal/provider"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
)

type ProviderBuilder func(name string, cfg appconfig.ProviderConfig) (provider.Provider, error)

type ProviderAuthReport struct {
	Name       string               `json:"name"`
	Type       string               `json:"type"`
	Healthy    bool                 `json:"healthy"`
	AuthState  string               `json:"auth_state,omitempty"`
	BuildError string               `json:"build_error,omitempty"`
	PingError  string               `json:"ping_error,omitempty"`
	Auth       *provider.AuthStatus `json:"auth,omitempty"`
}

func RunProviderDiagnostics(ctx context.Context, out io.Writer, configPath string, jsonOutput bool, includePing bool, manager LocalServicesManager, builder ProviderBuilder) error {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	reports := CollectProviderAuthReports(ctx, cfg, includePing, builder)
	var llmStatus *localservices.StackStatus
	if includePing && manager != nil {
		status, err := manager.Status(ctx, cfg)
		if err == nil {
			llmStatus = &status
		}
	}
	if jsonOutput {
		payload := map[string]any{"providers": reports}
		if llmStatus != nil {
			payload["local_services"] = llmStatus
		}
		return WriteJSON(out, payload)
	}
	PrintProviderAuthReports(out, reports)
	if llmStatus != nil {
		_, _ = fmt.Fprintln(out, "local_services:")
		_ = PrintLLMStatus(out, *llmStatus, false)
	}
	return nil
}

func CollectProviderAuthReports(ctx context.Context, cfg *appconfig.Config, includePing bool, builder ProviderBuilder) []ProviderAuthReport {
	if builder == nil {
		builder = rtpkg.BuildProvider
	}
	providerNames := cfg.ProviderNamesForSurfaces()
	names := make([]string, 0, len(providerNames))
	for _, name := range providerNames {
		names = append(names, name)
	}
	sort.Strings(names)

	reports := make([]ProviderAuthReport, 0, len(names))
	for _, name := range names {
		provCfg := cfg.Providers[name]
		report := ProviderAuthReport{Name: name, Type: provCfg.Type, Healthy: true}
		p, err := builder(name, provCfg)
		if err != nil {
			report.Healthy = false
			report.BuildError = err.Error()
			reports = append(reports, report)
			continue
		}
		if reporter, ok := p.(provider.AuthStatusReporter); ok {
			status, err := reporter.AuthStatus(ctx)
			if err != nil {
				report.Healthy = false
				if report.Auth == nil {
					report.Auth = &provider.AuthStatus{Provider: name, Detail: err.Error()}
				}
				var pe *provider.ProviderError
				if rtpkg.ErrorAsProviderError(err, &pe) {
					report.Auth.Remediation = pe.Remediation
				}
			} else {
				report.Auth = status
			}
		}
		if includePing {
			if pinger, ok := p.(provider.Pinger); ok {
				timeout := 2 * time.Second
				if name == "anthropic" {
					timeout = 5 * time.Second
				}
				pingCtx, cancel := context.WithTimeout(ctx, timeout)
				pingErr := pinger.Ping(pingCtx)
				cancel()
				if pingErr != nil {
					report.Healthy = false
					report.PingError = pingErr.Error()
					if report.Auth == nil {
						report.Auth = &provider.AuthStatus{Provider: name, Detail: pingErr.Error()}
					}
					var pe *provider.ProviderError
					if rtpkg.ErrorAsProviderError(pingErr, &pe) && report.Auth.Remediation == "" {
						report.Auth.Remediation = pe.Remediation
					}
				}
				if pingErr == nil {
					if reporter, ok := p.(provider.AuthStatusReporter); ok {
						if status, err := reporter.AuthStatus(ctx); err == nil {
							report.Auth = status
						}
					}
				}
			}
		}
		if report.Auth != nil {
			report.AuthState = provider.AuthStatusState(report.Auth, time.Now())
			if report.AuthState != "" && report.AuthState != "ready" {
				report.Healthy = false
			}
		}
		reports = append(reports, report)
	}
	return reports
}

func PrintProviderAuthReports(out io.Writer, reports []ProviderAuthReport) {
	for _, report := range reports {
		status := providerReportStatus(report)
		_, _ = fmt.Fprintf(out, "%s (%s): %s\n", report.Name, report.Type, status)
		if report.BuildError != "" {
			_, _ = fmt.Fprintf(out, "  build_error: %s\n", report.BuildError)
			continue
		}
		if report.PingError != "" {
			_, _ = fmt.Fprintf(out, "  ping_error: %s\n", report.PingError)
		}
		if report.Auth == nil {
			_, _ = fmt.Fprintln(out, "  auth: unavailable")
			continue
		}
		if report.AuthState != "" {
			_, _ = fmt.Fprintf(out, "  auth_state: %s\n", report.AuthState)
		}
		_, _ = fmt.Fprintf(out, "  auth_mode: %s\n", valueOrDefault(report.Auth.Mode, "unknown"))
		if report.Auth.Source != "" {
			_, _ = fmt.Fprintf(out, "  source: %s\n", report.Auth.Source)
		}
		if report.Auth.StorePath != "" {
			_, _ = fmt.Fprintf(out, "  store_path: %s\n", report.Auth.StorePath)
		}
		if report.Auth.SourcePath != "" {
			_, _ = fmt.Fprintf(out, "  source_path: %s\n", report.Auth.SourcePath)
		}
		_, _ = fmt.Fprintf(out, "  has_access_token: %t\n", report.Auth.HasAccessToken)
		_, _ = fmt.Fprintf(out, "  has_refresh_token: %t\n", report.Auth.HasRefreshToken)
		if !report.Auth.LastRefresh.IsZero() {
			_, _ = fmt.Fprintf(out, "  last_refresh: %s\n", report.Auth.LastRefresh.Format(time.RFC3339))
		}
		if !report.Auth.ExpiresAt.IsZero() {
			_, _ = fmt.Fprintf(out, "  expires_at: %s\n", report.Auth.ExpiresAt.Format(time.RFC3339))
		} else {
			_, _ = fmt.Fprintln(out, "  expires_at: unknown")
		}
		if report.Auth.Detail != "" {
			_, _ = fmt.Fprintf(out, "  detail: %s\n", report.Auth.Detail)
		}
		if report.Auth.Remediation != "" {
			_, _ = fmt.Fprintf(out, "  remediation: %s\n", report.Auth.Remediation)
		}
	}
}

func providerReportStatus(report ProviderAuthReport) string {
	if report.BuildError != "" || report.PingError != "" {
		return "unhealthy"
	}
	switch report.AuthState {
	case "", "ready":
		if report.Healthy {
			return "healthy"
		}
		return "unhealthy"
	case "expired_access_token":
		return "expired"
	case "access_token_expires_soon":
		return "expires_soon"
	case "missing_credentials", "missing_access_token":
		return "missing_auth"
	default:
		return report.AuthState
	}
}
