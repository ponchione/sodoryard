package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/operator"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/ponchione/sodoryard/internal/server"
)

func TestDiagnosticsEndpointsReportSupportSnapshot(t *testing.T) {
	ctx := context.Background()
	db := newChainInspectorTestDB(t)
	store := chain.NewStore(db)
	chainID, err := store.StartChain(ctx, chain.ChainSpec{ChainID: "diag-chain", SourceTask: "diagnose"})
	if err != nil {
		t.Fatalf("StartChain returned error: %v", err)
	}

	projectRoot := t.TempDir()
	configPath := filepath.Join(projectRoot, "yard.yaml")
	cfg := &config.Config{
		ProjectRoot: projectRoot,
		Routing: config.RoutingConfig{
			Default: config.RouteConfig{Provider: "codex", Model: "test-model"},
		},
		Providers: map[string]config.ProviderConfig{
			"codex": {Type: "codex", Model: "test-model"},
		},
	}
	cfg.LocalServices.Mode = "manual"

	opSvc, err := operator.NewForRuntime(&rtpkg.OrchestratorRuntime{
		Config:       cfg,
		Database:     db,
		ChainStore:   store,
		BrainBackend: &chainTestBrain{docs: map[string]string{}},
		Cleanup:      func() {},
	}, operator.Options{})
	if err != nil {
		t.Fatalf("NewForRuntime returned error: %v", err)
	}
	t.Cleanup(opSvc.Close)

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewDiagnosticsHandler(srv, cfg, opSvc, nil, server.DiagnosticsOptions{
		YardVersion: "test-version",
		ConfigPath:  configPath,
	}, newTestLogger())
	_, base := startServer(t, srv)

	var body diagnosticsBody
	getJSON(t, base+"/api/diagnostics", &body)
	if body.YardVersion != "test-version" {
		t.Fatalf("yard_version = %q, want test-version", body.YardVersion)
	}
	if body.APIVersion != "desktop-v1" {
		t.Fatalf("api_version = %q, want desktop-v1", body.APIVersion)
	}
	if body.Project.RootPath != projectRoot {
		t.Fatalf("project.root_path = %q, want %q", body.Project.RootPath, projectRoot)
	}
	if body.Project.ConfigPath != configPath {
		t.Fatalf("project.config_path = %q, want %q", body.Project.ConfigPath, configPath)
	}
	if body.Project.LocalServicesMode != "manual" {
		t.Fatalf("project.local_services_mode = %q, want manual", body.Project.LocalServicesMode)
	}
	if body.Runtime.Provider != "codex" || body.Runtime.Model != "test-model" {
		t.Fatalf("runtime provider/model = %q/%q, want codex/test-model", body.Runtime.Provider, body.Runtime.Model)
	}
	if len(body.Providers) != 1 || body.Providers[0].Name != "codex" || body.Providers[0].Type != "codex" {
		t.Fatalf("providers = %+v, want codex provider", body.Providers)
	}
	if !diagnosticsChainContains(body.RecentChains, chainID) {
		t.Fatalf("recent_chains = %+v, missing %q", body.RecentChains, chainID)
	}
	if !stringSliceContains(body.Capabilities, "diagnostics") || !stringSliceContains(body.Capabilities, "diagnostics_export") {
		t.Fatalf("capabilities = %+v, missing diagnostics capabilities", body.Capabilities)
	}

	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(base+"/api/diagnostics/export", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST /api/diagnostics/export failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export status = %d, want 200", resp.StatusCode)
	}
	disposition := resp.Header.Get("Content-Disposition")
	if !strings.Contains(disposition, "yard-diagnostics-") || !strings.Contains(disposition, ".json") {
		t.Fatalf("Content-Disposition = %q, want diagnostics JSON filename", disposition)
	}
	var exported diagnosticsBody
	if err := json.NewDecoder(resp.Body).Decode(&exported); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if exported.Project.RootPath != projectRoot || !diagnosticsChainContains(exported.RecentChains, chainID) {
		t.Fatalf("exported diagnostics = %+v, want project and chain snapshot", exported)
	}
}

type diagnosticsBody struct {
	YardVersion string `json:"yard_version"`
	APIVersion  string `json:"api_version"`
	Project     struct {
		RootPath          string `json:"root_path"`
		ConfigPath        string `json:"config_path"`
		LocalServicesMode string `json:"local_services_mode"`
	} `json:"project"`
	Runtime struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	} `json:"runtime"`
	Providers []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"providers"`
	RecentChains []struct {
		ID string `json:"id"`
	} `json:"recent_chains"`
	Capabilities []string `json:"capabilities"`
}

func diagnosticsChainContains(chains []struct {
	ID string `json:"id"`
}, want string) bool {
	for _, chain := range chains {
		if chain.ID == want {
			return true
		}
	}
	return false
}
