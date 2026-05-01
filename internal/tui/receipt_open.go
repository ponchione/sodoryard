package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type ReceiptOpenMode string

const (
	ReceiptOpenPager  ReceiptOpenMode = "pager"
	ReceiptOpenEditor ReceiptOpenMode = "editor"
)

type ReceiptOpenRequest struct {
	Mode    ReceiptOpenMode
	Path    string
	Content string
}

type ReceiptOpener func(ReceiptOpenRequest) tea.Cmd

func defaultReceiptOpener(req ReceiptOpenRequest) tea.Cmd {
	return func() tea.Msg {
		tmp, err := os.CreateTemp("", "yard-receipt-*.md")
		if err != nil {
			return receiptOpenedMsg{Mode: req.Mode, Path: req.Path, Err: fmt.Errorf("create temp receipt: %w", err)}
		}
		tempPath := tmp.Name()
		cleanup := func() {
			_ = os.Remove(tempPath)
		}
		if _, err := tmp.WriteString(req.Content); err != nil {
			_ = tmp.Close()
			cleanup()
			return receiptOpenedMsg{Mode: req.Mode, Path: req.Path, Err: fmt.Errorf("write temp receipt: %w", err)}
		}
		if err := tmp.Close(); err != nil {
			cleanup()
			return receiptOpenedMsg{Mode: req.Mode, Path: req.Path, Err: fmt.Errorf("close temp receipt: %w", err)}
		}

		cmd := receiptOpenCommand(req.Mode, tempPath)
		return tea.ExecProcess(cmd, func(err error) tea.Msg {
			cleanup()
			return receiptOpenedMsg{Mode: req.Mode, Path: req.Path, Err: err}
		})()
	}
}

func receiptOpenCommand(mode ReceiptOpenMode, path string) *exec.Cmd {
	envName := "PAGER"
	fallback := "less"
	if mode == ReceiptOpenEditor {
		envName = "EDITOR"
		fallback = "vi"
	}
	command := strings.TrimSpace(os.Getenv(envName))
	if command == "" {
		command = fallback
	}
	parts := strings.Fields(command)
	if len(parts) == 0 {
		parts = []string{fallback}
	}
	args := append([]string{}, parts[1:]...)
	args = append(args, path)
	return exec.Command(parts[0], args...)
}
