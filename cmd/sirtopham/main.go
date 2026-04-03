package main

import (
	"fmt"
	"os"

	appconfig "github.com/ponchione/sirtopham/internal/config"
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

	rootCmd.PersistentFlags().StringVar(&configPath, "config", appconfig.DefaultConfigFilename(""), "Path to config file")

	initCmd := newInitCmd(&configPath)
	indexCmd := newIndexCmd(&configPath)

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Show or validate configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not yet implemented")
			return nil
		},
	}

	authCmd := newAuthCmd(&configPath)
	doctorCmd := newDoctorCmd(&configPath)
	serveCmd := newServeCmd(&configPath)
	brainServeCmd := newBrainServeCmd()

	rootCmd.AddCommand(serveCmd, brainServeCmd, initCmd, indexCmd, configCmd, authCmd, doctorCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
