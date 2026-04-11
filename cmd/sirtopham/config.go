package main

import (
	"fmt"
	"io"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or validate configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfig(cmd.OutOrStdout(), *configPath)
		},
	}
	return cmd
}

func runConfig(out io.Writer, configPath string) error {
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	brainVaultPath := "<disabled>"
	if cfg.Brain.Enabled {
		brainVaultPath = cfg.BrainVaultPath()
	}

	_, _ = fmt.Fprintln(out, "config: valid")
	_, _ = fmt.Fprintf(out, "config_path: %s\n", configPath)
	_, _ = fmt.Fprintf(out, "project_root: %s\n", cfg.ProjectRoot)
	_, _ = fmt.Fprintf(out, "server_address: %s\n", cfg.ServerAddress())
	_, _ = fmt.Fprintf(out, "default_provider: %s\n", cfg.Routing.Default.Provider)
	_, _ = fmt.Fprintf(out, "default_model: %s\n", cfg.Routing.Default.Model)
	_, _ = fmt.Fprintf(out, "fallback_provider: %s\n", displayValue(cfg.Routing.Fallback.Provider))
	_, _ = fmt.Fprintf(out, "fallback_model: %s\n", displayValue(cfg.Routing.Fallback.Model))
	_, _ = fmt.Fprintf(out, "database_path: %s\n", cfg.DatabasePath())
	_, _ = fmt.Fprintf(out, "code_index_path: %s\n", cfg.CodeLanceDBPath())
	_, _ = fmt.Fprintf(out, "brain_vault_path: %s\n", brainVaultPath)
	_, _ = fmt.Fprintf(out, "embedding_base_url: %s\n", cfg.Embedding.BaseURL)
	_, _ = fmt.Fprintf(out, "brain_enabled: %t\n", cfg.Brain.Enabled)
	_, _ = fmt.Fprintf(out, "local_services_enabled: %t\n", cfg.LocalServices.Enabled)
	_, _ = fmt.Fprintf(out, "local_services_mode: %s\n", cfg.LocalServices.Mode)
	_, _ = fmt.Fprintf(out, "local_services_compose_file: %s\n", cfg.LocalServices.ComposeFile)
	_, _ = fmt.Fprintf(out, "local_services_project_dir: %s\n", cfg.LocalServices.ProjectDir)
	return nil
}
