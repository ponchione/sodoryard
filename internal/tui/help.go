package tui

import "strings"

func (m Model) renderHelp() string {
	lines := []string{
		m.styles.title.Render("Help"),
		"q or ctrl+c   quit",
		"?             toggle help",
		"tab           next screen",
		"a             chat",
		"enter/i       edit chat message on chat screen",
		"enter         send chat message while editing",
		"alt+enter     insert newline while editing chat",
		"ctrl+j        insert newline while editing chat",
		"ctrl+u        clear composer text while editing chat",
		"ctrl+g        cancel running chat turn",
		"N             start a new chat on chat screen",
		"enter         open selected chain receipts",
		"esc           back",
		"r             refresh",
		"/             edit filter on chains or receipts",
		"esc           exit filter editing and keep the query",
		"backspace     edit filter text",
		"ctrl+u        clear filter text while filtering",
		"j/k           move selection",
		"up/down       move selection",
		"d/l/c/p       dashboard/launch/chains/receipts",
		"i             edit selected launch field",
		"b/m/n/v       launch preset, mode, add role/list entry, preview",
		"turns/tokens  set per-step caps from the launch screen fields",
		"-/ctrl+u      remove or clear manual roster/constrained roles",
		"B             save current launch shape/caps as a custom preset",
		"s/L           save or load the current launch draft",
		"S             start previewed launch",
		"F             follow selected chain",
		"P             pause selected chain",
		"R             resume selected paused chain",
		"X             cancel selected chain with confirmation",
		"/approvals    list approval controls for selected or named chain",
		"w             show web inspector target without starting yard serve",
		"o             open selected receipt in PAGER",
		"E             open selected receipt in EDITOR",
		"",
		m.styles.subtle.Render("Resume uses the same operator service path as other chain controls."),
	}
	return strings.Join(lines, "\n")
}
