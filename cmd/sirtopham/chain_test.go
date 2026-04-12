package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/agent"
	appconfig "github.com/ponchione/sodoryard/internal/config"
)

type fakeChainLoop struct{}

func (fakeChainLoop) RunTurn(ctx context.Context, req agent.RunTurnRequest) (*agent.TurnResult, error) {
	return &agent.TurnResult{FinalText: "done", IterationCount: 1, Duration: time.Second}, nil
}
func (fakeChainLoop) Close() {}

func TestRootIncludesPhase3Subcommands(t *testing.T) {
	cmd := newRootCmd()
	names := []string{}
	for _, child := range cmd.Commands() {
		names = append(names, child.Name())
	}
	for _, want := range []string{"chain", "status", "logs", "receipt", "cancel", "pause", "resume"} {
		if !contains(names, want) {
			t.Fatalf("missing subcommand %q in %v", want, names)
		}
	}
}

func TestChainCommandRequiresTaskOrSpecs(t *testing.T) {
	var buf bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"chain"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "one of --task or --specs is required") {
		t.Fatalf("error = %v, want task/specs requirement", err)
	}
}

func TestBuildChainTaskIncludesSpecsAndChainID(t *testing.T) {
	msg := buildChainTask(chainFlags{Specs: "specs/a.md,specs/b.md"}, "chain-1")
	if !strings.Contains(msg, "specs/a.md") || !strings.Contains(msg, "chain-1") {
		t.Fatalf("message = %q", msg)
	}
}

func TestChainSpecFromFlags(t *testing.T) {
	spec := chainSpecFromFlags("chain-1", chainFlags{Specs: "specs/a.md", Task: "ignored", MaxSteps: 7, MaxResolverLoops: 4, MaxDuration: time.Hour, TokenBudget: 123})
	if spec.ChainID != "chain-1" || len(spec.SourceSpecs) != 1 || spec.MaxSteps != 7 || spec.MaxResolverLoops != 4 || spec.TokenBudget != 123 {
		t.Fatalf("unexpected spec: %+v", spec)
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

var _ = fakeChainLoop{}
var _ = appconfig.Config{}
