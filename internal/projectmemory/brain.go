package projectmemory

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
)

type BrainBackend struct {
	runtime *Runtime
	owned   bool
}

func OpenBrainBackend(ctx context.Context, cfg Config) (*BrainBackend, error) {
	runtime, err := Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &BrainBackend{runtime: runtime, owned: true}, nil
}

func NewBrainBackend(runtime *Runtime) *BrainBackend {
	return &BrainBackend{runtime: runtime}
}

func (b *BrainBackend) Close() error {
	if b == nil || !b.owned || b.runtime == nil {
		return nil
	}
	return b.runtime.Close()
}

func (b *BrainBackend) ReadDocument(ctx context.Context, path string) (string, error) {
	_, content, err := b.runtime.ReadDocument(ctx, path)
	return content, err
}

func (b *BrainBackend) WriteDocument(ctx context.Context, path string, content string) error {
	normalized, err := normalizeDocumentPath(path)
	if err != nil {
		return err
	}
	return b.runtime.WriteDocument(ctx, WriteDocumentArgs{
		Path:    normalized,
		Content: content,
		Actor:   "brain_backend",
		Kind:    inferDocumentKind(normalized),
		Title:   inferDocumentTitle(normalized, content),
	})
}

func (b *BrainBackend) PatchDocument(ctx context.Context, path string, operation string, content string) error {
	doc, current, err := b.runtime.ReadDocument(ctx, path)
	if err != nil {
		return err
	}
	updated, err := applyPatchOperation(current, operation, content)
	if err != nil {
		return err
	}
	return b.PatchDocumentWithExpectedHash(ctx, path, operation, doc.ContentHash, updated)
}

func (b *BrainBackend) PatchDocumentWithExpectedHash(ctx context.Context, path string, operation string, expectedOldHash string, newContent string) error {
	normalized, err := normalizeDocumentPath(path)
	if err != nil {
		return err
	}
	return b.runtime.PatchDocument(ctx, PatchDocumentArgs{
		Path:            normalized,
		Operation:       operation,
		ExpectedOldHash: expectedOldHash,
		NewContent:      newContent,
		Actor:           "brain_backend",
		Kind:            inferDocumentKind(normalized),
		Title:           inferDocumentTitle(normalized, newContent),
	})
}

func (b *BrainBackend) SearchKeyword(ctx context.Context, query string) ([]brain.SearchHit, error) {
	return b.SearchKeywordLimit(ctx, query, 10)
}

func (b *BrainBackend) SearchKeywordLimit(ctx context.Context, query string, maxResults int) ([]brain.SearchHit, error) {
	hits, err := b.runtime.SearchDocuments(ctx, query, maxResults)
	if err != nil {
		return nil, err
	}
	out := make([]brain.SearchHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, brain.SearchHit{Path: hit.Path, Snippet: hit.Snippet, Score: hit.Score})
	}
	return out, nil
}

func (b *BrainBackend) ListDocuments(ctx context.Context, directory string) ([]string, error) {
	return b.runtime.ListDocuments(ctx, directory)
}

func (b *BrainBackend) ReadBrainIndexState(ctx context.Context) (BrainIndexState, bool, error) {
	return b.runtime.ReadBrainIndexState(ctx)
}

func (b *BrainBackend) MarkBrainIndexClean(ctx context.Context, indexedAt time.Time, metadataJSON string) error {
	return b.runtime.MarkBrainIndexClean(ctx, MarkBrainIndexCleanArgs{
		ProjectID:       DefaultProjectID,
		LastIndexedAtUS: uint64(indexedAt.UTC().UnixMicro()),
		MetadataJSON:    metadataJSON,
	})
}

func (b *BrainBackend) ReadCodeIndexState(ctx context.Context) (CodeIndexState, bool, error) {
	return b.runtime.ReadCodeIndexState(ctx)
}

func (b *BrainBackend) ListCodeFileIndexStates(ctx context.Context) ([]CodeFileIndexState, error) {
	return b.runtime.ListCodeFileIndexStates(ctx)
}

func (b *BrainBackend) MarkCodeIndexClean(ctx context.Context, revision string, indexedAt time.Time, files []CodeFileIndexArg, deletedPaths []string, metadataJSON string) error {
	return b.runtime.MarkCodeIndexClean(ctx, MarkCodeIndexCleanArgs{
		ProjectID:         DefaultProjectID,
		LastIndexedCommit: revision,
		LastIndexedAtUS:   uint64(indexedAt.UTC().UnixMicro()),
		Files:             files,
		DeletedPaths:      deletedPaths,
		MetadataJSON:      metadataJSON,
	})
}

func inferDocumentKind(path string) string {
	switch {
	case strings.HasPrefix(path, "conventions/"):
		return "convention"
	case strings.HasPrefix(path, "receipts/"):
		return "receipt"
	default:
		return documentKindBrain
	}
}

func inferDocumentTitle(path string, content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			title := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			if title != "" {
				return title
			}
		}
	}
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func applyPatchOperation(current string, operation string, content string) (string, error) {
	switch operation {
	case "append":
		if !strings.HasSuffix(current, "\n") {
			current += "\n"
		}
		return current + "\n" + content, nil
	case "prepend":
		if !strings.HasPrefix(current, "---") {
			return content + "\n\n" + current, nil
		}
		rest := current[3:]
		idx := strings.Index(rest, "\n---")
		if idx < 0 {
			return content + "\n\n" + current, nil
		}
		fmEnd := 3 + idx + 4
		fm := current[:fmEnd]
		body := strings.TrimLeft(current[fmEnd:], "\n")
		return fm + "\n\n" + content + "\n\n" + body, nil
	case "replace_section":
		heading := firstPatchHeading(content)
		if heading == "" {
			return "", fmt.Errorf("replace_section content must start with a heading")
		}
		return replaceSectionContent(current, heading, content)
	default:
		return "", fmt.Errorf("unsupported patch operation: %s", operation)
	}
}

func firstPatchHeading(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			return line
		}
		if line != "" {
			return ""
		}
	}
	return ""
}

func replaceSectionContent(current string, section string, newContent string) (string, error) {
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
		if level == targetLevel && strings.TrimSpace(text) == strings.TrimSpace(targetText) {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return "", fmt.Errorf("Section '%s' not found", strings.TrimSpace(targetText))
	}
	end := len(lines)
	for i := targetIdx + 1; i < len(lines); i++ {
		level, _ := parseHeading(lines[i])
		if level > 0 && level <= targetLevel {
			end = i
			break
		}
	}
	replacement := strings.Split(strings.TrimRight(newContent, "\n"), "\n")
	out := append([]string{}, lines[:targetIdx]...)
	out = append(out, replacement...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n"), nil
}

func parseHeading(line string) (int, string) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return 0, ""
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level >= len(trimmed) || trimmed[level] != ' ' {
		return 0, ""
	}
	return level, strings.TrimSpace(trimmed[level:])
}
