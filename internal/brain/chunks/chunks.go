package chunks

import (
	"fmt"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/brain/parser"
	"github.com/ponchione/sodoryard/internal/codeintel"
)

const ShortDocumentThreshold = 1000

type Chunk struct {
	ID                   string
	ChunkIndex           int
	DocumentPath         string
	DocumentTitle        string
	Tags                 []string
	SectionHeading       string
	Text                 string
	LineStart            int
	LineEnd              int
	DocumentContentHash  string
	DocumentUpdatedAt    time.Time
	HasDocumentUpdatedAt bool
}

func BuildDocument(doc parser.Document) []Chunk {
	body := strings.TrimSpace(doc.Body)
	if body == "" {
		body = strings.TrimSpace(doc.Content)
	}
	if body == "" {
		return nil
	}

	sectionHeadings := levelTwoHeadings(doc.Headings)
	if len(body) <= ShortDocumentThreshold || len(sectionHeadings) == 0 {
		return []Chunk{newChunk(doc, 0, "", body, 1, lineCount(body))}
	}

	lines := strings.Split(doc.Body, "\n")
	chunks := make([]Chunk, 0, len(sectionHeadings))
	for i, heading := range sectionHeadings {
		startIdx := heading.Line - 1
		endLine := len(lines)
		if i+1 < len(sectionHeadings) {
			endLine = sectionHeadings[i+1].Line - 1
		}
		if startIdx < 0 || startIdx >= len(lines) || endLine < heading.Line {
			continue
		}
		text := strings.TrimSpace(strings.Join(lines[startIdx:endLine], "\n"))
		if text == "" {
			continue
		}
		chunks = append(chunks, newChunk(doc, len(chunks), heading.Text, text, heading.Line, endLine))
	}
	if len(chunks) == 0 {
		return []Chunk{newChunk(doc, 0, "", body, 1, lineCount(body))}
	}
	return chunks
}

func newChunk(doc parser.Document, idx int, sectionHeading string, text string, lineStart, lineEnd int) Chunk {
	text = codeintel.TruncateUTF8(strings.TrimSpace(text), codeintel.MaxBodyLength)
	if lineEnd < lineStart {
		lineEnd = lineStart
	}
	return Chunk{
		ID:                   chunkID(doc, idx),
		ChunkIndex:           idx,
		DocumentPath:         doc.Path,
		DocumentTitle:        doc.Title,
		Tags:                 append([]string(nil), doc.Tags...),
		SectionHeading:       sectionHeading,
		Text:                 text,
		LineStart:            lineStart,
		LineEnd:              lineEnd,
		DocumentContentHash:  doc.ContentHash,
		DocumentUpdatedAt:    doc.UpdatedAt,
		HasDocumentUpdatedAt: doc.HasUpdatedAt,
	}
}

func chunkID(doc parser.Document, idx int) string {
	return codeintel.ContentHash(fmt.Sprintf("brain:%s:%s:%d", doc.Path, doc.ContentHash, idx))
}

func levelTwoHeadings(headings []parser.Heading) []parser.Heading {
	out := make([]parser.Heading, 0, len(headings))
	for _, heading := range headings {
		if heading.Level == 2 {
			out = append(out, heading)
		}
	}
	return out
}

func lineCount(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}
