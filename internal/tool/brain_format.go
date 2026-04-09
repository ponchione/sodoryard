package tool

import (
	"fmt"
	"strings"
)

func formatBrainSearchResult(query string, hits []formattedSearchHit) string {
	label := "documents"
	if len(hits) == 1 {
		label = "document"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d brain %s for %q:\n", len(hits), label, query))
	for i, hit := range hits {
		sb.WriteString(fmt.Sprintf("- %s — %s\n", hit.Path, hit.Title))
		sb.WriteString("  " + hit.Snippet)
		if i < len(hits)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

type formattedSearchHit struct {
	Path    string
	Title   string
	Snippet string
}

func compactSnippet(s string) string {
	parts := strings.Fields(s)
	return strings.TrimSpace(strings.Join(parts, " "))
}

func formatBrainReadDocument(path string, frontmatter string, wikilinks []string, body string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Brain document: %s\n", path))

	if frontmatter != "" {
		sb.WriteString("\nFrontmatter:\n")
		for _, line := range strings.Split(frontmatter, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			sb.WriteString("- " + line + "\n")
		}
	}

	if len(wikilinks) > 0 {
		sb.WriteString("\nOutgoing links:\n")
		for _, link := range wikilinks {
			sb.WriteString("- [[" + link + "]]\n")
		}
	}

	sb.WriteString("\nContent:\n```md\n")
	sb.WriteString(strings.TrimRight(body, "\n"))
	sb.WriteString("\n```")
	return sb.String()
}

func formatBrainDocumentPreview(content string, maxLines int) string {
	trimmed := strings.TrimRight(content, "\n")
	lines := strings.Split(trimmed, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		trimmed = strings.Join(lines[:maxLines], "\n") + fmt.Sprintf("\n[showing first %d of %d lines]", maxLines, len(lines))
	}
	return "```md\n" + trimmed + "\n```"
}

func formatHeadingList(headings []string) string {
	if len(headings) == 0 {
		return "(none)"
	}
	var sb strings.Builder
	for i, heading := range headings {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("- " + heading)
	}
	return sb.String()
}

func brainIndexStaleReminder() string {
	return "Derived brain index is now stale. Run `sirtopham index brain` to refresh indexed brain metadata."
}
