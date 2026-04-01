package tool

import (
	"fmt"
	"sort"
	"strings"
)

type candidateMatch struct {
	line    int
	snippet string
}

type fileEditMatchAnalysis struct {
	count            int
	candidateLines   string
	candidateSnippets string
}

func filePreview(content string, maxLines int) string {
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return "(empty file)"
	}
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

func snippetFromLineWindow(lines []string, startLine, maxLines int) string {
	if len(lines) == 0 {
		return ""
	}
	if startLine < 1 {
		startLine = 1
	}
	if maxLines <= 0 {
		maxLines = 1
	}
	startIdx := startLine - 1
	if startIdx >= len(lines) {
		startIdx = len(lines) - 1
	}
	endIdx := startIdx + maxLines
	if endIdx > len(lines) {
		endIdx = len(lines)
	}
	return strings.Join(lines[startIdx:endIdx], "\\n")
}

func candidateMatches(content, needle string) []candidateMatch {
	if needle == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	matches := make([]candidateMatch, 0)
	needleLineCount := strings.Count(needle, "\n") + 1
	for i, line := range lines {
		if strings.Contains(line, needle) {
			matches = append(matches, candidateMatch{line: i + 1, snippet: line})
		}
	}
	if len(matches) > 0 {
		return matches
	}
	for i := 0; i < len(content); {
		idx := strings.Index(content[i:], needle)
		if idx < 0 {
			break
		}
		pos := i + idx
		line := 1 + strings.Count(content[:pos], "\n")
		matches = append(matches, candidateMatch{line: line, snippet: snippetFromLineWindow(lines, line, min(needleLineCount+1, 3))})
		i = pos + len(needle)
	}
	return matches
}

func analyzeFileEditMatches(content, needle string) fileEditMatchAnalysis {
	matches := candidateMatches(content, needle)
	analysis := fileEditMatchAnalysis{count: strings.Count(content, needle)}
	if len(matches) == 0 {
		analysis.candidateLines = "unknown"
		analysis.candidateSnippets = "unknown"
		return analysis
	}
	lines := make([]int, 0, len(matches))
	for _, match := range matches {
		lines = append(lines, match.line)
	}
	sort.Ints(lines)
	lineParts := make([]string, 0, min(len(lines), 5))
	for _, line := range lines {
		if len(lineParts) >= 5 {
			break
		}
		lineParts = append(lineParts, fmt.Sprintf("line %d", line))
	}
	if len(lines) > 5 {
		lineParts = append(lineParts, fmt.Sprintf("... (%d total)", len(lines)))
	}
	snippetParts := make([]string, 0, min(len(matches), 3))
	for _, match := range matches {
		if len(snippetParts) >= 3 {
			break
		}
		snippetParts = append(snippetParts, fmt.Sprintf("line %d: %s", match.line, match.snippet))
	}
	if len(matches) > 3 {
		snippetParts = append(snippetParts, fmt.Sprintf("... (%d total)", len(matches)))
	}
	analysis.candidateLines = strings.Join(lineParts, ", ")
	analysis.candidateSnippets = strings.Join(snippetParts, " | ")
	return analysis
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
