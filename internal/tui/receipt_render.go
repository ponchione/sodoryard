package tui

import (
	"fmt"
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
	body := strings.TrimSpace(parsed.RawBody)
	if body == "" {
		lines = append(lines, "", styles.subtle.Render("Receipt body is empty."))
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "", styles.section.Render("Body"))
	lines = append(lines, renderChatContent(styles, body, maxInt(32, width), styles.chatAgent)...)
	return strings.Join(lines, "\n")
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
