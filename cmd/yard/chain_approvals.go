package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/operator"
)

func newYardChainApprovalsCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "approvals <chain-id>", Short: "List pending and decided chain approvals", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := openYardReadOnlyOperator(cmd.Context(), *configPath)
		if err != nil {
			return err
		}
		defer svc.Close()
		approvals, err := svc.ListApprovals(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if len(approvals) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no approvals")
			return nil
		}
		for _, approval := range approvals {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), formatApproval(approval))
		}
		return nil
	}}
}

func newYardChainApproveCmd(configPath *string) *cobra.Command {
	var reason string
	cmd := &cobra.Command{Use: "approve <chain-id> <approval-id>", Short: "Approve a pending chain approval", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := openYardReadOnlyOperator(cmd.Context(), *configPath)
		if err != nil {
			return err
		}
		defer svc.Close()
		result, err := svc.ApproveChainApproval(cmd.Context(), args[0], args[1], reason)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", result.Message)
		return nil
	}}
	cmd.Flags().StringVar(&reason, "reason", "", "Optional approval note")
	return cmd
}

func newYardChainDenyCmd(configPath *string) *cobra.Command {
	var reason string
	cmd := &cobra.Command{Use: "deny <chain-id> <approval-id>", Short: "Deny a pending chain approval", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := openYardReadOnlyOperator(cmd.Context(), *configPath)
		if err != nil {
			return err
		}
		defer svc.Close()
		result, err := svc.DenyChainApproval(cmd.Context(), args[0], args[1], reason)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", result.Message)
		return nil
	}}
	cmd.Flags().StringVar(&reason, "reason", "", "Optional denial note")
	return cmd
}

func formatApproval(approval operator.ApprovalView) string {
	parts := []string{
		approval.ID,
		"status=" + valueOrUnset(approval.Status),
		"tool=" + valueOrUnset(approval.ToolName),
	}
	if approval.RiskLevel != "" {
		parts = append(parts, "risk="+approval.RiskLevel)
	}
	if approval.StepID != "" {
		parts = append(parts, "step="+approval.StepID)
	}
	if approval.Reason != "" {
		parts = append(parts, "reason="+quoteApprovalValue(approval.Reason))
	}
	if approval.DecisionReason != "" {
		parts = append(parts, "decision_reason="+quoteApprovalValue(approval.DecisionReason))
	}
	return strings.Join(parts, " ")
}

func quoteApprovalValue(value string) string {
	return fmt.Sprintf("%q", value)
}
