package tool

import (
	"fmt"
	"strings"
)

const defaultMaxOutputTokens = 50000

// truncateResult checks if a tool result's content exceeds the token limit
// and truncates it with a helpful notice if so.
// Token estimation uses chars/4 as a rough heuristic.
func truncateResult(result *ToolResult, maxTokens int, toolName string) bool {
	if maxTokens <= 0 {
		maxTokens = defaultMaxOutputTokens
	}
	maxChars := maxTokens * 4

	if len(result.Content) <= maxChars {
		return false
	}

	// Count total lines for the notice.
	totalLines := strings.Count(result.Content, "\n") + 1

	// Truncate to maxChars, then find the last newline to avoid mid-line cut.
	truncated := result.Content[:maxChars]
	if lastNL := strings.LastIndex(truncated, "\n"); lastNL > 0 {
		truncated = truncated[:lastNL]
	}
	shownLines := strings.Count(truncated, "\n") + 1

	notice := truncationNotice(toolName, shownLines, totalLines)
	result.Content = truncated + "\n" + notice
	return true
}

// truncationNotice returns a contextually appropriate truncation message
// based on the tool that produced the output.
func truncationNotice(toolName string, shownLines, totalLines int) string {
	switch toolName {
	case "file_read":
		return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Use file_read with line_start/line_end for specific sections.]", shownLines, totalLines)
	case "search_text", "search_semantic":
		return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Try a more specific query to narrow results.]", shownLines, totalLines)
	case "git_diff":
		return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Use a path filter to narrow the diff.]", shownLines, totalLines)
	case "shell":
		return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Consider piping to head/tail or grep for specific output.]", shownLines, totalLines)
	case "list_directory":
		return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Use a deeper path or reduce depth to narrow results.]", shownLines, totalLines)
	case "find_files":
		return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Use a more specific pattern or path to narrow results.]", shownLines, totalLines)
	case "test_run":
		return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Use a path or filter to run fewer tests.]", shownLines, totalLines)
	case "db_sqlc":
		return fmt.Sprintf("[Output truncated — showing first %d lines of %d. Use the path parameter to scope to a specific sqlc project.]", shownLines, totalLines)
	default:
		return fmt.Sprintf("[Output truncated — showing first %d lines of %d.]", shownLines, totalLines)
	}
}
