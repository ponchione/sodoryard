package tui

import (
	"fmt"
	"strings"
)

func (m Model) renderReceipts() string {
	lines := []string{m.styles.title.Render("Receipts")}
	if m.notice != "" {
		lines = append(lines, m.styles.subtle.Render(m.notice))
	}
	if m.detail == nil {
		lines = append(lines, m.styles.subtle.Render("Select a chain before opening receipts."))
		return strings.Join(lines, "\n")
	}
	lines = append(lines, fmt.Sprintf("chain: %s", m.detail.Chain.ID), "")
	if len(m.receiptItems) == 0 {
		lines = append(lines, m.styles.subtle.Render("No receipts recorded."))
		return strings.Join(lines, "\n")
	}
	for i, item := range m.receiptItems {
		line := fmt.Sprintf("%-18s %s", item.Label, item.Path)
		if i == m.receiptCursor {
			line = m.styles.selected.Render("> " + line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", m.styles.title.Render("Content"))
	if m.receipt != nil {
		lines = append(lines, fmt.Sprintf("path: %s", m.receipt.Path))
		lines = append(lines, "controls: o pager  E editor")
	}
	if m.err != nil {
		lines = append(lines, m.styles.error.Render(m.err.Error()))
	} else {
		lines = append(lines, m.viewport.View())
	}
	return strings.Join(lines, "\n")
}
