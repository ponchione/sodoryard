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
		"N             start a new chat on chat screen",
		"enter         open selected chain receipts",
		"esc           back",
		"r             refresh",
		"/             edit filter on chains or receipts",
		"esc           exit filter editing and keep the query",
		"backspace     edit filter text",
		"ctrl+u        clear filter text",
		"j/k           move selection",
		"up/down       move selection",
		"d/l/c/p       dashboard/launch/chains/receipts",
		"i             edit launch task",
		"b/m/n/v       launch preset, mode, add role/list entry, preview",
		"-/ctrl+u      remove or clear manual roster/constrained roles",
		"B             save current launch role shape as a custom preset",
		"s/L           save or load the current launch draft",
		"S             start previewed launch",
		"F             follow selected chain",
		"P             pause selected chain",
		"R             show foreground resume command",
		"X             cancel selected chain with confirmation",
		"w             show web inspector target without starting yard serve",
		"o             open selected receipt in PAGER",
		"E             open selected receipt in EDITOR",
		"",
		m.styles.subtle.Render("Resume is still handled by yard chain resume because it continues runner execution."),
	}
	return strings.Join(lines, "\n")
}
