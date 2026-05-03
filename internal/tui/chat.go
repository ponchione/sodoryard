package tui

import (
	"fmt"
	"strings"

	"github.com/ponchione/sodoryard/internal/operator"
)

func (m Model) renderChat() string {
	lines := []string{m.styles.title.Render("Chat")}
	if m.notice != "" {
		lines = append(lines, m.styles.subtle.Render(m.notice))
	}
	if m.chatConversationID != "" {
		lines = append(lines, fmt.Sprintf("conversation: %s", m.chatConversationID))
	}
	lines = append(lines, fmt.Sprintf("runtime: %s:%s", valueOrUnknown(m.status.Provider), valueOrUnknown(m.status.Model)), "")
	if len(m.chatMessages) == 0 {
		lines = append(lines, m.styles.subtle.Render("No messages yet."))
	} else {
		lines = append(lines, renderChatMessages(m.chatMessages, m.contentWidth()-4, maxInt(6, m.height-12))...)
	}
	lines = append(lines, "", m.styles.title.Render("Message"))
	prompt := "> " + m.chatInput
	if m.chatEdit {
		prompt += "_"
	}
	lines = append(lines, prompt)
	if m.err != nil {
		lines = append(lines, "", m.styles.error.Render(m.err.Error()))
	}
	return strings.Join(lines, "\n")
}

func renderChatMessages(messages []operator.ChatMessage, width int, maxLines int) []string {
	if width <= 0 {
		width = 72
	}
	var lines []string
	for _, message := range messages {
		label := "assistant"
		if message.Role == "user" {
			label = "you"
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			content = "[no visible text]"
		}
		wrapped := wrapChatText(content, maxInt(20, width-len(label)-2))
		if len(wrapped) == 0 {
			wrapped = []string{""}
		}
		lines = append(lines, fmt.Sprintf("%s: %s", label, wrapped[0]))
		for _, line := range wrapped[1:] {
			lines = append(lines, strings.Repeat(" ", len(label)+2)+line)
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
