package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/cmdutil"
	yardeval "github.com/ponchione/sodoryard/internal/eval"
)

func newYardChainEvalCmd(configPath *string) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "eval <chain-id>",
		Short: "Evaluate a stored chain against deterministic flow invariants",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openYardReadOnlyOperator(cmd.Context(), *configPath)
			if err != nil {
				return err
			}
			defer svc.Close()

			detail, err := svc.GetChainDetail(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			report := yardeval.EvaluateChainFlow(detail.Chain, detail.Steps, detail.RecentEvents)
			if jsonOut {
				if err := cmdutil.WriteJSON(cmd.OutOrStdout(), report); err != nil {
					return err
				}
			} else {
				renderYardEvalReport(cmd.OutOrStdout(), report)
			}
			if report.Status != yardeval.StatusPass {
				return fmt.Errorf("chain eval %s failed", args[0])
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON output")
	return cmd
}
