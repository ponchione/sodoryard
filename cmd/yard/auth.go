package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/localservices"
	"github.com/ponchione/sodoryard/internal/provider"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/spf13/cobra"
)

type yardAuthProviderReport struct {
	Name       string               `json:"name"`
	Type       string               `json:"type"`
	Healthy    bool                 `json:"healthy"`
	BuildError string               `json:"build_error,omitempty"`
	PingError  string               `json:"ping_error,omitempty"`
	Auth       *provider.AuthStatus `json:"auth,omitempty"`
}

func newYardAuthCmd(configPath *string) *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Inspect provider authentication state",
	}
	authCmd.AddCommand(newYardAuthStatusCmd(configPath))
	return authCmd
}

func newYardDoctorCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run lightweight auth diagnostics for configured providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return yardRunProviderDiagnostics(cmd, *configPath, false, true)
		},
	}
	return cmd
}

func newYardAuthStatusCmd(configPath *string) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show auth mode, source, and expiry for each provider without probing connectivity",
		RunE: func(cmd *cobra.Command, args []string) error {
			return yardRunProviderDiagnostics(cmd, *configPath, jsonOutput, false)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit auth status as JSON")
	return cmd
}

func yardRunProviderDiagnostics(cmd *cobra.Command, configPath string, jsonOutput bool, includePing bool) error {
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	reports := yardCollectProviderAuthReports(cmd.Context(), cfg, includePing)
	var llmStatus *localservices.StackStatus
	if includePing {
		mgr := localservices.NewManager(nil)
		status, err := mgr.Status(cmd.Context(), cfg)
		if err == nil {
			llmStatus = &status
		}
	}
	if jsonOutput {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		payload := map[string]any{"providers": reports}
		if llmStatus != nil {
			payload["local_services"] = llmStatus
		}
		return enc.Encode(payload)
	}
	yardPrintProviderAuthReports(cmd.OutOrStdout(), reports)
	if llmStatus != nil {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "local_services:")
		_ = yardPrintLLMStatus(cmd.OutOrStdout(), *llmStatus, false)
	}
	return nil
}

func yardCollectProviderAuthReports(ctx context.Context, cfg *appconfig.Config, includePing bool) []yardAuthProviderReport {
	providerNames := cfg.ProviderNamesForSurfaces()
	names := make([]string, 0, len(providerNames))
	for _, name := range providerNames {
		names = append(names, name)
	}
	sort.Strings(names)

	reports := make([]yardAuthProviderReport, 0, len(names))
	for _, name := range names {
		provCfg := cfg.Providers[name]
		report := yardAuthProviderReport{Name: name, Type: provCfg.Type, Healthy: true}
		p, err := rtpkg.BuildProvider(name, provCfg)
		if err != nil {
			report.Healthy = false
			report.BuildError = err.Error()
			reports = append(reports, report)
			continue
		}
		if reporter, ok := p.(provider.AuthStatusReporter); ok {
			status, err := reporter.AuthStatus(ctx)
			if err != nil {
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
					if rtpkg.ErrorAsProviderError(pingErr, &pe) {
						if report.Auth.Remediation == "" {
							report.Auth.Remediation = pe.Remediation
						}
					}
				}
			}
		}
		reports = append(reports, report)
	}
	return reports
}

func yardErrorAsProviderError(err error, out **provider.ProviderError) bool {
	if err == nil {
		return false
	}
	return errors.As(err, out)
}

func yardPrintProviderAuthReports(out io.Writer, reports []yardAuthProviderReport) {
	for _, report := range reports {
		status := "healthy"
		if !report.Healthy {
			status = "unhealthy"
		}
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
		_, _ = fmt.Fprintf(out, "  auth_mode: %s\n", yardBlankIfEmpty(report.Auth.Mode, "unknown"))
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

func yardBlankIfEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
