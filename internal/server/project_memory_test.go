package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
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

func TestProjectMemoryTokenEndpointMintsUsableSubscribeToken(t *testing.T) {
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

	resp, err := http.Post(base+"/api/project-memory/token", "application/json", http.NoBody)
	if err != nil {
		t.Fatalf("token request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Token     string `json:"token"`
		TokenType string `json:"token_type"`
		Identity  string `json:"identity"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode token: %v", err)
	}
	if body.Token == "" {
		t.Fatal("token response omitted token")
	}
	if body.TokenType != "bearer" {
		t.Fatalf("token_type = %q, want bearer", body.TokenType)
	}
	if body.Identity == "" {
		t.Fatal("token response omitted identity")
	}
	expiresAt, err := time.Parse(time.RFC3339, body.ExpiresAt)
	if err != nil {
		t.Fatalf("expires_at = %q, want RFC3339 timestamp: %v", body.ExpiresAt, err)
	}
	if !expiresAt.After(time.Now().UTC()) {
		t.Fatalf("expires_at = %s, want future expiry", body.ExpiresAt)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(base, "http") + "/api/project-memory/subscribe?token=" + url.QueryEscape(body.Token)
	conn, dialResp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{"v2.bsatn.shunter"},
	})
	if err != nil {
		if dialResp != nil {
			t.Fatalf("subscribe websocket dial with token failed with status %d: %v", dialResp.StatusCode, err)
		}
		t.Fatalf("subscribe websocket dial with token failed: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test complete")

	if got := conn.Subprotocol(); got != "v2.bsatn.shunter" {
		t.Fatalf("subprotocol = %q, want v2.bsatn.shunter", got)
	}
}

func TestProjectMemoryTokenEndpointUnavailableWhenProtocolDisabled(t *testing.T) {
	backend, err := projectmemory.OpenBrainBackend(context.Background(), projectmemory.Config{
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("OpenBrainBackend: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewProjectMemoryHandler(srv, backend, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Post(base+"/api/project-memory/token", "application/json", http.NoBody)
	if err != nil {
		t.Fatalf("token request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("token status = %d, want 503", resp.StatusCode)
	}
}
