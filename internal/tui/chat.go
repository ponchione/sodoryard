package tui

import (
	"fmt"
	"strings"

	"github.com/ponchione/sodoryard/internal/operator"
)

func (m Model) renderChat() string {
	lines := []string{
		m.styles.title.Render("Chat"),
		m.styles.chatMeta.Render(fmt.Sprintf("project %s  runtime %s:%s", valueOrUnknown(m.status.ProjectName), valueOrUnknown(m.status.Provider), valueOrUnknown(m.status.Model))),
	}
	if m.notice != "" {
		lines = append(lines, m.styles.subtle.Render(m.notice))
	}
	if m.chatConversationID != "" {
		lines = append(lines, m.styles.chatMeta.Render(fmt.Sprintf("conversation %s", m.chatConversationID)))
	}
	lines = append(lines, "")
	if len(m.chatMessages) == 0 {
		lines = append(lines, renderEmptyChat(m.contentWidth())...)
	} else {
		lines = append(lines, m.renderChatMessages(maxInt(24, m.contentWidth()-4), maxInt(8, m.height-13))...)
	}
	lines = append(lines, "", m.styles.section.Render("Message"))
	prompt := m.chatInput
	if strings.TrimSpace(prompt) == "" && !m.chatEdit {
		prompt = "Press enter or i to write a message."
	}
	if m.chatEdit {
		prompt += "_"
	}
	lines = append(lines, m.styles.composer.Width(maxInt(24, m.contentWidth()-6)).Render(prompt))
	if m.err != nil {
		lines = append(lines, "", m.styles.error.Render(m.err.Error()))
	}
	return strings.Join(lines, "\n")
}

func renderEmptyChat(width int) []string {
	return wrapChatText("Start a raw provider chat for specs, plans, and design notes. This surface does not apply a Yard agent role prompt.", maxInt(24, width-8))
}

func (m Model) renderChatMessages(width int, maxLines int) []string {
	return renderChatMessages(m.styles, m.chatMessages, width, maxLines)
}

func renderChatMessages(styles styles, messages []operator.ChatMessage, width int, maxLines int) []string {
	if width <= 0 {
		width = 72
	}
	var lines []string
	for _, message := range messages {
		label := styles.chatAgentLabel.Render("ASSISTANT")
		bodyStyle := styles.chatAgent
		if message.Role == "user" {
			label = styles.chatUserLabel.Render("YOU")
			bodyStyle = styles.chatUser
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			content = "[no visible text]"
		}
		wrapped := wrapChatText(content, maxInt(20, width-2))
		if len(wrapped) == 0 {
			wrapped = []string{""}
		}
		lines = append(lines, label)
		for _, line := range wrapped {
			if line == "" {
				lines = append(lines, "")
				continue
			}
			lines = append(lines, bodyStyle.Render("  "+line))
		}
		lines = append(lines, "")
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if maxLines > 0 && len(lines) > maxLines {
		return lines[len(lines)-maxLines:]
	}
	return lines
}

func wrapChatText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		words := strings.Fields(paragraph)
		line := ""
		for _, word := range words {
			if line == "" {
				line = word
				continue
			}
			if len(line)+1+len(word) > width {
				lines = append(lines, line)
				line = word
				continue
			}
			line += " " + word
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
