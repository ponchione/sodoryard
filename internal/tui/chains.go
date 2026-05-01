package tui

import (
	"fmt"
	"strings"
)

func (m Model) renderChains() string {
	chainLines := []string{m.styles.title.Render("Chains")}
	if m.notice != "" {
		chainLines = append(chainLines, m.styles.subtle.Render(m.notice))
	}
	if len(m.chains) == 0 {
		chainLines = append(chainLines, m.styles.subtle.Render("No chains found."))
	} else {
		for i, ch := range m.chains {
			line := fmt.Sprintf("%s  %-16s  steps=%d tokens=%d  %s", ch.ID, ch.Status, ch.TotalSteps, ch.TotalTokens, trimOneLine(ch.SourceTask, 48))
			if i == m.chainCursor {
				line = m.styles.selected.Render("> " + line)
			} else {
				line = "  " + line
			}
			chainLines = append(chainLines, line)
		}
	}
	detailLines := []string{"", m.styles.title.Render("Detail")}
	if m.detail == nil {
		detailLines = append(detailLines, m.styles.subtle.Render("Select a chain to load details."))
	} else {
		ch := m.detail.Chain
		controls := controlsForStatus(ch.Status)
		detailLines = append(detailLines,
			fmt.Sprintf("chain: %s", ch.ID),
			fmt.Sprintf("status: %s", ch.Status),
			"controls: "+controls,
			fmt.Sprintf("summary: %s", trimOneLine(ch.Summary, 90)),
			"",
			m.styles.title.Render("Steps"),
		)
		if len(m.detail.Steps) == 0 {
			detailLines = append(detailLines, m.styles.subtle.Render("No steps recorded."))
		} else {
			for _, step := range m.detail.Steps {
				detailLines = append(detailLines, fmt.Sprintf("%d  %-18s %-12s verdict=%s receipt=%s", step.SequenceNum, step.Role, step.Status, step.Verdict, step.ReceiptPath))
			}
		}
		eventTitle := "Recent events"
		events := m.detail.RecentEvents
		if m.follow {
			eventTitle = fmt.Sprintf("Following %s", m.followID)
			events = m.followLog
		}
		detailLines = append(detailLines, "", m.styles.title.Render(eventTitle))
		detailLines = append(detailLines, renderEventLines(events, 8)...)
	}
	if m.confirm.Action != "" {
		detailLines = append(detailLines, "", m.styles.error.Render(fmt.Sprintf("Confirm %s for chain %s? y/n", m.confirm.Action, m.confirm.ChainID)))
	}
	if m.err != nil {
		detailLines = append(detailLines, "", m.styles.error.Render(m.err.Error()))
	}
	return strings.Join(append(chainLines, detailLines...), "\n")
}

func controlsForStatus(status string) string {
	parts := []string{"F follow"}
	if canPauseChain(status) {
		parts = append(parts, "P pause")
	}
	if status == "paused" {
		parts = append(parts, "R resume command")
	}
	if canCancelChain(status) {
		parts = append(parts, "X cancel")
	}
	if len(parts) == 1 {
		parts = append(parts, "terminal")
	}
	return strings.Join(parts, "  ")
}
