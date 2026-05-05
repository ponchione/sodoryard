package context

import (
	stdctx "context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ponchione/sodoryard/internal/brain"
)

const defaultConventionBulletLimit = 10

// NoopConventionSource is the v0.1 placeholder convention loader.
//
// It remains useful for tests and runtimes that intentionally do not inject a
// real convention cache.
type NoopConventionSource struct{}

// Load returns no conventions and no error.
func (NoopConventionSource) Load(stdctx.Context) (string, error) {
	return "", nil
}

// BrainBackendConventionSource loads project conventions through Shunter
// project memory.
type BrainBackendConventionSource struct {
	backend     brain.Backend
	bulletLimit int
}

func NewBrainBackendConventionSource(backend brain.Backend) *BrainBackendConventionSource {
	return &BrainBackendConventionSource{
		backend:     backend,
		bulletLimit: defaultConventionBulletLimit,
	}
}

func (s *BrainBackendConventionSource) Load(ctx stdctx.Context) (string, error) {
	if s == nil || s.backend == nil {
		return "", nil
	}
	paths, err := s.backend.ListDocuments(ctx, "conventions")
	if err != nil {
		return "", nil
	}
	sort.Strings(paths)
	limit := s.bulletLimit
	if limit <= 0 {
		limit = defaultConventionBulletLimit
	}
	bullets := make([]string, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, path := range paths {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			continue
		}
		content, err := s.backend.ReadDocument(ctx, path)
		if err != nil {
			return "", fmt.Errorf("read convention document %s: %w", filepath.Base(path), err)
		}
		for _, bullet := range extractConventionBullets(filepath.Base(path), content) {
			bullet = strings.TrimSpace(bullet)
			if bullet == "" {
				continue
			}
			if _, ok := seen[bullet]; ok {
				continue
			}
			seen[bullet] = struct{}{}
			bullets = append(bullets, bullet)
			if len(bullets) >= limit {
				return strings.Join(bullets, "\n"), nil
			}
		}
	}
	return strings.Join(bullets, "\n"), nil
}

func extractConventionBullets(filename string, content string) []string {
	body := stripMarkdownFrontmatter(content)
	lines := strings.Split(body, "\n")

	var (
		title       string
		bullets     []string
		paragraph   []string
		inCodeFence bool
	)
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			if len(paragraph) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "```") {
			inCodeFence = !inCodeFence
			continue
		}
		if inCodeFence {
			continue
		}
		if heading, ok := markdownHeading(line); ok {
			if title == "" {
				title = heading
			}
			continue
		}
		if bullet, ok := markdownBullet(line); ok {
			bullets = append(bullets, bullet)
			continue
		}
		paragraph = append(paragraph, line)
	}
	if len(bullets) > 0 {
		return bullets
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.ToSlash(filename), filepath.Ext(filename))
		title = strings.ReplaceAll(title, "-", " ")
		title = strings.ReplaceAll(title, "_", " ")
		title = strings.TrimSpace(title)
	}
	if len(paragraph) == 0 {
		if title == "" {
			return nil
		}
		return []string{title}
	}
	summary := strings.Join(paragraph, " ")
	summary = strings.Join(strings.Fields(summary), " ")
	if title == "" {
		return []string{summary}
	}
	return []string{fmt.Sprintf("%s: %s", title, summary)}
}

func stripMarkdownFrontmatter(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---\n") {
		return content
	}
	lines := strings.Split(trimmed, "\n")
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return content
}

func markdownHeading(line string) (string, bool) {
	if !strings.HasPrefix(line, "#") {
		return "", false
	}
	heading := strings.TrimLeft(line, "#")
	heading = strings.TrimSpace(heading)
	if heading == "" {
		return "", false
	}
	return heading, true
}

func markdownBullet(line string) (string, bool) {
	for _, prefix := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(line, prefix) {
			bullet := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if bullet == "" {
				return "", false
			}
			return bullet, true
		}
	}
	return "", false
}
