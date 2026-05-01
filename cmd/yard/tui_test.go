package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/operator"
	tuiapp "github.com/ponchione/sodoryard/internal/tui"
)

func TestRootCommandRegistersTUI(t *testing.T) {
	root := newRootCmd()
	cmd, _, err := root.Find([]string{"tui"})
	if err != nil {
		t.Fatalf("Find(tui) returned error: %v", err)
	}
	if cmd == nil || cmd.Use != "tui" {
		t.Fatalf("Find(tui) = %#v, want tui command", cmd)
	}
}

func TestRootCommandDoesNotRegisterPublicRun(t *testing.T) {
	root := newRootCmd()
	for _, cmd := range root.Commands() {
		if cmd.Name() == "run" {
			t.Fatal("root command still registers public run command")
		}
	}
}

func TestTUICmdOpensReadOnlyOperatorAndRunsTUI(t *testing.T) {
	oldOpen := openYardReadOnlyOperator
	oldRun := runYardTUI
	t.Cleanup(func() {
		openYardReadOnlyOperator = oldOpen
		runYardTUI = oldRun
	})

	configPath := "test-yard.yaml"
	var openedConfig string
	var ran bool
	openYardReadOnlyOperator = func(ctx context.Context, path string) (*operator.Service, error) {
		openedConfig = path
		return &operator.Service{}, nil
	}
	runYardTUI = func(ctx context.Context, svc tuiapp.Operator, opts tuiapp.Options) error {
		if svc == nil {
			t.Fatal("runYardTUI received nil service")
		}
		ran = true
		return nil
	}

	cmd := newYardTUICmd(&configPath)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if openedConfig != configPath {
		t.Fatalf("opened config = %q, want %q", openedConfig, configPath)
	}
	if !ran {
		t.Fatal("runYardTUI was not called")
	}
}

func TestTUICmdWrapsReadOnlyOpenError(t *testing.T) {
	oldOpen := openYardReadOnlyOperator
	oldRun := runYardTUI
	t.Cleanup(func() {
		openYardReadOnlyOperator = oldOpen
		runYardTUI = oldRun
	})

	openYardReadOnlyOperator = func(ctx context.Context, path string) (*operator.Service, error) {
		return nil, errors.New("boom")
	}
	runYardTUI = func(ctx context.Context, svc tuiapp.Operator, opts tuiapp.Options) error {
		t.Fatal("runYardTUI should not be called after open error")
		return nil
	}

	configPath := "test-yard.yaml"
	cmd := newYardTUICmd(&configPath)
	err := cmd.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "open operator: boom") {
		t.Fatalf("Execute error = %v, want wrapped open operator error", err)
	}
}
