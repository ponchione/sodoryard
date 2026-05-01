// Command yard is the unified operator-facing CLI for railway projects.
// It consolidates operator commands under a single binary with a single --help.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	appconfig "github.com/ponchione/sodoryard/internal/config"
)

var version = "dev"

func newRootCmd() *cobra.Command {
	var configPath string

	rootCmd := &cobra.Command{
		Use:          "yard",
		Short:        "Yard — railway project operator CLI",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "yard %s\n", version)
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVar(&configPath, "config", appconfig.ConfigFilename, "Path to yard.yaml config file")

	rootCmd.AddCommand(
		newInitCmd(),
		newYardServeCmd(&configPath),
		newYardRunCmd(&configPath),
		newYardIndexCmd(&configPath),
		newYardAuthCmd(&configPath),
		newYardDoctorCmd(&configPath),
		newYardConfigCmd(&configPath),
		newYardLLMCmd(&configPath),
		newYardBrainCmd(&configPath),
		newYardChainCmd(&configPath),
		newYardTUICmd(&configPath),
	)
	return rootCmd
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		if coded, ok := err.(interface{ ExitCode() int }); ok {
			os.Exit(coded.ExitCode())
		}
		os.Exit(1)
	}
}
