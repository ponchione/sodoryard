package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type ActiveExecution struct {
	ExecutionID     string
	OrchestratorPID int
}

type ActiveStepProcess struct {
	StepID    string
	ProcessID int
}

type TerminalChainClosure struct {
	Status    string
	EventType EventType
	Summary   *string
	Extra     map[string]any
}

var ErrChainAlreadyRunning = errors.New("chain already running")

const StatusWaitingApproval = "waiting_approval"

func NextControlStatus(currentStatus string, targetStatus string) (string, error) {
	switch targetStatus {
	case "paused":
		switch currentStatus {
		case "running", "pause_requested":
			return "pause_requested", nil
		case "paused":
			return "paused", nil
		default:
			return "", fmt.Errorf("chain is %s and cannot be paused", currentStatus)
		}
	case "cancelled":
		switch currentStatus {
		case "running", "pause_requested", "cancel_requested", StatusWaitingApproval:
			return "cancel_requested", nil
		case "paused", "cancelled":
			return "cancelled", nil
		default:
			return "", fmt.Errorf("chain is %s and cannot be cancelled", currentStatus)
		}
	case "running":
		switch currentStatus {
		case "paused", "running":
			return "running", nil
		default:
			return "", fmt.Errorf("chain is %s and cannot be resumed", currentStatus)
		}
	default:
		return "", fmt.Errorf("unsupported chain status transition to %s", targetStatus)
	}
}

func ResumeExecutionReady(currentStatus string) (bool, error) {
	switch currentStatus {
	case "paused":
		return true, nil
	case "running":
		return false, ErrChainAlreadyRunning
	case "pause_requested":
		return false, fmt.Errorf("chain is %s and cannot be resumed until paused", currentStatus)
	default:
		return false, fmt.Errorf("chain is %s and cannot be resumed", currentStatus)
	}
}

func FinalizeControlStatus(status string) (string, bool) {
	switch status {
	case "pause_requested":
		return "paused", true
	case "cancel_requested":
		return "cancelled", true
	default:
		return "", false
	}
}

func FinalizeControlEventType(status string) (EventType, bool) {
	switch status {
	case "pause_requested":
		return EventChainPaused, true
	case "cancel_requested":
		return EventChainCancelled, true
	default:
		return "", false
	}
}

func TerminalEventTypeForStatus(status string) (EventType, bool) {
	switch status {
	case "paused":
		return EventChainPaused, true
	case "cancelled":
		return EventChainCancelled, true
	case "completed", "partial", "failed":
		return EventChainCompleted, true
	default:
		return "", false
	}
}

func BuildTerminalEventPayload(events []Event, status string, extra map[string]any) map[string]any {
	payload := make(map[string]any, len(extra)+2)
	for key, value := range extra {
		if key == "status" || key == "execution_id" {
			continue
		}
		payload[key] = value
	}
	payload["status"] = status
	if activeExec, ok := LatestActiveExecution(events); ok && activeExec.ExecutionID != "" {
		payload["execution_id"] = activeExec.ExecutionID
	}
	return payload
}

func ApplyTerminalChainClosure(ctx context.Context, store *Store, chainID string, closure TerminalChainClosure) error {
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		return err
	}
	if closure.Summary != nil {
		if err := store.CompleteChain(ctx, chainID, closure.Status, *closure.Summary); err != nil {
			return err
		}
	} else {
		if err := store.SetChainStatus(ctx, chainID, closure.Status); err != nil {
			return err
		}
	}
	payload := BuildTerminalEventPayload(events, closure.Status, closure.Extra)
	if err := store.LogEvent(ctx, chainID, "", closure.EventType, payload); err != nil {
		return err
	}
	return nil
}

func CloseTerminalizedActiveExecution(ctx context.Context, store *Store, chainID string, status string, extra map[string]any) error {
	eventType, ok := TerminalEventTypeForStatus(status)
	if !ok {
		return nil
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		return err
	}
	if _, ok := LatestActiveExecution(events); !ok {
		return nil
	}
	return ApplyTerminalChainClosure(ctx, store, chainID, TerminalChainClosure{Status: status, EventType: eventType, Extra: extra})
}

func LatestActiveExecution(events []Event) (ActiveExecution, bool) {
	terminated := make(map[string]struct{})
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		switch event.EventType {
		case EventChainPaused, EventChainCancelled, EventChainCompleted:
			var payload struct {
				ExecutionID string `json:"execution_id"`
			}
			if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil {
				continue
			}
			if payload.ExecutionID != "" {
				terminated[payload.ExecutionID] = struct{}{}
			}
		case EventChainStarted, EventChainResumed:
			var payload struct {
				OrchestratorPID int    `json:"orchestrator_pid"`
				ExecutionID     string `json:"execution_id"`
				ActiveExecution *bool  `json:"active_execution"`
			}
			if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil {
				continue
			}
			if payload.OrchestratorPID <= 0 && payload.ExecutionID == "" {
				continue
			}
			if payload.ActiveExecution != nil && !*payload.ActiveExecution {
				continue
			}
			if payload.ExecutionID != "" {
				if _, done := terminated[payload.ExecutionID]; done {
					continue
				}
			}
			return ActiveExecution{ExecutionID: payload.ExecutionID, OrchestratorPID: payload.OrchestratorPID}, true
		}
	}
	return ActiveExecution{}, false
}

func LatestActiveStepProcess(events []Event) (ActiveStepProcess, bool) {
	terminatedSteps := make(map[string]struct{})
	terminatedPIDs := make(map[int]struct{})
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		switch event.EventType {
		case EventStepCompleted, EventStepFailed:
			if event.StepID != "" {
				terminatedSteps[event.StepID] = struct{}{}
			}
		case EventStepProcessExited:
			if event.StepID != "" {
				terminatedSteps[event.StepID] = struct{}{}
			}
			var payload struct {
				ProcessID int `json:"process_id"`
			}
			if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil {
				continue
			}
			if payload.ProcessID > 0 {
				terminatedPIDs[payload.ProcessID] = struct{}{}
			}
		case EventStepProcessStarted:
			var payload struct {
				ProcessID     int   `json:"process_id"`
				ActiveProcess *bool `json:"active_process"`
			}
			if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil {
				continue
			}
			if payload.ProcessID <= 0 {
				continue
			}
			if payload.ActiveProcess != nil && !*payload.ActiveProcess {
				continue
			}
			if event.StepID != "" {
				if _, done := terminatedSteps[event.StepID]; done {
					continue
				}
			}
			if _, done := terminatedPIDs[payload.ProcessID]; done {
				continue
			}
			return ActiveStepProcess{StepID: event.StepID, ProcessID: payload.ProcessID}, true
		}
	}
	return ActiveStepProcess{}, false
}

func ShouldStopScheduling(status string) bool {
	switch status {
	case "paused", "cancelled", "pause_requested", "cancel_requested", StatusWaitingApproval:
		return true
	default:
		return false
	}
}
