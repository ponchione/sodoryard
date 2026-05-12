package provider

import (
	"context"
	"errors"
)

// ProviderCall describes one routed provider invocation as seen by hooks.
type ProviderCall struct {
	Provider  string
	Operation string
	Request   *Request
}

// ProviderHook observes or gates provider calls. Before hooks run in
// registration order; after hooks run in reverse order around the provider call.
type ProviderHook interface {
	BeforeProviderCall(context.Context, ProviderCall) (context.Context, error)
	AfterProviderCall(context.Context, ProviderCall, Usage, error) error
}

// RunProviderBeforeHooks applies hooks in deterministic registration order and
// returns the number of hooks that successfully ran.
func RunProviderBeforeHooks(ctx context.Context, hooks []ProviderHook, call ProviderCall) (context.Context, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for i, hook := range hooks {
		if hook == nil {
			continue
		}
		next, err := hook.BeforeProviderCall(ctx, call)
		if err != nil {
			return ctx, i, err
		}
		if next != nil {
			ctx = next
		}
	}
	return ctx, len(hooks), nil
}

// RunProviderAfterHooks applies successfully-started hooks in reverse order.
// If multiple after hooks fail, their errors are joined.
func RunProviderAfterHooks(ctx context.Context, hooks []ProviderHook, call ProviderCall, usage Usage, callErr error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var errs []error
	effectiveErr := callErr
	for i := len(hooks) - 1; i >= 0; i-- {
		hook := hooks[i]
		if hook == nil {
			continue
		}
		if err := hook.AfterProviderCall(ctx, call, usage, effectiveErr); err != nil {
			errs = append(errs, err)
			effectiveErr = errors.Join(effectiveErr, err)
		}
	}
	return errors.Join(errs...)
}
