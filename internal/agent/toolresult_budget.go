package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ponchione/sodoryard/internal/provider"
)

type AggregateToolResultBudgetReport struct {
	OriginalChars       int
	FinalChars          int
	MaxChars            int
	ReplacedResults     int
	PersistedResults    int
	InlineShrunkResults int
	CharsSaved          int
}

func applyAggregateToolResultBudget(ctx context.Context, store ToolResultStore, results []provider.ToolResult, toolCalls []provider.ToolCall, maxChars int) ([]provider.ToolResult, AggregateToolResultBudgetReport) {
	report := AggregateToolResultBudgetReport{MaxChars: maxChars}
	if maxChars <= 0 || len(results) == 0 {
		for _, result := range results {
			report.OriginalChars += len(result.Content)
		}
		report.FinalChars = report.OriginalChars
		return results, report
	}

	totalChars := 0
	for _, result := range results {
		totalChars += len(result.Content)
	}
	report.OriginalChars = totalChars
	if totalChars <= maxChars {
		report.FinalChars = totalChars
		return results, report
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
		leftRank := aggregateBudgetPriority(left.toolName)
		rightRank := aggregateBudgetPriority(right.toolName)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		leftLen := len(budgeted[left.index].Content)
		rightLen := len(budgeted[right.index].Content)
		if leftLen != rightLen {
			return leftLen > rightLen
		}
		return left.index < right.index
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
		usedPersistence := false
		persistedPath := ""
		if candidate.toolName != "file_read" && store != nil {
			if ref, err := store.PersistToolResult(ctx, current.ToolUseID, candidate.toolName, current.Content); err == nil {
				persistedBudget := preferredPersistedToolResultBudget(candidate.toolName, targetLen, len(current.Content))
				persisted := buildPersistedToolResultMessage(ref, current.ToolUseID, candidate.toolName, current.Content, persistedBudget)
				if len(persisted) < len(current.Content) {
					shrunk = persisted
					usedPersistence = true
					persistedPath = ref
				}
			}
		}
		if !usedPersistence && len(shrunk) > targetLen {
			shrunk = shrinkToolResultForAggregateBudget(shrunk, targetLen, candidate.toolName)
		}
		if shrunk == current.Content {
			continue
		}
		report.ReplacedResults++
		if usedPersistence {
			report.PersistedResults++
		} else {
			report.InlineShrunkResults++
		}
		totalChars -= len(current.Content) - len(shrunk)
		budgeted[candidate.index].Content = shrunk
		detailFields := map[string]any{
			"returned_size": len(shrunk),
			"truncated":     true,
		}
		if persistedPath != "" {
			detailFields["persisted_path"] = persistedPath
		}
		budgeted[candidate.index].Details = provider.MergeToolResultDetails(current.Details, detailFields)
	}

	report.FinalChars = totalChars
	report.CharsSaved = report.OriginalChars - report.FinalChars
	return budgeted, report
}

func aggregateBudgetPriority(toolName string) int {
	switch toolName {
	case "shell":
		return 0
	case "file_read":
		return 2
	default:
		return 1
	}
}

func preferredPersistedToolResultBudget(toolName string, targetLen int, contentLen int) int {
	minPersistedBudget := 120
	if toolName == "shell" {
		minPersistedBudget = 180
	}
	if contentLen <= 1 {
		return targetLen
	}
	if targetLen >= minPersistedBudget {
		return targetLen
	}
	if contentLen <= minPersistedBudget {
		return contentLen - 1
	}
	return minPersistedBudget
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

func buildPersistedToolResultMessage(ref string, toolUseID string, toolName string, content string, maxChars int) string {
	baseLines := []string{"[persisted_tool_result]", "path=" + ref}
	if toolName != "" {
		baseLines = append(baseLines, "tool="+toolName)
	}
	if toolUseID != "" {
		baseLines = append(baseLines, "tool_use_id="+toolUseID)
	}
	base := strings.Join(baseLines, "\n")
	if maxChars > 0 && len(base) > maxChars {
		return compactPersistedToolResultReference(ref, maxChars)
	}

	preview := strings.TrimSpace(content)
	if preview == "" {
		preview = "(no preview available)"
	}
	previewHeader := "\npreview=\n"
	previewBudget := 160
	if maxChars > 0 {
		previewBudget = maxChars - len(base) - len(previewHeader)
		if previewBudget <= 0 {
			return base
		}
	}
	preview = buildPersistedToolResultPreview(toolName, preview, previewBudget)
	return base + previewHeader + preview
}

func buildPersistedToolResultPreview(toolName string, content string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	if len(content) <= maxChars {
		return content
	}
	if toolName == "shell" {
		if failureFocused := buildShellFailurePreview(content, maxChars); failureFocused != "" {
			return failureFocused
		}
		tail := content[len(content)-maxChars:]
		if idx := strings.Index(tail, "\n"); idx >= 0 && idx < len(tail)-1 {
			tail = tail[idx+1:]
		}
		if len(tail) > maxChars {
			tail = tail[len(tail)-maxChars:]
		}
		if tail != "" {
			return tail
		}
	}
	return content[:maxChars]
}

func buildShellFailurePreview(content string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	start := -1
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "FAIL") || strings.HasPrefix(upper, "ERROR") || strings.Contains(line, "panic:") || strings.Contains(line, "PANIC:") {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	preview := strings.TrimSpace(strings.Join(lines[start:], "\n"))
	if preview == "" {
		return ""
	}
	if len(preview) <= maxChars {
		return preview
	}
	return strings.TrimSpace(preview[:maxChars])
}

func compactPersistedToolResultReference(ref string, maxChars int) string {
	if maxChars <= 0 {
		return ref
	}
	if len(ref) <= maxChars {
		return ref
	}
	pathLine := "path=" + ref
	if len(pathLine) <= maxChars {
		return pathLine
	}
	base := "[persisted_tool_result]\n" + pathLine
	if len(base) <= maxChars {
		return base
	}
	return ref[:maxChars]
}
