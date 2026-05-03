package tui

import "github.com/charmbracelet/lipgloss"

type styles struct {
	status         lipgloss.Style
	nav            lipgloss.Style
	navTitle       lipgloss.Style
	navItem        lipgloss.Style
	navActive      lipgloss.Style
	title          lipgloss.Style
	subtle         lipgloss.Style
	selected       lipgloss.Style
	error          lipgloss.Style
	footer         lipgloss.Style
	panel          lipgloss.Style
	section        lipgloss.Style
	chatMeta       lipgloss.Style
	chatUserLabel  lipgloss.Style
	chatAgentLabel lipgloss.Style
	chatUser       lipgloss.Style
	chatAgent      lipgloss.Style
	composer       lipgloss.Style
}

func newStyles() styles {
	return styles{
		status:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")).Background(lipgloss.Color("235")).Padding(0, 1),
		nav:            lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Padding(1, 2, 0, 1),
		navTitle:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250")),
		navItem:        lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		navActive:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("28")).Padding(0, 1),
		title:          lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")),
		subtle:         lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		selected:       lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("58")),
		error:          lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		footer:         lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Padding(0, 1),
		panel:          lipgloss.NewStyle().Padding(1, 2, 0, 1),
		section:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250")),
		chatMeta:       lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		chatUserLabel:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("87")),
		chatAgentLabel: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220")),
		chatUser:       lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		chatAgent:      lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
		composer:       lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Border(lipgloss.NormalBorder(), true).BorderForeground(lipgloss.Color("238")).Padding(0, 1),
	}
}
