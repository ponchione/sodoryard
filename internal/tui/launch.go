package tui

import (
	"fmt"
	"strings"

	"github.com/ponchione/sodoryard/internal/chaininput"
	"github.com/ponchione/sodoryard/internal/operator"
)

func (m Model) renderLaunch() string {
	lines := []string{m.styles.title.Render("Launch")}
	if m.notice != "" {
		lines = append(lines, m.styles.subtle.Render(m.notice))
	}
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("preset: %s", m.activeLaunchPresetName()))
	lines = append(lines, m.renderLaunchField(launchFieldTask, "task", renderLaunchTask(m.launch.SourceTask, m.launchEdit && m.launchField == launchFieldTask)))
	lines = append(lines, m.renderLaunchField(launchFieldSpecs, "specs", renderLaunchSpecs(m.launch.SpecsText, m.launchEdit && m.launchField == launchFieldSpecs)))
	lines = append(lines, m.renderLaunchField(launchFieldMode, "mode", string(m.launch.Mode)))
	role := m.launchRoleDisplay()
	if role == "" {
		role = "none"
	}
	roleLabel := "role"
	switch m.launch.Mode {
	case operator.LaunchModeManualRoster:
		roleLabel = "roster"
	case operator.LaunchModeConstrained:
		roleLabel = "allowed"
	}
	lines = append(lines, m.renderLaunchField(launchFieldRole, roleLabel, role))
	lines = append(lines, "", "controls: b preset  B save preset  i edit task/specs  m mode  n add role  - remove role  ctrl+u clear roles  s save  L load  v preview  S start")
	lines = append(lines, "", m.styles.title.Render("Preview"))
	if m.preview == nil {
		lines = append(lines, m.styles.subtle.Render("No preview yet."))
	} else {
		lines = append(lines,
			fmt.Sprintf("summary: %s", m.preview.Summary),
			fmt.Sprintf("mode: %s", m.preview.Mode),
			fmt.Sprintf("role: %s", m.preview.Role),
		)
		if len(m.preview.Roster) > 0 {
			lines = append(lines, fmt.Sprintf("roster: %s", strings.Join(m.preview.Roster, " -> ")))
		}
		if len(m.preview.AllowedRoles) > 0 {
			lines = append(lines, fmt.Sprintf("allowed: %s", strings.Join(m.preview.AllowedRoles, ", ")))
		}
		lines = append(lines, "", m.styles.title.Render("Compiled task"), trimOneLine(m.preview.CompiledTask, 120))
		if len(m.preview.Warnings) > 0 {
			lines = append(lines, "", m.styles.title.Render("Warnings"))
			for _, warning := range m.preview.Warnings {
				lines = append(lines, m.styles.subtle.Render(warning.Message))
			}
		}
	}
	if m.confirm.Action == "launch" {
		lines = append(lines, "", m.styles.error.Render("Confirm launch start? y/n"))
	}
	if m.err != nil {
		lines = append(lines, "", m.styles.error.Render(m.err.Error()))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderLaunchField(field launchField, label string, value string) string {
	line := fmt.Sprintf("%-6s %s", label+":", value)
	if m.launchField == field {
		return m.styles.selected.Render("> " + line)
	}
	return "  " + line
}

func renderLaunchTask(task string, editing bool) string {
	task = strings.Join(strings.Fields(task), " ")
	if task == "" {
		task = "empty"
	}
	if editing {
		return task + " _"
	}
	return trimOneLine(task, 96)
}

func renderLaunchSpecs(specs string, editing bool) string {
	specs = strings.Join(strings.Fields(specs), " ")
	if specs == "" {
		specs = "empty"
	}
	if editing {
		return specs + " _"
	}
	return trimOneLine(specs, 96)
}

func (m *Model) ensureLaunchDefaults() {
	if m.launch.Mode == "" {
		m.launch.Mode = operator.LaunchModeOneStep
	}
	if m.launch.Role == "" {
		m.launch.Role = firstLaunchRole(m.roles)
	}
	if m.launch.Mode == operator.LaunchModeManualRoster && len(m.launch.Roster) == 0 && m.launch.Role != "" {
		m.launch.Roster = []string{m.launch.Role}
	}
	if m.launch.Mode == operator.LaunchModeConstrained && len(m.launch.AllowedRoles) == 0 && m.launch.Role != "" {
		m.launch.AllowedRoles = []string{m.launch.Role}
	}
	if m.launchField > launchFieldRole {
		m.launchField = launchFieldTask
	}
}

func (m *Model) toggleLaunchMode() {
	switch m.launch.Mode {
	case operator.LaunchModeOneStep:
		m.launch.Mode = operator.LaunchModeOrchestrator
		m.notice = "launch mode set to orchestrator"
	case operator.LaunchModeOrchestrator:
		m.launch.Mode = operator.LaunchModeConstrained
		if len(m.launch.AllowedRoles) == 0 && m.launch.Role != "" {
			m.launch.AllowedRoles = []string{m.launch.Role}
		}
		m.notice = "launch mode set to constrained orchestration"
	case operator.LaunchModeConstrained:
		m.launch.Mode = operator.LaunchModeManualRoster
		if len(m.launch.Roster) == 0 && m.launch.Role != "" {
			m.launch.Roster = []string{m.launch.Role}
		}
		m.notice = "launch mode set to manual roster"
	default:
		m.launch.Mode = operator.LaunchModeOneStep
		m.notice = "launch mode set to one-step"
	}
	m.clearLaunchPreview()
	m.err = nil
}

func (m *Model) nextLaunchRole() {
	if len(m.roles) == 0 {
		m.notice = "no roles configured"
		return
	}
	if m.launch.Mode == operator.LaunchModeConstrained {
		next := m.nextAllowedRole()
		m.launch.AllowedRoles = append(m.launch.AllowedRoles, next)
		m.launch.Role = next
		m.notice = fmt.Sprintf("allowed constrained role added: %s", next)
		m.clearLaunchPreview()
		m.err = nil
		return
	}
	if m.launch.Mode == operator.LaunchModeManualRoster {
		next := m.nextRosterRole()
		m.launch.Roster = append(m.launch.Roster, next)
		m.launch.Role = next
		m.notice = fmt.Sprintf("added %s to manual roster", next)
		m.clearLaunchPreview()
		m.err = nil
		return
	}
	current := m.launch.Role
	next := firstLaunchRole(m.roles)
	for i, role := range m.roles {
		if role.Name == current {
			next = m.roles[(i+1)%len(m.roles)].Name
			break
		}
	}
	m.launch.Role = next
	m.notice = fmt.Sprintf("launch role set to %s", next)
	m.clearLaunchPreview()
	m.err = nil
}

func (m *Model) removeLastLaunchRole() {
	switch m.launch.Mode {
	case operator.LaunchModeManualRoster:
		if len(m.launch.Roster) == 0 {
			m.notice = "manual roster is empty"
			return
		}
		removed := m.launch.Roster[len(m.launch.Roster)-1]
		m.launch.Roster = append([]string(nil), m.launch.Roster[:len(m.launch.Roster)-1]...)
		m.launch.Role = lastString(m.launch.Roster)
		m.notice = fmt.Sprintf("removed %s from manual roster", removed)
	case operator.LaunchModeConstrained:
		if len(m.launch.AllowedRoles) == 0 {
			m.notice = "constrained allowed roles are empty"
			return
		}
		removed := m.launch.AllowedRoles[len(m.launch.AllowedRoles)-1]
		m.launch.AllowedRoles = append([]string(nil), m.launch.AllowedRoles[:len(m.launch.AllowedRoles)-1]...)
		m.launch.Role = lastString(m.launch.AllowedRoles)
		m.notice = fmt.Sprintf("removed %s from constrained allowed roles", removed)
	default:
		m.notice = "role list controls apply to manual roster or constrained orchestration"
		return
	}
	m.clearLaunchPreview()
	m.err = nil
}

func (m *Model) clearLaunchRoleList() {
	switch m.launch.Mode {
	case operator.LaunchModeManualRoster:
		if len(m.launch.Roster) == 0 {
			m.notice = "manual roster is already empty"
			return
		}
		m.launch.Roster = nil
		m.launch.Role = ""
		m.notice = "manual roster cleared"
	case operator.LaunchModeConstrained:
		if len(m.launch.AllowedRoles) == 0 {
			m.notice = "constrained allowed roles are already empty"
			return
		}
		m.launch.AllowedRoles = nil
		m.launch.Role = ""
		m.notice = "constrained allowed roles cleared"
	default:
		m.notice = "role list controls apply to manual roster or constrained orchestration"
		return
	}
	m.clearLaunchPreview()
	m.err = nil
}

func (m *Model) moveLaunchField(delta int) {
	next := int(m.launchField) + delta
	if next < int(launchFieldTask) {
		next = int(launchFieldRole)
	}
	if next > int(launchFieldRole) {
		next = int(launchFieldTask)
	}
	m.launchField = launchField(next)
}

func (m Model) launchFieldEditable() bool {
	return m.launchField == launchFieldTask || m.launchField == launchFieldSpecs
}

func (m Model) launchFieldLabel() string {
	switch m.launchField {
	case launchFieldSpecs:
		return "specs"
	case launchFieldMode:
		return "mode"
	case launchFieldRole:
		return "role"
	default:
		return "task"
	}
}

func (m Model) launchFieldText() string {
	if m.launchField == launchFieldSpecs {
		return m.launch.SpecsText
	}
	return m.launch.SourceTask
}

func (m *Model) setLaunchFieldText(value string) {
	if m.launchField == launchFieldSpecs {
		m.launch.SpecsText = value
		return
	}
	m.launch.SourceTask = value
}

func parseLaunchSpecs(value string) []string {
	return chaininput.ParseSpecs(value)
}

func (m Model) launchRequest() operator.LaunchRequest {
	req := operator.LaunchRequest{
		Mode:        m.launch.Mode,
		Role:        m.launch.Role,
		SourceTask:  m.launch.SourceTask,
		SourceSpecs: parseLaunchSpecs(m.launch.SpecsText),
	}
	if m.launch.Mode == operator.LaunchModeConstrained {
		req.AllowedRoles = append([]string(nil), m.launch.AllowedRoles...)
	}
	if m.launch.Mode == operator.LaunchModeManualRoster {
		req.Roster = append([]string(nil), m.launch.Roster...)
	}
	return req
}

func (m *Model) applyLaunchRequest(req operator.LaunchRequest) {
	m.launch.Mode = req.Mode
	m.launch.Role = req.Role
	m.launch.AllowedRoles = append([]string(nil), req.AllowedRoles...)
	m.launch.Roster = append([]string(nil), req.Roster...)
	m.launch.SourceTask = req.SourceTask
	m.launch.SpecsText = strings.Join(req.SourceSpecs, ", ")
	m.ensureLaunchDefaults()
	m.clearLaunchPreview()
}

func (m *Model) clearLaunchPreview() {
	m.preview = nil
	m.previewReq = nil
}

func sameLaunchRequest(left operator.LaunchRequest, right operator.LaunchRequest) bool {
	if left.Mode != right.Mode || left.Role != right.Role || left.SourceTask != right.SourceTask {
		return false
	}
	if len(left.Roster) != len(right.Roster) {
		return false
	}
	for i := range left.Roster {
		if left.Roster[i] != right.Roster[i] {
			return false
		}
	}
	if len(left.AllowedRoles) != len(right.AllowedRoles) {
		return false
	}
	for i := range left.AllowedRoles {
		if left.AllowedRoles[i] != right.AllowedRoles[i] {
			return false
		}
	}
	if len(left.SourceSpecs) != len(right.SourceSpecs) {
		return false
	}
	for i := range left.SourceSpecs {
		if left.SourceSpecs[i] != right.SourceSpecs[i] {
			return false
		}
	}
	return true
}

func firstLaunchRole(roles []operator.AgentRoleSummary) string {
	for _, role := range roles {
		if role.Name != "orchestrator" {
			return role.Name
		}
	}
	if len(roles) == 0 {
		return ""
	}
	return roles[0].Name
}

func (m Model) launchRoleDisplay() string {
	if m.launch.Mode == operator.LaunchModeManualRoster {
		return strings.Join(m.launch.Roster, " -> ")
	}
	if m.launch.Mode == operator.LaunchModeConstrained {
		return strings.Join(m.launch.AllowedRoles, ", ")
	}
	return m.launch.Role
}

func (m Model) nextAllowedRole() string {
	current := ""
	if len(m.launch.AllowedRoles) > 0 {
		current = m.launch.AllowedRoles[len(m.launch.AllowedRoles)-1]
	} else {
		current = m.launch.Role
	}
	next := firstLaunchRole(m.roles)
	for offset := range m.roles {
		if m.roles[offset].Name != current {
			continue
		}
		for step := 1; step <= len(m.roles); step++ {
			candidate := m.roles[(offset+step)%len(m.roles)].Name
			if candidate != "orchestrator" {
				return candidate
			}
		}
		break
	}
	return next
}

func (m Model) nextRosterRole() string {
	current := ""
	if len(m.launch.Roster) > 0 {
		current = m.launch.Roster[len(m.launch.Roster)-1]
	} else {
		current = m.launch.Role
	}
	next := firstLaunchRole(m.roles)
	for i, role := range m.roles {
		if role.Name == current {
			next = m.roles[(i+1)%len(m.roles)].Name
			break
		}
	}
	return next
}

func lastString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func dropLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}
