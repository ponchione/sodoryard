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
	visibleChains := m.visibleChains()
	if filterLine := renderFilterLine("chains", m.chainFilter, m.filterEdit && m.filterScreen == screenChains, len(visibleChains), len(m.chains)); filterLine != "" {
		chainLines = append(chainLines, filterLine)
	}
	if len(visibleChains) == 0 {
		message := "No chains found."
		if len(m.chains) > 0 {
			message = "No chains match filter."
		}
		chainLines = append(chainLines, m.styles.subtle.Render(message))
	} else {
		for i, ch := range visibleChains {
			current := "idle"
			if ch.CurrentStep != nil {
				current = fmt.Sprintf("%s/%s", valueOrUnknown(ch.CurrentStep.Role), valueOrUnknown(ch.CurrentStep.Status))
			}
			line := fmt.Sprintf("%s  %-16s  steps=%d tokens=%d  current=%s  %s", ch.ID, ch.Status, ch.TotalSteps, ch.TotalTokens, current, trimOneLine(ch.SourceTask, 48))
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
			fmt.Sprintf("health: %s", renderChainHealth(m.styles, chainDetailHealth(m.detail))),
			fmt.Sprintf("budgets: %s", renderChainBudgetLine(ch, len(m.detail.Steps))),
			fmt.Sprintf("current: %s", renderCurrentStep(currentStepSummary(m.detail.Steps))),
			"controls: "+controls,
			fmt.Sprintf("summary: %s", trimOneLine(ch.Summary, 90)),
		)
		if len(ch.SourceSpecs) > 0 {
			detailLines = append(detailLines, fmt.Sprintf("specs: %s", trimOneLine(strings.Join(ch.SourceSpecs, ", "), 90)))
		}
		detailLines = append(detailLines,
			"",
			m.styles.title.Render("Steps"),
		)
		if len(m.detail.Steps) == 0 {
			detailLines = append(detailLines, m.styles.subtle.Render("No steps recorded."))
		} else {
			for _, step := range m.detail.Steps {
				detailLines = append(detailLines, renderStepLine(step))
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
	parts := []string{"F follow", "w web"}
	hasStateControl := false
	if canPauseChain(status) {
		parts = append(parts, "P pause")
		hasStateControl = true
	}
	if status == "paused" {
		parts = append(parts, "R resume")
		hasStateControl = true
	}
	if canCancelChain(status) {
		parts = append(parts, "X cancel")
		hasStateControl = true
	}
	if !hasStateControl {
		parts = append(parts, "terminal")
	}
	return strings.Join(parts, "  ")
}
