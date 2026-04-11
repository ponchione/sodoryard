package main

import (
	"context"
	"errors"
	"testing"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/provider"
)

type testAuthProvider struct {
	name       string
	authStatus *provider.AuthStatus
	authErr    error
	pingErr    error
	pingCalls  int
}

func (p *testAuthProvider) Name() string { return p.name }

func (p *testAuthProvider) Models(context.Context) ([]provider.Model, error) { return nil, nil }

func (p *testAuthProvider) Complete(context.Context, *provider.Request) (*provider.Response, error) {
	return nil, nil
}

func (p *testAuthProvider) Stream(context.Context, *provider.Request) (<-chan provider.StreamEvent, error) {
	return nil, nil
}

func (p *testAuthProvider) Ping(context.Context) error {
	p.pingCalls++
	return p.pingErr
}

func (p *testAuthProvider) AuthStatus(context.Context) (*provider.AuthStatus, error) {
	if p.authErr != nil {
		return nil, p.authErr
	}
	return p.authStatus, nil
}

func TestCollectProviderAuthReports_StatusSkipsPing(t *testing.T) {
	orig := buildProviderForAuthReports
	defer func() { buildProviderForAuthReports = orig }()

	providerStub := &testAuthProvider{
		name:       "codex",
		authStatus: &provider.AuthStatus{Provider: "codex", Mode: "oauth", Source: "codex_cli_store"},
		pingErr:    errors.New("ping should not run for auth status"),
	}
	buildProviderForAuthReports = func(name string, cfg appconfig.ProviderConfig) (provider.Provider, error) {
		return providerStub, nil
	}

	cfg := &appconfig.Config{Providers: map[string]appconfig.ProviderConfig{"codex": {Type: "codex"}}}
	reports := collectProviderAuthReports(context.Background(), cfg, false)
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if providerStub.pingCalls != 0 {
		t.Fatalf("expected no ping calls for auth status, got %d", providerStub.pingCalls)
	}
	if !reports[0].Healthy {
		t.Fatalf("expected read-only auth status to stay healthy, got %+v", reports[0])
	}
}

func TestCollectProviderAuthReports_UsesConfiguredProvidersForSurfaces(t *testing.T) {
	orig := buildProviderForAuthReports
	defer func() { buildProviderForAuthReports = orig }()

	providerStub := &testAuthProvider{
		name:       "codex",
		authStatus: &provider.AuthStatus{Provider: "codex", Mode: "oauth", Source: "sirtopham_store"},
	}
	buildProviderForAuthReports = func(name string, cfg appconfig.ProviderConfig) (provider.Provider, error) {
		if name != "codex" {
			t.Fatalf("buildProviderForAuthReports called for %q, want codex only", name)
		}
		return providerStub, nil
	}

	cfg := &appconfig.Config{
		ConfiguredProviders: []string{"codex"},
		Providers: map[string]appconfig.ProviderConfig{
			"anthropic":  {Type: "anthropic"},
			"openrouter": {Type: "openai-compatible"},
			"codex":      {Type: "codex"},
		},
	}
	reports := collectProviderAuthReports(context.Background(), cfg, false)
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].Name != "codex" {
		t.Fatalf("report name = %q, want codex", reports[0].Name)
	}
}

func TestCollectProviderAuthReports_DoctorRunsPing(t *testing.T) {
	orig := buildProviderForAuthReports
	defer func() { buildProviderForAuthReports = orig }()

	providerStub := &testAuthProvider{
		name:       "codex",
		authStatus: &provider.AuthStatus{Provider: "codex", Mode: "oauth", Source: "sirtopham_store"},
		pingErr:    errors.New("ping failed"),
	}
	buildProviderForAuthReports = func(name string, cfg appconfig.ProviderConfig) (provider.Provider, error) {
		return providerStub, nil
	}

	cfg := &appconfig.Config{Providers: map[string]appconfig.ProviderConfig{"codex": {Type: "codex"}}}
	reports := collectProviderAuthReports(context.Background(), cfg, true)
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if providerStub.pingCalls != 1 {
		t.Fatalf("expected ping call for doctor, got %d", providerStub.pingCalls)
	}
	if reports[0].Healthy {
		t.Fatalf("expected doctor report to reflect ping failure, got %+v", reports[0])
	}
	if reports[0].PingError == "" {
		t.Fatalf("expected ping error in doctor report, got %+v", reports[0])
	}
}
