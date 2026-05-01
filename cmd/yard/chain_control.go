package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/operator"
)

func yardSetChainStatus(cmd *cobra.Command, configPath string, chainID string, status string) error {
	svc, err := openYardReadOnlyOperator(cmd.Context(), configPath)
	if err != nil {
		return err
	}
	defer svc.Close()

	var result operator.ControlResult
	switch status {
	case "paused":
		result, err = svc.PauseChain(cmd.Context(), chainID)
	case "cancelled":
		result, err = svc.CancelChain(cmd.Context(), chainID)
	default:
		err = fmt.Errorf("unsupported chain status transition to %s", status)
	}
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain %s %s\n", chainID, result.Message)
	return nil
}
