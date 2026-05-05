package openai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ponchione/sodoryard/internal/provider"
)

func TestNewOpenAIProvider_DirectAPIKey(t *testing.T) {
	p, err := NewOpenAIProvider(OpenAIConfig{
		Name:          "test",
		BaseURL:       "http://localhost:8080/v1",
		APIKey:        "sk-test-key",
		Model:         "qwen2.5-coder-7b",
		ContextLength: 32768,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.name != "test" {
		t.Errorf("expected name 'test', got %q", p.name)
	}
	if p.baseURL != "http://localhost:8080/v1" {
		t.Errorf("expected baseURL 'http://localhost:8080/v1', got %q", p.baseURL)
	}
	if p.apiKey != "sk-test-key" {
		t.Errorf("expected apiKey 'sk-test-key', got %q", p.apiKey)
	}
	if p.model != "qwen2.5-coder-7b" {
		t.Errorf("expected model 'qwen2.5-coder-7b', got %q", p.model)
	}
	if p.contextLength != 32768 {
		t.Errorf("expected contextLength 32768, got %d", p.contextLength)
	}
}

func TestNewOpenAIProvider_APIKeyFromEnv(t *testing.T) {
	const envVar = "SODORYARD_TEST_OPENAI_KEY"
	os.Setenv(envVar, "sk-from-env")
	defer os.Unsetenv(envVar)

	p, err := NewOpenAIProvider(OpenAIConfig{
		Name:          "envtest",
		BaseURL:       "http://localhost:8080/v1",
		APIKeyEnv:     envVar,
		Model:         "test-model",
		ContextLength: 4096,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.apiKey != "sk-from-env" {
		t.Errorf("expected apiKey 'sk-from-env', got %q", p.apiKey)
	}
}

func TestNewOpenAIProvider_APIKeyEnvUnset(t *testing.T) {
	const envVar = "SODORYARD_TEST_OPENAI_UNSET_KEY"
	os.Unsetenv(envVar)

	_, err := NewOpenAIProvider(OpenAIConfig{
		Name:          "envtest",
		BaseURL:       "http://localhost:8080/v1",
		APIKeyEnv:     envVar,
		Model:         "test-model",
		ContextLength: 4096,
	})
	if err == nil {
		t.Fatal("expected error for unset env var")
	}
	expected := "environment variable '" + envVar + "' is not set or empty"
	if !contains(err.Error(), expected) {
		t.Errorf("expected error containing %q, got %q", expected, err.Error())
	}
}

func TestNewOpenAIProvider_KeylessLocalMode(t *testing.T) {
	p, err := NewOpenAIProvider(OpenAIConfig{
		Name:          "local",
		BaseURL:       "http://localhost:8080/v1",
		Model:         "test-model",
		ContextLength: 4096,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.apiKey != "" {
		t.Errorf("expected empty apiKey, got %q", p.apiKey)
	}
}

func TestNewOpenAIProvider_ValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		cfg      OpenAIConfig
		expected string
	}{
		{
			name:     "missing name",
			cfg:      OpenAIConfig{BaseURL: "http://localhost", Model: "m", ContextLength: 4096},
			expected: "openai provider config: name is required",
		},
		{
			name:     "missing base_url",
			cfg:      OpenAIConfig{Name: "test", Model: "m", ContextLength: 4096},
			expected: "openai provider 'test': base_url is required",
		},
		{
			name:     "missing model",
			cfg:      OpenAIConfig{Name: "test", BaseURL: "http://localhost", ContextLength: 4096},
			expected: "openai provider 'test': model is required",
		},
		{
			name:     "zero context_length",
			cfg:      OpenAIConfig{Name: "test", BaseURL: "http://localhost", Model: "m", ContextLength: 0},
			expected: "openai provider 'test': context_length must be a positive integer",
		},
		{
			name:     "negative context_length",
			cfg:      OpenAIConfig{Name: "test", BaseURL: "http://localhost", Model: "m", ContextLength: -1},
			expected: "openai provider 'test': context_length must be a positive integer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewOpenAIProvider(tt.cfg)
			if err == nil {
				t.Fatal("expected error")
			}
			if err.Error() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, err.Error())
			}
		})
	}
}

func TestNewOpenAIProvider_TrailingSlashStripping(t *testing.T) {
	p, err := NewOpenAIProvider(OpenAIConfig{
		Name:          "test",
		BaseURL:       "http://localhost:8080/v1/",
		Model:         "m",
		ContextLength: 4096,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.baseURL != "http://localhost:8080/v1" {
		t.Errorf("expected baseURL without trailing slash, got %q", p.baseURL)
	}

	// Multiple trailing slashes.
	p2, err := NewOpenAIProvider(OpenAIConfig{
		Name:          "test",
		BaseURL:       "http://localhost:8080/v1///",
		Model:         "m",
		ContextLength: 4096,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p2.baseURL != "http://localhost:8080/v1" {
		t.Errorf("expected baseURL without trailing slashes, got %q", p2.baseURL)
	}
}

func TestOpenAIProvider_Name(t *testing.T) {
	p, _ := NewOpenAIProvider(OpenAIConfig{
		Name:          "local",
		BaseURL:       "http://localhost:8080/v1",
		Model:         "m",
		ContextLength: 4096,
	})
	if p.Name() != "local" {
		t.Errorf("expected name 'local', got %q", p.Name())
	}
}

func TestOpenAIProvider_Models(t *testing.T) {
	p, _ := NewOpenAIProvider(OpenAIConfig{
		Name:          "test",
		BaseURL:       "http://localhost:8080/v1",
		Model:         "qwen2.5-coder-7b",
		ContextLength: 32768,
	})
	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ID != "qwen2.5-coder-7b" {
		t.Errorf("expected model ID 'qwen2.5-coder-7b', got %q", models[0].ID)
	}
	if models[0].ContextWindow != 32768 {
		t.Errorf("expected context window 32768, got %d", models[0].ContextWindow)
	}
}

func TestOpenAIProvider_ContextLength(t *testing.T) {
	p, _ := NewOpenAIProvider(OpenAIConfig{
		Name:          "test",
		BaseURL:       "http://localhost:8080/v1",
		Model:         "m",
		ContextLength: 16384,
	})
	if p.ContextLength() != 16384 {
		t.Errorf("expected 16384, got %d", p.ContextLength())
	}
}

func TestComplete_ReturnsProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(OpenAIConfig{
		Name:          "test",
		BaseURL:       srv.URL,
		Model:         "test-model",
		ContextLength: 4096,
	})

	_, err := p.Complete(context.Background(), &provider.Request{MaxTokens: 100})
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T: %v", err, err)
	}
	if pe.StatusCode != 429 {
		t.Errorf("expected status 429, got %d", pe.StatusCode)
	}
	if !pe.Retriable {
		t.Error("expected Retriable=true for 429")
	}
}

func TestComplete_ReturnsProviderError_AuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(OpenAIConfig{
		Name:          "test",
		BaseURL:       srv.URL,
		Model:         "test-model",
		ContextLength: 4096,
	})

	_, err := p.Complete(context.Background(), &provider.Request{MaxTokens: 100})
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T: %v", err, err)
	}
	if pe.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", pe.StatusCode)
	}
	if pe.Retriable {
		t.Error("expected Retriable=false for 401")
	}
}

func TestComplete_ReturnsProviderError_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(OpenAIConfig{
		Name:          "test",
		BaseURL:       srv.URL,
		Model:         "test-model",
		ContextLength: 4096,
	})

	_, err := p.Complete(context.Background(), &provider.Request{MaxTokens: 100})
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T: %v", err, err)
	}
	if pe.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", pe.StatusCode)
	}
	if !pe.Retriable {
		t.Error("expected Retriable=true for 500")
	}
}

func TestStream_ReturnsProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	p, _ := NewOpenAIProvider(OpenAIConfig{
		Name:          "test",
		BaseURL:       srv.URL,
		Model:         "test-model",
		ContextLength: 4096,
	})

	_, err := p.Stream(context.Background(), &provider.Request{MaxTokens: 100})
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *provider.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *provider.ProviderError, got %T: %v", err, err)
	}
	if pe.StatusCode != 429 {
		t.Errorf("expected status 429, got %d", pe.StatusCode)
	}
	if !pe.Retriable {
		t.Error("expected Retriable=true for 429")
	}
}
