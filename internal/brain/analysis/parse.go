package analysis

import (
	"strings"

	brainparser "github.com/ponchione/sodoryard/internal/brain/parser"
)

func ParseDocument(docPath, content string) (Document, error) {
	parsed, err := brainparser.ParseDocument(docPath, content)
	if err != nil {
		return Document{}, err
	}

	seenLinks := map[string]struct{}{}
	wikilinks := make([]string, 0, len(parsed.Wikilinks))
	for _, link := range parsed.Wikilinks {
		target := strings.TrimSpace(link.Target)
		if target == "" {
			continue
		}
		if _, ok := seenLinks[target]; ok {
			continue
		}
		seenLinks[target] = struct{}{}
		wikilinks = append(wikilinks, target)
	}

	return Document{
		Path:         parsed.Path,
		Content:      parsed.Content,
		Frontmatter:  parsed.Frontmatter,
		Tags:         append([]string(nil), parsed.Tags...),
		Wikilinks:    wikilinks,
		UpdatedAt:    parsed.UpdatedAt,
		HasUpdatedAt: parsed.HasUpdatedAt,
		Title:        parsed.Title,
	}, nil
}

func normalizeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	tag = strings.TrimPrefix(tag, "#")
	return strings.ToLower(tag)
}
