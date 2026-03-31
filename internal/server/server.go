// Package server implements the HTTP/WebSocket server for the web UI.
package server

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Config holds server configuration.
type Config struct {
	Host       string
	Port       int
	DevMode    bool
	FrontendFS fs.FS // nil if no embedded frontend available
}

// Server is the HTTP server for the sirtopham web interface.
type Server struct {
	httpServer *http.Server
	mux        *http.ServeMux
	logger     *slog.Logger
	host       string
	port       int
	devMode    bool
	frontendFS fs.FS

	// ready is closed once the server is listening, after which listenAddr is safe to read.
	ready      chan struct{}
	listenAddr string
}

// New creates a new Server. Call Start to begin listening.
func New(cfg Config, logger *slog.Logger) *Server {
	mux := http.NewServeMux()
	s := &Server{
		mux:        mux,
		logger:     logger,
		host:       cfg.Host,
		port:       cfg.Port,
		devMode:    cfg.DevMode,
		frontendFS: cfg.FrontendFS,
		ready:      make(chan struct{}),
		httpServer: &http.Server{
			Handler:           mux, // middleware wraps this in applyMiddleware
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
	s.registerCoreRoutes()
	s.applyMiddleware()
	return s
}

// Addr returns the configured listen address.
func (s *Server) Addr() string {
	return fmt.Sprintf("%s:%d", s.host, s.port)
}

// ListenAddr returns the actual address the server is listening on.
// Blocks until the server is ready. Returns Addr() if Start hasn't been called.
func (s *Server) ListenAddr() string {
	<-s.ready
	return s.listenAddr
}

// Ready returns a channel that is closed when the server is listening.
func (s *Server) Ready() <-chan struct{} {
	return s.ready
}

// HandleFunc registers a handler on the server's mux.
// Pattern uses Go 1.22+ syntax: "GET /api/foo", "POST /api/bar/{id}".
func (s *Server) HandleFunc(pattern string, handler http.HandlerFunc) {
	s.mux.HandleFunc(pattern, handler)
}

// Handle registers an http.Handler on the server's mux.
func (s *Server) Handle(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
}

// Start begins listening. Blocks until the server stops or context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.Addr())
	if err != nil {
		return fmt.Errorf("server listen: %w", err)
	}
	s.listenAddr = ln.Addr().String()
	close(s.ready) // Signal that the server is listening and listenAddr is set.
	s.logger.Info("server listening", "addr", s.listenAddr, "dev_mode", s.devMode)

	errCh := make(chan error, 1)
	go func() { errCh <- s.httpServer.Serve(ln) }()

	select {
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		return s.Shutdown()
	}
}

// Shutdown gracefully shuts down the server with a 10-second deadline.
func (s *Server) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.logger.Info("server shutting down")
	return s.httpServer.Shutdown(ctx)
}

// registerCoreRoutes sets up routes that are always present.
func (s *Server) registerCoreRoutes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)

	if s.frontendFS != nil {
		s.mux.Handle("/", staticHandler(s.logger, s.frontendFS))
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
