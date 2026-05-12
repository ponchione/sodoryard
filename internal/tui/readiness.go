package tui

import (
	"fmt"
	"strings"

	"github.com/ponchione/sodoryard/internal/operator"
)

type readinessState string

const (
	readinessOK        readinessState = "ok"
	readinessAttention readinessState = "attention"
	readinessFailing   readinessState = "failing"
)

type readinessRow struct {
	Label  string
	State  readinessState
	Detail string
	Action string
}

func runtimeReadinessRows(status operator.RuntimeStatus) []readinessRow {
	return []readinessRow{
		{
			Label:  "project",
			State:  readinessForPresent(status.ProjectName, status.ProjectRoot),
			Detail: strings.TrimSpace(status.ProjectName),
			Action: "yard config",
		},
		{
			Label:  "provider",
			State:  readinessForPresent(status.Provider, status.Model),
			Detail: strings.TrimSpace(status.Provider + ":" + status.Model),
			Action: "yard config",
		},
		{
			Label:  "auth",
			State:  authReadiness(status.AuthStatus),
			Detail: valueOrUnknown(status.AuthStatus),
			Action: "yard auth status",
		},
		{
			Label:  "code index",
			State:  indexReadiness(status.CodeIndex, false),
			Detail: renderIndexStatus(status.CodeIndex),
			Action: "yard index",
		},
		{
			Label:  "brain index",
			State:  indexReadiness(status.BrainIndex, true),
			Detail: renderIndexStatus(status.BrainIndex),
			Action: "yard brain index",
		},
		{
			Label:  "local services",
			State:  localServicesReadiness(status.LocalServicesStatus),
			Detail: valueOrUnknown(status.LocalServicesStatus),
			Action: "yard llm status",
		},
	}
}

func renderReadinessRows(styles styles, rows []readinessRow) []string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		detail := row.Detail
		if strings.TrimSpace(detail) == ":" {
			detail = "unknown"
		}
		action := ""
		if row.State != readinessOK && strings.TrimSpace(row.Action) != "" {
			action = "  fix: " + row.Action
		}
		lines = append(lines, fmt.Sprintf("%-9s  %-14s  %s%s", renderReadinessBadge(styles, row.State), row.Label, valueOrUnknown(detail), action))
	}
	return lines
}

func renderReadinessBadge(styles styles, state readinessState) string {
	switch state {
	case readinessOK:
		return styles.success.Render("OK")
	case readinessFailing:
		return styles.error.Render("FAIL")
	default:
		return styles.warning.Render("WARN")
	}
}

func runtimeReadinessSummary(status operator.RuntimeStatus) readinessState {
	summary := readinessOK
	for _, row := range runtimeReadinessRows(status) {
		switch row.State {
		case readinessFailing:
			return readinessFailing
		case readinessAttention:
			summary = readinessAttention
		}
	}
	if len(status.Warnings) > 0 && summary == readinessOK {
		summary = readinessAttention
	}
	return summary
}

func readinessForPresent(values ...string) readinessState {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return readinessAttention
		}
	}
	return readinessOK
}

func authReadiness(status string) readinessState {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch {
	case normalized == "":
		return readinessAttention
	case strings.HasPrefix(normalized, "ready"):
		return readinessOK
	case strings.Contains(normalized, "expired"), strings.Contains(normalized, "missing"), strings.Contains(normalized, "unavailable"), strings.Contains(normalized, "not registered"), strings.Contains(normalized, "not ready"):
		return readinessFailing
	default:
		return readinessAttention
	}
}

func indexReadiness(status operator.RuntimeIndexStatus, brain bool) readinessState {
	normalized := strings.ToLower(strings.TrimSpace(status.Status))
	if normalized == "" {
		return readinessAttention
	}
	if brain && normalized == "disabled" {
		return readinessOK
	}
	switch normalized {
	case "indexed", "clean":
		if strings.TrimSpace(status.StaleSince) != "" || strings.TrimSpace(status.StaleReason) != "" {
			return readinessAttention
		}
		return readinessOK
	case "stale", "never_indexed", "never indexed", "unknown", "unavailable":
		return readinessAttention
	default:
		return readinessAttention
	}
}

func localServicesReadiness(status string) readinessState {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch normalized {
	case "", "unknown":
		return readinessAttention
	case "disabled", "manual", "enabled", "ready", "running":
		return readinessOK
	default:
		if strings.Contains(normalized, "error") || strings.Contains(normalized, "failed") || strings.Contains(normalized, "down") {
			return readinessAttention
		}
		return readinessOK
	}
}

func dashboardActionLines(status operator.RuntimeStatus) []string {
	seen := map[string]bool{}
	var actions []string
	add := func(action string) {
		action = strings.TrimSpace(action)
		if action == "" || seen[action] {
			return
		}
		seen[action] = true
		actions = append(actions, action)
	}
	for _, row := range runtimeReadinessRows(status) {
		if row.State != readinessOK {
			add(row.Action)
		}
	}
	for _, warning := range status.Warnings {
		lower := strings.ToLower(warning.Message)
		switch {
		case strings.Contains(lower, "yard brain index"):
			add("yard brain index")
		case strings.Contains(lower, "yard index"):
			add("yard index")
		case strings.Contains(lower, "auth"), strings.Contains(lower, "provider"):
			add("yard auth status")
		case strings.Contains(lower, "llm"), strings.Contains(lower, "local service"):
			add("yard llm status")
		}
	}
	if len(status.Warnings) > 0 {
		add("yard doctor")
	}
	return actions
}
