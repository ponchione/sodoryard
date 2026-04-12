// Command yard is the operator-facing CLI for railway project bootstrap
// and (in future phases) other top-level operator workflows. Today its
// only subcommand is `init`.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "yard",
		Short:        "Yard — railway project operator CLI",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "yard %s\n", version)
			return nil
		},
	}
	rootCmd.AddCommand(newInitCmd())
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
