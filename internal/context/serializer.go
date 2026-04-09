package context

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type serializedCodeEntry struct {
	order       int
	name        string
	description string
	body        string
	language    string
	lineStart   int
	lineEnd     int
}

// MarkdownSerializer renders a BudgetResult into the stable cache-block-2 format.
type MarkdownSerializer struct{}

// Serialize converts selected budget content into deterministic markdown.
func (MarkdownSerializer) Serialize(result *BudgetResult, seenFiles SeenFileLookup) (string, error) {
	if result == nil || budgetResultIsEmpty(result) {
		return "", nil
	}

	var sections []string
	if brain := serializeProjectBrain(result.SelectedBrainHits); brain != "" {
		sections = append(sections, brain)
	}
	if code := serializeRelevantCode(result, seenFiles); code != "" {
		sections = append(sections, code)
	}
	if structural := serializeStructuralContext(result.SelectedGraphHits); structural != "" {
		sections = append(sections, structural)
	}
	if conventions := serializeConventions(result.ConventionText); conventions != "" {
		sections = append(sections, conventions)
	}
	if git := serializeGitContext(result.GitContext); git != "" {
		sections = append(sections, git)
	}
	return strings.Join(sections, "\n\n"), nil
}

func budgetResultIsEmpty(result *BudgetResult) bool {
	return len(result.SelectedFileResults) == 0 &&
		len(result.SelectedBrainHits) == 0 &&
		len(result.SelectedRAGHits) == 0 &&
		len(result.SelectedGraphHits) == 0 &&
		strings.TrimSpace(result.ConventionText) == "" &&
		strings.TrimSpace(result.GitContext) == ""
}

func serializeRelevantCode(result *BudgetResult, seenFiles SeenFileLookup) string {
	groups := make(map[string][]serializedCodeEntry)

	for _, file := range result.SelectedFileResults {
		groups[file.FilePath] = append(groups[file.FilePath], serializedCodeEntry{
			order:       0,
			name:        "Explicit file requested by user",
			description: "Explicit file requested by user.",
			body:        file.Content,
			language:    languageTag(file.FilePath, ""),
		})
	}
	for _, hit := range result.SelectedRAGHits {
		groups[hit.FilePath] = append(groups[hit.FilePath], serializedCodeEntry{
			order:       1,
			name:        hit.Name,
			description: hit.Description,
			body:        hit.Body,
			language:    languageTag(hit.FilePath, hit.Language),
			lineStart:   hit.LineStart,
			lineEnd:     hit.LineEnd,
		})
	}
	if len(groups) == 0 {
		return ""
	}

	paths := make([]string, 0, len(groups))
	for path := range groups {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var b strings.Builder
	b.WriteString("## Relevant Code\n")
	for i, path := range paths {
		if i > 0 {
			b.WriteString("\n")
		}
		header := "### " + path
		if seenFiles != nil {
			if seen, turn := seenFiles.Contains(path); seen {
				header += fmt.Sprintf(" [previously viewed in turn %d]", turn)
			}
		}
		b.WriteString(header)
		b.WriteString("\n\n")

		entries := groups[path]
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].order != entries[j].order {
				return entries[i].order < entries[j].order
			}
			if entries[i].lineStart != entries[j].lineStart {
				return entries[i].lineStart < entries[j].lineStart
			}
			return entries[i].name < entries[j].name
		})

		for idx, entry := range entries {
			if idx > 0 {
				b.WriteString("\n")
			}
			if entry.lineStart > 0 && entry.lineEnd > 0 {
				b.WriteString(fmt.Sprintf("%s (lines %d-%d)\n", entry.name, entry.lineStart, entry.lineEnd))
			} else {
				b.WriteString(entry.name)
				b.WriteString("\n")
			}
			if strings.TrimSpace(entry.description) != "" {
				b.WriteString(entry.description)
				b.WriteString("\n")
			}
			b.WriteString("```")
			b.WriteString(entry.language)
			b.WriteString("\n")
			b.WriteString(strings.TrimSpace(entry.body))
			b.WriteString("\n```\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func serializeStructuralContext(hits []GraphHit) string {
	if len(hits) == 0 {
		return ""
	}
	sorted := append([]GraphHit(nil), hits...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].FilePath != sorted[j].FilePath {
			return sorted[i].FilePath < sorted[j].FilePath
		}
		if sorted[i].RelationshipType != sorted[j].RelationshipType {
			return sorted[i].RelationshipType < sorted[j].RelationshipType
		}
		return sorted[i].SymbolName < sorted[j].SymbolName
	})

	var lines []string
	for _, hit := range sorted {
		line := fmt.Sprintf("- %s: %s — %s", hit.RelationshipType, hit.SymbolName, hit.FilePath)
		if hit.Depth > 0 {
			line += fmt.Sprintf(" (depth %d)", hit.Depth)
		}
		lines = append(lines, line)
	}
	return "## Structural Context\n\n" + strings.Join(lines, "\n")
}

func serializeProjectBrain(hits []BrainHit) string {
	if len(hits) == 0 {
		return ""
	}
	sorted := append([]BrainHit(nil), hits...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].MatchScore != sorted[j].MatchScore {
			return sorted[i].MatchScore > sorted[j].MatchScore
		}
		return sorted[i].DocumentPath < sorted[j].DocumentPath
	})

	var sections []string
	for _, hit := range sorted {
		var b strings.Builder
		heading := strings.TrimSpace(hit.Title)
		if heading == "" {
			heading = hit.DocumentPath
		}
		b.WriteString("### ")
		b.WriteString(heading)
		b.WriteString("\n")
		b.WriteString("Path: `")
		b.WriteString(hit.DocumentPath)
		b.WriteString("`\n")
		if mode := strings.TrimSpace(hit.MatchMode); mode != "" {
			b.WriteString("Match: ")
			b.WriteString(mode)
			b.WriteString("\n")
		}
		if len(hit.Tags) > 0 {
			tags := append([]string(nil), hit.Tags...)
			sort.Strings(tags)
			b.WriteString("Tags: ")
			b.WriteString(strings.Join(tags, ", "))
			b.WriteString("\n")
		}
		if snippet := strings.TrimSpace(hit.Snippet); snippet != "" {
			b.WriteString("\n")
			b.WriteString(formatBrainExcerpt(snippet))
		}
		sections = append(sections, strings.TrimSpace(b.String()))
	}
	return "## Project Brain\n\n" + strings.Join(sections, "\n\n")
}

func formatBrainExcerpt(text string) string {
	lines := nonEmptyLines(text)
	if len(lines) == 0 {
		return ""
	}
	for i, line := range lines {
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n")
}

func serializeConventions(text string) string {
	lines := normalizeBulletLines(text)
	if len(lines) == 0 {
		return ""
	}
	return "## Project Conventions\n\n" + strings.Join(lines, "\n")
}

func serializeGitContext(text string) string {
	lines := nonEmptyLines(text)
	if len(lines) == 0 {
		return ""
	}
	return fmt.Sprintf("## Recent Changes (last %d commits)\n\n%s", len(lines), strings.Join(lines, "\n"))
}

func normalizeBulletLines(text string) []string {
	lines := nonEmptyLines(text)
	for i, line := range lines {
		if strings.HasPrefix(line, "- ") {
			continue
		}
		lines[i] = "- " + line
	}
	return lines
}

func nonEmptyLines(text string) []string {
	rawLines := strings.Split(text, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func languageTag(filePath string, explicit string) string {
	if explicit != "" {
		return explicit
	}
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".sql":
		return "sql"
	case ".md":
		return "markdown"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return "text"
	}
}
