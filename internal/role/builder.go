package role

import (
	"fmt"
	"strings"

	"github.com/ponchione/sodoryard/internal/brain"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	appcontext "github.com/ponchione/sodoryard/internal/context"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/provider"
	"github.com/ponchione/sodoryard/internal/tool"
	"github.com/ponchione/sodoryard/internal/toolgroup"
)

type BuilderDeps struct {
	BrainBackend      brain.Backend
	BrainSearcher     appcontext.BrainSearcher
	SemanticSearcher  tool.SemanticSearcher
	ProviderRuntime   provider.Provider
	Queries           *appdb.Queries
	ProjectID         string
	CustomToolFactory map[string]func() tool.Tool
}

type toolGroupRegistrar func(*tool.Registry, *appconfig.Config, *appconfig.BrainConfig, BuilderDeps)

var toolGroupRegistrars = map[string]toolGroupRegistrar{
	toolgroup.Brain: func(registry *tool.Registry, _ *appconfig.Config, brainCfg *appconfig.BrainConfig, deps BuilderDeps) {
		tool.RegisterBrainToolsWithProviderRuntimeAndIndex(registry, deps.BrainBackend, deps.BrainSearcher, *brainCfg, deps.ProviderRuntime, deps.Queries, deps.ProjectID)
	},
	toolgroup.File: func(registry *tool.Registry, _ *appconfig.Config, _ *appconfig.BrainConfig, _ BuilderDeps) {
		tool.RegisterFileTools(registry)
	},
	toolgroup.FileRead: func(registry *tool.Registry, _ *appconfig.Config, _ *appconfig.BrainConfig, _ BuilderDeps) {
		tool.RegisterFileReadTools(registry)
	},
	toolgroup.Git: func(registry *tool.Registry, _ *appconfig.Config, _ *appconfig.BrainConfig, _ BuilderDeps) {
		tool.RegisterGitTools(registry)
	},
	toolgroup.Shell: func(registry *tool.Registry, cfg *appconfig.Config, _ *appconfig.BrainConfig, _ BuilderDeps) {
		tool.RegisterShellTool(registry, tool.ShellConfig{
			TimeoutSeconds: cfg.Agent.ShellTimeoutSeconds,
			Denylist:       cfg.Agent.ShellDenylist,
		})
	},
	toolgroup.Search: func(registry *tool.Registry, _ *appconfig.Config, _ *appconfig.BrainConfig, deps BuilderDeps) {
		tool.RegisterSearchTools(registry, deps.SemanticSearcher)
	},
	toolgroup.Directory: func(registry *tool.Registry, _ *appconfig.Config, _ *appconfig.BrainConfig, _ BuilderDeps) {
		tool.RegisterDirectoryTools(registry)
	},
	toolgroup.Test: func(registry *tool.Registry, _ *appconfig.Config, _ *appconfig.BrainConfig, _ BuilderDeps) {
		tool.RegisterTestTool(registry)
	},
	toolgroup.SQLC: func(registry *tool.Registry, _ *appconfig.Config, _ *appconfig.BrainConfig, _ BuilderDeps) {
		tool.RegisterSqlcTool(registry)
	},
}

func BuildRegistry(cfg *appconfig.Config, roleCfg appconfig.AgentRoleConfig, deps BuilderDeps) (*tool.Registry, appconfig.BrainConfig, error) {
	if cfg == nil {
		return nil, appconfig.BrainConfig{}, fmt.Errorf("role builder: config is required")
	}

	brainCfg := cfg.Brain
	brainCfg.BrainWritePaths = append([]string(nil), roleCfg.BrainWritePaths...)
	brainCfg.BrainDenyPaths = append([]string(nil), roleCfg.BrainDenyPaths...)

	registry := tool.NewRegistry()
	for _, group := range roleCfg.Tools {
		name := strings.TrimSpace(group)
		if name == "" {
			continue
		}
		registrar, ok := toolGroupRegistrars[name]
		if !ok {
			return nil, appconfig.BrainConfig{}, fmt.Errorf("role builder: unsupported tool group %q", group)
		}
		registrar(registry, cfg, &brainCfg, deps)
	}

	if len(roleCfg.CustomTools) > 0 {
		if deps.CustomToolFactory == nil {
			return nil, appconfig.BrainConfig{}, fmt.Errorf("role builder: custom_tools are not implemented: caller did not provide a CustomToolFactory")
		}
		for _, name := range roleCfg.CustomTools {
			ctor, ok := deps.CustomToolFactory[name]
			if !ok {
				return nil, appconfig.BrainConfig{}, fmt.Errorf("role builder: custom tool %q not provided by factory", name)
			}
			registry.Register(ctor())
		}
	}

	return registry, brainCfg, nil
}
