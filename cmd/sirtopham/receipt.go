package main

import (
	"fmt"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/receipt"
	"github.com/spf13/cobra"
)

func newReceiptCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "receipt <chain-id> [step]", Short: "Show orchestrator or step receipt", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := appconfig.Load(*configPath)
		if err != nil {
			return err
		}
		rt, err := buildOrchestratorRuntime(cmd.Context(), cfg)
		if err != nil {
			return err
		}
		defer rt.Cleanup()
		path := receipt.OrchestratorPath(args[0])
		if len(args) == 2 {
			steps, err := rt.ChainStore.ListSteps(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			for _, step := range steps {
				if fmt.Sprintf("%d", step.SequenceNum) == args[1] {
					path = step.ReceiptPath
					break
				}
			}
		}
		content, err := rt.BrainBackend.ReadDocument(cmd.Context(), path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(cmd.OutOrStdout(), content)
		return nil
	}}
}
