package main

import (
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"testing"
	"time"
)

func TestYardChainStartExposesMaxResolverLoopsFlag(t *testing.T) {
	configPath := "yard.yaml"
	cmd := newYardChainStartCmd(&configPath)
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

func TestYardChainSpecFromFlagsUsesMaxResolverLoops(t *testing.T) {
	spec := yardChainSpecFromFlags("chain-1", yardChainFlags{Specs: "specs/a.md", MaxSteps: 7, MaxResolverLoops: 9, MaxDuration: time.Hour, TokenBudget: 123})
	if spec.MaxResolverLoops != 9 {
		t.Fatalf("MaxResolverLoops = %d, want 9", spec.MaxResolverLoops)
	}
}

func TestApplyYardChainOverrides(t *testing.T) {
	cfg := &appconfig.Config{ProjectRoot: "/old/project"}
	flags := yardChainFlags{ProjectRoot: "/new/project", Brain: "/new/brain"}

	applyYardChainOverrides(cfg, flags)

	if cfg.ProjectRoot != "/new/project" {
		t.Fatalf("ProjectRoot = %q, want /new/project", cfg.ProjectRoot)
	}
	if cfg.Brain.VaultPath != "/new/brain" {
		t.Fatalf("Brain.VaultPath = %q, want /new/brain", cfg.Brain.VaultPath)
	}
}

func TestValidateYardChainFlagsRejectsInvalidNumericFlags(t *testing.T) {
	tests := []struct {
		name    string
		flags   yardChainFlags
		wantErr string
	}{
		{name: "missing task and specs", flags: yardChainFlags{}, wantErr: "one of --task or --specs is required"},
		{name: "nonpositive max steps", flags: yardChainFlags{Task: "x", MaxSteps: 0, MaxResolverLoops: 0, MaxDuration: time.Second, TokenBudget: 1}, wantErr: "--max-steps must be > 0"},
		{name: "negative resolver loops", flags: yardChainFlags{Task: "x", MaxSteps: 1, MaxResolverLoops: -1, MaxDuration: time.Second, TokenBudget: 1}, wantErr: "--max-resolver-loops must be >= 0"},
		{name: "nonpositive max duration", flags: yardChainFlags{Task: "x", MaxSteps: 1, MaxResolverLoops: 0, MaxDuration: 0, TokenBudget: 1}, wantErr: "--max-duration must be > 0"},
		{name: "nonpositive token budget", flags: yardChainFlags{Task: "x", MaxSteps: 1, MaxResolverLoops: 0, MaxDuration: time.Second, TokenBudget: 0}, wantErr: "--token-budget must be > 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateYardChainFlags(tc.flags); err == nil || err.Error() != tc.wantErr {
				t.Fatalf("validateYardChainFlags() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateYardChainFlagsAcceptsZeroResolverLoops(t *testing.T) {
	flags := yardChainFlags{Task: "x", MaxSteps: 1, MaxResolverLoops: 0, MaxDuration: time.Second, TokenBudget: 1}
	if err := validateYardChainFlags(flags); err != nil {
		t.Fatalf("validateYardChainFlags() error = %v, want nil", err)
	}
}

func TestValidateYardChainFlagsAcceptsChainIDOnlyForResume(t *testing.T) {
	flags := yardChainFlags{ChainID: "chain-1", MaxSteps: 1, MaxResolverLoops: 0, MaxDuration: time.Second, TokenBudget: 1}
	if err := validateYardChainFlags(flags); err != nil {
		t.Fatalf("validateYardChainFlags() error = %v, want nil", err)
	}
}

func TestValidateYardChainStatusTransition(t *testing.T) {
	if err := validateYardChainStatusTransition("paused", "running", "chain-1"); err != nil {
		t.Fatalf("resume paused chain error = %v, want nil", err)
	}
	if err := validateYardChainStatusTransition("completed", "running", "chain-1"); err == nil {
		t.Fatal("expected completed chain resume to fail")
	}
	if err := validateYardChainStatusTransition("running", "paused", "chain-1"); err != nil {
		t.Fatalf("pause running chain error = %v, want nil", err)
	}
}
