package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/spf13/cobra"
)

func newCancelCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "cancel <chain-id>", Short: "Cancel a chain", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return setChainStatus(cmd, *configPath, args[0], "cancelled", chain.EventChainCancelled, "cancelled")
	}}
}

func setChainStatus(cmd *cobra.Command, configPath string, chainID string, status string, eventType chain.EventType, message string) error {
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return err
	}
	rt, err := buildOrchestratorRuntime(cmd.Context(), cfg)
	if err != nil {
		return err
	}
	defer rt.Cleanup()

	existing, err := rt.ChainStore.GetChain(cmd.Context(), chainID)
	if err != nil {
		return err
	}
	if err := validateChainStatusTransition(existing.Status, status, chainID); err != nil {
		return err
	}
	nextStatus, err := chain.NextControlStatus(existing.Status, status)
	if err != nil {
		return fmt.Errorf("chain %s %w", chainID, err)
	}
	if existing.Status == nextStatus {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain %s already %s\n", chainID, message)
		return nil
	}
	if nextStatus == "cancel_requested" {
		_ = signalActiveChainProcess(cmd.Context(), rt.ChainStore, chainID)
	}
	if err := rt.ChainStore.SetChainStatus(cmd.Context(), chainID, nextStatus); err != nil {
		return err
	}
	_ = rt.ChainStore.LogEvent(cmd.Context(), chainID, "", eventType, map[string]any{"status": nextStatus})
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "chain %s %s\n", chainID, controlStatusMessage(status, nextStatus, message))
	return nil
}

func controlStatusMessage(targetStatus string, persistedStatus string, fallback string) string {
	switch {
	case targetStatus == "paused" && persistedStatus == "pause_requested":
		return "pause requested"
	case targetStatus == "cancelled" && persistedStatus == "cancel_requested":
		return "cancel requested"
	default:
		return fallback
	}
}

func signalActiveChainProcess(ctx context.Context, store *chain.Store, chainID string) error {
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		return err
	}
	pid := 0
	for i := len(events) - 1; i >= 0; i-- {
		var payload struct {
			OrchestratorPID int `json:"orchestrator_pid"`
		}
		if err := json.Unmarshal([]byte(events[i].EventData), &payload); err != nil {
			continue
		}
		if payload.OrchestratorPID > 0 {
			pid = payload.OrchestratorPID
			break
		}
	}
	if pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		return err
	}
	return nil
}

func validateChainStatusTransition(currentStatus string, targetStatus string, chainID string) error {
	_, err := chain.NextControlStatus(currentStatus, targetStatus)
	if err != nil {
		return fmt.Errorf("chain %s %w", chainID, err)
	}
	return nil
}
