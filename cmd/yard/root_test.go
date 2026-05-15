package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRootCommandShowsHelpByDefault(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "Available Commands:", "chain", "serve"} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help = %q, want %q", help, want)
		}
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
