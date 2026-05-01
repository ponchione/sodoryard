package operator

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

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
	req.Roster = append([]string(nil), preview.Roster...)

	startOpts := chainrun.Options{
		Mode:             chainrun.Mode(req.Mode),
		Role:             req.Role,
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
	req.Roster = normalizeRoster(req.Roster)
	if req.Mode == "" {
		if len(req.Roster) > 0 {
			req.Mode = LaunchModeManualRoster
		} else if req.Role != "" {
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
	case LaunchModeManualRoster:
		return fmt.Sprintf("Run manual roster: %s", strings.Join(req.Roster, " -> "))
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
	roster := normalizeRoster(req.Roster)
	if len(roster) == 0 && strings.TrimSpace(req.Role) != "" {
		roster = normalizeRoster(strings.Split(req.Role, ","))
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

func normalizeRoster(roles []string) []string {
	normalized := make([]string, 0, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		normalized = append(normalized, role)
	}
	return normalized
}

func chainrunRoster(roles []string) []chainrun.StepRequest {
	roster := make([]chainrun.StepRequest, 0, len(roles))
	for _, role := range roles {
		roster = append(roster, chainrun.StepRequest{Role: role})
	}
	return roster
}
