package main

import (
	"context"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/ponchione/sodoryard/internal/tool"
)

// orchestratorRuntime is a local alias for the extracted OrchestratorRuntime.
// Command files reference this type so we keep the thin wrapper.
type orchestratorRuntime = rtpkg.OrchestratorRuntime

func buildOrchestratorRuntime(ctx context.Context, cfg *appconfig.Config) (*orchestratorRuntime, error) {
	return rtpkg.BuildOrchestratorRuntime(ctx, cfg)
}

func buildOrchestratorRegistry(rt *orchestratorRuntime, roleCfg appconfig.AgentRoleConfig, chainID string) (*tool.Registry, error) {
	return rtpkg.BuildOrchestratorRegistry(rt, roleCfg, chainID)
}
