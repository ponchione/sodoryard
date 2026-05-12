package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/projectmemory"
	"github.com/ponchione/sodoryard/internal/server"
	"nhooyr.io/websocket"
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

func TestProjectMemorySubscribeRouteAcceptsShunterWebSocket(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(base, "http") + "/api/project-memory/subscribe"
	conn, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{"v1.bsatn.shunter"},
	})
	if err != nil {
		if resp != nil {
			t.Fatalf("subscribe websocket dial failed with status %d: %v", resp.StatusCode, err)
		}
		t.Fatalf("subscribe websocket dial failed: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test complete")

	if got := conn.Subprotocol(); got != "v1.bsatn.shunter" {
		t.Fatalf("subprotocol = %q, want v1.bsatn.shunter", got)
	}
}
