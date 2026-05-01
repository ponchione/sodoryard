package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/operator"
	tuiapp "github.com/ponchione/sodoryard/internal/tui"
)

var runYardTUI = func(ctx context.Context, svc tuiapp.Operator, opts tuiapp.Options) error {
	return tuiapp.Run(ctx, svc, opts)
}

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
			return runYardTUI(cmd.Context(), svc, tuiapp.Options{})
		},
	}
}

var _ tuiapp.Operator = (*operator.Service)(nil)
