package codex

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ponchione/sodoryard/internal/provider"
)

func TestName(t *testing.T) {
	p := &CodexProvider{}
	if p.Name() != "codex" {
		t.Errorf("expected %q, got %q", "codex", p.Name())
	}
}

func TestModels(t *testing.T) {
	p := &CodexProvider{}
	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(models) != 5 {
		t.Fatalf("expected 5 models, got %d", len(models))
	}

	expectedModels := []struct {
		id            string
		contextWindow int
	}{
		{"gpt-5.4", 400000},
		{"gpt-5.4-mini", 400000},
		{"gpt-5.3-codex", 400000},
		{"gpt-5.3-codex-spark", 400000},
		{"gpt-5.2", 400000},
	}

	for i, exp := range expectedModels {
		if models[i].ID != exp.id {
			t.Errorf("model %d: expected ID %q, got %q", i, exp.id, models[i].ID)
		}
		if models[i].ContextWindow != exp.contextWindow {
			t.Errorf("model %d: expected context window %d, got %d", i, exp.contextWindow, models[i].ContextWindow)
		}
		if !models[i].SupportsTools {
			t.Errorf("model %d: expected SupportsTools=true", i)
		}
		if models[i].SupportsThinking {
			t.Errorf("model %d: expected SupportsThinking=false", i)
		}
	}
}

func TestWithBaseURL_TrimsTrailingSlash(t *testing.T) {
	p := &CodexProvider{}
	opt := WithBaseURL("https://chatgpt.com/backend-api/codex/")
	opt(p)
	if p.baseURL != "https://chatgpt.com/backend-api/codex" {
		t.Errorf("expected %q, got %q", "https://chatgpt.com/backend-api/codex", p.baseURL)
	}
}

func TestResponsesEndpointURL_DefaultCodexEndpoint(t *testing.T) {
	p := &CodexProvider{baseURL: "https://chatgpt.com/backend-api/codex"}
	if got := p.responsesEndpointURL(); got != "https://chatgpt.com/backend-api/codex/responses" {
		t.Fatalf("expected chatgpt codex responses URL, got %q", got)
	}
}

func TestResponsesEndpointURL_OpenAICompatibleEndpoint(t *testing.T) {
	p := &CodexProvider{baseURL: "https://example.test"}
	if got := p.responsesEndpointURL(); got != "https://example.test/v1/responses" {
		t.Fatalf("expected v1 responses URL, got %q", got)
	}
}

func TestUsesChatGPTCodexEndpoint(t *testing.T) {
	if !(&CodexProvider{baseURL: "https://chatgpt.com/backend-api/codex"}).usesChatGPTCodexEndpoint() {
		t.Fatal("expected chatgpt codex endpoint to be detected")
	}
	if (&CodexProvider{baseURL: "https://example.test"}).usesChatGPTCodexEndpoint() {
		t.Fatal("did not expect generic base URL to be detected as chatgpt codex endpoint")
	}
}

func TestNewCodexProvider_CodexNotOnPath(t *testing.T) {
	// Save original PATH and set to empty temp dir
	tmpDir := t.TempDir()
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir)
	defer os.Setenv("PATH", origPath)

	_, err := NewCodexProvider()
	if err == nil {
		t.Fatal("expected error when codex is not on PATH")
	}
	if !strings.Contains(err.Error(), "Codex CLI not found on PATH") {
		t.Errorf("expected error containing %q, got %q", "Codex CLI not found on PATH", err.Error())
	}
}

func TestComplete_RetriableAfterRetriesExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/responses" {
			w.WriteHeader(503)
			w.Write([]byte(`{"error":"service unavailable"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	p, _ := NewCodexProvider(WithBaseURL(srv.URL), WithHTTPClient(&http.Client{Timeout: 5 * time.Second}))

	// Seed a valid cached token to skip credential flow
	p.mu.Lock()
	p.cachedToken = "test-token"
	p.tokenExpiry = time.Now().Add(1 * time.Hour)
	p.mu.Unlock()

	_, err := p.Complete(context.Background(), &provider.Request{Model: "o3", MaxTokens: 100})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T", err)
	}
	if !pe.Retriable {
		t.Error("expected Retriable=true after retries exhausted on transient error")
	}
	if pe.StatusCode != 503 {
		t.Errorf("expected status 503, got %d", pe.StatusCode)
	}
}
