// Command yard is the unified operator-facing CLI for railway projects.
// It consolidates operator commands under a single binary with a single --help.
package main

import (
	"os"

	"github.com/spf13/cobra"

	appconfig "github.com/ponchione/sodoryard/internal/config"
)

var version = "dev"

func newRootCmd() *cobra.Command {
	var configPath string

	rootCmd := &cobra.Command{
		Use:          "yard",
		Short:        "Yard — terminal operator console and project CLI",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runYardTUICommand(cmd, configPath)
		},
	}

	rootCmd.PersistentFlags().StringVar(&configPath, "config", appconfig.ConfigFilename, "Path to yard.yaml config file")

	rootCmd.AddCommand(
		newInitCmd(),
		newYardServeCmd(&configPath),
		newYardIndexCmd(&configPath),
		newYardAuthCmd(&configPath),
		newYardDoctorCmd(&configPath),
		newYardConfigCmd(&configPath),
		newYardLLMCmd(&configPath),
		newYardBrainCmd(&configPath),
		newYardMemoryCmd(&configPath),
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
