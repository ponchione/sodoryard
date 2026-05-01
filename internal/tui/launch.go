package tui

import (
	"fmt"
	"strings"

	"github.com/ponchione/sodoryard/internal/operator"
)

func (m Model) renderLaunch() string {
	lines := []string{m.styles.title.Render("Launch")}
	if m.notice != "" {
		lines = append(lines, m.styles.subtle.Render(m.notice))
	}
	lines = append(lines, "")
	lines = append(lines, m.renderLaunchField(launchFieldTask, "task", renderLaunchTask(m.launch.SourceTask, m.launchEdit && m.launchField == launchFieldTask)))
	lines = append(lines, m.renderLaunchField(launchFieldSpecs, "specs", renderLaunchSpecs(m.launch.SpecsText, m.launchEdit && m.launchField == launchFieldSpecs)))
	lines = append(lines, m.renderLaunchField(launchFieldMode, "mode", string(m.launch.Mode)))
	role := m.launch.Role
	if role == "" {
		role = "none"
	}
	lines = append(lines, m.renderLaunchField(launchFieldRole, "role", role))
	lines = append(lines, "", "controls: i edit task/specs  m mode  n role  v preview  S start")
	lines = append(lines, "", m.styles.title.Render("Preview"))
	if m.preview == nil {
		lines = append(lines, m.styles.subtle.Render("No preview yet."))
	} else {
		lines = append(lines,
			fmt.Sprintf("summary: %s", m.preview.Summary),
			fmt.Sprintf("mode: %s", m.preview.Mode),
			fmt.Sprintf("role: %s", m.preview.Role),
			"",
			m.styles.title.Render("Compiled task"),
			trimOneLine(m.preview.CompiledTask, 120),
		)
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
	if m.launchField > launchFieldRole {
		m.launchField = launchFieldTask
	}
}

func (m *Model) toggleLaunchMode() {
	if m.launch.Mode == operator.LaunchModeOneStep {
		m.launch.Mode = operator.LaunchModeOrchestrator
		m.notice = "launch mode set to orchestrator"
	} else {
		m.launch.Mode = operator.LaunchModeOneStep
		m.notice = "launch mode set to one-step"
	}
	m.preview = nil
	m.err = nil
}

func (m *Model) nextLaunchRole() {
	if len(m.roles) == 0 {
		m.notice = "no roles configured"
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
	m.preview = nil
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
	if strings.TrimSpace(value) == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var specs []string
	for _, part := range strings.Split(value, ",") {
		spec := strings.TrimSpace(part)
		if spec == "" {
			continue
		}
		if _, ok := seen[spec]; ok {
			continue
		}
		seen[spec] = struct{}{}
		specs = append(specs, spec)
	}
	return specs
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

func dropLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}
