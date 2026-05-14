package server

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// applyMiddleware wraps the server's mux with the middleware chain.
// Must be called before Start. Called automatically by New.
func (s *Server) applyMiddleware() {
	handler := http.Handler(s.mux)
	handler = unsafeMethodOriginGuard(s.devMode, handler)
	handler = requestLogger(s.logger, handler)
	handler = panicRecovery(s.logger, handler)
	if s.devMode {
		handler = cors(handler)
	}
	s.httpServer.Handler = handler
}

const defaultDevCORSOrigin = "http://localhost:5173"

// unsafeMethodOriginGuard blocks browser-originated CSRF attempts against the
// local API while preserving non-browser clients such as curl and tests.
func unsafeMethodOriginGuard(devMode bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSafeHTTPMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
			if !allowedRequestOrigin(r, origin, devMode) {
				writeError(w, http.StatusForbidden, "cross-origin request blocked")
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" && !allowedRequestOrigin(r, referer, devMode) {
			writeError(w, http.StatusForbidden, "cross-origin request blocked")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isSafeHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func allowedRequestOrigin(r *http.Request, rawOrigin string, devMode bool) bool {
	origin, ok := parseOrigin(rawOrigin)
	if !ok {
		return false
	}
	if sameOrigin(origin, requestOrigin(r)) {
		return true
	}
	return devMode && allowedDevOrigin(origin)
}

func parseOrigin(raw string) (*url.URL, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, false
	}
	return &url.URL{Scheme: parsed.Scheme, Host: parsed.Host}, true
}

func requestOrigin(r *http.Request) *url.URL {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return &url.URL{Scheme: scheme, Host: r.Host}
}

func sameOrigin(a, b *url.URL) bool {
	return a != nil &&
		b != nil &&
		strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Host, b.Host)
}

func allowedDevOrigin(origin *url.URL) bool {
	if origin == nil {
		return false
	}
	host := origin.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// requestLogger logs method, path, status, and duration for each request.
func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// statusWriter captures the status code for logging.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the underlying ResponseWriter for http.ResponseController
// and WebSocket hijack support.
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Hijack implements http.Hijacker, required for WebSocket upgrade.
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

// panicRecovery catches panics in handlers and returns 500.
func panicRecovery(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("panic recovered",
					"error", fmt.Sprintf("%v", err),
					"method", r.Method,
					"path", r.URL.Path,
				)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// cors adds permissive CORS headers for dev mode loopback renderers.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedCORSOrigin(r))
		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Yard-Desktop-Session")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func allowedCORSOrigin(r *http.Request) string {
	if origin, ok := parseOrigin(strings.TrimSpace(r.Header.Get("Origin"))); ok && allowedDevOrigin(origin) {
		return origin.String()
	}
	return defaultDevCORSOrigin
}
