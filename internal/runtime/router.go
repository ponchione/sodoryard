package runtime

import (
	"context"
	"fmt"
	"log/slog"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	"github.com/ponchione/sodoryard/internal/provider/router"
	"github.com/ponchione/sodoryard/internal/provider/tracking"
)

type ProviderRouterOptions struct {
	ProviderNames []string
	LogAuthStatus bool
	MemoryBackend any
}

func BuildProviderRouter(ctx context.Context, cfg *appconfig.Config, queries *appdb.Queries, logger *slog.Logger, opts ProviderRouterOptions) (*router.Router, error) {
	if cfg == nil {
		return nil, fmt.Errorf("runtime config is required")
	}
	routerCfg := router.RouterConfig{
		Default:  router.RouteTarget{Provider: cfg.Routing.Default.Provider, Model: cfg.Routing.Default.Model},
		Fallback: router.RouteTarget{Provider: cfg.Routing.Fallback.Provider, Model: cfg.Routing.Fallback.Model},
	}
	subCallStore, err := buildSubCallStore(cfg, queries, opts.MemoryBackend)
	if err != nil {
		return nil, err
	}
	provRouter, err := router.NewRouter(routerCfg, subCallStore, logger)
	if err != nil {
		return nil, fmt.Errorf("create router: %w", err)
	}

	providerNames := opts.ProviderNames
	if len(providerNames) == 0 {
		providerNames = providerMapNames(cfg.Providers)
	}
	for _, name := range providerNames {
		provCfg, ok := cfg.Providers[name]
		if !ok {
			continue
		}
		p, err := BuildProvider(name, provCfg)
		if err != nil {
			return nil, fmt.Errorf("build provider %q: %w", name, err)
		}
		if err := provRouter.RegisterProvider(p); err != nil {
			return nil, fmt.Errorf("register provider %q: %w", name, err)
		}
		if opts.LogAuthStatus {
			LogProviderAuthStatus(ctx, logger, name, provCfg, p)
		}
	}
	if err := provRouter.Validate(ctx); err != nil {
		return nil, fmt.Errorf("validate providers: %w", err)
	}
	return provRouter, nil
}

func buildSubCallStore(cfg *appconfig.Config, queries *appdb.Queries, memoryBackend any) (tracking.SubCallStore, error) {
	if cfg != nil && cfg.Memory.Backend == "shunter" {
		if recorder, ok := memoryBackend.(projectmemory.SubCallRecorder); ok && recorder != nil {
			return tracking.NewProjectMemorySubCallStore(recorder), nil
		}
		return nil, fmt.Errorf("shunter memory backend requires a project memory sub-call recorder")
	}
	return tracking.NewSQLiteSubCallStore(queries), nil
}

func providerMapNames(providers map[string]appconfig.ProviderConfig) []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}
