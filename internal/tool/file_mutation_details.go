package tool

import (
	"encoding/json"
	"strings"

	"github.com/ponchione/sodoryard/internal/provider"
)

func newFileMutationDetails(fields map[string]any) json.RawMessage {
	return provider.NewToolResultDetails("file_mutation", fields)
}

func detailLineCount(text string) int {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func firstChangedLine(content, needle string) int {
	if needle == "" {
		return 0
	}
	idx := strings.Index(content, needle)
	if idx < 0 {
		return 0
	}
	return strings.Count(content[:idx], "\n") + 1
}
