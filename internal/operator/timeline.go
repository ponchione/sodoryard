package operator

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/ponchione/sodoryard/internal/chain"
	tracepkg "github.com/ponchione/sodoryard/internal/trace"
)

func (s *Service) GetChainTimeline(ctx context.Context, chainID string) ([]ChainTimelineItem, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	events, err := store.ListEvents(ctx, chainID)
	if err != nil {
		return nil, err
	}
	return s.buildChainTimeline(ctx, chainID, events), nil
}

func (s *Service) buildChainTimeline(ctx context.Context, chainID string, events []chain.Event) []ChainTimelineItem {
	items := make([]ChainTimelineItem, 0, len(events))
	for _, event := range events {
		items = append(items, timelineItemFromEvent(event))
	}
	if s == nil || s.rt == nil || s.rt.TraceRecorder == nil {
		sortTimeline(items)
		return items
	}
	spans, err := s.rt.TraceRecorder.ListSpans(ctx, tracepkg.Query{ChainID: chainID, Limit: 1000})
	if err != nil {
		sortTimeline(items)
		return items
	}
	for _, span := range spans {
		items = append(items, timelineItemFromSpan(span))
	}
	sortTimeline(items)
	return items
}

func timelineItemFromEvent(event chain.Event) ChainTimelineItem {
	return ChainTimelineItem{
		ID:        fmt.Sprintf("event:%d", event.ID),
		Source:    "event",
		Kind:      "event",
		Name:      string(event.EventType),
		Status:    "",
		ChainID:   event.ChainID,
		StepID:    event.StepID,
		StartedAt: event.CreatedAt,
		EventType: string(event.EventType),
		EventData: event.EventData,
	}
}

func timelineItemFromSpan(span tracepkg.Span) ChainTimelineItem {
	var endedAt *time.Time
	if !span.EndedAt.IsZero() {
		endedAt = &span.EndedAt
	}
	return ChainTimelineItem{
		ID:             "span:" + span.ID,
		Source:         "span",
		Kind:           span.Kind,
		Name:           span.Name,
		Status:         span.Status,
		TraceID:        span.TraceID,
		SpanID:         span.ID,
		ParentSpanID:   span.ParentID,
		ConversationID: span.ConversationID,
		ChainID:        span.ChainID,
		StepID:         span.StepID,
		TurnNumber:     span.TurnNumber,
		Iteration:      span.Iteration,
		StartedAt:      span.StartedAt,
		EndedAt:        endedAt,
		DurationMs:     span.DurationMs,
		Attributes:     cloneAttributes(span.Attributes),
		Error:          span.Error,
	}
}

func sortTimeline(items []ChainTimelineItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].StartedAt.Equal(items[j].StartedAt) {
			return items[i].StartedAt.Before(items[j].StartedAt)
		}
		return items[i].ID < items[j].ID
	})
}

func cloneAttributes(attrs map[string]any) map[string]any {
	if len(attrs) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(attrs))
	for key, value := range attrs {
		out[key] = value
	}
	return out
}
