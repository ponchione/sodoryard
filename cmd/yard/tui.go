package main

import (
	"fmt"

	"github.com/spf13/cobra"

	tuiapp "github.com/ponchione/sodoryard/internal/tui"
)

func newYardTUICmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the terminal operator console",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openYardReadOnlyOperator(cmd.Context(), *configPath)
			if err != nil {
				return fmt.Errorf("open operator: %w", err)
			}
			defer svc.Close()
			return tuiapp.Run(cmd.Context(), svc, tuiapp.Options{})
		},
	}
}
