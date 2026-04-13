package main

import (
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"testing"
	"time"
)

func TestChainCommandExposesMaxResolverLoopsFlag(t *testing.T) {
	configPath := "sirtopham.yaml"
	cmd := newChainCmd(&configPath)
	flag := cmd.Flags().Lookup("max-resolver-loops")
	if flag == nil {
		t.Fatal("expected max-resolver-loops flag")
	}
	if flag.DefValue != "3" {
		t.Fatalf("default max-resolver-loops = %q, want 3", flag.DefValue)
	}
	if flag := cmd.Flags().Lookup("project"); flag == nil {
		t.Fatal("expected project flag")
	}
	if flag := cmd.Flags().Lookup("brain"); flag == nil {
		t.Fatal("expected brain flag")
	}
}

func TestChainSpecFromFlagsUsesMaxResolverLoops(t *testing.T) {
	spec := chainSpecFromFlags("chain-1", chainFlags{Specs: "specs/a.md", MaxSteps: 7, MaxResolverLoops: 9, MaxDuration: time.Hour, TokenBudget: 123})
	if spec.MaxResolverLoops != 9 {
		t.Fatalf("MaxResolverLoops = %d, want 9", spec.MaxResolverLoops)
	}
}

func TestApplyChainOverrides(t *testing.T) {
	cfg := &appconfig.Config{ProjectRoot: "/old/project"}
	flags := chainFlags{ProjectRoot: "/new/project", Brain: "/new/brain"}

	applyChainOverrides(cfg, flags)

	if cfg.ProjectRoot != "/new/project" {
		t.Fatalf("ProjectRoot = %q, want /new/project", cfg.ProjectRoot)
	}
	if cfg.Brain.VaultPath != "/new/brain" {
		t.Fatalf("Brain.VaultPath = %q, want /new/brain", cfg.Brain.VaultPath)
	}
}

func TestValidateChainFlagsRejectsInvalidNumericFlags(t *testing.T) {
	tests := []struct {
		name    string
		flags   chainFlags
		wantErr string
	}{
		{name: "missing task and specs", flags: chainFlags{}, wantErr: "one of --task or --specs is required"},
		{name: "nonpositive max steps", flags: chainFlags{Task: "x", MaxSteps: 0, MaxResolverLoops: 0, MaxDuration: time.Second, TokenBudget: 1}, wantErr: "--max-steps must be > 0"},
		{name: "negative resolver loops", flags: chainFlags{Task: "x", MaxSteps: 1, MaxResolverLoops: -1, MaxDuration: time.Second, TokenBudget: 1}, wantErr: "--max-resolver-loops must be >= 0"},
		{name: "nonpositive max duration", flags: chainFlags{Task: "x", MaxSteps: 1, MaxResolverLoops: 0, MaxDuration: 0, TokenBudget: 1}, wantErr: "--max-duration must be > 0"},
		{name: "nonpositive token budget", flags: chainFlags{Task: "x", MaxSteps: 1, MaxResolverLoops: 0, MaxDuration: time.Second, TokenBudget: 0}, wantErr: "--token-budget must be > 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateChainFlags(tc.flags); err == nil || err.Error() != tc.wantErr {
				t.Fatalf("validateChainFlags() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateChainFlagsAcceptsZeroResolverLoops(t *testing.T) {
	flags := chainFlags{Task: "x", MaxSteps: 1, MaxResolverLoops: 0, MaxDuration: time.Second, TokenBudget: 1}
	if err := validateChainFlags(flags); err != nil {
		t.Fatalf("validateChainFlags() error = %v, want nil", err)
	}
}

func TestValidateChainFlagsAcceptsChainIDOnlyForResume(t *testing.T) {
	flags := chainFlags{ChainID: "chain-1", MaxSteps: 1, MaxResolverLoops: 0, MaxDuration: time.Second, TokenBudget: 1}
	if err := validateChainFlags(flags); err != nil {
		t.Fatalf("validateChainFlags() error = %v, want nil", err)
	}
}

func TestValidateChainStatusTransition(t *testing.T) {
	if err := validateChainStatusTransition("paused", "running", "chain-1"); err != nil {
		t.Fatalf("resume paused chain error = %v, want nil", err)
	}
	if err := validateChainStatusTransition("pause_requested", "running", "chain-1"); err != nil {
		t.Fatalf("resume pause_requested chain error = %v, want nil", err)
	}
	if err := validateChainStatusTransition("completed", "running", "chain-1"); err == nil {
		t.Fatal("expected completed chain resume to fail")
	}
	if err := validateChainStatusTransition("running", "paused", "chain-1"); err != nil {
		t.Fatalf("pause running chain error = %v, want nil", err)
	}
	if err := validateChainStatusTransition("pause_requested", "cancelled", "chain-1"); err != nil {
		t.Fatalf("cancel pause_requested chain error = %v, want nil", err)
	}
}
