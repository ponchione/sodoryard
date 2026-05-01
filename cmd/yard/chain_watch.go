package main

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/operator"
)

type yardChainWatchHandle struct {
	cancel func()
	done   <-chan error
}

type yardChainWatchAdapter struct {
	handle *yardChainWatchHandle
}

func (a yardChainWatchAdapter) Wait(timeout time.Duration) error {
	if a.handle == nil {
		return nil
	}
	return a.handle.wait(timeout)
}

func startYardChainWatch(ctx context.Context, out io.Writer, store *chain.Store, chainID string, enabled bool, opts chainRenderOptions) *yardChainWatchHandle {
	if !enabled {
		return &yardChainWatchHandle{}
	}
	watchCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- yardFollowChainEvents(watchCtx, out, store, chainID, 0, opts)
	}()
	return &yardChainWatchHandle{cancel: cancel, done: done}
}

func (h *yardChainWatchHandle) wait(timeout time.Duration) error {
	if h == nil || h.done == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = yardChainWatchFlushTimeout
	}
	select {
	case err := <-h.done:
		if h.cancel != nil {
			h.cancel()
		}
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	case <-time.After(timeout):
		if h.cancel != nil {
			h.cancel()
		}
		select {
		case err := <-h.done:
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		case <-time.After(250 * time.Millisecond):
			return nil
		}
	}
}

func yardStreamChainEvents(ctx context.Context, out io.Writer, store *chain.Store, chainID string, afterID int64, opts chainRenderOptions) (int64, error) {
	events, err := store.ListEventsSince(ctx, chainID, afterID)
	if err != nil {
		return afterID, err
	}
	if lastID := renderYardChainEvents(out, events, opts); lastID != 0 {
		afterID = lastID
	}
	return afterID, nil
}

func yardFollowChainEvents(ctx context.Context, out io.Writer, store *chain.Store, chainID string, afterID int64, opts chainRenderOptions) error {
	for {
		nextAfterID, err := yardStreamChainEvents(ctx, out, store, chainID, afterID, opts)
		if err != nil {
			return err
		}
		afterID = nextAfterID
		ch, err := store.GetChain(ctx, chainID)
		if err != nil {
			return err
		}
		if !yardChainFollowStatusActive(ch.Status) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

func yardStreamOperatorChainEvents(ctx context.Context, out io.Writer, svc *operator.Service, chainID string, afterID int64, opts chainRenderOptions) (int64, error) {
	events, err := svc.ListEventsSince(ctx, chainID, afterID)
	if err != nil {
		return afterID, err
	}
	if lastID := renderYardChainEvents(out, events, opts); lastID != 0 {
		afterID = lastID
	}
	return afterID, nil
}

func yardFollowOperatorChainEvents(ctx context.Context, out io.Writer, svc *operator.Service, chainID string, afterID int64, opts chainRenderOptions) error {
	for {
		nextAfterID, err := yardStreamOperatorChainEvents(ctx, out, svc, chainID, afterID, opts)
		if err != nil {
			return err
		}
		afterID = nextAfterID
		detail, err := svc.GetChainDetail(ctx, chainID)
		if err != nil {
			return err
		}
		if !yardChainFollowStatusActive(detail.Chain.Status) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

func yardChainFollowStatusActive(status string) bool {
	return status == "running" || status == "pause_requested" || status == "cancel_requested"
}
