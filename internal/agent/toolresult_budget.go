package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ponchione/sirtopham/internal/provider"
)

func applyAggregateToolResultBudget(ctx context.Context, store ToolResultStore, results []provider.ToolResult, toolCalls []provider.ToolCall, maxChars int) []provider.ToolResult {
	if maxChars <= 0 || len(results) == 0 {
		return results
	}

	totalChars := 0
	for _, result := range results {
		totalChars += len(result.Content)
	}
	if totalChars <= maxChars {
		return results
	}

	budgeted := append([]provider.ToolResult(nil), results...)
	type candidate struct {
		index    int
		toolName string
	}
	candidates := make([]candidate, 0, len(budgeted))
	for i, result := range budgeted {
		candidates = append(candidates, candidate{index: i, toolName: toolNameFromResults(toolCalls, result.ToolUseID)})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		leftFileRead := left.toolName == "file_read"
		rightFileRead := right.toolName == "file_read"
		if leftFileRead != rightFileRead {
			return !leftFileRead
		}
		return len(budgeted[left.index].Content) > len(budgeted[right.index].Content)
	})

	for _, candidate := range candidates {
		if totalChars <= maxChars {
			break
		}
		current := budgeted[candidate.index]
		excess := totalChars - maxChars
		targetLen := len(current.Content) - excess
		if targetLen >= len(current.Content) {
			continue
		}
		if targetLen < 0 {
			targetLen = 0
		}
		shrunk := current.Content
		if candidate.toolName != "file_read" && store != nil {
			if ref, err := store.PersistToolResult(ctx, current.ToolUseID, candidate.toolName, current.Content); err == nil {
				persisted := buildPersistedToolResultMessage(ref, current.Content, targetLen)
				if len(persisted) < len(current.Content) {
					shrunk = persisted
				}
			}
		}
		if len(shrunk) > targetLen {
			shrunk = shrinkToolResultForAggregateBudget(shrunk, targetLen, candidate.toolName)
		}
		totalChars -= len(current.Content) - len(shrunk)
		budgeted[candidate.index].Content = shrunk
	}

	return budgeted
}

func shrinkToolResultForAggregateBudget(content string, maxChars int, toolName string) string {
	if maxChars <= 0 {
		return ""
	}
	if len(content) <= maxChars {
		return content
	}

	notice := fmt.Sprintf("[Output reduced to fit aggregate tool-result budget for %s.]", toolName)
	if toolName == "" {
		notice = "[Output reduced to fit aggregate tool-result budget.]"
	}
	if len(notice) >= maxChars {
		return notice[:maxChars]
	}

	contentBudget := maxChars - len(notice) - 1
	if contentBudget <= 0 {
		return notice[:maxChars]
	}
	if contentBudget > len(content) {
		contentBudget = len(content)
	}
	return content[:contentBudget] + "\n" + notice
}

func buildPersistedToolResultMessage(ref string, content string, maxChars int) string {
	base := fmt.Sprintf("[Full tool output persisted to %s]", ref)
	if maxChars > 0 && len(base) >= maxChars {
		return base[:maxChars]
	}
	previewHeader := "\nPreview:\n"
	previewBudget := 160
	if maxChars > 0 {
		previewBudget = maxChars - len(base) - len(previewHeader)
		if previewBudget < 0 {
			previewBudget = 0
		}
	}
	preview := strings.TrimSpace(content)
	if preview == "" {
		preview = "(no preview available)"
	}
	if len(preview) > previewBudget && previewBudget > 0 {
		preview = preview[:previewBudget]
	}
	if previewBudget <= 0 {
		return base
	}
	return base + previewHeader + preview
}
