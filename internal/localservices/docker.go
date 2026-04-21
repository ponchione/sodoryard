package localservices

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type shellRunner struct{}

func (shellRunner) Run(ctx context.Context, name string, args []string, dir string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	stdout, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(stdout)), "", nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return strings.TrimSpace(string(stdout)), strings.TrimSpace(string(exitErr.Stderr)), err
	}
	return strings.TrimSpace(string(stdout)), "", err
}

func probeDocker(ctx context.Context, runner CommandRunner) (dockerAvailable, daemonAvailable bool, problem string) {
	if runner == nil {
		runner = shellRunner{}
	}
	if _, _, err := runner.Run(ctx, "docker", []string{"version", "--format", "{{.Client.Version}}"}, ""); err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return false, false, "docker executable not found"
		}
		return false, false, fmt.Sprintf("docker unavailable: %v", err)
	}
	if _, stderr, err := runner.Run(ctx, "docker", []string{"info"}, ""); err != nil {
		if strings.TrimSpace(stderr) != "" {
			return true, false, fmt.Sprintf("docker daemon unavailable: %s", strings.TrimSpace(stderr))
		}
		return true, false, fmt.Sprintf("docker daemon unavailable: %v", err)
	}
	return true, true, ""
}

func probeCompose(ctx context.Context, runner CommandRunner) (bool, string) {
	if runner == nil {
		runner = shellRunner{}
	}
	_, stderr, err := runner.Run(ctx, "docker", []string{"compose", "version"}, "")
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return false, fmt.Sprintf("docker compose unavailable: %s", strings.TrimSpace(stderr))
		}
		return false, fmt.Sprintf("docker compose unavailable: %v", err)
	}
	return true, ""
}

func networkExists(ctx context.Context, runner CommandRunner, network string) bool {
	if runner == nil {
		runner = shellRunner{}
	}
	_, _, err := runner.Run(ctx, "docker", []string{"network", "inspect", network}, "")
	return err == nil
}

func ensureNetwork(ctx context.Context, runner CommandRunner, network string) error {
	if networkExists(ctx, runner, network) {
		return nil
	}
	_, stderr, err := runner.Run(ctx, "docker", []string{"network", "create", network}, "")
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return fmt.Errorf("create docker network %s: %s", network, strings.TrimSpace(stderr))
		}
		return fmt.Errorf("create docker network %s: %w", network, err)
	}
	return nil
}

func composeUp(ctx context.Context, runner CommandRunner, projectDir, composeFile string, services []string) error {
	args := []string{"compose", "-f", composeFile, "up", "-d"}
	args = append(args, services...)
	_, stderr, err := runner.Run(ctx, "docker", args, projectDir)
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return fmt.Errorf("docker compose up failed: %s", normalizeComposeUpError(strings.TrimSpace(stderr)))
		}
		return fmt.Errorf("docker compose up failed: %w", err)
	}
	return nil
}

func normalizeComposeUpError(stderr string) string {
	trimmed := strings.TrimSpace(stderr)
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "container name") && strings.Contains(lower, "already in use") {
		return trimmed + " | remediation: remove the conflicting container (for example `docker rm -f <name>`) or run `yard llm down` if it belongs to this repo-owned stack. If this happens across multiple repo-owned stacks, remove explicit container_name entries or rely on compose project scoping instead."
	}
	return trimmed
}

func composeDown(ctx context.Context, runner CommandRunner, projectDir, composeFile string) error {
	_, stderr, err := runner.Run(ctx, "docker", []string{"compose", "-f", composeFile, "down"}, projectDir)
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return fmt.Errorf("docker compose down failed: %s", strings.TrimSpace(stderr))
		}
		return fmt.Errorf("docker compose down failed: %w", err)
	}
	return nil
}

func composeLogs(ctx context.Context, runner CommandRunner, projectDir, composeFile string, tail int, services []string) (string, error) {
	args := []string{"compose", "-f", composeFile, "logs", "--tail", fmt.Sprintf("%d", tail)}
	args = append(args, services...)
	stdout, stderr, err := runner.Run(ctx, "docker", args, projectDir)
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return "", fmt.Errorf("docker compose logs failed: %s", strings.TrimSpace(stderr))
		}
		return "", fmt.Errorf("docker compose logs failed: %w", err)
	}
	return stdout, nil
}
