package codex

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

func TestPing_TreatsNon401AsAuthSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer cached-token" {
			t.Fatalf("Authorization = %q", got)
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	p := &CodexProvider{
		baseURL:     srv.URL,
		httpClient:  &http.Client{Timeout: 2 * time.Second},
		cachedToken: "cached-token",
		tokenExpiry: time.Now().Add(10 * time.Minute),
	}
	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("expected ping success on non-401 response, got %v", err)
	}
}

func TestPing_AuthFailureReturnsTypedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := &CodexProvider{
		baseURL:     srv.URL,
		httpClient:  &http.Client{Timeout: 2 * time.Second},
		cachedToken: "cached-token",
		tokenExpiry: time.Now().Add(10 * time.Minute),
	}
	err := p.Ping(context.Background())
	if err == nil {
		t.Fatal("expected auth failure")
	}
	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected provider error, got %T", err)
	}
	if pe.AuthKind != provider.AuthInvalidCredentials {
		t.Fatalf("expected typed auth error, got %+v", pe)
	}
	if pe.Remediation == "" {
		t.Fatalf("expected remediation message, got %+v", pe)
	}
}
