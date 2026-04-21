package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	appconfig "github.com/ponchione/sodoryard/internal/config"
)

const defaultCLIConfigPath = appconfig.ConfigFilename

var version = "dev"

func newRootCmd() *cobra.Command {
	var configPath string

	rootCmd := &cobra.Command{
		Use:          "tidmouth",
		Short:        "Internal engine binary used by yard orchestration",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(os.Stdout, "tidmouth %s\n", version)
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVar(&configPath, "config", defaultCLIConfigPath, "Path to config file")

	indexCmd := newIndexCmd(&configPath)
	runCmd := newRunCmd(&configPath)

	rootCmd.AddCommand(runCmd, indexCmd)
	return rootCmd
}

func main() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		if coded, ok := err.(interface{ ExitCode() int }); ok {
			os.Exit(coded.ExitCode())
		}
		os.Exit(1)
	}
}
