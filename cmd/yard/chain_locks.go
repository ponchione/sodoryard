package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/operator"
)

func newYardChainLocksCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{Use: "locks", Short: "Inspect project orchestration locks", RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := openYardReadOnlyOperator(cmd.Context(), *configPath)
		if err != nil {
			return err
		}
		defer svc.Close()
		locks, err := svc.ListProjectLocks(cmd.Context())
		if err != nil {
			return err
		}
		renderYardProjectLocks(cmd.OutOrStdout(), locks)
		return nil
	}}
	cmd.AddCommand(newYardChainLocksForceReleaseCmd(configPath))
	return cmd
}

func newYardChainLocksForceReleaseCmd(configPath *string) *cobra.Command {
	var reason string
	cmd := &cobra.Command{Use: "force-release <lock-name>", Short: "Force release a stale project lock", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := openYardOperator(cmd.Context(), *configPath)
		if err != nil {
			return err
		}
		defer svc.Close()
		result, err := svc.ForceReleaseProjectLock(cmd.Context(), args[0], reason)
		if err != nil {
			return err
		}
		if !result.Released {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", result.Message)
			return nil
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "lock %s force released owner_chain=%s owner_step=%s role=%s\n", result.LockName, result.OwnerChainID, result.OwnerStepID, result.OwnerRole)
		return nil
	}}
	cmd.Flags().StringVar(&reason, "reason", "", "Operator reason recorded in the force-release audit event")
	return cmd
}

func renderYardProjectLocks(out io.Writer, locks []operator.ProjectLockView) {
	if len(locks) == 0 {
		_, _ = fmt.Fprintln(out, "locks=0")
		return
	}
	for _, lock := range locks {
		_, _ = fmt.Fprintf(out, "lock=%s owner_chain=%s owner_step=%s role=%s acquired=%s heartbeat=%s expires=%s stale=%t metadata=%s\n",
			lock.LockName,
			valueOrUnset(lock.OwnerChainID),
			valueOrUnset(lock.OwnerStepID),
			valueOrUnset(lock.OwnerRole),
			formatLockTime(lock.AcquiredAt),
			formatLockTime(lock.HeartbeatAt),
			formatLockTime(lock.ExpiresAt),
			lock.Stale,
			valueOrUnset(strings.TrimSpace(lock.MetadataJSON)),
		)
	}
}

func formatLockTime(value time.Time) string {
	if value.IsZero() {
		return "<unset>"
	}
	return value.UTC().Format(time.RFC3339)
}
