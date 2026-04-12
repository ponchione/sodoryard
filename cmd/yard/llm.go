package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/localservices"
	"github.com/spf13/cobra"
)

func newYardLLMCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{Use: "llm", Short: "Manage repo-owned local LLM services"}
	cmd.AddCommand(newYardLLMStatusCmd(configPath), newYardLLMUpCmd(configPath), newYardLLMDownCmd(configPath), newYardLLMLogsCmd(configPath))
	return cmd
}

func newYardLLMStatusCmd(configPath *string) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show local LLM stack status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := appconfig.Load(*configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			mgr := localservices.NewManager(nil)
			status, err := mgr.Status(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			return yardPrintLLMStatus(cmd.OutOrStdout(), status, jsonOutput)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stack status as JSON")
	return cmd
}

func newYardLLMUpCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Ensure required local LLM services are up",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := appconfig.Load(*configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			mgr := localservices.NewManager(nil)
			status, err := mgr.EnsureUp(cmd.Context(), cfg)
			if err != nil {
				_ = yardPrintLLMStatus(cmd.ErrOrStderr(), status, false)
				return err
			}
			return yardPrintLLMStatus(cmd.OutOrStdout(), status, false)
		},
	}
}

func newYardLLMDownCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Stop managed local LLM services",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := appconfig.Load(*configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if !cfg.LocalServices.Enabled {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "local services are disabled in config")
				return nil
			}
			mgr := localservices.NewManager(nil)
			if err := mgr.Down(cmd.Context(), cfg); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "local LLM stack stopped")
			return nil
		},
	}
}

func newYardLLMLogsCmd(configPath *string) *cobra.Command {
	var tail int
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show recent logs from managed local LLM services",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := appconfig.Load(*configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if !cfg.LocalServices.Enabled {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "local services are disabled in config")
				return nil
			}
			mgr := localservices.NewManager(nil)
			logs, err := mgr.Logs(cmd.Context(), cfg, tail)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), logs)
			return nil
		},
	}
	cmd.Flags().IntVar(&tail, "tail", 100, "Number of recent log lines")
	return cmd
}

func yardNewLLMManager() interface {
	Status(context.Context, *appconfig.Config) (localservices.StackStatus, error)
	EnsureUp(context.Context, *appconfig.Config) (localservices.StackStatus, error)
	Down(context.Context, *appconfig.Config) error
	Logs(context.Context, *appconfig.Config, int) (string, error)
} {
	return localservices.NewManager(nil)
}

func yardPrintLLMStatus(out io.Writer, status localservices.StackStatus, jsonOutput bool) error {
	if jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}
	_, _ = fmt.Fprintf(out, "mode: %s\n", yardBlankIfEmpty(status.Mode, "manual"))
	_, _ = fmt.Fprintf(out, "compose_file: %s\n", yardBlankIfEmpty(status.ComposeFile, "<unset>"))
	_, _ = fmt.Fprintf(out, "project_dir: %s\n", yardBlankIfEmpty(status.ProjectDir, "<unset>"))
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
