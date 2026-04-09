package parser

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ponchione/sirtopham/internal/codeintel"
	"gopkg.in/yaml.v3"
)

type ParsedLink struct {
	Target  string
	Display string
	Raw     string
}

type Heading struct {
	Level int
	Text  string
	Line  int
}

type Document struct {
	Path         string
	Title        string
	Content      string
	Body         string
	ContentHash  string
	Tags         []string
	Frontmatter  map[string]any
	Wikilinks    []ParsedLink
	Headings     []Heading
	TokenCount   int
	UpdatedAt    time.Time
	HasUpdatedAt bool
}

var (
	wikilinkRegexp  = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	inlineTagRegexp = regexp.MustCompile(`(?:^|\s)#([[:alnum:]_/-]+)\b`)
	headingRegexp   = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)
)

func ParseDocument(docPath, content string) (Document, error) {
	return ParseDocumentWithModTime(docPath, content, time.Time{}, false)
}

func ParseDocumentWithFileModTime(docPath, content string, modTime time.Time) (Document, error) {
	return ParseDocumentWithModTime(docPath, content, modTime, true)
}

func ParseDocumentWithModTime(docPath, content string, modTime time.Time, hasModTime bool) (Document, error) {
	frontmatter, body := splitFrontmatter(content)
	var fm map[string]any
	if frontmatter != "" {
		if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
			return Document{}, fmt.Errorf("parse frontmatter for %s: %w", docPath, err)
		}
	}
	if fm == nil {
		fm = map[string]any{}
	}

	updatedAt, hasUpdatedAt := extractUpdatedAt(fm)
	if !hasUpdatedAt && hasModTime && !modTime.IsZero() {
		updatedAt = modTime.UTC()
		hasUpdatedAt = true
	}

	return Document{
		Path:         docPath,
		Title:        extractTitle(docPath, body),
		Content:      content,
		Body:         body,
		ContentHash:  codeintel.ContentHash(content),
		Tags:         extractTags(body, fm),
		Frontmatter:  fm,
		Wikilinks:    extractWikilinks(content),
		Headings:     extractHeadings(body),
		TokenCount:   approximateTokenCount(content),
		UpdatedAt:    updatedAt,
		HasUpdatedAt: hasUpdatedAt,
	}, nil
}

func splitFrontmatter(content string) (string, string) {
	if !strings.HasPrefix(content, "---") {
		return "", content
	}
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", content
	}
	fm := strings.TrimSpace(rest[:idx])
	body := strings.TrimLeft(rest[idx+4:], "\n")
	return fm, body
}

func extractWikilinks(content string) []ParsedLink {
	matches := wikilinkRegexp.FindAllStringSubmatch(content, -1)
	seen := map[string]struct{}{}
	links := make([]ParsedLink, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		raw := strings.TrimSpace(match[1])
		if raw == "" {
			continue
		}
		targetPart := raw
		display := ""
		if idx := strings.Index(targetPart, "|"); idx >= 0 {
			display = strings.TrimSpace(targetPart[idx+1:])
			targetPart = targetPart[:idx]
		}
		target := normalizeLinkTarget(targetPart)
		if target == "" {
			continue
		}
		key := target + "|" + display
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		links = append(links, ParsedLink{
			Target:  target,
			Display: display,
			Raw:     raw,
		})
	}
	return links
}

func extractTags(body string, fm map[string]any) []string {
	seen := map[string]struct{}{}
	var tags []string
	add := func(tag string) {
		tag = normalizeTag(tag)
		if tag == "" {
			return
		}
		if _, ok := seen[tag]; ok {
			return
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}

	if raw, ok := fm["tags"]; ok {
		switch typed := raw.(type) {
		case []any:
			for _, item := range typed {
				if s, ok := item.(string); ok {
					add(s)
				}
			}
		case []string:
			for _, item := range typed {
				add(item)
			}
		case string:
			for _, part := range strings.Split(typed, ",") {
				add(part)
			}
		}
	}

	insideFence := false
	fenceMarker := ""
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if nextInside, nextMarker, toggled := advanceFenceState(insideFence, fenceMarker, trimmed); toggled {
			insideFence = nextInside
			fenceMarker = nextMarker
			continue
		}
		if insideFence || strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, match := range inlineTagRegexp.FindAllStringSubmatch(line, -1) {
			if len(match) >= 2 {
				add(match[1])
			}
		}
	}

	slices.Sort(tags)
	return tags
}

func extractHeadings(body string) []Heading {
	var headings []Heading
	insideFence := false
	fenceMarker := ""
	for i, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if nextInside, nextMarker, toggled := advanceFenceState(insideFence, fenceMarker, trimmed); toggled {
			insideFence = nextInside
			fenceMarker = nextMarker
			continue
		}
		if insideFence {
			continue
		}
		match := headingRegexp.FindStringSubmatch(trimmed)
		if len(match) != 3 {
			continue
		}
		headings = append(headings, Heading{
			Level: len(match[1]),
			Text:  strings.TrimSpace(match[2]),
			Line:  i + 1,
		})
	}
	return headings
}

func advanceFenceState(inside bool, marker string, trimmed string) (bool, string, bool) {
	if trimmed == "" {
		return inside, marker, false
	}
	if inside {
		if isClosingFence(trimmed, marker) {
			return false, "", true
		}
		return inside, marker, false
	}
	if opener := openingFenceMarker(trimmed); opener != "" {
		return true, opener, true
	}
	return inside, marker, false
}

func openingFenceMarker(trimmed string) string {
	if trimmed == "" {
		return ""
	}
	first := trimmed[0]
	if first != '`' && first != '~' {
		return ""
	}
	run := 0
	for run < len(trimmed) && trimmed[run] == first {
		run++
	}
	if run < 3 {
		return ""
	}
	return strings.Repeat(string(first), run)
}

func isClosingFence(trimmed string, marker string) bool {
	if marker == "" || len(trimmed) < len(marker) {
		return false
	}
	markerChar := marker[0]
	run := 0
	for run < len(trimmed) && trimmed[run] == markerChar {
		run++
	}
	if run < len(marker) {
		return false
	}
	return strings.TrimSpace(trimmed[run:]) == ""
}

func extractUpdatedAt(fm map[string]any) (time.Time, bool) {
	for _, key := range []string{"updated_at", "updated", "modified", "last_updated"} {
		raw, ok := fm[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case time.Time:
			return typed.UTC(), true
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed == "" {
				continue
			}
			for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04:05", "2006-01-02 15:04"} {
				parsed, err := time.Parse(layout, trimmed)
				if err == nil {
					return parsed.UTC(), true
				}
			}
		}
	}
	return time.Time{}, false
}

func normalizeLinkTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if idx := strings.Index(target, "#"); idx >= 0 {
		target = target[:idx]
	}
	target = strings.TrimSpace(strings.ReplaceAll(target, `\`, "/"))
	target = path.Clean(target)
	if target == "." {
		return ""
	}
	return strings.TrimSuffix(target, ".md")
}

func normalizeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	tag = strings.TrimPrefix(tag, "#")
	return strings.ToLower(tag)
}

func extractTitle(docPath, body string) string {
	for _, heading := range extractHeadings(body) {
		if heading.Level == 1 {
			return heading.Text
		}
	}
	base := path.Base(docPath)
	return strings.TrimSuffix(base, path.Ext(base))
}

func approximateTokenCount(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return (len(text) + 3) / 4
}
