package trace

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/id"
)

const (
	KindProvider    = "provider"
	KindToolBatch   = "tool_batch"
	KindTool        = "tool"
	KindChain       = "chain"
	KindContext     = "context"
	KindCompression = "compression"
	KindReceipt     = "receipt"
	KindReindex     = "reindex"

	StatusRunning   = "running"
	StatusOK        = "ok"
	StatusError     = "error"
	StatusCancelled = "cancelled"
)

const (
	EnvTraceID      = "YARD_TRACE_ID"
	EnvParentSpanID = "YARD_TRACE_PARENT_SPAN_ID"
	EnvChainID      = "YARD_TRACE_CHAIN_ID"
	EnvStepID       = "YARD_TRACE_STEP_ID"
)

type Span struct {
	ID             string
	TraceID        string
	ParentID       string
	ConversationID string
	ChainID        string
	StepID         string
	TurnNumber     int
	Iteration      int
	Name           string
	Kind           string
	Status         string
	StartedAt      time.Time
	EndedAt        time.Time
	DurationMs     int64
	Attributes     map[string]any
	Error          string
}

type SpanStart struct {
	ID             string
	TraceID        string
	ParentID       string
	ConversationID string
	ChainID        string
	StepID         string
	TurnNumber     int
	Iteration      int
	Name           string
	Kind           string
	StartedAt      time.Time
	Attributes     map[string]any
}

type SpanEnd struct {
	ID         string
	Status     string
	EndedAt    time.Time
	DurationMs int64
	Error      string
}

type Query struct {
	TraceID        string
	ConversationID string
	ChainID        string
	Limit          int
}

type Recorder interface {
	StartSpan(context.Context, Span) error
	EndSpan(context.Context, SpanEnd) error
	ListSpans(context.Context, Query) ([]Span, error)
}

type NoopRecorder struct{}

func (NoopRecorder) StartSpan(context.Context, Span) error            { return nil }
func (NoopRecorder) EndSpan(context.Context, SpanEnd) error           { return nil }
func (NoopRecorder) ListSpans(context.Context, Query) ([]Span, error) { return nil, nil }

type contextKey struct{}

type contextState struct {
	traceID        string
	parentSpanID   string
	currentSpanID  string
	conversationID string
	chainID        string
	stepID         string
	turnNumber     int
	iteration      int
}

type Scope struct {
	TraceID        string
	ParentSpanID   string
	ConversationID string
	ChainID        string
	StepID         string
	TurnNumber     int
	Iteration      int
}

type ActiveSpan struct {
	recorder Recorder
	span     Span
	started  bool
}

func ContextWithScope(ctx context.Context, scope Scope) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	state := stateFromContext(ctx)
	if strings.TrimSpace(scope.TraceID) != "" {
		state.traceID = strings.TrimSpace(scope.TraceID)
	}
	if strings.TrimSpace(scope.ParentSpanID) != "" {
		state.parentSpanID = strings.TrimSpace(scope.ParentSpanID)
	}
	if strings.TrimSpace(scope.ConversationID) != "" {
		state.conversationID = strings.TrimSpace(scope.ConversationID)
	}
	if strings.TrimSpace(scope.ChainID) != "" {
		state.chainID = strings.TrimSpace(scope.ChainID)
	}
	if strings.TrimSpace(scope.StepID) != "" {
		state.stepID = strings.TrimSpace(scope.StepID)
	}
	if scope.TurnNumber > 0 {
		state.turnNumber = scope.TurnNumber
	}
	if scope.Iteration > 0 {
		state.iteration = scope.Iteration
	}
	return context.WithValue(ctx, contextKey{}, state)
}

func ContextFromEnv(ctx context.Context) context.Context {
	return ContextWithScope(ctx, Scope{
		TraceID:      os.Getenv(EnvTraceID),
		ParentSpanID: os.Getenv(EnvParentSpanID),
		ChainID:      os.Getenv(EnvChainID),
		StepID:       os.Getenv(EnvStepID),
	})
}

func EnvForChild(span *ActiveSpan, chainID, stepID string) []string {
	if span == nil || !span.started {
		return nil
	}
	return []string{
		EnvTraceID + "=" + span.span.TraceID,
		EnvParentSpanID + "=" + span.span.ID,
		EnvChainID + "=" + strings.TrimSpace(chainID),
		EnvStepID + "=" + strings.TrimSpace(stepID),
	}
}

func StartSpan(ctx context.Context, recorder Recorder, start SpanStart) (context.Context, *ActiveSpan) {
	if ctx == nil {
		ctx = context.Background()
	}
	active := &ActiveSpan{}
	if recorder == nil || isNoopRecorder(recorder) {
		return ctx, active
	}
	state := stateFromContext(ctx)
	span := Span{
		ID:             firstNonEmpty(start.ID, id.New()),
		TraceID:        firstNonEmpty(start.TraceID, state.traceID, id.New()),
		ParentID:       firstNonEmpty(start.ParentID, state.currentSpanID, state.parentSpanID),
		ConversationID: firstNonEmpty(start.ConversationID, state.conversationID),
		ChainID:        firstNonEmpty(start.ChainID, state.chainID),
		StepID:         firstNonEmpty(start.StepID, state.stepID),
		TurnNumber:     firstPositive(start.TurnNumber, state.turnNumber),
		Iteration:      firstPositive(start.Iteration, state.iteration),
		Name:           strings.TrimSpace(start.Name),
		Kind:           strings.TrimSpace(start.Kind),
		Status:         StatusRunning,
		StartedAt:      start.StartedAt,
		Attributes:     cloneAttributes(start.Attributes),
	}
	if span.StartedAt.IsZero() {
		span.StartedAt = time.Now().UTC()
	}
	if span.Name == "" {
		span.Name = "span"
	}
	if span.Kind == "" {
		span.Kind = "runtime"
	}
	if err := recorder.StartSpan(ctx, span); err != nil {
		return ctx, active
	}
	next := state
	next.traceID = span.TraceID
	next.currentSpanID = span.ID
	next.parentSpanID = ""
	next.conversationID = span.ConversationID
	next.chainID = span.ChainID
	next.stepID = span.StepID
	next.turnNumber = span.TurnNumber
	next.iteration = span.Iteration
	active.recorder = recorder
	active.span = span
	active.started = true
	return context.WithValue(ctx, contextKey{}, next), active
}

func (s *ActiveSpan) ID() string {
	if s == nil || !s.started {
		return ""
	}
	return s.span.ID
}

func (s *ActiveSpan) TraceID() string {
	if s == nil || !s.started {
		return ""
	}
	return s.span.TraceID
}

func (s *ActiveSpan) End(ctx context.Context, status string, err error) {
	if s == nil || !s.started || s.recorder == nil {
		return
	}
	endedAt := time.Now().UTC()
	if status == "" {
		status = StatusOK
	}
	errText := ""
	if err != nil {
		errText = err.Error()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = StatusCancelled
		} else if status == StatusOK {
			status = StatusError
		}
	}
	duration := endedAt.Sub(s.span.StartedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	_ = s.recorder.EndSpan(ctx, SpanEnd{
		ID:         s.span.ID,
		Status:     status,
		EndedAt:    endedAt,
		DurationMs: duration,
		Error:      errText,
	})
	s.started = false
}

func StatusForError(err error) string {
	if err == nil {
		return StatusOK
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return StatusCancelled
	}
	return StatusError
}

func stateFromContext(ctx context.Context) contextState {
	if ctx == nil {
		return contextState{}
	}
	state, ok := ctx.Value(contextKey{}).(contextState)
	if !ok {
		return contextState{}
	}
	return state
}

func isNoopRecorder(recorder Recorder) bool {
	switch recorder.(type) {
	case NoopRecorder, *NoopRecorder:
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func cloneAttributes(attrs map[string]any) map[string]any {
	if len(attrs) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(attrs))
	for key, value := range attrs {
		if strings.TrimSpace(key) == "" || value == nil {
			continue
		}
		out[key] = value
	}
	return out
}
