package main

import (
	"io"

	"github.com/ponchione/sodoryard/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newYardConfigCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or validate configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return yardRunConfig(cmd.OutOrStdout(), *configPath)
		},
	}
	return cmd
}

func yardRunConfig(out io.Writer, configPath string) error {
	return cmdutil.RunConfig(out, configPath)
}
