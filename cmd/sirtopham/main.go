package main

import (
	"fmt"
	"os"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/spf13/cobra"
)

const defaultCLIConfigPath = appconfig.ConfigFilename

var version = "dev"

func newRootCmd() *cobra.Command {
	var configPath string
	rootCmd := &cobra.Command{
		Use:          "sirtopham",
		Short:        "SirTopham chain orchestrator",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "sirtopham %s\n", version)
			return nil
		},
	}
	rootCmd.PersistentFlags().StringVar(&configPath, "config", defaultCLIConfigPath, "Path to yard.yaml config file")
	rootCmd.AddCommand(
		newChainCmd(&configPath),
		newRunChainBackgroundCmd(&configPath),
		newStatusCmd(&configPath),
		newLogsCmd(&configPath),
		newReceiptCmd(&configPath),
		newCancelCmd(&configPath),
		newPauseCmd(&configPath),
		newResumeCmd(&configPath),
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
