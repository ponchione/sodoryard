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
	composer.Placeholder = "Message configured model..."
	composer.ShowLineNumbers = false
	composer.Prompt = ""
	composer.EndOfBufferCharacter = ' '
	composer.KeyMap.InsertNewline.SetKeys("alt+enter", "ctrl+j")
	composer.KeyMap.InsertNewline.SetHelp("alt+enter", "newline")
	composer.FocusedStyle.Base = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	composer.BlurredStyle.Base = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	composer.FocusedStyle.CursorLine = lipgloss.NewStyle()
	composer.BlurredStyle.CursorLine = lipgloss.NewStyle()
	composer.FocusedStyle.Placeholder = styles.subtle
	composer.BlurredStyle.Placeholder = styles.subtle
	composer.FocusedStyle.Text = styles.chatUser
	composer.BlurredStyle.Text = styles.chatUser
	return composer
}

func (m Model) renderChat() string {
	return m.renderConsole()
}

func (m Model) chatUsageLine() string {
	var parts []string
	if m.chatInputTokens > 0 || m.chatOutputTokens > 0 {
		parts = append(parts, fmt.Sprintf("last turn tokens in:%d out:%d", m.chatInputTokens, m.chatOutputTokens))
	}
	if strings.TrimSpace(m.chatStopReason) != "" {
		parts = append(parts, "stop:"+m.chatStopReason)
	}
	return strings.Join(parts, "  ")
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
			chunks := splitLongChatWord(word, width)
			for _, chunk := range chunks {
				if line == "" {
					line = chunk
					continue
				}
				if len(line)+1+len(chunk) > width {
					lines = append(lines, line)
					line = chunk
					continue
				}
				line += " " + chunk
			}
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func splitLongChatWord(word string, width int) []string {
	if width <= 0 || len(word) <= width {
		return []string{word}
	}
	runes := []rune(word)
	chunks := make([]string, 0, (len(runes)/width)+1)
	for len(runes) > 0 {
		n := width
		if n > len(runes) {
			n = len(runes)
		}
		chunks = append(chunks, string(runes[:n]))
		runes = runes[n:]
	}
	return chunks
}
