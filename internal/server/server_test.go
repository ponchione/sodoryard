package server_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"testing"
	"testing/fstest"

	"github.com/ponchione/sodoryard/internal/server"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestServer creates a server with port 0 and starts it, returning the
// server and a cleanup function. Port 0 lets the OS pick an available port.
func newTestServer(t *testing.T, cfg server.Config) (*server.Server, string) {
	t.Helper()
	cfg.Port = 0
	cfg.Host = "127.0.0.1"
	srv := server.New(cfg, newTestLogger())

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()

	// ListenAddr blocks until ready — no polling needed.
	addr := srv.ListenAddr()

	t.Cleanup(func() {
		cancel()
		<-errCh
	})

	return srv, "http://" + addr
}

func TestHealthEndpoint(t *testing.T) {
	_, base := newTestServer(t, server.Config{})

	resp, err := http.Get(base + "/api/health")
	if err != nil {
		t.Fatalf("health check request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %q", body["status"])
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}
}

func TestServerStartAndShutdown(t *testing.T) {
	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()

	addr := srv.ListenAddr()

	// Verify it's listening.
	resp, err := http.Get("http://" + addr + "/api/health")
	if err != nil {
		t.Fatalf("server not reachable: %v", err)
	}
	resp.Body.Close()

	// Shutdown via context cancellation.
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("unexpected error from Start: %v", err)
	}
}

func TestHandleFuncRegistration(t *testing.T) {
	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())

	srv.HandleFunc("GET /api/custom", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"custom":"yes"}`))
	})

	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/custom")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestPanicRecovery(t *testing.T) {
	cfg := server.Config{}
	srv := server.New(cfg, newTestLogger())

	srv.HandleFunc("GET /api/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/panic")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

func TestCORSDevMode(t *testing.T) {
	_, base := newTestServer(t, server.Config{DevMode: true})

	resp, err := http.Get(base + "/api/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	origin := resp.Header.Get("Access-Control-Allow-Origin")
	if origin != "http://localhost:5173" {
		t.Fatalf("expected CORS origin http://localhost:5173, got %q", origin)
	}
}

func TestCORSProdMode(t *testing.T) {
	_, base := newTestServer(t, server.Config{DevMode: false})

	resp, err := http.Get(base + "/api/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	origin := resp.Header.Get("Access-Control-Allow-Origin")
	if origin != "" {
		t.Fatalf("expected no CORS header in prod mode, got %q", origin)
	}
}

func TestCORSPreflightOptions(t *testing.T) {
	_, base := newTestServer(t, server.Config{DevMode: true})

	req, _ := http.NewRequest(http.MethodOptions, base+"/api/health", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("preflight request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS, got %d", resp.StatusCode)
	}
}

func TestStaticFileServing(t *testing.T) {
	frontendFS := fstest.MapFS{
		"index.html":  {Data: []byte("<html>app</html>")},
		"assets/style.css": {Data: []byte("body{}")},
	}

	_, base := newTestServer(t, server.Config{FrontendFS: frontendFS})

	// Root serves index.html.
	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatalf("root request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "<html>app</html>" {
		t.Fatalf("expected index.html content, got %q", string(body))
	}

	// Static file.
	resp, err = http.Get(base + "/assets/style.css")
	if err != nil {
		t.Fatalf("css request failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "body{}" {
		t.Fatalf("expected CSS content, got %q", string(body))
	}
}

func TestSPAFallback(t *testing.T) {
	frontendFS := fstest.MapFS{
		"index.html": {Data: []byte("<html>spa</html>")},
	}

	_, base := newTestServer(t, server.Config{FrontendFS: frontendFS})

	// Unknown path should fallback to index.html.
	resp, err := http.Get(base + "/some/client/route")
	if err != nil {
		t.Fatalf("spa fallback request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "<html>spa</html>" {
		t.Fatalf("expected index.html fallback, got %q", string(body))
	}
}

func TestStaticDoesNotInterceptAPI(t *testing.T) {
	frontendFS := fstest.MapFS{
		"index.html": {Data: []byte("<html>spa</html>")},
	}

	_, base := newTestServer(t, server.Config{FrontendFS: frontendFS})

	// API routes should not be served by static handler.
	resp, err := http.Get(base + "/api/health")
	if err != nil {
		t.Fatalf("api request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /api/health, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Fatalf("expected health ok, got %v", body)
	}
}

func TestNoFrontendFS(t *testing.T) {
	// No FrontendFS — dev mode, only API routes should work.
	_, base := newTestServer(t, server.Config{})

	// Root should 404 (no static handler registered).
	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	// Without a frontend FS, GET / has no handler → 404.
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 without frontendFS, got %d", resp.StatusCode)
	}

	// API still works.
	resp, err = http.Get(base + "/api/health")
	if err != nil {
		t.Fatalf("api request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// startServer is a helper for tests that create the server themselves.
func startServer(t *testing.T, srv *server.Server) (*server.Server, string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()

	addr := srv.ListenAddr()

	t.Cleanup(func() {
		cancel()
		<-errCh
	})

	return srv, "http://" + addr
}

