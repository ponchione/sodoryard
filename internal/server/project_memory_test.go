package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ponchione/sodoryard/internal/projectmemory"
	"github.com/ponchione/sodoryard/internal/server"
)

func TestProjectMemoryContractEndpoint(t *testing.T) {
	backend, err := projectmemory.OpenBrainBackend(context.Background(), projectmemory.Config{
		DataDir:        t.TempDir(),
		EnableProtocol: true,
	})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewProjectMemoryHandler(srv, backend, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/project-memory/contract")
	if err != nil {
		t.Fatalf("contract request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("contract status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Module struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"module"`
		Schema struct {
			Version uint32 `json:"version"`
		} `json:"schema"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode contract: %v", err)
	}
	if body.Module.Name != projectmemory.ModuleName {
		t.Fatalf("module name = %q, want %q", body.Module.Name, projectmemory.ModuleName)
	}
	if body.Module.Version != projectmemory.ModuleVersion {
		t.Fatalf("module version = %q, want %q", body.Module.Version, projectmemory.ModuleVersion)
	}
	if body.Schema.Version == 0 {
		t.Fatal("schema version was not exported")
	}
}

func TestProjectMemorySubscribeRouteIsMountedSeparately(t *testing.T) {
	backend, err := projectmemory.OpenBrainBackend(context.Background(), projectmemory.Config{
		DataDir:        t.TempDir(),
		EnableProtocol: true,
	})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewProjectMemoryHandler(srv, backend, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/project-memory/subscribe")
	if err != nil {
		t.Fatalf("subscribe request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		t.Fatal("subscribe route returned 404; project memory protocol was not mounted")
	}
}
