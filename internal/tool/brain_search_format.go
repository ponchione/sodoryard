package tool

import (
	"fmt"
	"path/filepath"
	"strings"

	appcontext "github.com/ponchione/sodoryard/internal/context"
)

func formatRuntimeBrainSearchHits(hits []appcontext.BrainSearchResult) []formattedSearchHit {
	formatted := make([]formattedSearchHit, 0, len(hits))
	for _, hit := range hits {
		label := strings.TrimSpace(hit.MatchMode)
		if label == "" {
			label = strings.Join(hit.MatchSources, "+")
		}
		title := strings.TrimSpace(hit.Title)
		if title == "" {
			title = titleFromPath(hit.DocumentPath)
		}
		if label != "" && label != "keyword" {
			title = fmt.Sprintf("%s [%s]", title, label)
		}
		formatted = append(formatted, formattedSearchHit{
			Path:    hit.DocumentPath,
			Title:   title,
			Snippet: compactSnippet(hit.Snippet),
		})
	}
	return formatted
}

func describeBrainSearchQuery(query string, tags []string) string {
	if len(tags) == 0 {
		return query
	}
	tagLabel := "tags: " + strings.Join(tags, ", ")
	if query == "" {
		return tagLabel
	}
	return query + " (" + tagLabel + ")"
}

func pluralizeBrainSearchResults(count int) string {
	if count == 1 {
		return "result"
	}
	return "results"
}

// titleFromPath extracts a human-readable title from a brain document path.
func titleFromPath(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	// Convert hyphens and underscores to spaces, title-case.
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return titleCase(name)
}

// titleCase capitalizes the first letter of each word.
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
