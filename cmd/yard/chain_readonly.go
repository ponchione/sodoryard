package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newYardChainStatusCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "status [chain-id]", Short: "Show chain status", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := openYardReadOnlyOperator(cmd.Context(), *configPath)
		if err != nil {
			return err
		}
		defer svc.Close()
		if len(args) == 0 {
			chains, err := svc.ListChains(cmd.Context(), 20)
			if err != nil {
				return err
			}
			for _, ch := range chains {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\tsteps=%d\ttokens=%d\n", ch.ID, ch.Status, ch.TotalSteps, ch.TotalTokens)
			}
			return nil
		}
		detail, err := svc.GetChainDetail(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		ch := detail.Chain
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain=%s status=%s steps=%d tokens=%d duration=%d summary=%s\n", ch.ID, ch.Status, ch.TotalSteps, ch.TotalTokens, ch.TotalDurationSecs, ch.Summary)
		for _, step := range detail.Steps {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "step=%d role=%s status=%s verdict=%s receipt=%s\n", step.SequenceNum, step.Role, step.Status, step.Verdict, step.ReceiptPath)
		}
		return nil
	}}
}

func newYardChainLogsCmd(configPath *string) *cobra.Command {
	var follow bool
	var verbosity string
	cmd := &cobra.Command{Use: "logs <chain-id>", Short: "Show chain event log", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := openYardReadOnlyOperator(cmd.Context(), *configPath)
		if err != nil {
			return err
		}
		defer svc.Close()
		if !follow {
			events, err := svc.ListEvents(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			renderYardChainEvents(cmd.OutOrStdout(), events, chainRenderOptions{Verbosity: normalizeChainVerbosity(verbosity)})
			return nil
		}
		return yardFollowOperatorChainEvents(cmd.Context(), cmd.OutOrStdout(), svc, args[0], 0, chainRenderOptions{Verbosity: normalizeChainVerbosity(verbosity)})
	}}
	cmd.Flags().BoolVar(&follow, "follow", false, "Poll and print new events until the chain stops")
	cmd.Flags().StringVar(&verbosity, "verbosity", chainVerbosityNormal, "Chain log verbosity: normal or debug")
	return cmd
}

func newYardChainReceiptCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "receipt <chain-id> [step]", Short: "Show orchestrator or step receipt", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := openYardReadOnlyOperator(cmd.Context(), *configPath)
		if err != nil {
			return err
		}
		defer svc.Close()
		step := ""
		if len(args) == 2 {
			step = args[1]
		}
		receipt, err := svc.ReadReceipt(cmd.Context(), args[0], step)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(cmd.OutOrStdout(), receipt.Content)
		return nil
	}}
}
