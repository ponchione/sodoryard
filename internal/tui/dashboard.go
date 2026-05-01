package tui

import (
	"fmt"
	"strings"
)

func (m Model) renderDashboard() string {
	lines := []string{
		m.styles.title.Render("Dashboard"),
		fmt.Sprintf("project: %s", valueOrUnknown(m.status.ProjectName)),
		fmt.Sprintf("root: %s", valueOrUnknown(m.status.ProjectRoot)),
		fmt.Sprintf("provider: %s", valueOrUnknown(m.status.Provider)),
		fmt.Sprintf("model: %s", valueOrUnknown(m.status.Model)),
		fmt.Sprintf("active chains: %d", m.status.ActiveChains),
		"",
		m.styles.title.Render("Recent chains"),
	}
	if len(m.chains) == 0 {
		lines = append(lines, m.styles.subtle.Render("No chains found."))
	} else {
		for i, ch := range m.chains {
			if i >= 8 {
				break
			}
			task := ch.SourceTask
			if task == "" && len(ch.SourceSpecs) > 0 {
				task = strings.Join(ch.SourceSpecs, ", ")
			}
			lines = append(lines, fmt.Sprintf("%s  %s  steps=%d tokens=%d  %s", ch.ID, ch.Status, ch.TotalSteps, ch.TotalTokens, trimOneLine(task, 48)))
		}
	}
	if len(m.status.Warnings) > 0 {
		lines = append(lines, "", m.styles.title.Render("Warnings"))
		for _, warning := range m.status.Warnings {
			lines = append(lines, m.styles.error.Render(warning.Message))
		}
	}
	if m.err != nil {
		lines = append(lines, "", m.styles.error.Render(m.err.Error()))
	}
	return strings.Join(lines, "\n")
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}
