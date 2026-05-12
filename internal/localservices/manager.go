package localservices

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	appconfig "github.com/ponchione/sodoryard/internal/config"
)

func (m *Manager) Status(ctx context.Context, cfg *appconfig.Config) (StackStatus, error) {
	status := StackStatus{
		Mode:          cfg.LocalServices.Mode,
		ComposeFile:   cfg.LocalServices.ComposeFile,
		ProjectDir:    cfg.LocalServices.ProjectDir,
		NetworkStatus: map[string]bool{},
	}
	if !cfg.LocalServices.Enabled {
		return status, nil
	}
	dockerAvailable, daemonAvailable, dockerProblem := probeDocker(ctx, m.runner)
	status.DockerAvailable = dockerAvailable
	status.DaemonAvailable = daemonAvailable
	if dockerProblem != "" {
		status.Problems = append(status.Problems, dockerProblem)
	}
	composeAvailable, composeProblem := probeCompose(ctx, m.runner)
	status.ComposeAvailable = composeAvailable
	if composeProblem != "" {
		status.Problems = append(status.Problems, composeProblem)
	}
	if info, err := os.Stat(cfg.LocalServices.ComposeFile); err == nil && !info.IsDir() {
		status.ComposeFileExists = true
	} else {
		status.Problems = append(status.Problems, fmt.Sprintf("compose file missing: %s", cfg.LocalServices.ComposeFile))
	}
	for _, network := range cfg.LocalServices.RequiredNetworks {
		present := dockerAvailable && daemonAvailable && networkExists(ctx, m.runner, network)
		status.NetworkStatus[network] = present
		if !present {
			status.Problems = append(status.Problems, fmt.Sprintf("required docker network missing: %s", network))
		}
	}
	for _, name := range ConfiguredServiceNames(cfg.LocalServices) {
		service := cfg.LocalServices.Services[name]
		probed := ProbeService(ctx, m.client, name, service)
		status.Services = append(status.Services, probed)
		if service.Required {
			status.RequiredServices = append(status.RequiredServices, name)
			if !probed.Healthy {
				status.Problems = append(status.Problems, fmt.Sprintf("required service %s unhealthy: %s", name, strings.TrimSpace(probed.Detail)))
			}
		}
	}
	status.Remediation = remediationLines(cfg, status)
	return status, nil
}

func (m *Manager) EnsureUp(ctx context.Context, cfg *appconfig.Config) (StackStatus, error) {
	status, err := m.Status(ctx, cfg)
	if err != nil {
		return status, err
	}
	if status.AllRequiredHealthy() {
		return status, nil
	}
	switch cfg.LocalServices.Mode {
	case "off", "manual":
		return status, &ManagerError{Op: "ensure-up", Status: status}
	}
	if !status.DockerAvailable || !status.DaemonAvailable || !status.ComposeAvailable || !status.ComposeFileExists {
		return status, &ManagerError{Op: "ensure-up", Status: status}
	}
	if cfg.LocalServices.AutoCreateNetworks {
		for _, network := range cfg.LocalServices.RequiredNetworks {
			if err := ensureNetwork(ctx, m.runner, network); err != nil {
				status.Problems = append(status.Problems, err.Error())
				status.Remediation = remediationLines(cfg, status)
				return status, &ManagerError{Op: "ensure-up", Status: status}
			}
		}
	}
	if err := composeUp(ctx, m.runner, cfg.LocalServices.ProjectDir, cfg.LocalServices.ComposeFile, status.RequiredServices); err != nil {
		status.Problems = append(status.Problems, err.Error())
		status.Remediation = remediationLines(cfg, status)
		return status, &ManagerError{Op: "ensure-up", Status: status}
	}
	timeout := time.Duration(cfg.LocalServices.StartupTimeoutSeconds) * time.Second
	deadline := time.Now().Add(timeout)
	for {
		status, err = m.Status(ctx, cfg)
		if err != nil {
			return status, err
		}
		if status.AllRequiredHealthy() {
			return status, nil
		}
		if time.Now().After(deadline) {
			status.Problems = append(status.Problems, fmt.Sprintf("timed out waiting for services after %s", timeout))
			status.Remediation = remediationLines(cfg, status)
			return status, &ManagerError{Op: "ensure-up", Status: status}
		}
		m.sleep(time.Duration(cfg.LocalServices.HealthcheckIntervalSeconds) * time.Second)
	}
}

func (m *Manager) Down(ctx context.Context, cfg *appconfig.Config) error {
	return composeDown(ctx, m.runner, cfg.LocalServices.ProjectDir, cfg.LocalServices.ComposeFile)
}

func (m *Manager) Logs(ctx context.Context, cfg *appconfig.Config, tail int) (string, error) {
	return composeLogs(ctx, m.runner, cfg.LocalServices.ProjectDir, cfg.LocalServices.ComposeFile, tail, ConfiguredServiceNames(cfg.LocalServices))
}

func remediationLines(cfg *appconfig.Config, status StackStatus) []string {
	if len(status.Problems) == 0 {
		return nil
	}
	lines := []string{}
	if !status.ComposeFileExists {
		lines = append(lines, fmt.Sprintf("verify compose file exists: %s", cfg.LocalServices.ComposeFile))
	}
	lines = append(lines,
		"inspect stack status: yard llm status",
		"inspect stack logs: yard llm logs",
		fmt.Sprintf("start stack manually: docker compose -f %s up -d", cfg.LocalServices.ComposeFile),
	)
	return lines
}
