package vault

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/ponchione/sodoryard/internal/brain"
)

var ErrPathTraversal = errors.New("path escapes vault root")

type Client struct {
	root string
}

func New(vaultPath string) (*Client, error) {
	if strings.TrimSpace(vaultPath) == "" {
		return nil, fmt.Errorf("vault path is required")
	}
	root, err := filepath.Abs(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("resolve vault path: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat vault path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("vault path must be a directory: %s", root)
	}
	return &Client{root: root}, nil
}

func (c *Client) ReadDocument(ctx context.Context, path string) (string, error) {
	resolved, err := c.resolve(path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("Document not found: %s", path)
		}
		return "", err
	}
	return string(data), nil
}

func (c *Client) WriteDocument(ctx context.Context, path string, content string) error {
	resolved, err := c.resolve(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(resolved), ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, resolved)
}

func (c *Client) PatchDocument(ctx context.Context, path string, operation string, content string) error {
	current, err := c.ReadDocument(ctx, path)
	if err != nil {
		return err
	}
	var updated string
	switch operation {
	case "append":
		updated = appendContent(current, content)
	case "prepend":
		updated = prependContent(current, content)
	case "replace_section":
		heading := firstHeading(content)
		if heading == "" {
			return fmt.Errorf("replace_section content must start with a heading")
		}
		updated, err = replaceSectionContent(current, heading, content)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported patch operation: %s", operation)
	}
	return c.WriteDocument(ctx, path, updated)
}

func (c *Client) SearchKeyword(ctx context.Context, query string, maxResults int) ([]brain.SearchHit, error) {
	if maxResults <= 0 {
		maxResults = 10
	}
	query = normalizeForKeyword(query)
	if query == "" {
		return nil, nil
	}
	var hits []brain.SearchHit
	err := filepath.WalkDir(c.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".obsidian" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(c.root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		normPath := normalizeForKeyword(filepath.ToSlash(rel))
		normText := normalizeForKeyword(text)
		score := 0.0
		if strings.Contains(normPath, query) {
			score += 2
		}
		count := strings.Count(normText, query)
		if count > 0 {
			score += float64(count)
		}
		if score == 0 {
			return nil
		}
		hits = append(hits, brain.SearchHit{
			Path:    filepath.ToSlash(rel),
			Snippet: snippetAround(text, query),
			Score:   score,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Score > hits[j].Score
	})
	if len(hits) > maxResults {
		hits = hits[:maxResults]
	}
	return hits, nil
}

func (c *Client) ListDocuments(ctx context.Context, directory string) ([]string, error) {
	base := c.root
	if directory != "" {
		resolved, err := c.resolve(directory)
		if err != nil {
			return nil, err
		}
		base = resolved
	}
	var files []string
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("Directory not found: %s", directory)
			}
			return err
		}
		if d.IsDir() {
			if d.Name() == ".obsidian" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(c.root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func (c *Client) resolve(rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", ErrPathTraversal
	}
	if filepath.IsAbs(rel) {
		return "", ErrPathTraversal
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrPathTraversal
	}
	resolved := filepath.Join(c.root, clean)
	relToRoot, err := filepath.Rel(c.root, resolved)
	if err != nil {
		return "", err
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", ErrPathTraversal
	}
	return resolved, nil
}

func appendContent(current, addition string) string {
	if !strings.HasSuffix(current, "\n") {
		current += "\n"
	}
	return current + "\n" + addition
}

func prependContent(current, addition string) string {
	if !strings.HasPrefix(current, "---") {
		return addition + "\n\n" + current
	}
	rest := current[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return addition + "\n\n" + current
	}
	fmEnd := 3 + idx + 4
	fm := current[:fmEnd]
	body := strings.TrimLeft(current[fmEnd:], "\n")
	return fm + "\n\n" + addition + "\n\n" + body
}

func replaceSectionContent(current, section, newContent string) (string, error) {
	lines := strings.Split(current, "\n")
	targetLevel, targetText := parseHeading(section)
	if targetLevel == 0 {
		targetText = section
	}
	targetIdx := -1
	for i, line := range lines {
		level, text := parseHeading(line)
		if level == 0 {
			continue
		}
		if targetLevel > 0 && level == targetLevel && strings.TrimSpace(text) == strings.TrimSpace(targetText) {
			targetIdx = i
			break
		}
		if targetLevel == 0 && strings.TrimSpace(text) == strings.TrimSpace(targetText) {
			targetIdx = i
			targetLevel = level
			break
		}
	}
	if targetIdx < 0 {
		return "", fmt.Errorf("Section '%s' not found", section)
	}
	endIdx := len(lines)
	for i := targetIdx + 1; i < len(lines); i++ {
		level, _ := parseHeading(lines[i])
		if level > 0 && level <= targetLevel {
			endIdx = i
			break
		}
	}
	parts := append([]string{}, lines[:targetIdx]...)
	parts = append(parts, strings.Split(newContent, "\n")...)
	if endIdx < len(lines) {
		parts = append(parts, lines[endIdx:]...)
	}
	return strings.Join(parts, "\n"), nil
}

func parseHeading(line string) (int, string) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return 0, ""
	}
	level := 0
	for _, ch := range trimmed {
		if ch == '#' {
			level++
		} else {
			break
		}
	}
	if level == 0 || level > 6 {
		return 0, ""
	}
	return level, strings.TrimSpace(trimmed[level:])
}

func firstHeading(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if level, text := parseHeading(line); level > 0 {
			return strings.Repeat("#", level) + " " + text
		}
	}
	return ""
}

func snippetAround(content string, query string) string {
	if len(content) <= 120 {
		return content
	}
	lower := strings.ToLower(content)
	idx := strings.Index(lower, query)
	if idx < 0 {
		// query may be a normalized multi-word phrase that does not appear
		// literally in the raw note body (because of punctuation or
		// hyphens between the words). Fall back to the first word of the
		// query to at least anchor the snippet near a relevant region.
		firstWord := query
		if sp := strings.IndexByte(query, ' '); sp > 0 {
			firstWord = query[:sp]
		}
		if firstWord != "" {
			idx = strings.Index(lower, firstWord)
		}
	}
	if idx < 0 {
		return content[:120]
	}
	start := max(idx-40, 0)
	end := min(idx+80, len(content))
	return content[start:end]
}

// normalizeForKeyword lowercases s and collapses runs of non-alphanumeric
// characters (whitespace, line breaks, punctuation, hyphens, etc.) into a
// single ASCII space. Leading and trailing separators are stripped. The
// result is suitable for substring matching so that multi-word keyword
// queries can still hit note bodies that contain commas, hyphens, or line
// breaks between the words — e.g. the query "minimal content first layout"
// will now hit a body that says "minimal, content-first layout".
func normalizeForKeyword(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := true
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			prevSpace = false
			continue
		}
		if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	out := b.String()
	return strings.TrimRight(out, " ")
}
