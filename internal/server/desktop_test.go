package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	"github.com/ponchione/sodoryard/internal/server"
)

func TestDesktopCapabilitiesEndpointReportsProjectMemoryProtocol(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := filepath.Join(projectRoot, "yard.yaml")
	cfg := &config.Config{ProjectRoot: projectRoot}
	cfg.Memory.Backend = "shunter"

	backend, err := projectmemory.OpenBrainBackend(context.Background(), projectmemory.Config{
		DataDir:        filepath.Join(projectRoot, ".yard", "shunter", "project-memory"),
		EnableProtocol: true,
	})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewDesktopHandler(srv, cfg, backend, server.DesktopOptions{
		YardVersion: "test-version",
		ConfigPath:  configPath,
	})
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/desktop/capabilities")
	if err != nil {
		t.Fatalf("capabilities request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("capabilities status = %d, want 200", resp.StatusCode)
	}

	var body desktopCapabilitiesBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}

	if body.YardVersion != "test-version" {
		t.Fatalf("yard_version = %q, want test-version", body.YardVersion)
	}
	if body.APIVersion != "desktop-v1" {
		t.Fatalf("api_version = %q, want desktop-v1", body.APIVersion)
	}
	if body.ProjectRoot != projectRoot {
		t.Fatalf("project_root = %q, want %q", body.ProjectRoot, projectRoot)
	}
	if body.ConfigPath != configPath {
		t.Fatalf("config_path = %q, want %q", body.ConfigPath, configPath)
	}
	for _, capability := range []string{
		"runtime_status",
		"runtime_local_services",
		"conversation_chat",
		"project_memory_protocol",
		"project_memory_subscriptions",
		"project_memory_contract",
		"chains_read",
		"chains_control",
		"launch_preview",
		"launch_start",
		"launch_drafts",
		"launch_presets",
		"project_tree",
		"project_file_preview",
		"context_reports",
		"metrics",
	} {
		if !stringSliceContains(body.Capabilities, capability) {
			t.Fatalf("capabilities missing %q in %#v", capability, body.Capabilities)
		}
	}

	if body.ProjectMemory == nil {
		t.Fatal("project_memory missing")
	}
	if body.ProjectMemory.Backend != "shunter" {
		t.Fatalf("project_memory.backend = %q, want shunter", body.ProjectMemory.Backend)
	}
	if body.ProjectMemory.Module != projectmemory.ModuleName {
		t.Fatalf("project_memory.module = %q, want %q", body.ProjectMemory.Module, projectmemory.ModuleName)
	}
	if body.ProjectMemory.SchemaVersion == 0 {
		t.Fatal("project_memory.schema_version was not reported")
	}
	if body.ProjectMemory.ContractVersion != 1 {
		t.Fatalf("project_memory.contract_version = %d, want 1", body.ProjectMemory.ContractVersion)
	}
	if body.ProjectMemory.ShunterVersion != "v1.1.0" {
		t.Fatalf("project_memory.shunter_version = %q, want v1.1.0", body.ProjectMemory.ShunterVersion)
	}
	if body.ProjectMemory.DefaultSubprotocol != "v2.bsatn.shunter" {
		t.Fatalf("project_memory.default_subprotocol = %q, want v2.bsatn.shunter", body.ProjectMemory.DefaultSubprotocol)
	}
	if !stringSliceContains(body.ProjectMemory.SupportedSubprotocols, "v1.bsatn.shunter") {
		t.Fatalf("project_memory.supported_subprotocols = %#v, want v1 compatibility", body.ProjectMemory.SupportedSubprotocols)
	}
	wantSubscribeURL := "ws" + strings.TrimPrefix(base, "http") + "/api/project-memory/subscribe"
	if body.ProjectMemory.SubscribeURL != wantSubscribeURL {
		t.Fatalf("project_memory.subscribe_url = %q, want %q", body.ProjectMemory.SubscribeURL, wantSubscribeURL)
	}
	if !strings.HasPrefix(body.ProjectMemory.GeneratedBindingHash, "sha256:") || len(body.ProjectMemory.GeneratedBindingHash) != len("sha256:")+64 {
		t.Fatalf("project_memory.generated_binding_hash = %q, want sha256 hash", body.ProjectMemory.GeneratedBindingHash)
	}
}

func TestDesktopCapabilitiesOmitsProtocolCapabilitiesWhenProjectMemoryProtocolDisabled(t *testing.T) {
	projectRoot := t.TempDir()
	cfg := &config.Config{ProjectRoot: projectRoot}
	cfg.Memory.Backend = "shunter"

	backend, err := projectmemory.OpenBrainBackend(context.Background(), projectmemory.Config{
		DataDir: filepath.Join(projectRoot, ".yard", "shunter", "project-memory"),
	})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewDesktopHandler(srv, cfg, backend, server.DesktopOptions{ConfigPath: filepath.Join(projectRoot, "yard.yaml")})
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/desktop/capabilities")
	if err != nil {
		t.Fatalf("capabilities request failed: %v", err)
	}
	defer resp.Body.Close()

	var body desktopCapabilitiesBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}
	if stringSliceContains(body.Capabilities, "project_memory_protocol") {
		t.Fatalf("capabilities unexpectedly include project_memory_protocol: %#v", body.Capabilities)
	}
	if stringSliceContains(body.Capabilities, "project_memory_subscriptions") {
		t.Fatalf("capabilities unexpectedly include project_memory_subscriptions: %#v", body.Capabilities)
	}
	if !stringSliceContains(body.Capabilities, "project_memory_contract") {
		t.Fatalf("capabilities missing project_memory_contract: %#v", body.Capabilities)
	}
	if body.ProjectMemory == nil {
		t.Fatal("project_memory missing")
	}
	if body.ProjectMemory.SubscribeURL != "" {
		t.Fatalf("project_memory.subscribe_url = %q, want empty when protocol disabled", body.ProjectMemory.SubscribeURL)
	}
}

type desktopCapabilitiesBody struct {
	YardVersion   string   `json:"yard_version"`
	APIVersion    string   `json:"api_version"`
	ProjectRoot   string   `json:"project_root"`
	ConfigPath    string   `json:"config_path"`
	Capabilities  []string `json:"capabilities"`
	ProjectMemory *struct {
		Backend               string   `json:"backend"`
		Module                string   `json:"module"`
		SchemaVersion         uint32   `json:"schema_version"`
		ContractVersion       uint32   `json:"contract_version"`
		ShunterVersion        string   `json:"shunter_version"`
		DefaultSubprotocol    string   `json:"default_subprotocol"`
		SupportedSubprotocols []string `json:"supported_subprotocols"`
		SubscribeURL          string   `json:"subscribe_url"`
		GeneratedBindingHash  string   `json:"generated_binding_hash"`
	} `json:"project_memory"`
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
