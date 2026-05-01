package tui

import "github.com/charmbracelet/lipgloss"

type styles struct {
	status   lipgloss.Style
	nav      lipgloss.Style
	title    lipgloss.Style
	subtle   lipgloss.Style
	selected lipgloss.Style
	error    lipgloss.Style
	footer   lipgloss.Style
	panel    lipgloss.Style
}

func newStyles() styles {
	return styles{
		status:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24")).Padding(0, 1),
		nav:      lipgloss.NewStyle().Foreground(lipgloss.Color("246")).PaddingRight(2),
		title:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")),
		subtle:   lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		selected: lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("58")),
		error:    lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		footer:   lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		panel:    lipgloss.NewStyle().Padding(0, 1),
	}
}
