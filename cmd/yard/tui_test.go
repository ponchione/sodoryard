package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/operator"
	tuiapp "github.com/ponchione/sodoryard/internal/tui"
)

func TestRootCommandHidesTUICompatibilityCommand(t *testing.T) {
	root := newRootCmd()
	cmd, _, err := root.Find([]string{"tui"})
	if err != nil {
		t.Fatalf("Find(tui) returned error: %v", err)
	}
	if cmd == nil || cmd.Use != "tui" {
		t.Fatalf("Find(tui) = %#v, want tui command", cmd)
	}
	if !cmd.Hidden {
		t.Fatal("tui compatibility command is public")
	}

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("help Execute returned error: %v", err)
	}
	if strings.Contains(out.String(), "tui") {
		t.Fatalf("root help still exposes tui command:\n%s", out.String())
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

func TestHelpOmitsRemovedBrainCompatibilityCommands(t *testing.T) {
	root := newRootCmd()
	for _, cmd := range root.Commands() {
		if cmd.Name() == "memory" {
			t.Fatal("root command still registers public memory command")
		}
	}

	var rootOut bytes.Buffer
	root.SetOut(&rootOut)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("root help Execute returned error: %v", err)
	}
	rootHelp := rootOut.String()
	for _, removed := range []string{"memory", "brain serve"} {
		if strings.Contains(rootHelp, removed) {
			t.Fatalf("root help exposes removed command %q:\n%s", removed, rootHelp)
		}
	}

	brainRoot := newRootCmd()
	var brainOut bytes.Buffer
	brainRoot.SetOut(&brainOut)
	brainRoot.SetArgs([]string{"brain", "--help"})
	if err := brainRoot.Execute(); err != nil {
		t.Fatalf("brain help Execute returned error: %v", err)
	}
	brainHelp := brainOut.String()
	for _, removed := range []string{"serve", "vault"} {
		if strings.Contains(brainHelp, removed) {
			t.Fatalf("brain help exposes removed term %q:\n%s", removed, brainHelp)
		}
	}
	if !strings.Contains(brainHelp, "index") {
		t.Fatalf("brain help = %q, want index command", brainHelp)
	}
}

func TestRootCommandRunsTUIByDefault(t *testing.T) {
	oldOpen := openYardOperator
	oldDegraded := openYardDegradedOperator
	oldRun := runYardTUI
	t.Cleanup(func() {
		openYardOperator = oldOpen
		openYardDegradedOperator = oldDegraded
		runYardTUI = oldRun
	})

	configPath := "root-yard.yaml"
	var openedConfig string
	var ran bool
	var gotOptions tuiapp.Options
	openYardOperator = func(ctx context.Context, path string) (*operator.Service, error) {
		openedConfig = path
		return &operator.Service{}, nil
	}
	openYardDegradedOperator = func(ctx context.Context, path string, cause error) (*operator.Service, error) {
		t.Fatal("openYardDegradedOperator should not be called after successful open")
		return nil, nil
	}
	runYardTUI = func(ctx context.Context, svc tuiapp.Operator, opts tuiapp.Options) error {
		if svc == nil {
			t.Fatal("runYardTUI received nil service")
		}
		gotOptions = opts
		ran = true
		return nil
	}

	root := newRootCmd()
	root.SetArgs([]string{"--config", configPath})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if openedConfig != configPath {
		t.Fatalf("opened config = %q, want %q", openedConfig, configPath)
	}
	if !ran {
		t.Fatal("runYardTUI was not called")
	}
	if gotOptions.WebBaseURL != "http://localhost:8090" {
		t.Fatalf("WebBaseURL = %q, want default yard serve URL", gotOptions.WebBaseURL)
	}
}

func TestTUICmdOpensOperatorAndRunsTUI(t *testing.T) {
	oldOpen := openYardOperator
	oldDegraded := openYardDegradedOperator
	oldRun := runYardTUI
	t.Cleanup(func() {
		openYardOperator = oldOpen
		openYardDegradedOperator = oldDegraded
		runYardTUI = oldRun
	})

	configPath := "test-yard.yaml"
	var openedConfig string
	var ran bool
	var gotOptions tuiapp.Options
	openYardOperator = func(ctx context.Context, path string) (*operator.Service, error) {
		openedConfig = path
		return &operator.Service{}, nil
	}
	openYardDegradedOperator = func(ctx context.Context, path string, cause error) (*operator.Service, error) {
		t.Fatal("openYardDegradedOperator should not be called after successful open")
		return nil, nil
	}
	runYardTUI = func(ctx context.Context, svc tuiapp.Operator, opts tuiapp.Options) error {
		if svc == nil {
			t.Fatal("runYardTUI received nil service")
		}
		gotOptions = opts
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
	if gotOptions.WebBaseURL != "http://localhost:8090" {
		t.Fatalf("WebBaseURL = %q, want default yard serve URL", gotOptions.WebBaseURL)
	}
}

func TestTUICmdFallsBackToDegradedOperator(t *testing.T) {
	oldOpen := openYardOperator
	oldDegraded := openYardDegradedOperator
	oldRun := runYardTUI
	t.Cleanup(func() {
		openYardOperator = oldOpen
		openYardDegradedOperator = oldDegraded
		runYardTUI = oldRun
	})

	fullErr := errors.New("provider auth failed")
	configPath := "test-yard.yaml"
	var degradedConfig string
	var degradedCause error
	var ran bool
	openYardOperator = func(ctx context.Context, path string) (*operator.Service, error) {
		return nil, fullErr
	}
	openYardDegradedOperator = func(ctx context.Context, path string, cause error) (*operator.Service, error) {
		degradedConfig = path
		degradedCause = cause
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
	if degradedConfig != configPath || !errors.Is(degradedCause, fullErr) {
		t.Fatalf("degraded open = config %q cause %v, want %q %v", degradedConfig, degradedCause, configPath, fullErr)
	}
	if !ran {
		t.Fatal("runYardTUI was not called")
	}
}

func TestTUICmdWrapsFullAndDegradedOpenErrors(t *testing.T) {
	oldOpen := openYardOperator
	oldDegraded := openYardDegradedOperator
	oldRun := runYardTUI
	t.Cleanup(func() {
		openYardOperator = oldOpen
		openYardDegradedOperator = oldDegraded
		runYardTUI = oldRun
	})

	openYardOperator = func(ctx context.Context, path string) (*operator.Service, error) {
		return nil, errors.New("full boom")
	}
	openYardDegradedOperator = func(ctx context.Context, path string, cause error) (*operator.Service, error) {
		return nil, errors.New("degraded boom")
	}
	runYardTUI = func(ctx context.Context, svc tuiapp.Operator, opts tuiapp.Options) error {
		t.Fatal("runYardTUI should not be called after open errors")
		return nil
	}

	configPath := "test-yard.yaml"
	cmd := newYardTUICmd(&configPath)
	err := cmd.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "open operator: full boom") || !strings.Contains(err.Error(), "degraded operator open also failed: degraded boom") {
		t.Fatalf("Execute error = %v, want wrapped full and degraded open errors", err)
	}
}
