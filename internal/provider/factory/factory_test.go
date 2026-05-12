package factory

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/provider"
)

type fakeFactory struct {
	typeName string
	err      error
}

func (f fakeFactory) Type() string { return f.typeName }

func (f fakeFactory) Build(context.Context, string, appconfig.ProviderConfig, Deps) (provider.Provider, error) {
	if f.err != nil {
		return nil, f.err
	}
	return fakeProvider{name: f.typeName}, nil
}

type fakeProvider struct {
	name string
}

func (p fakeProvider) Complete(context.Context, *provider.Request) (*provider.Response, error) {
	return nil, nil
}

func (p fakeProvider) Stream(context.Context, *provider.Request) (<-chan provider.StreamEvent, error) {
	return nil, nil
}

func (p fakeProvider) Models(context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "fake", Name: "fake"}}, nil
}

func (p fakeProvider) Name() string { return p.name }

func (p fakeProvider) AuthStatus(context.Context) (*provider.AuthStatus, error) {
	return &provider.AuthStatus{Provider: p.name}, nil
}

func (p fakeProvider) Ping(context.Context) error { return nil }

func TestDefaultRegistryTypes(t *testing.T) {
	got := DefaultRegistry().Types()
	want := []string{"anthropic", "codex", "openai-compatible"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Types = %v, want %v", got, want)
	}
}

func TestRegistryRejectsDuplicateTypes(t *testing.T) {
	_, err := NewRegistry(fakeFactory{typeName: "fake"}, fakeFactory{typeName: "fake"})
	if err == nil || !strings.Contains(err.Error(), `duplicate provider factory type "fake"`) {
		t.Fatalf("NewRegistry duplicate error = %v, want duplicate type", err)
	}
}

func TestRegistryBuildsRegisteredFactory(t *testing.T) {
	r, err := NewRegistry(fakeFactory{typeName: "fake"})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}
	p, err := r.Build(context.Background(), "fake", appconfig.ProviderConfig{Type: "fake"}, Deps{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if p == nil || p.Name() != "fake" {
		t.Fatalf("provider = %#v, want fake provider", p)
	}
}

func TestRegistryBuildRejectsUnknownType(t *testing.T) {
	_, err := DefaultRegistry().Build(context.Background(), "missing", appconfig.ProviderConfig{Type: "missing"}, Deps{})
	if err == nil || !strings.Contains(err.Error(), `unsupported provider type: "missing"`) {
		t.Fatalf("Build unknown error = %v, want unsupported provider type", err)
	}
}

func TestRegistryBuildPropagatesFactoryErrors(t *testing.T) {
	wantErr := errors.New("build failed")
	r, err := NewRegistry(fakeFactory{typeName: "fake", err: wantErr})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}
	_, err = r.Build(context.Background(), "fake", appconfig.ProviderConfig{Type: "fake"}, Deps{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Build error = %v, want %v", err, wantErr)
	}
}

func TestDefaultRegistryBuildsAliasedCodexProvider(t *testing.T) {
	p, err := DefaultRegistry().Build(context.Background(), "codex-alt", appconfig.ProviderConfig{Type: "codex"}, Deps{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if p.Name() != "codex-alt" {
		t.Fatalf("Name = %q, want codex-alt", p.Name())
	}
}

func TestWithProviderAliasDelegatesOptionalInterfaces(t *testing.T) {
	aliased := WithProviderAlias("outer", fakeProvider{name: "inner"})
	if aliased.Name() != "outer" {
		t.Fatalf("Name = %q, want outer", aliased.Name())
	}
	if err := aliased.(provider.Pinger).Ping(context.Background()); err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}
	status, err := aliased.(provider.AuthStatusReporter).AuthStatus(context.Background())
	if err != nil {
		t.Fatalf("AuthStatus returned error: %v", err)
	}
	if status.Provider != "outer" {
		t.Fatalf("AuthStatus provider = %q, want outer", status.Provider)
	}
}
