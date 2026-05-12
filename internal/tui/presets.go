package tui

import (
	"strings"

	"github.com/ponchione/sodoryard/internal/operator"
)

type launchPreset struct {
	Name          string
	Mode          operator.LaunchMode
	Role          string
	AllowedRoles  []string
	Roster        []string
	StepMaxTurns  int
	StepMaxTokens int
}

func availableLaunchPresets(roles []operator.AgentRoleSummary, customPresets []operator.LaunchPreset) []launchPreset {
	available := roleSet(roles)
	var presets []launchPreset
	if roleAvailable(available, "coder") {
		presets = append(presets, launchPreset{Name: "solo coder", Mode: operator.LaunchModeOneStep, Role: "coder"})
	} else if role := firstLaunchRole(roles); role != "" {
		presets = append(presets, launchPreset{Name: "solo role", Mode: operator.LaunchModeOneStep, Role: role})
	}
	if roleAvailable(available, "orchestrator") {
		presets = append(presets, launchPreset{Name: "sir topham decides", Mode: operator.LaunchModeOrchestrator, Role: "orchestrator"})
	}
	if roleAvailable(available, "planner") && roleAvailable(available, "coder") {
		presets = append(presets, launchPreset{Name: "plan then code", Mode: operator.LaunchModeManualRoster, Role: "coder", Roster: []string{"planner", "coder"}})
		presets = append(presets, launchPreset{Name: "planner/coder constrained", Mode: operator.LaunchModeConstrained, Role: "coder", AllowedRoles: []string{"planner", "coder"}})
	}
	if roleAvailable(available, "coder") && roleAvailable(available, "correctness-auditor") {
		presets = append(presets, launchPreset{Name: "code then audit", Mode: operator.LaunchModeManualRoster, Role: "correctness-auditor", Roster: []string{"coder", "correctness-auditor"}})
	}
	for _, custom := range customPresets {
		if strings.TrimSpace(custom.Name) == "" {
			continue
		}
		req := custom.Request
		presets = append(presets, launchPreset{
			Name:          custom.Name,
			Mode:          req.Mode,
			Role:          req.Role,
			AllowedRoles:  append([]string(nil), req.AllowedRoles...),
			Roster:        append([]string(nil), req.Roster...),
			StepMaxTurns:  req.StepMaxTurns,
			StepMaxTokens: req.StepMaxTokens,
		})
	}
	return presets
}

func roleSet(roles []operator.AgentRoleSummary) map[string]struct{} {
	available := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		name := strings.TrimSpace(role.Name)
		if name == "" {
			continue
		}
		available[name] = struct{}{}
	}
	return available
}

func roleAvailable(available map[string]struct{}, role string) bool {
	_, ok := available[role]
	return ok
}

func (m Model) activeLaunchPresetName() string {
	for _, preset := range availableLaunchPresets(m.roles, m.customPresets) {
		if launchDraftMatchesPreset(m.launch, preset) {
			return preset.Name
		}
	}
	return "custom"
}

func launchDraftMatchesPreset(draft launchDraft, preset launchPreset) bool {
	if draft.Mode != preset.Mode || draft.Role != preset.Role {
		return false
	}
	if draft.StepMaxTurns != preset.StepMaxTurns || draft.StepMaxTokens != preset.StepMaxTokens {
		return false
	}
	if !sameStringSlice(draft.Roster, preset.Roster) {
		return false
	}
	if !sameStringSlice(draft.AllowedRoles, preset.AllowedRoles) {
		return false
	}
	return true
}

func sameStringSlice(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (m *Model) nextLaunchPreset() {
	presets := availableLaunchPresets(m.roles, m.customPresets)
	if len(presets) == 0 {
		m.notice = "no launch presets available for configured roles"
		return
	}
	next := 0
	current := m.activeLaunchPresetName()
	for i, preset := range presets {
		if preset.Name == current {
			next = (i + 1) % len(presets)
			break
		}
	}
	m.applyLaunchPreset(presets[next])
}

func (m *Model) applyLaunchPreset(preset launchPreset) {
	m.launch.Mode = preset.Mode
	m.launch.Role = preset.Role
	m.launch.AllowedRoles = append([]string(nil), preset.AllowedRoles...)
	m.launch.Roster = append([]string(nil), preset.Roster...)
	m.launch.StepMaxTurns = preset.StepMaxTurns
	m.launch.StepMaxTokens = preset.StepMaxTokens
	m.notice = "launch preset set to " + preset.Name
	m.clearLaunchPreview()
	m.err = nil
}

func (m *Model) upsertCustomPreset(preset operator.LaunchPreset) {
	for i := range m.customPresets {
		if m.customPresets[i].ID == preset.ID || (m.customPresets[i].Name != "" && m.customPresets[i].Name == preset.Name) {
			m.customPresets[i] = preset
			return
		}
	}
	m.customPresets = append(m.customPresets, preset)
}

func customLaunchPresetName(req operator.LaunchRequest) string {
	switch req.Mode {
	case operator.LaunchModeManualRoster:
		if len(req.Roster) > 0 {
			return "custom roster " + strings.Join(req.Roster, " -> ")
		}
	case operator.LaunchModeConstrained:
		if len(req.AllowedRoles) > 0 {
			return "custom constrained " + strings.Join(req.AllowedRoles, ", ")
		}
	case operator.LaunchModeOrchestrator:
		return "custom sir topham decides"
	case operator.LaunchModeOneStep:
		if strings.TrimSpace(req.Role) != "" {
			return "custom one-step " + strings.TrimSpace(req.Role)
		}
	}
	return "custom launch preset"
}
