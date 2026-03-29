package indexer

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ponchione/sirtopham/internal/codeintel"
)

// langFromExt maps a file extension to the language identifier.
func langFromExt(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".md":
		return "markdown"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".py":
		return "python"
	default:
		return ""
	}
}

// matchesAnyGlob returns true if relPath matches any of the provided glob patterns.
func matchesAnyGlob(patterns []string, relPath string) bool {
	for _, pat := range patterns {
		if matchesGlob(pat, relPath) {
			return true
		}
	}
	return false
}

// matchesGlob checks a single glob pattern against a relative path.
func matchesGlob(pattern, relPath string) bool {
	// Handle **/ prefix: match any directory depth.
	if strings.HasPrefix(pattern, "**/") {
		suffix := pattern[3:]
		if matched, _ := filepath.Match(suffix, filepath.Base(relPath)); matched {
			return true
		}
		parts := strings.Split(relPath, "/")
		for i := range parts {
			sub := strings.Join(parts[i:], "/")
			if matched, _ := filepath.Match(suffix, sub); matched {
				return true
			}
		}
		return false
	}

	// Trailing / means directory prefix match.
	if trimmed, ok := strings.CutSuffix(pattern, "/"); ok {
		return strings.HasPrefix(relPath, pattern) || strings.HasPrefix(relPath, trimmed)
	}

	matched, _ := filepath.Match(pattern, relPath)
	return matched
}

// newChunk creates a fully populated Chunk from a RawChunk.
func newChunk(raw codeintel.RawChunk, projectName, filePath, language, description string) codeintel.Chunk {
	body := raw.Body
	if len(body) > codeintel.MaxBodyLength {
		body = body[:codeintel.MaxBodyLength]
	}

	return codeintel.Chunk{
		ID:               codeintel.ChunkID(filePath, raw.ChunkType, raw.Name, raw.LineStart),
		ProjectName:      projectName,
		FilePath:         filePath,
		Language:         language,
		ChunkType:        raw.ChunkType,
		Name:             raw.Name,
		Signature:        raw.Signature,
		Body:             body,
		Description:      description,
		LineStart:        raw.LineStart,
		LineEnd:          raw.LineEnd,
		ContentHash:      codeintel.ContentHash(body),
		IndexedAt:        time.Now(),
		Calls:            raw.Calls,
		TypesUsed:        raw.TypesUsed,
		ImplementsIfaces: raw.Implements,
		Imports:          raw.Imports,
	}
}

// formatRelationshipContext renders chunk relationship metadata as text
// for the describer's relationshipContext parameter.
func formatRelationshipContext(chunks []codeintel.Chunk) string {
	if len(chunks) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Relationship Context\n\n")

	hasContent := false
	for _, c := range chunks {
		if len(c.Calls) == 0 && len(c.CalledBy) == 0 && len(c.TypesUsed) == 0 &&
			len(c.ImplementsIfaces) == 0 {
			continue
		}

		hasContent = true
		fmt.Fprintf(&b, "### %s (%s)\n", c.Name, c.ChunkType)

		if len(c.Calls) > 0 {
			b.WriteString("Calls: ")
			names := make([]string, len(c.Calls))
			for i, ref := range c.Calls {
				if ref.Package != "" {
					names[i] = ref.Package + "." + ref.Name
				} else {
					names[i] = ref.Name
				}
			}
			b.WriteString(strings.Join(names, ", "))
			b.WriteByte('\n')
		}

		if len(c.CalledBy) > 0 {
			b.WriteString("Called by: ")
			names := make([]string, len(c.CalledBy))
			for i, ref := range c.CalledBy {
				if ref.Package != "" {
					names[i] = ref.Package + "." + ref.Name
				} else {
					names[i] = ref.Name
				}
			}
			b.WriteString(strings.Join(names, ", "))
			b.WriteByte('\n')
		}

		if len(c.TypesUsed) > 0 {
			fmt.Fprintf(&b, "Types used: %s\n", strings.Join(c.TypesUsed, ", "))
		}

		if len(c.ImplementsIfaces) > 0 {
			fmt.Fprintf(&b, "Implements: %s\n", strings.Join(c.ImplementsIfaces, ", "))
		}

		b.WriteByte('\n')
	}

	if !hasContent {
		return ""
	}
	return strings.TrimRight(b.String(), "\n")
}
