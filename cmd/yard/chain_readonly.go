package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/operator"
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

func newYardChainMetricsCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "metrics <chain-id>", Short: "Show chain dogfooding metrics", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := openYardReadOnlyOperator(cmd.Context(), *configPath)
		if err != nil {
			return err
		}
		defer svc.Close()
		report, err := svc.GetChainMetrics(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		renderYardChainMetrics(cmd.OutOrStdout(), report)
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

func renderYardChainMetrics(out io.Writer, report operator.ChainMetricsReport) {
	_, _ = fmt.Fprintf(out, "chain=%s health=%s status=%s\n", report.ChainID, report.Health, report.Status)
	_, _ = fmt.Fprintf(out, "steps recorded=%d rows=%d completed=%d running=%d pending=%d failed=%d budget=%d pct=%.1f\n", report.TotalSteps, report.StepRows, report.CompletedSteps, report.RunningSteps, report.PendingSteps, report.FailedSteps, report.MaxSteps, report.StepBudgetPct)
	_, _ = fmt.Fprintf(out, "tokens recorded=%d step_sum=%d budget=%d pct=%.1f\n", report.TotalTokens, report.StepTokenTotal, report.TokenBudget, report.TokenBudgetPct)
	_, _ = fmt.Fprintf(out, "turns step_sum=%d\n", report.StepTurnTotal)
	_, _ = fmt.Fprintf(out, "duration recorded=%ds step_sum=%ds budget=%ds pct=%.1f\n", report.TotalDurationSecs, report.StepDurationSecs, report.MaxDurationSecs, report.DurationBudgetPct)
	_, _ = fmt.Fprintf(out, "resolver_loops used=%d budget=%d pct=%.1f\n", report.ResolverLoops, report.MaxResolverLoops, report.ResolverLoopPct)
	_, _ = fmt.Fprintf(out, "events total=%d output=%d step_failed=%d safety_limit=%d reindex_started=%d reindex_done=%d process_started=%d process_exited=%d\n", report.EventTotal, report.OutputEvents, report.StepFailedEvents, report.SafetyLimitEvents, report.ReindexStartedEvents, report.ReindexDoneEvents, report.ProcessStartedEvents, report.ProcessExitedEvents)
	_, _ = fmt.Fprintf(out, "warnings=%d\n", len(report.Warnings))
	for _, warning := range report.Warnings {
		_, _ = fmt.Fprintf(out, "warning: %s\n", warning.Message)
	}
	for _, step := range report.Steps {
		_, _ = fmt.Fprintf(out, "step=%d role=%s status=%s verdict=%s tokens=%d turns=%d duration=%ds exit=%s receipt=%s", step.SequenceNum, valueOrUnset(step.Role), valueOrUnset(step.Status), valueOrUnset(step.Verdict), step.TokensUsed, step.TurnsUsed, step.DurationSecs, exitCodeLabel(step.ExitCode), valueOrUnset(step.ReceiptPath))
		if step.ErrorMessage != "" {
			_, _ = fmt.Fprintf(out, " error=%q", step.ErrorMessage)
		}
		_, _ = fmt.Fprintln(out)
	}
}

func valueOrUnset(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<unset>"
	}
	return value
}

func exitCodeLabel(exitCode *int) string {
	if exitCode == nil {
		return "<unset>"
	}
	return fmt.Sprintf("%d", *exitCode)
}
