package context

import (
	"encoding/json"
	"path"
	"regexp"

	"github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/provider"
)

const defaultMomentumLookbackTurns = 2

var searchResultPathPattern = regexp.MustCompile(`(?:\./)?[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*\.[A-Za-z0-9]+`)

type toolPathInput struct {
	Path string `json:"path"`
}

// HistoryMomentumTracker implements the v0.1 recent-history momentum heuristic.
//
// It scans persisted conversation rows from the most recent turns and derives a
// deduplicated file list plus a longest common directory prefix.
type HistoryMomentumTracker struct{}

// Apply enriches ContextNeeds with momentum derived from recent history when the
// current turn is weak-signal. Strong-signal turns clear any stale momentum.
func (HistoryMomentumTracker) Apply(recentHistory []db.Message, needs *ContextNeeds, cfg config.ContextConfig) {
	if needs == nil {
		return
	}
	if len(needs.ExplicitFiles) > 0 || len(needs.ExplicitSymbols) > 0 {
		needs.MomentumFiles = nil
		needs.MomentumModule = ""
		return
	}

	files := momentumFilesFromHistory(selectRecentTurnMessages(recentHistory, cfg.MomentumLookbackTurns))
	if len(files) == 0 {
		return
	}
	needs.MomentumFiles = files
	needs.MomentumModule = longestCommonDirectoryPrefix(files)
}

func selectRecentTurnMessages(history []db.Message, lookbackTurns int) []db.Message {
	if len(history) == 0 {
		return nil
	}
	if lookbackTurns <= 0 {
		lookbackTurns = defaultMomentumLookbackTurns
	}

	selectedTurns := make(map[int64]struct{})
	for i := len(history) - 1; i >= 0 && len(selectedTurns) < lookbackTurns; i-- {
		if history[i].TurnNumber <= 0 {
			continue
		}
		selectedTurns[history[i].TurnNumber] = struct{}{}
	}
	if len(selectedTurns) == 0 {
		return history
	}

	selected := make([]db.Message, 0, len(history))
	for _, message := range history {
		if _, ok := selectedTurns[message.TurnNumber]; ok {
			selected = append(selected, message)
		}
	}
	return selected
}

func momentumFilesFromHistory(history []db.Message) []string {
	var files []string
	for _, message := range history {
		if !message.Content.Valid {
			continue
		}
		switch message.Role {
		case "assistant":
			for _, file := range filesFromAssistantToolUses(message.Content.String) {
				appendUnique(&files, file)
			}
		case "tool":
			if !message.ToolName.Valid {
				continue
			}
			if message.ToolName.String != "search_text" && message.ToolName.String != "search_semantic" {
				continue
			}
			for _, match := range searchResultPathPattern.FindAllString(message.Content.String, -1) {
				appendUnique(&files, match)
			}
		}
	}
	return files
}

func filesFromAssistantToolUses(raw string) []string {
	blocks, err := provider.ContentBlocksFromRaw(json.RawMessage(raw))
	if err != nil {
		return nil
	}

	var files []string
	for _, block := range blocks {
		if block.Type != "tool_use" {
			continue
		}
		if block.Name != "file_read" && block.Name != "file_write" && block.Name != "file_edit" {
			continue
		}
		var input toolPathInput
		if err := json.Unmarshal(block.Input, &input); err != nil || input.Path == "" {
			continue
		}
		if normalized, _, ok := normalizePathToken(input.Path); ok {
			appendUnique(&files, normalized)
			continue
		}
		appendUnique(&files, input.Path)
	}
	return files
}

func longestCommonDirectoryPrefix(files []string) string {
	if len(files) == 0 {
		return ""
	}

	directories := make([]string, 0, len(files))
	for _, file := range files {
		dir := path.Dir(file)
		if dir == "." || dir == "/" {
			return ""
		}
		directories = append(directories, dir)
	}
	return commonPathPrefix(directories)
}
