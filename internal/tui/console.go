package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/ponchione/sodoryard/internal/operator"
)

type consoleEntryKind string

const (
	consoleEntryUser      consoleEntryKind = "user"
	consoleEntryAssistant consoleEntryKind = "assistant"
	consoleEntryCommand   consoleEntryKind = "command"
	consoleEntrySystem    consoleEntryKind = "system"
	consoleEntryError     consoleEntryKind = "error"
)

type consoleEntry struct {
	Kind      consoleEntryKind
	Title     string
	Body      string
	CreatedAt time.Time
}

func (m *Model) appendConsoleEntry(kind consoleEntryKind, title string, body string) {
	entry := consoleEntry{
		Kind:      kind,
		Title:     strings.TrimSpace(title),
		Body:      strings.TrimSpace(body),
		CreatedAt: time.Now(),
	}
	m.consoleEntries = append(m.consoleEntries, entry)
	if len(m.consoleEntries) > 200 {
		m.consoleEntries = append([]consoleEntry(nil), m.consoleEntries[len(m.consoleEntries)-200:]...)
	}
	m.updateConsoleViewportContent(true)
}

func (m *Model) appendChatMessages(messages []operator.ChatMessage) {
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			content = "[no visible text]"
		}
		if message.Role == "user" && m.chatPendingPrompt != "" && strings.TrimSpace(m.chatPendingPrompt) == content {
			continue
		}
		kind := consoleEntryAssistant
		title := "ASSISTANT"
		if message.Role == "user" {
			kind = consoleEntryUser
			title = "YOU"
		}
		m.appendConsoleEntry(kind, title, content)
	}
}

func (m Model) renderConsole() string {
	lines := []string{
		m.styles.title.Render("Yard Console"),
		m.styles.chatMeta.Render(fmt.Sprintf("project %s  runtime %s:%s  effort %s", valueOrUnknown(m.status.ProjectName), valueOrUnknown(m.status.Provider), valueOrUnknown(m.status.Model), valueOrUnknown(m.status.ReasoningEffort))),
	}
	if m.notice != "" {
		lines = append(lines, m.styles.subtle.Render(m.notice))
	}
	if m.chatConversationID != "" {
		lines = append(lines, m.styles.chatMeta.Render(fmt.Sprintf("conversation %s", m.chatConversationID)))
	}
	if m.follow {
		lines = append(lines, m.styles.chatMeta.Render(fmt.Sprintf("following %s  /unfollow stops events", m.followID)))
	}
	if m.chatRunning {
		lines = append(lines, m.styles.chatMeta.Render("generating response  ctrl+g cancels"))
	} else if usage := m.chatUsageLine(); usage != "" {
		lines = append(lines, m.styles.chatMeta.Render(usage))
	}
	lines = append(lines, "")

	consoleViewport := m.consoleViewport
	consoleViewport.Width = maxInt(24, m.contentWidth()-2)
	consoleViewport.Height = m.consoleViewportHeight()
	consoleViewport.SetContent(m.consoleTranscriptContent())
	consoleViewport.SetYOffset(consoleViewport.YOffset)
	lines = append(lines, consoleViewport.View())
	lines = append(lines, "", m.styles.section.Render("Message"))
	lines = append(lines, m.renderComposerBox())
	if m.err != nil {
		lines = append(lines, "", m.styles.error.Render(m.err.Error()))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) updateConsoleViewportContent(stickBottom bool) {
	m.consoleViewport.Width = maxInt(24, m.contentWidth()-2)
	m.consoleViewport.Height = m.consoleViewportHeight()
	m.consoleViewport.SetContent(m.consoleTranscriptContent())
	m.consoleViewport.SetYOffset(m.consoleViewport.YOffset)
	if stickBottom {
		m.consoleViewport.GotoBottom()
	}
}

func (m Model) consoleTranscriptContent() string {
	return strings.Join(m.renderConsoleTranscript(maxInt(24, m.contentWidth()-4)), "\n")
}

func (m Model) renderConsoleTranscript(width int) []string {
	if len(m.consoleEntries) == 0 {
		if len(m.chatMessages) > 0 {
			return renderChatMessages(m.styles, m.chatMessages, width, 0)
		}
		return renderEmptyConsole(width)
	}
	var lines []string
	for _, entry := range m.consoleEntries {
		lines = append(lines, renderConsoleEntryLabel(m.styles, entry))
		body := strings.TrimSpace(entry.Body)
		if body == "" {
			body = "[no visible text]"
		}
		style := consoleEntryBodyStyle(m.styles, entry.Kind)
		for _, line := range renderChatContent(m.styles, body, maxInt(20, width-2), style) {
			lines = append(lines, line)
		}
		lines = append(lines, "")
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func (m Model) renderComposerBox() string {
	border := lipgloss.Color("238")
	if m.chatEdit {
		border = lipgloss.Color("42")
	}
	return m.styles.composer.
		Width(maxInt(28, m.contentWidth()-4)).
		BorderForeground(border).
		Render(m.chatComposer.View())
}

func renderEmptyConsole(width int) []string {
	text := "Type a message for raw provider chat, or start with / for Yard commands. Try /help, /status, /chains, /start, /follow, or /new."
	return wrapChatText(text, maxInt(24, width-8))
}

func renderConsoleEntryLabel(styles styles, entry consoleEntry) string {
	label := strings.TrimSpace(entry.Title)
	if label == "" {
		label = strings.ToUpper(string(entry.Kind))
	}
	switch entry.Kind {
	case consoleEntryUser:
		return styles.chatUserLabel.Render(label)
	case consoleEntryAssistant:
		return styles.chatAgentLabel.Render(label)
	case consoleEntryError:
		return styles.error.Render(label)
	case consoleEntrySystem:
		return styles.info.Render(label)
	default:
		return styles.section.Render(label)
	}
}

func consoleEntryBodyStyle(styles styles, kind consoleEntryKind) lipgloss.Style {
	switch kind {
	case consoleEntryUser:
		return styles.chatUser
	case consoleEntryError:
		return styles.error
	case consoleEntrySystem:
		return styles.subtle
	default:
		return styles.chatAgent
	}
}

func (m *Model) resetConsoleSession(scope string) {
	if m.chatCancel != nil {
		m.chatCancel()
		m.chatCancel = nil
	}
	m.chatRunning = false
	m.chatPendingPrompt = ""
	m.chatConversationID = ""
	m.chatMessages = nil
	m.chatInput = ""
	m.chatComposer.SetValue("")
	m.chatEdit = true
	m.chatInputTokens = 0
	m.chatOutputTokens = 0
	m.chatStopReason = ""
	m.follow = false
	m.followID = ""
	m.followAfter = 0
	m.followLog = nil
	m.err = nil
	m.notice = "new console session"
	m.screen = screenChat
	if scope != "chat" {
		m.consoleEntries = nil
	}
	m.appendConsoleEntry(consoleEntrySystem, "NEW SESSION", fmt.Sprintf("New Yard console session\nproject=%s\nmodel=%s:%s", valueOrUnknown(m.status.ProjectName), valueOrUnknown(m.status.Provider), valueOrUnknown(m.status.Model)))
}
