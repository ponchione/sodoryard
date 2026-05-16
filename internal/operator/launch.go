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
	"github.com/ponchione/sodoryard/internal/modelcap"
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
	req, err = normalizeLaunchForExecution(cfg, req, true)
	if err != nil {
		return LaunchPreview{}, err
	}
	template, ok := launchTemplateForMode(req.Mode)
	if !ok {
		return LaunchPreview{}, fmt.Errorf("unsupported launch mode %s", req.Mode)
	}
	compiled := compileLaunchTask(req)
	steps := compileLaunchPreviewSteps(req)
	return LaunchPreview{
		Mode:               req.Mode,
		Template:           template,
		Role:               req.Role,
		AllowedRoles:       append([]string(nil), req.AllowedRoles...),
		Roster:             append([]string(nil), req.Roster...),
		SourceTask:         req.SourceTask,
		SourceSpecs:        append([]string(nil), req.SourceSpecs...),
		Steps:              steps,
		Summary:            summarizeLaunch(req),
		CompiledTask:       compiled,
		WorkPacketMarkdown: compileWorkPacketMarkdown(req),
		RunSheetMarkdown:   compileRunSheetMarkdown(req, steps),
		StepMaxTurns:       req.StepMaxTurns,
		StepMaxTokens:      req.StepMaxTokens,
		AllowApprovalWait:  req.AllowApprovalWait,
		Warnings:           launchWarnings(cfg, req),
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
	req, err = normalizeLaunchForExecution(cfg, req, true)
	if err != nil {
		return StartResult{}, err
	}
	req = withLaunchDefaults(req)
	req.Mode = preview.Mode
	req.Role = preview.Role
	req.AllowedRoles = append([]string(nil), preview.AllowedRoles...)
	req.Roster = append([]string(nil), preview.Roster...)
	req.Steps = launchStepsFromPreview(preview.Steps)

	startOpts := chainrun.Options{
		Mode:              chainrun.Mode(req.Mode),
		Role:              req.Role,
		AllowedRoles:      append([]string(nil), req.AllowedRoles...),
		Roster:            chainrunSteps(req.Steps),
		Step:              chainrunStep(req.Steps),
		SourceSpecs:       append([]string(nil), req.SourceSpecs...),
		SourceTask:        req.SourceTask,
		MaxSteps:          req.MaxSteps,
		MaxResolverLoops:  req.MaxResolverLoops,
		MaxDuration:       req.MaxDuration,
		TokenBudget:       req.TokenBudget,
		StepMaxTurns:      req.StepMaxTurns,
		StepMaxTokens:     req.StepMaxTokens,
		AllowApprovalWait: req.AllowApprovalWait,
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
	req.TemplateID = strings.TrimSpace(req.TemplateID)
	req.Mode = LaunchMode(strings.TrimSpace(string(req.Mode)))
	req.Role = strings.TrimSpace(req.Role)
	req.SourceTask = strings.TrimSpace(req.SourceTask)
	req.SourceSpecs = chaininput.NormalizeSpecs(req.SourceSpecs)
	req.AllowedRoles = chaininput.NormalizeRoleSet(req.AllowedRoles)
	req.Roster = chaininput.NormalizeRoleList(req.Roster)
	req.Steps = normalizeLaunchSteps(req.Steps)
	if len(req.Steps) == 0 {
		switch {
		case len(req.Roster) > 0:
			req.Steps = launchStepsFromRoles(req.Roster)
		case req.Role != "" && (req.Mode == "" || req.Mode == LaunchModeOneStep):
			req.Steps = []LaunchRosterStep{{Role: req.Role}}
		case req.Role != "" && req.Mode == LaunchModeManualRoster:
			req.Roster = chaininput.ParseRoleList(req.Role)
			req.Steps = launchStepsFromRoles(req.Roster)
		}
	}
	if req.Mode == "" {
		if len(req.Steps) > 1 {
			req.Mode = LaunchModeManualRoster
		} else if len(req.Steps) == 1 {
			req.Mode = LaunchModeOneStep
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

func resolveLaunchTemplateRequest(req LaunchRequest) (LaunchRequest, error) {
	req.TemplateID = strings.TrimSpace(req.TemplateID)
	req.Mode = LaunchMode(strings.TrimSpace(string(req.Mode)))
	if req.TemplateID != "" {
		template, ok := LaunchTemplateForID(req.TemplateID)
		if !ok {
			return LaunchRequest{}, fmt.Errorf("unknown launch template %q", req.TemplateID)
		}
		if req.Mode != "" && req.Mode != template.Mode {
			return LaunchRequest{}, fmt.Errorf("launch template %q uses mode %s, not %s", req.TemplateID, template.Mode, req.Mode)
		}
		req.Mode = template.Mode
	}
	return normalizeLaunchRequest(req), nil
}

func normalizeLaunchForExecution(cfg *appconfig.Config, req LaunchRequest, requireWorkPacket bool) (LaunchRequest, error) {
	req, err := resolveLaunchTemplateRequest(req)
	if err != nil {
		return LaunchRequest{}, err
	}
	if err := validateLaunchStepCaps(req); err != nil {
		return LaunchRequest{}, err
	}
	if requireWorkPacket && req.SourceTask == "" && len(req.SourceSpecs) == 0 {
		return LaunchRequest{}, fmt.Errorf("one of task or global sources is required")
	}
	if len(req.Steps) > 0 && len(req.Roster) > 0 {
		resolvedRoster, err := resolveRoleList(cfg, req.Roster, "launch roster role")
		if err != nil {
			return LaunchRequest{}, err
		}
		resolvedSteps, err := resolveLaunchSteps(cfg, req.Steps)
		if err != nil {
			return LaunchRequest{}, err
		}
		if !sameRoleList(resolvedRoster, launchStepRoles(resolvedSteps)) {
			return LaunchRequest{}, fmt.Errorf("launch roster does not match structured steps")
		}
		req.Roster = resolvedRoster
		req.Steps = resolvedSteps
	}
	switch req.Mode {
	case LaunchModeOneStep:
		if len(req.Steps) != 1 {
			return LaunchRequest{}, fmt.Errorf("one-step launch requires exactly one step")
		}
		steps, err := resolveLaunchSteps(cfg, req.Steps)
		if err != nil {
			return LaunchRequest{}, err
		}
		req.Steps = steps
		if req.Role != "" {
			roleName, _, err := cfg.ResolveAgentRole(req.Role)
			if err != nil {
				return LaunchRequest{}, fmt.Errorf("resolve launch role: %w", err)
			}
			if roleName != req.Steps[0].Role {
				return LaunchRequest{}, fmt.Errorf("one-step launch role does not match structured step")
			}
		}
		req.Role = req.Steps[0].Role
		req.Roster = nil
		req.AllowedRoles = nil
	case LaunchModeManualRoster:
		if len(req.Steps) == 0 {
			return LaunchRequest{}, fmt.Errorf("manual roster requires at least one step")
		}
		steps, err := resolveLaunchSteps(cfg, req.Steps)
		if err != nil {
			return LaunchRequest{}, err
		}
		req.Steps = steps
		req.Roster = launchStepRoles(req.Steps)
		req.Role = strings.Join(req.Roster, ",")
		req.AllowedRoles = nil
	case LaunchModeOrchestrator:
		if len(req.Steps) > 0 {
			return LaunchRequest{}, fmt.Errorf("dispatcher launch cannot include structured steps")
		}
		roleName, _, err := cfg.ResolveAgentRole("orchestrator")
		if err != nil {
			return LaunchRequest{}, fmt.Errorf("agent role %q not found in config", "orchestrator")
		}
		req.Role = roleName
		req.AllowedRoles = nil
		req.Roster = nil
	case LaunchModeConstrained:
		if len(req.Steps) > 0 {
			return LaunchRequest{}, fmt.Errorf("dispatcher launch cannot include structured steps")
		}
		roleName, _, err := cfg.ResolveAgentRole("orchestrator")
		if err != nil {
			return LaunchRequest{}, fmt.Errorf("agent role %q not found in config", "orchestrator")
		}
		allowedRoles, err := resolveLaunchAllowedRoles(cfg, req)
		if err != nil {
			return LaunchRequest{}, err
		}
		req.Role = roleName
		req.AllowedRoles = allowedRoles
		req.Roster = nil
	default:
		return LaunchRequest{}, fmt.Errorf("unsupported launch mode %s", req.Mode)
	}
	return req, nil
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

func compileWorkPacketMarkdown(req LaunchRequest) string {
	var b strings.Builder
	b.WriteString("Work packet\n\n")
	if req.SourceTask != "" {
		b.WriteString("Task:\n")
		b.WriteString(req.SourceTask)
		b.WriteString("\n\n")
	} else {
		b.WriteString("Task:\nNo task text was provided.\n\n")
	}
	b.WriteString("Global sources:\n")
	writeMarkdownList(&b, req.SourceSpecs)
	if req.Mode == LaunchModeConstrained && len(req.AllowedRoles) > 0 {
		b.WriteString("\nAllowed roles:\n")
		writeMarkdownList(&b, req.AllowedRoles)
	}
	return strings.TrimSpace(b.String())
}

func compileLaunchPreviewSteps(req LaunchRequest) []LaunchPreviewStep {
	if req.Mode != LaunchModeOneStep && req.Mode != LaunchModeManualRoster {
		return nil
	}
	steps := make([]LaunchPreviewStep, 0, len(req.Steps))
	for i, step := range req.Steps {
		sequence := i + 1
		effectiveSources := effectiveStepSources(req.SourceSpecs, step.Sources)
		receives := []string{"global work packet"}
		dossierSummary := dossierReceiveSummary(step)
		if dossierSummary != "" {
			receives = append(receives, dossierSummary)
		}
		if sequence == 1 {
			receives = append(receives, "prior receipts: none")
		} else {
			receives = append(receives, fmt.Sprintf("prior receipts: %d", sequence-1))
		}
		produces := fmt.Sprintf("%s receipt", step.Role)
		previewStep := LaunchPreviewStep{
			Sequence:          sequence,
			Role:              step.Role,
			Note:              step.Note,
			GlobalSources:     append([]string(nil), req.SourceSpecs...),
			DossierSources:    append([]string(nil), step.Sources...),
			EffectiveSources:  effectiveSources,
			PriorReceiptCount: sequence - 1,
			Receives:          receives,
			Produces:          produces,
		}
		previewStep.BriefMarkdown = compileStepBriefMarkdown(req, previewStep)
		steps = append(steps, previewStep)
	}
	return steps
}

func compileRunSheetMarkdown(req LaunchRequest, steps []LaunchPreviewStep) string {
	var b strings.Builder
	b.WriteString("Run sheet\n\n")
	b.WriteString("Work packet\n")
	if req.SourceTask != "" {
		b.WriteString("- Task: ")
		b.WriteString(req.SourceTask)
		b.WriteByte('\n')
	} else {
		b.WriteString("- Task: No task text was provided.\n")
	}
	b.WriteString("- Global sources:\n")
	if len(req.SourceSpecs) == 0 {
		b.WriteString("  - None.\n")
	} else {
		for _, source := range req.SourceSpecs {
			b.WriteString("  - ")
			b.WriteString(source)
			b.WriteByte('\n')
		}
	}
	for _, step := range steps {
		b.WriteByte('\n')
		b.WriteString(fmt.Sprintf("%d. %s\n", step.Sequence, step.Role))
		b.WriteString("   Receives:\n")
		for _, receive := range step.Receives {
			b.WriteString("   - ")
			b.WriteString(receive)
			b.WriteByte('\n')
		}
		b.WriteString("   Produces: ")
		b.WriteString(step.Produces)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func compileStepBriefMarkdown(req LaunchRequest, step LaunchPreviewStep) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d. %s\n", step.Sequence, step.Role))
	b.WriteString("Receives:\n")
	for _, receive := range step.Receives {
		b.WriteString("- ")
		b.WriteString(receive)
		b.WriteByte('\n')
	}
	b.WriteString("Effective sources:\n")
	writeMarkdownList(&b, step.EffectiveSources)
	if step.Note != "" {
		b.WriteString("\nDossier note:\n")
		b.WriteString(step.Note)
		b.WriteByte('\n')
	}
	b.WriteString("\nProduces: ")
	b.WriteString(step.Produces)
	if req.Mode == LaunchModeOneStep {
		b.WriteString("\nMode: one-step chain")
	}
	return strings.TrimSpace(b.String())
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

func launchWarnings(cfg *appconfig.Config, req LaunchRequest) []RuntimeWarning {
	var warnings []RuntimeWarning
	if len(req.SourceSpecs) == 0 {
		warnings = append(warnings, RuntimeWarning{Message: "no source specs selected"})
	}
	model, err := modelcap.ResolveConfiguredModel(cfg, "", "")
	if err != nil {
		warnings = append(warnings, RuntimeWarning{Message: "model capability metadata unavailable: " + err.Error()})
		return warnings
	}
	if !model.SupportsTools && launchRequiresTools(cfg, req) {
		warnings = append(warnings, RuntimeWarning{Message: fmt.Sprintf("default model %s:%s does not report tool support, but launch roles require tools", model.Provider, model.ID)})
	}
	return warnings
}

func validateLaunchStepCaps(req LaunchRequest) error {
	if req.StepMaxTurns < 0 {
		return fmt.Errorf("step_max_turns must not be negative")
	}
	if req.StepMaxTokens < 0 {
		return fmt.Errorf("step_max_tokens must not be negative")
	}
	return nil
}

func launchRequiresTools(cfg *appconfig.Config, req LaunchRequest) bool {
	if cfg == nil {
		return false
	}
	for _, role := range launchRoleNames(req) {
		roleCfg, ok := cfg.AgentRoles[role]
		if !ok {
			continue
		}
		if len(roleCfg.Tools) > 0 || len(roleCfg.CustomTools) > 0 {
			return true
		}
	}
	return false
}

func launchRoleNames(req LaunchRequest) []string {
	switch req.Mode {
	case LaunchModeOneStep:
		if len(req.Steps) == 1 {
			return []string{req.Steps[0].Role}
		}
		if req.Role == "" {
			return nil
		}
		return []string{req.Role}
	case LaunchModeManualRoster:
		if len(req.Steps) > 0 {
			return launchStepRoles(req.Steps)
		}
		return append([]string(nil), req.Roster...)
	case LaunchModeConstrained:
		return append([]string{"orchestrator"}, req.AllowedRoles...)
	default:
		return []string{"orchestrator"}
	}
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

func normalizeLaunchSteps(steps []LaunchRosterStep) []LaunchRosterStep {
	normalized := make([]LaunchRosterStep, 0, len(steps))
	for _, step := range steps {
		role := strings.TrimSpace(step.Role)
		if role == "" {
			continue
		}
		normalized = append(normalized, LaunchRosterStep{
			Role:    role,
			Note:    strings.TrimSpace(step.Note),
			Sources: chaininput.NormalizeSpecs(step.Sources),
		})
	}
	return normalized
}

func launchStepsFromRoles(roles []string) []LaunchRosterStep {
	steps := make([]LaunchRosterStep, 0, len(roles))
	for _, role := range roles {
		if role = strings.TrimSpace(role); role != "" {
			steps = append(steps, LaunchRosterStep{Role: role})
		}
	}
	return steps
}

func launchStepRoles(steps []LaunchRosterStep) []string {
	roles := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.Role != "" {
			roles = append(roles, step.Role)
		}
	}
	return roles
}

func resolveLaunchSteps(cfg *appconfig.Config, steps []LaunchRosterStep) ([]LaunchRosterStep, error) {
	resolved := normalizeLaunchSteps(steps)
	for i := range resolved {
		roleName, _, err := cfg.ResolveAgentRole(resolved[i].Role)
		if err != nil {
			return nil, fmt.Errorf("resolve launch step %d role: %w", i+1, err)
		}
		resolved[i].Role = roleName
	}
	return resolved, nil
}

func resolveRoleList(cfg *appconfig.Config, roles []string, label string) ([]string, error) {
	resolved := chaininput.NormalizeRoleList(roles)
	for i := range resolved {
		roleName, _, err := cfg.ResolveAgentRole(resolved[i])
		if err != nil {
			return nil, fmt.Errorf("resolve %s %d: %w", label, i+1, err)
		}
		resolved[i] = roleName
	}
	return resolved, nil
}

func sameRoleList(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func launchStepsFromPreview(steps []LaunchPreviewStep) []LaunchRosterStep {
	out := make([]LaunchRosterStep, 0, len(steps))
	for _, step := range steps {
		out = append(out, LaunchRosterStep{
			Role:    step.Role,
			Note:    step.Note,
			Sources: append([]string(nil), step.DossierSources...),
		})
	}
	return out
}

func effectiveStepSources(global []string, dossier []string) []string {
	out := append([]string(nil), global...)
	seen := make(map[string]struct{}, len(out)+len(dossier))
	for _, source := range out {
		seen[source] = struct{}{}
	}
	for _, source := range dossier {
		if _, ok := seen[source]; ok {
			continue
		}
		seen[source] = struct{}{}
		out = append(out, source)
	}
	return out
}

func dossierReceiveSummary(step LaunchRosterStep) string {
	parts := make([]string, 0, 2)
	if len(step.Sources) > 0 {
		label := "source"
		if len(step.Sources) != 1 {
			label = "sources"
		}
		parts = append(parts, fmt.Sprintf("%d %s", len(step.Sources), label))
	}
	if step.Note != "" {
		parts = append(parts, "note present")
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("%s dossier: %s", step.Role, strings.Join(parts, ", "))
}

func writeMarkdownList(b *strings.Builder, values []string) {
	if len(values) == 0 {
		b.WriteString("- None.\n")
		return
	}
	for _, value := range values {
		b.WriteString("- ")
		b.WriteString(value)
		b.WriteByte('\n')
	}
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

func chainrunSteps(steps []LaunchRosterStep) []chainrun.StepRequest {
	roster := make([]chainrun.StepRequest, 0, len(steps))
	for _, step := range steps {
		roster = append(roster, chainrun.StepRequest{
			Role:    step.Role,
			Note:    step.Note,
			Sources: append([]string(nil), step.Sources...),
		})
	}
	return roster
}

func chainrunStep(steps []LaunchRosterStep) chainrun.StepRequest {
	if len(steps) == 0 {
		return chainrun.StepRequest{}
	}
	return chainrun.StepRequest{
		Role:    steps[0].Role,
		Note:    steps[0].Note,
		Sources: append([]string(nil), steps[0].Sources...),
	}
}
