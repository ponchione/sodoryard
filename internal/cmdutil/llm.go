package cmdutil

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/localservices"
)

type LocalServicesManager interface {
	Status(ctx context.Context, cfg *appconfig.Config) (localservices.StackStatus, error)
	EnsureUp(ctx context.Context, cfg *appconfig.Config) (localservices.StackStatus, error)
	Down(ctx context.Context, cfg *appconfig.Config) error
	Logs(ctx context.Context, cfg *appconfig.Config, tail int) (string, error)
}

func LoadConfig(configPath string) (*appconfig.Config, error) {
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

func RunLLMStatus(ctx context.Context, out io.Writer, configPath string, jsonOutput bool, manager LocalServicesManager) error {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	status, err := manager.Status(ctx, cfg)
	if err != nil {
		return err
	}
	return PrintLLMStatus(out, status, jsonOutput)
}

func RunLLMUp(ctx context.Context, out io.Writer, errOut io.Writer, configPath string, manager LocalServicesManager) error {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	status, err := manager.EnsureUp(ctx, cfg)
	if err != nil {
		_ = PrintLLMStatus(errOut, status, false)
		return err
	}
	return PrintLLMStatus(out, status, false)
}

func RunLLMDown(ctx context.Context, out io.Writer, configPath string, manager LocalServicesManager) error {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	if !cfg.LocalServices.Enabled {
		_, _ = fmt.Fprintln(out, "local services are disabled in config")
		return nil
	}
	if err := manager.Down(ctx, cfg); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "local LLM stack stopped")
	return nil
}

func RunLLMLogs(ctx context.Context, out io.Writer, configPath string, tail int, manager LocalServicesManager) error {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	if !cfg.LocalServices.Enabled {
		_, _ = fmt.Fprintln(out, "local services are disabled in config")
		return nil
	}
	logs, err := manager.Logs(ctx, cfg, tail)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, logs)
	return nil
}

func PrintLLMStatus(out io.Writer, status localservices.StackStatus, jsonOutput bool) error {
	if jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}
	_, _ = fmt.Fprintf(out, "mode: %s\n", blankIfEmpty(status.Mode, "manual"))
	_, _ = fmt.Fprintf(out, "compose_file: %s\n", blankIfEmpty(status.ComposeFile, "<unset>"))
	_, _ = fmt.Fprintf(out, "project_dir: %s\n", blankIfEmpty(status.ProjectDir, "<unset>"))
	_, _ = fmt.Fprintf(out, "docker_available: %t\n", status.DockerAvailable)
	_, _ = fmt.Fprintf(out, "docker_daemon_available: %t\n", status.DaemonAvailable)
	_, _ = fmt.Fprintf(out, "compose_available: %t\n", status.ComposeAvailable)
	_, _ = fmt.Fprintf(out, "compose_file_exists: %t\n", status.ComposeFileExists)
	if len(status.NetworkStatus) > 0 {
		names := make([]string, 0, len(status.NetworkStatus))
		for name := range status.NetworkStatus {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			_, _ = fmt.Fprintf(out, "network.%s: %t\n", name, status.NetworkStatus[name])
		}
	}
	for _, svc := range status.Services {
		_, _ = fmt.Fprintf(out, "service.%s.healthy: %t\n", svc.Name, svc.Healthy)
		_, _ = fmt.Fprintf(out, "service.%s.reachable: %t\n", svc.Name, svc.Reachable)
		_, _ = fmt.Fprintf(out, "service.%s.models_ready: %t\n", svc.Name, svc.ModelsReady)
		if strings.TrimSpace(svc.Detail) != "" {
			_, _ = fmt.Fprintf(out, "service.%s.detail: %s\n", svc.Name, svc.Detail)
		}
	}
	for _, problem := range status.Problems {
		_, _ = fmt.Fprintf(out, "problem: %s\n", problem)
	}
	for _, remediation := range status.Remediation {
		_, _ = fmt.Fprintf(out, "remediation: %s\n", remediation)
	}
	return nil
}

func blankIfEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
