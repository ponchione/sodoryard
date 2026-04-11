package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/localservices"
)

type fakeLLMManager struct {
	status     localservices.StackStatus
	logs       string
	statusErr  error
	upErr      error
	downErr    error
	logsErr    error
	upCalled   bool
	downCalled bool
	logsTail   int
}

func (f *fakeLLMManager) Status(context.Context, *appconfig.Config) (localservices.StackStatus, error) {
	return f.status, f.statusErr
}
func (f *fakeLLMManager) EnsureUp(context.Context, *appconfig.Config) (localservices.StackStatus, error) {
	f.upCalled = true
	return f.status, f.upErr
}
func (f *fakeLLMManager) Down(context.Context, *appconfig.Config) error {
	f.downCalled = true
	return f.downErr
}
func (f *fakeLLMManager) Logs(_ context.Context, _ *appconfig.Config, tail int) (string, error) {
	f.logsTail = tail
	return f.logs, f.logsErr
}

func writeTestConfig(t *testing.T) string {
	t.Helper()
	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, ".brain"), 0o755); err != nil {
		t.Fatalf("mkdir brain: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	configYAML := strings.Join([]string{
		"project_root: " + projectRoot,
		"brain:",
		"  enabled: false",
	}, "\n") + "\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath
}

func TestLLMStatusCommandPrintsStatus(t *testing.T) {
	orig := newLLMManager
	defer func() { newLLMManager = orig }()
	newLLMManager = func() llmManager {
		return &fakeLLMManager{status: localservices.StackStatus{
			Mode:              "manual",
			DockerAvailable:   true,
			DaemonAvailable:   true,
			ComposeAvailable:  true,
			ComposeFileExists: true,
			NetworkStatus:     map[string]bool{"llm-net": true},
			Services:          []localservices.ServiceStatus{{Name: "qwen-coder", Healthy: true, Reachable: true, ModelsReady: true}},
		}}
	}
	configFlag := writeTestConfig(t)
	cmd := newLLMStatusCmd(&configFlag)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"docker_available: true", "service.qwen-coder.healthy: true", "network.llm-net: true"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("output missing %q\n%s", want, buf.String())
		}
	}
}

func TestLLMUpCommandCallsEnsureUp(t *testing.T) {
	orig := newLLMManager
	defer func() { newLLMManager = orig }()
	manager := &fakeLLMManager{status: localservices.StackStatus{Mode: "auto"}}
	newLLMManager = func() llmManager { return manager }
	configFlag := writeTestConfig(t)
	cmd := newLLMUpCmd(&configFlag)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !manager.upCalled {
		t.Fatal("expected EnsureUp to be called")
	}
}

func TestLLMDownCommandCallsDown(t *testing.T) {
	orig := newLLMManager
	defer func() { newLLMManager = orig }()
	manager := &fakeLLMManager{}
	newLLMManager = func() llmManager { return manager }
	configFlag := writeTestConfig(t)
	cmd := newLLMDownCmd(&configFlag)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !manager.downCalled {
		t.Fatal("expected Down to be called")
	}
}

func TestLLMLogsCommandPrintsLogs(t *testing.T) {
	orig := newLLMManager
	defer func() { newLLMManager = orig }()
	manager := &fakeLLMManager{logs: "hello logs"}
	newLLMManager = func() llmManager { return manager }
	configFlag := writeTestConfig(t)
	cmd := newLLMLogsCmd(&configFlag)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(buf.String(), "hello logs") {
		t.Fatalf("output = %q, want logs", buf.String())
	}
}

func TestLLMUpCommandReturnsManagerError(t *testing.T) {
	orig := newLLMManager
	defer func() { newLLMManager = orig }()
	manager := &fakeLLMManager{status: localservices.StackStatus{Problems: []string{"not healthy"}}, upErr: errors.New("boom")}
	newLLMManager = func() llmManager { return manager }
	configFlag := writeTestConfig(t)
	cmd := newLLMUpCmd(&configFlag)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}
