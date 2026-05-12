package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ponchione/sodoryard/internal/operator"
	receiptpkg "github.com/ponchione/sodoryard/internal/receipt"
)

func renderReceiptViewportContent(styles styles, view *operator.ReceiptView, width int) string {
	if view == nil {
		return "No receipt loaded."
	}
	content := strings.TrimSpace(view.Content)
	if content == "" {
		return "Receipt is empty."
	}
	parsed, err := receiptpkg.Parse([]byte(view.Content))
	if err != nil {
		return strings.Join(renderChatContent(styles, view.Content, maxInt(32, width), styles.chatAgent), "\n")
	}
	lines := []string{
		styles.section.Render("Metadata"),
		fmt.Sprintf("agent: %s  verdict: %s  step: %d", parsed.Agent, parsed.Verdict, parsed.Step),
		fmt.Sprintf("turns: %d  tokens: %d  duration: %ds", parsed.TurnsUsed, parsed.TokensUsed, parsed.DurationSeconds),
	}
	if !parsed.Timestamp.IsZero() {
		lines = append(lines, "timestamp: "+parsed.Timestamp.Format("2006-01-02 15:04:05 MST"))
	}
	if warnings := parsed.StructuredMetadataWarnings(); len(warnings) > 0 {
		lines = append(lines, "warnings: "+renderLimitedReceiptList(warnings, 3))
	}
	if len(parsed.ChangedFiles) > 0 {
		lines = append(lines, "changed files: "+renderLimitedReceiptList(parsed.ChangedFiles, 5))
	}
	if len(parsed.Followups) > 0 {
		lines = append(lines, "followups: "+renderLimitedReceiptList(parsed.Followups, 3))
	}
	if len(parsed.Findings) > 0 {
		lines = append(lines, renderReceiptFindings(parsed.Findings)...)
	}
	body := strings.TrimSpace(parsed.RawBody)
	if body == "" {
		lines = append(lines, "", styles.subtle.Render("Receipt body is empty."))
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "", styles.section.Render("Body"))
	lines = append(lines, renderChatContent(styles, body, maxInt(32, width), styles.chatAgent)...)
	return strings.Join(lines, "\n")
}

func renderReceiptFindings(findings []receiptpkg.Finding) []string {
	lines := []string{fmt.Sprintf("findings: %d", len(findings))}
	for i, finding := range findings {
		if i >= 3 {
			lines = append(lines, fmt.Sprintf("- %d more finding(s)", len(findings)-i))
			break
		}
		parts := []string{"- " + valueOrUnknown(finding.ID)}
		if strings.TrimSpace(finding.Status) != "" {
			parts = append(parts, "status="+finding.Status)
		}
		if strings.TrimSpace(finding.Severity) != "" {
			parts = append(parts, "severity="+finding.Severity)
		}
		if location := receiptFindingLocation(finding); location != "" {
			parts = append(parts, "file="+location)
		}
		if strings.TrimSpace(finding.Summary) != "" {
			parts = append(parts, "summary="+strconv.Quote(trimOneLine(finding.Summary, 80)))
		}
		lines = append(lines, strings.Join(parts, " "))
	}
	return lines
}

func receiptFindingLocation(finding receiptpkg.Finding) string {
	file := strings.TrimSpace(finding.File)
	if file == "" {
		return ""
	}
	if finding.Line > 0 {
		return fmt.Sprintf("%s:%d", file, finding.Line)
	}
	return file
}

func renderLimitedReceiptList(values []string, limit int) string {
	if len(values) == 0 {
		return "none"
	}
	if limit <= 0 || limit > len(values) {
		limit = len(values)
	}
	visible := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		if trimmed := strings.TrimSpace(values[i]); trimmed != "" {
			visible = append(visible, trimOneLine(trimmed, 80))
		}
	}
	if len(values) > limit {
		visible = append(visible, fmt.Sprintf("+%d more", len(values)-limit))
	}
	if len(visible) == 0 {
		return "none"
	}
	return strings.Join(visible, ", ")
}

func (m Model) receiptItemMeta(item receiptItem) string {
	if m.detail == nil || strings.TrimSpace(item.Step) == "" {
		return ""
	}
	for _, step := range m.detail.Steps {
		if fmt.Sprintf("%d", step.SequenceNum) != item.Step {
			continue
		}
		parts := []string{valueOrUnknown(step.Status)}
		if strings.TrimSpace(step.Verdict) != "" {
			parts = append(parts, step.Verdict)
		}
		if step.TokensUsed > 0 {
			parts = append(parts, fmt.Sprintf("%dtok", step.TokensUsed))
		}
		return strings.Join(parts, " ")
	}
	return ""
}
