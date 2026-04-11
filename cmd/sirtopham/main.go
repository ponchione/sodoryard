package main

import (
	"fmt"
	"os"

	appconfig "github.com/ponchione/sodoryard/internal/config"
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

	configCmd := newConfigCmd(&configPath)
	llmCmd := newLLMCmd(&configPath)

	authCmd := newAuthCmd(&configPath)
	doctorCmd := newDoctorCmd(&configPath)
	serveCmd := newServeCmd(&configPath)
	runCmd := newRunCmd(&configPath)
	brainServeCmd := newBrainServeCmd()

	rootCmd.AddCommand(serveCmd, runCmd, brainServeCmd, initCmd, indexCmd, configCmd, llmCmd, authCmd, doctorCmd)

	if err := rootCmd.Execute(); err != nil {
		if coded, ok := err.(interface{ ExitCode() int }); ok {
			os.Exit(coded.ExitCode())
		}
		os.Exit(1)
	}
}
