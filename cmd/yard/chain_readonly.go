package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
)

func newYardChainStatusCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "status [chain-id]", Short: "Show chain status", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := appconfig.Load(*configPath)
		if err != nil {
			return err
		}
		rt, err := rtpkg.BuildOrchestratorRuntime(cmd.Context(), cfg)
		if err != nil {
			return err
		}
		defer rt.Cleanup()
		if len(args) == 0 {
			chains, err := rt.ChainStore.ListChains(cmd.Context(), 20)
			if err != nil {
				return err
			}
			for _, ch := range chains {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\tsteps=%d\ttokens=%d\n", ch.ID, ch.Status, ch.TotalSteps, ch.TotalTokens)
			}
			return nil
		}
		ch, err := rt.ChainStore.GetChain(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		steps, err := rt.ChainStore.ListSteps(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain=%s status=%s steps=%d tokens=%d duration=%d summary=%s\n", ch.ID, ch.Status, ch.TotalSteps, ch.TotalTokens, ch.TotalDurationSecs, ch.Summary)
		for _, step := range steps {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "step=%d role=%s status=%s verdict=%s receipt=%s\n", step.SequenceNum, step.Role, step.Status, step.Verdict, step.ReceiptPath)
		}
		return nil
	}}
}

func newYardChainLogsCmd(configPath *string) *cobra.Command {
	var follow bool
	var verbosity string
	cmd := &cobra.Command{Use: "logs <chain-id>", Short: "Show chain event log", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := appconfig.Load(*configPath)
		if err != nil {
			return err
		}
		rt, err := rtpkg.BuildOrchestratorRuntime(cmd.Context(), cfg)
		if err != nil {
			return err
		}
		defer rt.Cleanup()
		if !follow {
			events, err := rt.ChainStore.ListEvents(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			renderYardChainEvents(cmd.OutOrStdout(), events, chainRenderOptions{Verbosity: normalizeChainVerbosity(verbosity)})
			return nil
		}
		return yardFollowChainEvents(cmd.Context(), cmd.OutOrStdout(), rt.ChainStore, args[0], 0, chainRenderOptions{Verbosity: normalizeChainVerbosity(verbosity)})
	}}
	cmd.Flags().BoolVar(&follow, "follow", false, "Poll and print new events until the chain stops")
	cmd.Flags().StringVar(&verbosity, "verbosity", chainVerbosityNormal, "Chain log verbosity: normal or debug")
	return cmd
}

func newYardChainReceiptCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "receipt <chain-id> [step]", Short: "Show orchestrator or step receipt", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := appconfig.Load(*configPath)
		if err != nil {
			return err
		}
		rt, err := rtpkg.BuildOrchestratorRuntime(cmd.Context(), cfg)
		if err != nil {
			return err
		}
		defer rt.Cleanup()
		path := fmt.Sprintf("receipts/orchestrator/%s.md", args[0])
		if len(args) == 2 {
			steps, err := rt.ChainStore.ListSteps(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if stepPath, ok := yardReceiptPathForStep(args[0], args[1], steps); ok {
				path = stepPath
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

func yardReceiptPathForStep(chainID string, step string, steps []chain.Step) (string, bool) {
	for _, candidate := range steps {
		if fmt.Sprintf("%d", candidate.SequenceNum) == step {
			return candidate.ReceiptPath, true
		}
	}
	return fmt.Sprintf("receipts/orchestrator/%s.md", chainID), false
}
