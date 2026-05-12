//go:build sqlite_fts5
// +build sqlite_fts5

package operator

import (
	"context"
	"testing"

	"github.com/ponchione/sodoryard/internal/chain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	tracepkg "github.com/ponchione/sodoryard/internal/trace"
)

func TestGetChainTimelineCombinesEventsAndSpans(t *testing.T) {
	ctx := context.Background()
	db := newOperatorTestDB(t)
	store := chain.NewStore(db)
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "timeline-chain", SourceTask: "timeline"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}
	stepID, err := store.StartStep(ctx, chain.StepSpec{ChainID: chainID, SequenceNum: 1, Role: "coder", Task: "code"})
	if err != nil {
		t.Fatalf("StartStep returned error: %v", err)
	}
	if err := store.LogEvent(ctx, chainID, stepID, chain.EventStepStarted, map[string]any{"role": "coder"}); err != nil {
		t.Fatalf("LogEvent returned error: %v", err)
	}
	recorder := tracepkg.NewSQLiteRecorder(db)
	spanCtx := tracepkg.ContextWithScope(ctx, tracepkg.Scope{ChainID: chainID, StepID: stepID})
	spanCtx, span := tracepkg.StartSpan(spanCtx, recorder, tracepkg.SpanStart{Name: "provider.stream", Kind: tracepkg.KindProvider})
	span.End(spanCtx, tracepkg.StatusForError(context.DeadlineExceeded), context.DeadlineExceeded)

	svc, err := NewForRuntime(&rtpkg.OrchestratorRuntime{
		Config:        &appconfig.Config{ProjectRoot: t.TempDir()},
		ChainStore:    store,
		TraceRecorder: recorder,
		Cleanup:       func() {},
	}, Options{})
	if err != nil {
		t.Fatalf("NewForRuntime returned error: %v", err)
	}
	t.Cleanup(svc.Close)

	timeline, err := svc.GetChainTimeline(ctx, chainID)
	if err != nil {
		t.Fatalf("GetChainTimeline returned error: %v", err)
	}
	if len(timeline) != 2 {
		t.Fatalf("timeline = %+v, want event and span", timeline)
	}
	var sawEvent, sawSpan bool
	for _, item := range timeline {
		if item.Source == "event" && item.EventType == string(chain.EventStepStarted) {
			sawEvent = true
		}
		if item.Source == "span" && item.Kind == tracepkg.KindProvider && item.Status == tracepkg.StatusCancelled && item.StepID == stepID {
			sawSpan = true
		}
	}
	if !sawEvent || !sawSpan {
		t.Fatalf("timeline = %+v, want event and provider span", timeline)
	}
}
