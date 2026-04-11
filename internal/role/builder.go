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
)

type BuilderDeps struct {
	BrainBackend     brain.Backend
	BrainSearcher    appcontext.BrainSearcher
	SemanticSearcher tool.SemanticSearcher
	ProviderRuntime  provider.Provider
	Queries          *appdb.Queries
	ProjectID        string
}

func BuildRegistry(cfg *appconfig.Config, roleCfg appconfig.AgentRoleConfig, deps BuilderDeps) (*tool.Registry, appconfig.BrainConfig, error) {
	if cfg == nil {
		return nil, appconfig.BrainConfig{}, fmt.Errorf("role builder: config is required")
	}
	if len(roleCfg.CustomTools) > 0 {
		return nil, appconfig.BrainConfig{}, fmt.Errorf("role builder: custom_tools are not implemented by SirTopham and must be provided by the external orchestrator")
	}

	brainCfg := cfg.Brain
	brainCfg.BrainWritePaths = append([]string(nil), roleCfg.BrainWritePaths...)
	brainCfg.BrainDenyPaths = append([]string(nil), roleCfg.BrainDenyPaths...)

	registry := tool.NewRegistry()
	for _, group := range roleCfg.Tools {
		switch strings.TrimSpace(group) {
		case "brain":
			tool.RegisterBrainToolsWithProviderRuntimeAndIndex(registry, deps.BrainBackend, deps.BrainSearcher, brainCfg, deps.ProviderRuntime, deps.Queries, deps.ProjectID)
		case "file":
			tool.RegisterFileTools(registry)
		case "file:read":
			tool.RegisterFileReadTools(registry)
		case "git":
			tool.RegisterGitTools(registry)
		case "shell":
			tool.RegisterShellTool(registry, tool.ShellConfig{
				TimeoutSeconds: cfg.Agent.ShellTimeoutSeconds,
				Denylist:       cfg.Agent.ShellDenylist,
			})
		case "search":
			tool.RegisterSearchTools(registry, deps.SemanticSearcher)
		case "":
			continue
		default:
			return nil, appconfig.BrainConfig{}, fmt.Errorf("role builder: unsupported tool group %q", group)
		}
	}

	return registry, brainCfg, nil
}
