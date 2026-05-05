package chain

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	appdb "github.com/ponchione/sodoryard/internal/db"
)

type EventType string

const (
	EventChainStarted       EventType = "chain_started"
	EventStepStarted        EventType = "step_started"
	EventStepProcessStarted EventType = "step_process_started"
	EventStepProcessExited  EventType = "step_process_exited"
	EventStepOutput         EventType = "step_output"
	EventStepCompleted      EventType = "step_completed"
	EventStepFailed         EventType = "step_failed"
	EventReindexStarted     EventType = "reindex_started"
	EventReindexCompleted   EventType = "reindex_completed"
	EventResolverLoop       EventType = "resolver_loop"
	EventSafetyLimitHit     EventType = "safety_limit_hit"
	EventChainPaused        EventType = "chain_paused"
	EventChainResumed       EventType = "chain_resumed"
	EventChainCompleted     EventType = "chain_completed"
	EventChainCancelled     EventType = "chain_cancelled"
)

func (s *Store) LogEvent(ctx context.Context, chainID string, stepID string, eventType EventType, eventData any) error {
	if s != nil && s.memory != nil {
		return s.memory.LogEvent(ctx, chainID, stepID, eventType, eventData)
	}
	var payload sql.NullString
	if eventData != nil {
		b, err := json.Marshal(eventData)
		if err != nil {
			return fmt.Errorf("log event: marshal payload: %w", err)
		}
		payload = sql.NullString{String: string(b), Valid: true}
	}
	if err := s.q.CreateEvent(ctx, appdb.CreateEventParams{ChainID: chainID, StepID: nullableString(stepID), EventType: string(eventType), EventData: payload}); err != nil {
		return fmt.Errorf("log event: %w", err)
	}
	return nil
}

func (s *Store) ListEventsSince(ctx context.Context, chainID string, afterID int64) ([]Event, error) {
	if s != nil && s.memory != nil {
		return s.memory.ListEventsSince(ctx, chainID, afterID)
	}
	rows, err := s.q.ListEventsByChainSince(ctx, appdb.ListEventsByChainSinceParams{ChainID: chainID, ID: afterID})
	if err != nil {
		return nil, fmt.Errorf("list events since: %w", err)
	}
	events := make([]Event, 0, len(rows))
	for _, row := range rows {
		mapped, err := mapEvent(row)
		if err != nil {
			return nil, fmt.Errorf("list events since: %w", err)
		}
		events = append(events, mapped)
	}
	return events, nil
}
