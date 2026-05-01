package operator

import (
	"context"
	"errors"
	"fmt"

	"github.com/ponchione/sodoryard/internal/chain"
)

func (s *Service) PauseChain(ctx context.Context, chainID string) (ControlResult, error) {
	return s.setChainStatus(ctx, chainID, "paused", chain.EventChainPaused, "paused")
}

func (s *Service) ResumeChain(ctx context.Context, chainID string) (ControlResult, error) {
	return s.setChainStatus(ctx, chainID, "running", chain.EventChainResumed, "resumed")
}

func (s *Service) CancelChain(ctx context.Context, chainID string) (ControlResult, error) {
	return s.setChainStatus(ctx, chainID, "cancelled", chain.EventChainCancelled, "cancelled")
}

func (s *Service) setChainStatus(ctx context.Context, chainID string, targetStatus string, eventType chain.EventType, fallbackMessage string) (ControlResult, error) {
	store, err := s.store()
	if err != nil {
		return ControlResult{}, err
	}
	existing, err := store.GetChain(ctx, chainID)
	if err != nil {
		return ControlResult{}, err
	}
	nextStatus, err := chain.NextControlStatus(existing.Status, targetStatus)
	if err != nil {
		return ControlResult{}, fmt.Errorf("chain %s %w", chainID, err)
	}
	result := ControlResult{
		ChainID:        chainID,
		PreviousStatus: existing.Status,
		TargetStatus:   targetStatus,
		Status:         nextStatus,
		EventType:      eventType,
		Message:        controlStatusMessage(targetStatus, nextStatus, fallbackMessage),
	}
	if existing.Status == nextStatus {
		result.Already = true
		result.Message = "already " + fallbackMessage
		return result, nil
	}
	if nextStatus == "cancel_requested" {
		pids, signalErr := s.signalActiveChainProcesses(ctx, store, chainID)
		result.SignaledPIDs = pids
		if signalErr != nil {
			result.Warnings = append(result.Warnings, warningf("signal active chain process: %v", signalErr))
		}
	}
	if err := store.SetChainStatus(ctx, chainID, nextStatus); err != nil {
		return ControlResult{}, err
	}
	if err := store.LogEvent(ctx, chainID, "", eventType, map[string]any{"status": nextStatus}); err != nil {
		return ControlResult{}, err
	}
	return result, nil
}

func (s *Service) signalActiveChainProcesses(ctx context.Context, store *chain.Store, chainID string) ([]int, error) {
	if s == nil || s.processSignaler == nil {
		return nil, nil
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		return nil, err
	}
	signaled := make([]int, 0, 2)
	var firstErr error
	if stepProcess, ok := chain.LatestActiveStepProcess(events); ok && stepProcess.ProcessID > 0 {
		signaled = append(signaled, stepProcess.ProcessID)
		if err := s.processSignaler(stepProcess.ProcessID); err != nil && !errors.Is(err, ErrProcessNotRunning) {
			firstErr = err
		}
	}
	if exec, ok := chain.LatestActiveExecution(events); ok && exec.OrchestratorPID > 0 {
		signaled = append(signaled, exec.OrchestratorPID)
		if err := s.processSignaler(exec.OrchestratorPID); err != nil && !errors.Is(err, ErrProcessNotRunning) && firstErr == nil {
			firstErr = err
		}
	}
	return signaled, firstErr
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
