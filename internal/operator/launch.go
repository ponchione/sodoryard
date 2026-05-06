package operator

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/ponchione/sodoryard/internal/chaininput"
	"github.com/ponchione/sodoryard/internal/chainrun"
	appconfig "github.com/ponchione/sodoryard/internal/config"
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
	case LaunchModeConstrained:
		if _, ok := cfg.AgentRoles["orchestrator"]; !ok {
			return LaunchPreview{}, fmt.Errorf("agent role %q not found in config", "orchestrator")
		}
		allowedRoles, err := resolveLaunchAllowedRoles(cfg, req)
		if err != nil {
			return LaunchPreview{}, err
		}
		req.AllowedRoles = allowedRoles
		req.Role = "orchestrator"
	case LaunchModeManualRoster:
		roster, err := resolveLaunchRoster(cfg, req)
		if err != nil {
			return LaunchPreview{}, err
		}
		req.Roster = roster
		req.Role = strings.Join(roster, ",")
	default:
		return LaunchPreview{}, fmt.Errorf("unsupported launch mode %s", req.Mode)
	}
	compiled := compileLaunchTask(req)
	return LaunchPreview{
		Mode:         req.Mode,
		Role:         req.Role,
		AllowedRoles: append([]string(nil), req.AllowedRoles...),
		Roster:       append([]string(nil), req.Roster...),
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
	req.AllowedRoles = append([]string(nil), preview.AllowedRoles...)
	req.Roster = append([]string(nil), preview.Roster...)

	startOpts := chainrun.Options{
		Mode:             chainrun.Mode(req.Mode),
		Role:             req.Role,
		AllowedRoles:     append([]string(nil), req.AllowedRoles...),
		Roster:           chainrunRoster(req.Roster),
		SourceSpecs:      append([]string(nil), req.SourceSpecs...),
		SourceTask:       req.SourceTask,
		MaxSteps:         req.MaxSteps,
		MaxResolverLoops: req.MaxResolverLoops,
		MaxDuration:      req.MaxDuration,
		TokenBudget:      req.TokenBudget,
	}
	chainIDCh := make(chan string, 1)
	doneCh := make(chan startChainDone, 1)
	runnerCtx, runnerCancel := context.WithCancel(context.WithoutCancel(ctx))
	var startedMu sync.Mutex
	startedChainID := ""
	setStartedChainID := func(chainID string) {
		startedMu.Lock()
		startedChainID = chainID
		startedMu.Unlock()
	}
	getStartedChainID := func() string {
		startedMu.Lock()
		defer startedMu.Unlock()
		return startedChainID
	}
	startOpts.OnChainID = func(chainID string) {
		setStartedChainID(chainID)
		s.registerActiveStart(chainID, runnerCancel, doneCh)
		select {
		case chainIDCh <- chainID:
		default:
		}
	}
	starter := s.chainStarter
	if starter == nil {
		starter = chainrun.Start
	}
	go func() {
		defer func() {
			if chainID := getStartedChainID(); chainID != "" {
				s.unregisterActiveStart(chainID)
			}
		}()
		result, err := starter(runnerCtx, cfg, startOpts, chainrun.Deps{BuildRuntime: s.buildRuntime, ProcessID: func() int { return 0 }})
		if result != nil && result.ChainID != "" {
			setStartedChainID(result.ChainID)
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
		runnerCancel()
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
	req.SourceSpecs = chaininput.NormalizeSpecs(req.SourceSpecs)
	req.AllowedRoles = chaininput.NormalizeRoleSet(req.AllowedRoles)
	req.Roster = chaininput.NormalizeRoleList(req.Roster)
	if req.Mode == "" {
		if len(req.Roster) > 0 {
			req.Mode = LaunchModeManualRoster
		} else if len(req.AllowedRoles) > 0 {
			req.Mode = LaunchModeConstrained
		} else if req.Role != "" {
			req.Mode = LaunchModeOneStep
		} else {
			req.Mode = LaunchModeOrchestrator
		}
	}
	return req
}

func withLaunchDefaults(req LaunchRequest) LaunchRequest {
	limits := chaininput.NormalizeLimits(chaininput.Limits{
		MaxSteps:         req.MaxSteps,
		MaxResolverLoops: req.MaxResolverLoops,
		MaxDuration:      req.MaxDuration,
		TokenBudget:      req.TokenBudget,
	})
	req.MaxSteps = limits.MaxSteps
	req.MaxResolverLoops = limits.MaxResolverLoops
	req.MaxDuration = limits.MaxDuration
	req.TokenBudget = limits.TokenBudget
	return req
}

func compileLaunchTask(req LaunchRequest) string {
	var parts []string
	if req.SourceTask != "" {
		parts = append(parts, req.SourceTask)
	}
	if len(req.SourceSpecs) > 0 {
		parts = append(parts, "Specs: "+strings.Join(req.SourceSpecs, ", "))
	}
	if req.Mode == LaunchModeConstrained && len(req.AllowedRoles) > 0 {
		parts = append(parts, "Allowed roles: "+strings.Join(req.AllowedRoles, ", "))
	}
	return strings.Join(parts, "\n\n")
}

func summarizeLaunch(req LaunchRequest) string {
	switch req.Mode {
	case LaunchModeOneStep:
		return fmt.Sprintf("Run one %s step", req.Role)
	case LaunchModeManualRoster:
		return fmt.Sprintf("Run manual roster: %s", strings.Join(req.Roster, " -> "))
	case LaunchModeConstrained:
		return fmt.Sprintf("Run constrained orchestration with roles: %s", strings.Join(req.AllowedRoles, ", "))
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

func resolveLaunchRoster(cfg *appconfig.Config, req LaunchRequest) ([]string, error) {
	roster := chaininput.NormalizeRoleList(req.Roster)
	if len(roster) == 0 && strings.TrimSpace(req.Role) != "" {
		roster = chaininput.ParseRoleList(req.Role)
	}
	if len(roster) == 0 {
		return nil, fmt.Errorf("manual roster requires at least one role")
	}
	for i := range roster {
		roleName, _, err := cfg.ResolveAgentRole(roster[i])
		if err != nil {
			return nil, fmt.Errorf("resolve roster role %d: %w", i+1, err)
		}
		roster[i] = roleName
	}
	return roster, nil
}

func resolveLaunchAllowedRoles(cfg *appconfig.Config, req LaunchRequest) ([]string, error) {
	roles := chaininput.NormalizeRoleSet(req.AllowedRoles)
	if len(roles) == 0 && strings.TrimSpace(req.Role) != "" && req.Role != "orchestrator" {
		roles = chaininput.ParseRoleSet(req.Role)
	}
	if len(roles) == 0 {
		return nil, fmt.Errorf("constrained orchestration requires at least one allowed role")
	}
	for i := range roles {
		roleName, _, err := cfg.ResolveAgentRole(roles[i])
		if err != nil {
			return nil, fmt.Errorf("resolve constrained role %d: %w", i+1, err)
		}
		roles[i] = roleName
	}
	return roles, nil
}

func chainrunRoster(roles []string) []chainrun.StepRequest {
	roster := make([]chainrun.StepRequest, 0, len(roles))
	for _, role := range roles {
		roster = append(roster, chainrun.StepRequest{Role: role})
	}
	return roster
}
