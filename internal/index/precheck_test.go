package index

import (
	"context"
	"strings"
	"testing"

	appconfig "github.com/ponchione/sodoryard/internal/config"
)

func TestRunIndexPrecheckPassesWhenLocalServicesDisabled(t *testing.T) {
	cfg := appconfig.Default()
	cfg.Brain.Enabled = false
	cfg.LocalServices.Enabled = false
	if err := runIndexPrecheck(context.Background(), cfg); err != nil {
		t.Fatalf("runIndexPrecheck: %v", err)
	}
}

func TestRunIndexPrecheckFailsInManualModeWhenServicesMissing(t *testing.T) {
	cfg := appconfig.Default()
	cfg.Brain.Enabled = false
	cfg.LocalServices.Mode = "manual"
	cfg.LocalServices.Services["qwen-coder"] = appconfig.ManagedService{BaseURL: "http://127.0.0.1:1", HealthPath: "/health", ModelsPath: "/v1/models", Required: true}
	cfg.LocalServices.Services["nomic-embed"] = appconfig.ManagedService{BaseURL: "http://127.0.0.1:2", HealthPath: "/health", ModelsPath: "/v1/models", Required: true}
	err := runIndexPrecheck(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"index precheck", "local services ensure-up failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want substring %q", err, want)
		}
	}
}
