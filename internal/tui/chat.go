package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ponchione/sodoryard/internal/operator"
)

func newChatComposer(styles styles) textarea.Model {
	composer := textarea.New()
	composer.Placeholder = "Message GPT-5.5..."
	composer.ShowLineNumbers = false
	composer.Prompt = ""
	composer.EndOfBufferCharacter = ' '
	composer.KeyMap.InsertNewline.SetKeys("alt+enter", "ctrl+j")
	composer.KeyMap.InsertNewline.SetHelp("alt+enter", "newline")
	composer.FocusedStyle.Base = styles.composer.BorderForeground(lipgloss.Color("28"))
	composer.BlurredStyle.Base = styles.composer
	composer.FocusedStyle.CursorLine = lipgloss.NewStyle()
	composer.BlurredStyle.CursorLine = lipgloss.NewStyle()
	composer.FocusedStyle.Placeholder = styles.subtle
	composer.BlurredStyle.Placeholder = styles.subtle
	composer.FocusedStyle.Text = styles.chatUser
	composer.BlurredStyle.Text = styles.chatUser
	return composer
}

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
	lines = append(lines, m.chatComposer.View())
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
		lines = append(lines, label)
		for _, line := range renderChatContent(styles, content, maxInt(20, width-2), bodyStyle) {
			if line == "" {
				lines = append(lines, "")
				continue
			}
			lines = append(lines, line)
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

func renderChatContent(styles styles, text string, width int, bodyStyle lipgloss.Style) []string {
	if width <= 0 {
		width = 72
	}
	var rendered []string
	inCode := false
	for _, raw := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				inCode = false
				continue
			}
			inCode = true
			language := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			if language != "" {
				rendered = append(rendered, styles.chatCodeHeader.Render("  code "+language))
			}
			continue
		}
		if inCode {
			rendered = append(rendered, renderCodeLine(styles, raw, width))
			continue
		}
		if trimmed == "" {
			rendered = append(rendered, "")
			continue
		}
		if heading, ok := markdownHeading(trimmed); ok {
			rendered = append(rendered, styles.chatHeading.Render("  "+heading))
			continue
		}
		if prefix, content, ok := markdownListItem(trimmed); ok {
			rendered = append(rendered, renderWrappedContent(content, width, "  "+prefix, "  "+strings.Repeat(" ", len(prefix)), bodyStyle)...)
			continue
		}
		rendered = append(rendered, renderWrappedContent(trimmed, width, "  ", "  ", bodyStyle)...)
	}
	if len(rendered) == 0 {
		return []string{bodyStyle.Render("  [no visible text]")}
	}
	return rendered
}

func renderCodeLine(styles styles, line string, width int) string {
	line = strings.TrimRight(line, " \t")
	if line == "" {
		line = " "
	}
	if len(line) > width {
		line = line[:maxInt(1, width-3)] + "..."
	}
	return styles.chatCode.Width(maxInt(12, width)).Render(line)
}

func renderWrappedContent(text string, width int, firstPrefix string, nextPrefix string, style lipgloss.Style) []string {
	wrapped := wrapChatText(text, maxInt(12, width-len(firstPrefix)))
	if len(wrapped) == 0 {
		return []string{style.Render(firstPrefix)}
	}
	lines := make([]string, 0, len(wrapped))
	for i, line := range wrapped {
		prefix := nextPrefix
		if i == 0 {
			prefix = firstPrefix
		}
		lines = append(lines, style.Render(prefix+line))
	}
	return lines
}

func markdownHeading(line string) (string, bool) {
	if !strings.HasPrefix(line, "#") {
		return "", false
	}
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level >= len(line) || line[level] != ' ' {
		return "", false
	}
	return strings.TrimSpace(line[level:]), true
}

func markdownListItem(line string) (string, string, bool) {
	for _, marker := range []string{"- ", "* "} {
		if strings.HasPrefix(line, marker) {
			return "- ", strings.TrimSpace(line[len(marker):]), true
		}
	}
	dot := 0
	for dot < len(line) && line[dot] >= '0' && line[dot] <= '9' {
		dot++
	}
	if dot > 0 && dot+1 < len(line) && line[dot] == '.' && line[dot+1] == ' ' {
		return line[:dot+2], strings.TrimSpace(line[dot+2:]), true
	}
	return "", "", false
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
