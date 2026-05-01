package operator

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/chainrun"
)

func (s *Service) ListAgentRoles(ctx context.Context) ([]AgentRoleSummary, error) {
	_ = ctx
	cfg, err := s.config()
	if err != nil {
		return nil, err
	}
	roles := make([]AgentRoleSummary, 0, len(cfg.AgentRoles))
	for name := range cfg.AgentRoles {
		roles = append(roles, AgentRoleSummary{Name: name})
	}
	sort.Slice(roles, func(i int, j int) bool {
		return roles[i].Name < roles[j].Name
	})
	return roles, nil
}

func (s *Service) ValidateLaunch(ctx context.Context, req LaunchRequest) (LaunchPreview, error) {
	_ = ctx
	cfg, err := s.config()
	if err != nil {
		return LaunchPreview{}, err
	}
	req = normalizeLaunchRequest(req)
	if req.SourceTask == "" && len(req.SourceSpecs) == 0 {
		return LaunchPreview{}, fmt.Errorf("one of task or specs is required")
	}
	switch req.Mode {
	case LaunchModeOneStep:
		if req.Role == "" {
			return LaunchPreview{}, fmt.Errorf("role is required for one-step launch")
		}
		roleName, _, err := cfg.ResolveAgentRole(req.Role)
		if err != nil {
			return LaunchPreview{}, fmt.Errorf("resolve launch role: %w", err)
		}
		req.Role = roleName
	case LaunchModeOrchestrator:
		if _, ok := cfg.AgentRoles["orchestrator"]; !ok {
			return LaunchPreview{}, fmt.Errorf("agent role %q not found in config", "orchestrator")
		}
		req.Role = "orchestrator"
	default:
		return LaunchPreview{}, fmt.Errorf("unsupported launch mode %s", req.Mode)
	}
	compiled := compileLaunchTask(req)
	return LaunchPreview{
		Mode:         req.Mode,
		Role:         req.Role,
		Summary:      summarizeLaunch(req),
		CompiledTask: compiled,
		Warnings:     launchWarnings(req),
	}, nil
}

func (s *Service) StartChain(ctx context.Context, req LaunchRequest) (StartResult, error) {
	cfg, err := s.config()
	if err != nil {
		return StartResult{}, err
	}
	preview, err := s.ValidateLaunch(ctx, req)
	if err != nil {
		return StartResult{}, err
	}
	req = withLaunchDefaults(normalizeLaunchRequest(req))
	req.Mode = preview.Mode
	req.Role = preview.Role

	startOpts := chainrun.Options{
		Mode:             chainrun.Mode(req.Mode),
		Role:             req.Role,
		SourceSpecs:      append([]string(nil), req.SourceSpecs...),
		SourceTask:       req.SourceTask,
		MaxSteps:         req.MaxSteps,
		MaxResolverLoops: req.MaxResolverLoops,
		MaxDuration:      req.MaxDuration,
		TokenBudget:      req.TokenBudget,
	}
	chainIDCh := make(chan string, 1)
	startOpts.OnChainID = func(chainID string) {
		select {
		case chainIDCh <- chainID:
		default:
		}
	}
	doneCh := make(chan startChainDone, 1)
	starter := s.chainStarter
	if starter == nil {
		starter = chainrun.Start
	}
	processID := s.processID
	if processID == nil {
		processID = func() int { return 0 }
	}
	go func() {
		result, err := starter(context.WithoutCancel(ctx), cfg, startOpts, chainrun.Deps{BuildRuntime: s.buildRuntime, ProcessID: processID})
		if result != nil && result.ChainID != "" {
			select {
			case chainIDCh <- result.ChainID:
			default:
			}
		}
		doneCh <- startChainDone{Result: result, Err: err}
	}()

	select {
	case chainID := <-chainIDCh:
		return StartResult{ChainID: chainID, Status: "running", Preview: preview}, nil
	case done := <-doneCh:
		if done.Err != nil {
			return StartResult{}, done.Err
		}
		if done.Result == nil {
			return StartResult{}, fmt.Errorf("chain start returned no result")
		}
		return StartResult{ChainID: done.Result.ChainID, Status: done.Result.Status, Preview: preview}, nil
	case <-ctx.Done():
		return StartResult{}, ctx.Err()
	}
}

type startChainDone struct {
	Result *chainrun.Result
	Err    error
}

func normalizeLaunchRequest(req LaunchRequest) LaunchRequest {
	req.Role = strings.TrimSpace(req.Role)
	req.SourceTask = strings.TrimSpace(req.SourceTask)
	req.SourceSpecs = normalizeSourceSpecs(req.SourceSpecs)
	if req.Mode == "" {
		if req.Role != "" {
			req.Mode = LaunchModeOneStep
		} else {
			req.Mode = LaunchModeOrchestrator
		}
	}
	return req
}

func withLaunchDefaults(req LaunchRequest) LaunchRequest {
	if req.MaxSteps <= 0 {
		req.MaxSteps = 100
	}
	if req.MaxResolverLoops < 0 {
		req.MaxResolverLoops = 0
	} else if req.MaxResolverLoops == 0 {
		req.MaxResolverLoops = 3
	}
	if req.MaxDuration <= 0 {
		req.MaxDuration = 4 * time.Hour
	}
	if req.TokenBudget <= 0 {
		req.TokenBudget = 5_000_000
	}
	return req
}

func normalizeSourceSpecs(specs []string) []string {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(specs))
	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		if _, ok := seen[spec]; ok {
			continue
		}
		seen[spec] = struct{}{}
		normalized = append(normalized, spec)
	}
	return normalized
}

func compileLaunchTask(req LaunchRequest) string {
	var parts []string
	if req.SourceTask != "" {
		parts = append(parts, req.SourceTask)
	}
	if len(req.SourceSpecs) > 0 {
		parts = append(parts, "Specs: "+strings.Join(req.SourceSpecs, ", "))
	}
	return strings.Join(parts, "\n\n")
}

func summarizeLaunch(req LaunchRequest) string {
	switch req.Mode {
	case LaunchModeOneStep:
		return fmt.Sprintf("Run one %s step", req.Role)
	default:
		return "Run Sir Topham-managed orchestration"
	}
}

func launchWarnings(req LaunchRequest) []RuntimeWarning {
	var warnings []RuntimeWarning
	if len(req.SourceSpecs) == 0 {
		warnings = append(warnings, RuntimeWarning{Message: "no source specs selected"})
	}
	return warnings
}
