package tool

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

var hiddenStateSearchExcludes = map[string]struct{}{
	".git":      {},
	".yard":     {},
	".brain":    {},
	".obsidian": {},
}

// ripgrep JSON types for parsing --json output.
type rgMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type rgMatch struct {
	Path       rgPath       `json:"path"`
	Lines      rgText       `json:"lines"`
	LineNumber int          `json:"line_number"`
	Submatches []rgSubmatch `json:"submatches"`
}

type rgContext struct {
	Path       rgPath `json:"path"`
	Lines      rgText `json:"lines"`
	LineNumber int    `json:"line_number"`
}

type rgPath struct {
	Text string `json:"text"`
}

type rgText struct {
	Text string `json:"text"`
}

type rgSubmatch struct {
	Match rgText `json:"match"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

func isHiddenStateSearchPath(path string) bool {
	cleaned := filepath.Clean(path)
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if _, excluded := hiddenStateSearchExcludes[part]; excluded {
			return true
		}
	}
	return false
}

func formatRipgrepStream(r io.Reader, pattern string, maxResults int, stop func()) (formatted string, matchCount int, stoppedEarly bool, err error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var sb strings.Builder
	currentFile := ""
	printedCurrentFileHeader := false

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg rgMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "match":
			if maxResults > 0 && matchCount >= maxResults {
				if stop != nil {
					stop()
				}
				return finalizeRipgrepFormat(&sb, pattern, matchCount), matchCount, true, nil
			}
			var m rgMatch
			if json.Unmarshal(msg.Data, &m) != nil {
				continue
			}
			if m.Path.Text != currentFile {
				if currentFile != "" {
					sb.WriteString("\n")
				}
				currentFile = m.Path.Text
				printedCurrentFileHeader = true
				sb.WriteString(fmt.Sprintf("%s\n", currentFile))
			} else if !printedCurrentFileHeader {
				printedCurrentFileHeader = true
				sb.WriteString(fmt.Sprintf("%s\n", currentFile))
			}
			matchCount++
			text := strings.TrimRight(m.Lines.Text, "\n\r")
			sb.WriteString(fmt.Sprintf("> %4d  %s\n", m.LineNumber, text))
			if maxResults > 0 && matchCount >= maxResults {
				if stop != nil {
					stop()
				}
				return finalizeRipgrepFormat(&sb, pattern, matchCount), matchCount, true, nil
			}

		case "context":
			if maxResults > 0 && matchCount >= maxResults {
				continue
			}
			var c rgContext
			if json.Unmarshal(msg.Data, &c) != nil {
				continue
			}
			if c.Path.Text != currentFile {
				if currentFile != "" {
					sb.WriteString("\n")
				}
				currentFile = c.Path.Text
				printedCurrentFileHeader = true
				sb.WriteString(fmt.Sprintf("%s\n", currentFile))
			} else if !printedCurrentFileHeader {
				printedCurrentFileHeader = true
				sb.WriteString(fmt.Sprintf("%s\n", currentFile))
			}
			text := strings.TrimRight(c.Lines.Text, "\n\r")
			sb.WriteString(fmt.Sprintf("  %4d  %s\n", c.LineNumber, text))
		}
	}

	if err := scanner.Err(); err != nil {
		return "", matchCount, stoppedEarly, err
	}
	return finalizeRipgrepFormat(&sb, pattern, matchCount), matchCount, false, nil
}

func finalizeRipgrepFormat(sb *strings.Builder, pattern string, matchCount int) string {
	if matchCount == 0 {
		return fmt.Sprintf("No matches found for pattern: '%s'", pattern)
	}
	sb.WriteString(fmt.Sprintf("\n(%d matches)", matchCount))
	return sb.String()
}
