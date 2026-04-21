package cmdutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/localservices"
	"github.com/ponchione/sodoryard/internal/provider"
)

type fakeProviderBuilder func(name string, cfg appconfig.ProviderConfig) (provider.Provider, error)

func (f fakeProviderBuilder) Build(name string, cfg appconfig.ProviderConfig) (provider.Provider, error) {
	return f(name, cfg)
}

type fakeAuthProvider struct {
	name       string
	authStatus *provider.AuthStatus
	authErr    error
	pingErr    error
}

func (f *fakeAuthProvider) Name() string { return f.name }
func (f *fakeAuthProvider) Complete(ctx context.Context, req *provider.Request) (*provider.Response, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeAuthProvider) Stream(ctx context.Context, req *provider.Request) (<-chan provider.StreamEvent, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeAuthProvider) Models(ctx context.Context) ([]provider.Model, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeAuthProvider) AuthStatus(ctx context.Context) (*provider.AuthStatus, error) {
	return f.authStatus, f.authErr
}
func (f *fakeAuthProvider) Ping(ctx context.Context) error {
	return f.pingErr
}

func TestRunProviderDiagnosticsJSONIncludesLocalServicesForDoctor(t *testing.T) {
	configPath := writeLLMConfig(t, strings.Join([]string{
		"  enabled: true",
		"  mode: auto",
	}, "\n"))
	manager := &fakeLocalServicesManager{
		statusResult: localservices.StackStatus{Mode: "auto", Problems: []string{"compose missing"}},
	}
	builder := fakeProviderBuilder(func(name string, cfg appconfig.ProviderConfig) (provider.Provider, error) {
		return &fakeAuthProvider{
			name: name,
			authStatus: &provider.AuthStatus{
				Provider:       name,
				Mode:           "oauth",
				Source:         "codex auth",
				HasAccessToken: true,
				ExpiresAt:      time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
			},
		}, nil
	})
	var out bytes.Buffer

	if err := RunProviderDiagnostics(context.Background(), &out, configPath, true, true, manager, builder.Build); err != nil {
		t.Fatalf("RunProviderDiagnostics returned error: %v", err)
	}
	if manager.statusCalls != 1 {
		t.Fatalf("statusCalls = %d, want 1", manager.statusCalls)
	}

	var payload struct {
		Providers     []ProviderAuthReport        `json:"providers"`
		LocalServices localservices.StackStatus   `json:"local_services"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v\noutput=%s", err, out.String())
	}
	if len(payload.Providers) != 1 {
		t.Fatalf("providers len = %d, want 1", len(payload.Providers))
	}
	if payload.Providers[0].Name != "codex" || payload.Providers[0].Auth == nil || payload.Providers[0].Auth.Mode != "oauth" {
		t.Fatalf("provider payload = %#v, want codex auth report", payload.Providers[0])
	}
	if payload.LocalServices.Mode != "auto" {
		t.Fatalf("local_services.mode = %q, want auto", payload.LocalServices.Mode)
	}
}

func TestCollectProviderAuthReportsCapturesBuildAndPingFailures(t *testing.T) {
	cfg := &appconfig.Config{
		ConfiguredProviders: []string{"broken", "codex"},
		Providers: map[string]appconfig.ProviderConfig{
			"broken": {Type: "broken"},
			"codex":  {Type: "codex"},
		},
	}
	builder := fakeProviderBuilder(func(name string, cfg appconfig.ProviderConfig) (provider.Provider, error) {
		if name == "broken" {
			return nil, errors.New("build exploded")
		}
		return &fakeAuthProvider{
			name:    name,
			pingErr: errors.New("ping failed"),
		}, nil
	})

	reports := CollectProviderAuthReports(context.Background(), cfg, true, builder.Build)
	if len(reports) != 2 {
		t.Fatalf("reports len = %d, want 2", len(reports))
	}
	if reports[0].Name != "broken" || reports[0].Healthy || reports[0].BuildError != "build exploded" {
		t.Fatalf("broken report = %#v", reports[0])
	}
	if reports[1].Name != "codex" || reports[1].Healthy || reports[1].PingError != "ping failed" {
		t.Fatalf("codex report = %#v", reports[1])
	}
	if reports[1].Auth == nil || reports[1].Auth.Detail != "ping failed" {
		t.Fatalf("codex auth = %#v, want ping detail fallback", reports[1].Auth)
	}
}

func TestPrintProviderAuthReportsRendersReadableText(t *testing.T) {
	var out bytes.Buffer
	reports := []ProviderAuthReport{{
		Name:    "codex",
		Type:    "codex",
		Healthy: false,
		PingError: "token expired",
		Auth: &provider.AuthStatus{
			Mode:            "oauth",
			Source:          "codex auth",
			HasAccessToken:  true,
			HasRefreshToken: false,
			Detail:          "expired",
			Remediation:     "run codex refresh",
		},
	}}

	PrintProviderAuthReports(&out, reports)
	got := out.String()
	for _, want := range []string{
		"codex (codex): unhealthy",
		"  ping_error: token expired",
		"  auth_mode: oauth",
		"  has_access_token: true",
		"  remediation: run codex refresh",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q in %q", want, got)
		}
	}
}
