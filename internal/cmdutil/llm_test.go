package cmdutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/localservices"
)

type fakeLocalServicesManager struct {
	statusResult   localservices.StackStatus
	statusErr      error
	ensureUpResult localservices.StackStatus
	ensureUpErr    error
	logsResult     string
	logsErr        error
	downErr        error

	statusCalls   int
	ensureUpCalls int
	logsCalls     int
	downCalls     int
}

func (f *fakeLocalServicesManager) Status(ctx context.Context, cfg *appconfig.Config) (localservices.StackStatus, error) {
	f.statusCalls++
	return f.statusResult, f.statusErr
}

func (f *fakeLocalServicesManager) EnsureUp(ctx context.Context, cfg *appconfig.Config) (localservices.StackStatus, error) {
	f.ensureUpCalls++
	return f.ensureUpResult, f.ensureUpErr
}

func (f *fakeLocalServicesManager) Down(ctx context.Context, cfg *appconfig.Config) error {
	f.downCalls++
	return f.downErr
}

func (f *fakeLocalServicesManager) Logs(ctx context.Context, cfg *appconfig.Config, tail int) (string, error) {
	f.logsCalls++
	return f.logsResult, f.logsErr
}

func writeLLMConfig(t *testing.T, localServicesYAML string) string {
	t.Helper()
	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "yard.yaml")
	content := strings.Join([]string{
		"project_root: " + projectRoot,
		"routing:",
		"  default:",
		"    provider: codex",
		"    model: gpt-5.5",
		"providers:",
		"  codex:",
		"    type: codex",
		"    model: gpt-5.5",
		"local_services:",
		localServicesYAML,
	}, "\n") + "\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	return configPath
}

func TestRunLLMDownDisabledPrintsMessageWithoutCallingManager(t *testing.T) {
	configPath := writeLLMConfig(t, strings.Join([]string{
		"  enabled: false",
		"  mode: off",
	}, "\n"))
	manager := &fakeLocalServicesManager{}
	var out bytes.Buffer

	if err := RunLLMDown(context.Background(), &out, configPath, manager); err != nil {
		t.Fatalf("RunLLMDown returned error: %v", err)
	}
	if manager.downCalls != 0 {
		t.Fatalf("downCalls = %d, want 0 when services are disabled", manager.downCalls)
	}
	if got := out.String(); !strings.Contains(got, "local services are disabled in config") {
		t.Fatalf("output = %q, want disabled message", got)
	}
}

func TestRunLLMUpPrintsStatusToStderrOnFailure(t *testing.T) {
	configPath := writeLLMConfig(t, strings.Join([]string{
		"  enabled: true",
		"  mode: auto",
	}, "\n"))
	manager := &fakeLocalServicesManager{
		ensureUpResult: localservices.StackStatus{Mode: "auto", Problems: []string{"compose missing"}},
		ensureUpErr:    errors.New("boom"),
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	err := RunLLMUp(context.Background(), &out, &errOut, configPath, manager)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("RunLLMUp error = %v, want boom", err)
	}
	if manager.ensureUpCalls != 1 {
		t.Fatalf("ensureUpCalls = %d, want 1", manager.ensureUpCalls)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on failure", out.String())
	}
	if got := errOut.String(); !strings.Contains(got, "mode: auto") || !strings.Contains(got, "problem: compose missing") {
		t.Fatalf("stderr = %q, want formatted stack status", got)
	}
}

func TestRunLLMLogsPrintsReturnedLogs(t *testing.T) {
	configPath := writeLLMConfig(t, strings.Join([]string{
		"  enabled: true",
		"  mode: auto",
	}, "\n"))
	manager := &fakeLocalServicesManager{logsResult: "hello logs"}
	var out bytes.Buffer

	if err := RunLLMLogs(context.Background(), &out, configPath, 25, manager); err != nil {
		t.Fatalf("RunLLMLogs returned error: %v", err)
	}
	if manager.logsCalls != 1 {
		t.Fatalf("logsCalls = %d, want 1", manager.logsCalls)
	}
	if got := out.String(); !strings.Contains(got, "hello logs") {
		t.Fatalf("output = %q, want logs", got)
	}
}

func TestLoadConfigWrapsParseErrors(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "yard.yaml")
	if err := os.WriteFile(configPath, []byte("project_root: [oops\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "load config:") {
		t.Fatalf("error = %q, want wrapped load config message", got)
	}
}

func TestPrintLLMStatusTextIncludesNetworksAndProblems(t *testing.T) {
	status := localservices.StackStatus{
		Mode:              "auto",
		ComposeFile:       "/tmp/compose.yaml",
		ProjectDir:        "/tmp/project",
		DockerAvailable:   true,
		DaemonAvailable:   true,
		ComposeAvailable:  true,
		ComposeFileExists: true,
		NetworkStatus:     map[string]bool{"llm-net": true},
		Services:          []localservices.ServiceStatus{{Name: "qwen-coder", Healthy: true, Reachable: true, ModelsReady: true, Detail: "ready"}},
		Problems:          []string{"warn me"},
		Remediation:       []string{"do thing"},
	}
	var out bytes.Buffer

	if err := PrintLLMStatus(&out, status, false); err != nil {
		t.Fatalf("PrintLLMStatus returned error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"mode: auto", "network.llm-net: true", "service.qwen-coder.detail: ready", "problem: warn me", "remediation: do thing"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q in %q", want, got)
		}
	}
}

func TestRunLLMStatusReturnsManagerError(t *testing.T) {
	configPath := writeLLMConfig(t, strings.Join([]string{
		"  enabled: true",
		"  mode: auto",
	}, "\n"))
	manager := &fakeLocalServicesManager{statusErr: fmt.Errorf("status failed")}
	var out bytes.Buffer

	err := RunLLMStatus(context.Background(), &out, configPath, false, manager)
	if err == nil || err.Error() != "status failed" {
		t.Fatalf("RunLLMStatus error = %v, want status failed", err)
	}
}
