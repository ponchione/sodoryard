package localservices

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appconfig "github.com/ponchione/sodoryard/internal/config"
)

type fakeRunner struct {
	calls []string
	steps map[string]fakeRunResult
}

type fakeRunResult struct {
	stdout string
	stderr string
	err    error
}

func (f *fakeRunner) Run(_ context.Context, name string, args []string, dir string) (string, string, error) {
	call := strings.TrimSpace(strings.Join(append([]string{name}, args...), " "))
	if dir != "" {
		call = dir + " :: " + call
	}
	f.calls = append(f.calls, call)
	result, ok := f.steps[call]
	if !ok {
		return "", "", errors.New("unexpected command: " + call)
	}
	return result.stdout, result.stderr, result.err
}

func newTestConfig(t *testing.T) *appconfig.Config {
	t.Helper()
	projectRoot := t.TempDir()
	cfg := appconfig.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Brain.Enabled = false
	cfg.LocalServices.ComposeFile = filepath.Join(projectRoot, "ops", "llm", "docker-compose.yml")
	cfg.LocalServices.ProjectDir = filepath.Join(projectRoot, "ops", "llm")
	if err := os.MkdirAll(cfg.LocalServices.ProjectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	if err := os.WriteFile(cfg.LocalServices.ComposeFile, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
	return cfg
}

func healthyServiceServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/models":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"ok"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestProbeServiceHealthy(t *testing.T) {
	server := healthyServiceServer()
	defer server.Close()
	status := ProbeService(context.Background(), server.Client(), "qwen-coder", appconfig.ManagedService{
		BaseURL:    server.URL,
		HealthPath: "/health",
		ModelsPath: "/v1/models",
		Required:   true,
	})
	if !status.Healthy || !status.Reachable || !status.ModelsReady {
		t.Fatalf("status = %+v, want fully healthy", status)
	}
}

func TestProbeServiceNoModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/models":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	status := ProbeService(context.Background(), server.Client(), "nomic-embed", appconfig.ManagedService{
		BaseURL:    server.URL,
		HealthPath: "/health",
		ModelsPath: "/v1/models",
		Required:   true,
	})
	if status.Healthy {
		t.Fatalf("status = %+v, want unhealthy", status)
	}
	if !strings.Contains(status.Detail, "no models") {
		t.Fatalf("detail = %q, want no models", status.Detail)
	}
}

func TestManagerEnsureUpManualReturnsRemediation(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.LocalServices.Mode = "manual"
	cfg.LocalServices.Services["qwen-coder"] = appconfig.ManagedService{BaseURL: "http://127.0.0.1:1", HealthPath: "/health", ModelsPath: "/v1/models", Required: true}
	cfg.LocalServices.Services["nomic-embed"] = appconfig.ManagedService{BaseURL: "http://127.0.0.1:2", HealthPath: "/health", ModelsPath: "/v1/models", Required: true}
	runner := &fakeRunner{steps: map[string]fakeRunResult{
		"docker version --format {{.Client.Version}}": {stdout: "1"},
		"docker info":                    {stdout: "ok"},
		"docker compose version":         {stdout: "v2"},
		"docker network inspect llm-net": {stdout: "[]"},
	}}
	manager := NewManagerWithDeps(runner, defaultHTTPClient(), func(_ time.Duration) {})
	status, err := manager.EnsureUp(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected EnsureUp error")
	}
	var managerErr *ManagerError
	if !errors.As(err, &managerErr) {
		t.Fatalf("expected ManagerError, got %T", err)
	}
	if len(status.Remediation) == 0 {
		t.Fatalf("status = %+v, want remediation", status)
	}
}

func TestManagerEnsureUpReturnsImmediatelyWhenServicesAlreadyHealthy(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.LocalServices.Mode = "auto"
	qwen := healthyServiceServer()
	defer qwen.Close()
	embed := healthyServiceServer()
	defer embed.Close()
	cfg.LocalServices.Services["qwen-coder"] = appconfig.ManagedService{BaseURL: qwen.URL, HealthPath: "/health", ModelsPath: "/v1/models", Required: true}
	cfg.LocalServices.Services["nomic-embed"] = appconfig.ManagedService{BaseURL: embed.URL, HealthPath: "/health", ModelsPath: "/v1/models", Required: true}
	runner := &fakeRunner{steps: map[string]fakeRunResult{
		"docker version --format {{.Client.Version}}": {stdout: "1"},
		"docker info":                    {stdout: "ok"},
		"docker compose version":         {stdout: "v2"},
		"docker network inspect llm-net": {stdout: "[]"},
	}}
	manager := NewManagerWithDeps(runner, qwen.Client(), func(_ time.Duration) {})
	status, err := manager.EnsureUp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("EnsureUp: %v", err)
	}
	if !status.AllRequiredHealthy() {
		t.Fatalf("status = %+v, want healthy", status)
	}
	if len(status.Remediation) != 0 {
		t.Fatalf("status.Remediation = %v, want none for healthy stack", status.Remediation)
	}
	joined := strings.Join(runner.calls, "\n")
	if strings.Contains(joined, "docker compose -f") {
		t.Fatalf("expected no compose up when services are already healthy\n%s", joined)
	}
}

func TestManagerLogsUsesComposeLogs(t *testing.T) {
	cfg := newTestConfig(t)
	projectDir := cfg.LocalServices.ProjectDir
	composeFile := cfg.LocalServices.ComposeFile
	runner := &fakeRunner{steps: map[string]fakeRunResult{
		projectDir + " :: docker compose -f " + composeFile + " logs --tail 50 nomic-embed qwen-coder": {stdout: "log output"},
	}}
	manager := NewManagerWithDeps(runner, defaultHTTPClient(), nil)
	logs, err := manager.Logs(context.Background(), cfg, 50)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if logs != "log output" {
		t.Fatalf("logs = %q, want log output", logs)
	}
}

func TestNormalizeComposeUpErrorForContainerConflict(t *testing.T) {
	errText := "Error response from daemon: Conflict. The container name \"/qwen-coder-server\" is already in use by container abc123"
	normalized := normalizeComposeUpError(errText)
	if !strings.Contains(normalized, "docker rm -f <name>") {
		t.Fatalf("normalized error missing remediation, got: %s", normalized)
	}
	if !strings.Contains(normalized, "container_name") {
		t.Fatalf("normalized error missing container_name guidance, got: %s", normalized)
	}
}
