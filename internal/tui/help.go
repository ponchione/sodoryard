package tui

import "strings"

func (m Model) renderHelp() string {
	lines := []string{
		m.styles.title.Render("Help"),
		"q or ctrl+c   quit",
		"?             toggle help",
		"tab           next screen",
		"enter         open selected chain receipts",
		"esc           back",
		"r             refresh",
		"j/k           move selection",
		"up/down       move selection",
		"d/l/c/p       dashboard/launch/chains/receipts",
		"i             edit launch task",
		"m/n/v         launch mode/role/preview",
		"S             start previewed launch",
		"F             follow selected chain",
		"P             pause selected chain",
		"R             show foreground resume command",
		"X             cancel selected chain with confirmation",
		"o             open selected receipt in PAGER",
		"E             open selected receipt in EDITOR",
		"",
		m.styles.subtle.Render("Resume is still handled by yard chain resume because it continues runner execution."),
	}
	return strings.Join(lines, "\n")
}
