package main

import (
	"fmt"
	"os"

	appconfig "github.com/ponchione/sirtopham/internal/config"
	appdb "github.com/ponchione/sirtopham/internal/db"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	var configPath string

	rootCmd := &cobra.Command{
		Use:          "sirtopham",
		Short:        "A self-hosted AI coding agent",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(os.Stdout, "sirtopham %s\n", version)
			return nil
		},
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the sirtopham server",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not yet implemented")
			return nil
		},
	}
	serveCmd.Flags().Bool("dev", false, "Enable development mode")

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the sirtopham database schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := appconfig.Load(configPath)
			if err != nil {
				return err
			}

			database, err := appdb.OpenDB(cmd.Context(), cfg.DatabasePath())
			if err != nil {
				return err
			}
			defer database.Close()

			if err := appdb.Init(cmd.Context(), database); err != nil {
				return err
			}

			fmt.Fprintf(os.Stdout, "initialized database at %s\n", cfg.DatabasePath())
			return nil
		},
	}
	initCmd.Flags().StringVar(&configPath, "config", "sirtopham.yaml", "Path to sirtopham config file")

	indexCmd := &cobra.Command{
		Use:   "index",
		Short: "Index the codebase for RAG search",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not yet implemented")
			return nil
		},
	}

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Show or validate configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not yet implemented")
			return nil
		},
	}

	rootCmd.AddCommand(serveCmd, initCmd, indexCmd, configCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
