package provider

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type providerHookStub struct {
	name      string
	events    *[]string
	beforeErr error
	afterErr  error
}

func (h providerHookStub) BeforeProviderCall(ctx context.Context, call ProviderCall) (context.Context, error) {
	*h.events = append(*h.events, "before:"+h.name+":"+call.Operation)
	if h.beforeErr != nil {
		return ctx, h.beforeErr
	}
	return ctx, nil
}

func (h providerHookStub) AfterProviderCall(ctx context.Context, call ProviderCall, _ Usage, _ error) error {
	*h.events = append(*h.events, "after:"+h.name+":"+call.Provider)
	return h.afterErr
}

func TestProviderHooksRunInAroundOrder(t *testing.T) {
	var events []string
	hooks := []ProviderHook{
		providerHookStub{name: "a", events: &events},
		providerHookStub{name: "b", events: &events},
	}
	call := ProviderCall{Provider: "anthropic", Operation: "complete"}
	ctx, ran, err := RunProviderBeforeHooks(context.Background(), hooks, call)
	if err != nil {
		t.Fatalf("RunProviderBeforeHooks returned error: %v", err)
	}
	if ran != 2 {
		t.Fatalf("ran = %d, want 2", ran)
	}
	if err := RunProviderAfterHooks(ctx, hooks[:ran], call, Usage{InputTokens: 1}, nil); err != nil {
		t.Fatalf("RunProviderAfterHooks returned error: %v", err)
	}
	want := []string{"before:a:complete", "before:b:complete", "after:b:anthropic", "after:a:anthropic"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestProviderHooksReturnStartedCountOnBeforeError(t *testing.T) {
	var events []string
	blocked := errors.New("blocked")
	hooks := []ProviderHook{
		providerHookStub{name: "a", events: &events},
		providerHookStub{name: "b", events: &events, beforeErr: blocked},
	}
	call := ProviderCall{Provider: "anthropic", Operation: "stream"}
	ctx, ran, err := RunProviderBeforeHooks(context.Background(), hooks, call)
	if !errors.Is(err, blocked) {
		t.Fatalf("err = %v, want blocked", err)
	}
	if ran != 1 {
		t.Fatalf("ran = %d, want 1", ran)
	}
	_ = RunProviderAfterHooks(ctx, hooks[:ran], call, Usage{}, err)
	want := []string{"before:a:stream", "before:b:stream", "after:a:anthropic"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}
