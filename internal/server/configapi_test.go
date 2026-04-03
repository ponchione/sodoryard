package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"sort"
	"testing"

	"github.com/ponchione/sirtopham/internal/config"
	"github.com/ponchione/sirtopham/internal/provider"
	router "github.com/ponchione/sirtopham/internal/provider/router"
	"github.com/ponchione/sirtopham/internal/server"
)

func TestGetConfigIncludesToolOutputLimitAndStoreRoot(t *testing.T) {
	projectRoot := t.TempDir()
	cfg := config.Default()
	cfg.ProjectRoot = projectRoot
	cfg.Brain.Enabled = false
	cfg.Agent.ToolResultStoreRoot = filepath.Join(projectRoot, ".artifacts", "tool-results")
	cfg.Agent.MaxIterationsPerTurn = 42
	cfg.Agent.ExtendedThinking = false

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConfigHandler(srv, cfg, nil, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/config")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Agent struct {
			MaxIterations       int    `json:"max_iterations"`
			ExtendedThinking    bool   `json:"extended_thinking"`
			ToolOutputMaxTokens int    `json:"tool_output_max_tokens"`
			ToolResultStoreRoot string `json:"tool_result_store_root"`
		} `json:"agent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Agent.MaxIterations != 42 {
		t.Fatalf("agent.max_iterations = %d, want 42", body.Agent.MaxIterations)
	}
	if body.Agent.ExtendedThinking {
		t.Fatal("agent.extended_thinking = true, want false")
	}
	if body.Agent.ToolOutputMaxTokens != cfg.Agent.ToolOutputMaxTokens {
		t.Fatalf("agent.tool_output_max_tokens = %d, want %d", body.Agent.ToolOutputMaxTokens, cfg.Agent.ToolOutputMaxTokens)
	}
	if body.Agent.ToolResultStoreRoot != cfg.Agent.ToolResultStoreRoot {
		t.Fatalf("agent.tool_result_store_root = %q, want %q", body.Agent.ToolResultStoreRoot, cfg.Agent.ToolResultStoreRoot)
	}
}

// stubRuntimeInspector returns fixed model/auth/health data for testing.
type stubRuntimeInspector struct {
	models       []provider.Model
	modelsErr    error
	authStatuses map[string]*provider.AuthStatus
	authErr      error
	health       map[string]*router.ProviderHealth
}

func (s *stubRuntimeInspector) Models(_ context.Context) ([]provider.Model, error) {
	return s.models, s.modelsErr
}

func (s *stubRuntimeInspector) AuthStatuses(_ context.Context) (map[string]*provider.AuthStatus, error) {
	return s.authStatuses, s.authErr
}

func (s *stubRuntimeInspector) ProviderHealthMap() map[string]*router.ProviderHealth {
	return s.health
}

func TestProvidersEndpointGroupsModelsByProvider(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.Brain.Enabled = false
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {Type: "anthropic", Model: "claude-sonnet-4-20250514"},
		"openai":    {Type: "openai", Model: "gpt-4o"},
	}

	runtime := &stubRuntimeInspector{models: []provider.Model{
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Provider: "anthropic", ContextWindow: 200000, SupportsTools: true},
		{ID: "gpt-4o", Name: "GPT-4o", Provider: "openai", ContextWindow: 128000, SupportsTools: true},
	}, authStatuses: map[string]*provider.AuthStatus{
		"anthropic": {Provider: "anthropic", Mode: "api_key", Source: "env:ANTHROPIC_API_KEY", HasAccessToken: true},
		"openai":    {Provider: "openai", Detail: "auth status unavailable"},
	}, health: map[string]*router.ProviderHealth{
		"anthropic": {Healthy: true},
		"openai":    {Healthy: true},
	}}

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConfigHandler(srv, cfg, runtime, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/providers")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result []struct {
		Name   string           `json:"name"`
		Models []provider.Model `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Sort for deterministic ordering.
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })

	if len(result) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(result))
	}

	// Anthropic should have only Claude models.
	if result[0].Name != "anthropic" {
		t.Fatalf("expected first provider = anthropic, got %q", result[0].Name)
	}
	if len(result[0].Models) != 1 || result[0].Models[0].ID != "claude-sonnet-4-20250514" {
		t.Fatalf("anthropic models: %+v", result[0].Models)
	}

	// OpenAI should have only GPT models.
	if result[1].Name != "openai" {
		t.Fatalf("expected second provider = openai, got %q", result[1].Name)
	}
	if len(result[1].Models) != 1 || result[1].Models[0].ID != "gpt-4o" {
		t.Fatalf("openai models: %+v", result[1].Models)
	}
}

func TestProvidersEndpointIncludesAuthAndHealth(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.Brain.Enabled = false
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {Type: "anthropic", Model: "claude-sonnet-4-20250514"},
		"openai":    {Type: "openai", Model: "gpt-4o"},
	}

	runtime := &stubRuntimeInspector{authStatuses: map[string]*provider.AuthStatus{
		"anthropic": {Provider: "anthropic", Mode: "api_key", Source: "env:ANTHROPIC_API_KEY", HasAccessToken: true},
	}, health: map[string]*router.ProviderHealth{
		"anthropic": {Healthy: true},
		"openai":    {Healthy: false, LastError: context.DeadlineExceeded},
	}}

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConfigHandler(srv, cfg, runtime, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/auth/providers")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body []struct {
		Name      string               `json:"name"`
		Healthy   bool                 `json:"healthy"`
		LastError string               `json:"last_error"`
		Auth      *provider.AuthStatus `json:"auth"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	sort.Slice(body, func(i, j int) bool { return body[i].Name < body[j].Name })
	if len(body) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(body))
	}
	if body[0].Name != "anthropic" || body[0].Auth == nil || body[0].Auth.Mode != "api_key" {
		t.Fatalf("unexpected anthropic auth payload: %+v", body[0])
	}
	if body[1].Name != "openai" || body[1].Healthy || body[1].LastError == "" {
		t.Fatalf("expected unhealthy openai payload, got %+v", body[1])
	}
}

func TestAuthProvidersEndpointMarksMissingRuntimeProviderUnavailable(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.Brain.Enabled = false
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {Type: "anthropic", Model: "claude-sonnet-4-20250514"},
		"codex":     {Type: "codex", Model: "gpt-5.1-codex-mini"},
	}

	runtime := &stubRuntimeInspector{authStatuses: map[string]*provider.AuthStatus{
		"codex": {Provider: "codex", Mode: "oauth", Source: "sirtopham_store", HasAccessToken: true},
	}, health: map[string]*router.ProviderHealth{
		"codex": {Healthy: true},
	}}

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConfigHandler(srv, cfg, runtime, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/auth/providers")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body []struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Healthy bool   `json:"healthy"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	sort.Slice(body, func(i, j int) bool { return body[i].Name < body[j].Name })
	if len(body) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(body))
	}
	if body[0].Name != "anthropic" || body[0].Healthy || body[0].Status != "unavailable" {
		t.Fatalf("expected missing runtime provider to be unavailable, got %+v", body[0])
	}
	if body[1].Name != "codex" || !body[1].Healthy || body[1].Status != "available" {
		t.Fatalf("expected registered codex provider to stay available, got %+v", body[1])
	}
}

func TestConfigEndpointGroupsModelsByProvider(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.Brain.Enabled = false
	cfg.Routing.Default.Provider = "anthropic"
	cfg.Routing.Default.Model = "claude-sonnet-4-20250514"
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {Type: "anthropic", Model: "claude-sonnet-4-20250514"},
		"openai":    {Type: "openai", Model: "gpt-4o"},
	}

	runtime := &stubRuntimeInspector{models: []provider.Model{
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Provider: "anthropic", ContextWindow: 200000},
		{ID: "gpt-4o", Name: "GPT-4o", Provider: "openai", ContextWindow: 128000},
	}, authStatuses: map[string]*provider.AuthStatus{
		"anthropic": {Provider: "anthropic", Mode: "oauth", Source: "credential_file", StorePath: "/tmp/.claude/.credentials.json", HasAccessToken: true, HasRefreshToken: true},
		"openai":    {Provider: "openai", Detail: "auth status unavailable"},
	}, health: map[string]*router.ProviderHealth{
		"anthropic": {Healthy: true},
		"openai":    {Healthy: false, LastError: context.DeadlineExceeded},
	}}

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConfigHandler(srv, cfg, runtime, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/config")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Providers []struct {
			Name   string   `json:"name"`
			Models []string `json:"models"`
		} `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	sort.Slice(body.Providers, func(i, j int) bool { return body.Providers[i].Name < body.Providers[j].Name })

	if len(body.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(body.Providers))
	}

	// Anthropic should only have claude model.
	if len(body.Providers[0].Models) != 1 || body.Providers[0].Models[0] != "claude-sonnet-4-20250514" {
		t.Fatalf("anthropic models in /api/config: %v", body.Providers[0].Models)
	}

	// OpenAI should only have gpt model.
	if len(body.Providers[1].Models) != 1 || body.Providers[1].Models[0] != "gpt-4o" {
		t.Fatalf("openai models in /api/config: %v", body.Providers[1].Models)
	}
}

func TestConfigEndpointUsesAvailableRuntimeModelsEvenIfAuthStatusesFail(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	cfg.Brain.Enabled = false
	cfg.Routing.Default.Provider = "anthropic"
	cfg.Routing.Default.Model = "config-default-model"
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {Type: "anthropic", Model: "config-anthropic-model"},
		"openai":    {Type: "openai", Model: "config-openai-model"},
	}

	runtime := &stubRuntimeInspector{
		models: []provider.Model{
			{ID: "runtime-anthropic-model", Provider: "anthropic"},
			{ID: "runtime-openai-model", Provider: "openai"},
		},
		authErr: errors.New("auth status backend unavailable"),
		health: map[string]*router.ProviderHealth{
			"anthropic": {Healthy: true},
			"openai":    {Healthy: false, LastError: context.DeadlineExceeded},
		},
	}

	srv := server.New(server.Config{Host: "127.0.0.1", Port: 0}, newTestLogger())
	server.NewConfigHandler(srv, cfg, runtime, newTestLogger())
	_, base := startServer(t, srv)

	resp, err := http.Get(base + "/api/config")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Providers []struct {
			Name    string   `json:"name"`
			Models  []string `json:"models"`
			Healthy bool     `json:"healthy"`
			Status  string   `json:"status"`
			Auth    any      `json:"auth"`
		} `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	sort.Slice(body.Providers, func(i, j int) bool { return body.Providers[i].Name < body.Providers[j].Name })
	if len(body.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(body.Providers))
	}
	if got := body.Providers[0].Models; len(got) != 1 || got[0] != "runtime-anthropic-model" {
		t.Fatalf("expected runtime anthropic models, got %v", got)
	}
	if got := body.Providers[1].Models; len(got) != 1 || got[0] != "runtime-openai-model" {
		t.Fatalf("expected runtime openai models, got %v", got)
	}
	if body.Providers[1].Healthy {
		t.Fatalf("expected unhealthy openai runtime health, got %+v", body.Providers[1])
	}
	if body.Providers[1].Status != "unavailable" {
		t.Fatalf("expected unavailable openai status, got %+v", body.Providers[1])
	}
	if body.Providers[0].Auth != nil || body.Providers[1].Auth != nil {
		t.Fatalf("expected auth payloads to be omitted when auth status lookup fails, got %+v", body.Providers)
	}
}
