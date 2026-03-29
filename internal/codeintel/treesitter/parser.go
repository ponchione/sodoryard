package treesitter

import (
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"

	"github.com/ponchione/sirtopham/internal/codeintel"
)

// Parser dispatches to language-specific tree-sitter parsers or a fallback splitter.
type Parser struct{}

// New returns a new tree-sitter Parser.
func New() *Parser { return &Parser{} }

// Parse dispatches to a language-specific tree-sitter parser based on file extension.
func (p *Parser) Parse(filePath string, content []byte) ([]codeintel.RawChunk, error) {
	lang := detectLanguage(filePath)

	switch lang {
	case "go":
		return parseGo(content)
	case "markdown":
		return parseMarkdown(content)
	case "typescript":
		return parseTypeScript(content, false)
	case "tsx":
		return parseTypeScript(content, true)
	case "python":
		return parsePython(content)
	default:
		return parseFallback(filePath, content), nil
	}
}

// detectLanguage infers the language from a file path extension.
func detectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".md", ".markdown":
		return "markdown"
	case ".sql":
		return "sql"
	default:
		return ""
	}
}

// parseGo uses tree-sitter to extract top-level functions, methods, and type
// declarations from Go source code.
func parseGo(content []byte) ([]codeintel.RawChunk, error) {
	parser := sitter.NewParser()
	defer parser.Close()

	if err := parser.SetLanguage(sitter.NewLanguage(golang.Language())); err != nil {
		return nil, fmt.Errorf("tree-sitter set language: %w", err)
	}

	tree := parser.Parse(content, nil)
	if tree == nil {
		return nil, fmt.Errorf("tree-sitter parse returned nil tree")
	}
	defer tree.Close()

	root := tree.RootNode()
	childCount := root.ChildCount()

	var chunks []codeintel.RawChunk
	for i := uint(0); i < childCount; i++ {
		node := root.Child(i)
		if node == nil {
			continue
		}

		var chunkType codeintel.ChunkType
		switch node.Kind() {
		case "function_declaration":
			chunkType = codeintel.ChunkTypeFunction
		case "method_declaration":
			chunkType = codeintel.ChunkTypeMethod
		case "type_declaration":
			chunkType = codeintel.ChunkTypeType
		default:
			continue
		}

		name := extractGoName(node, chunkType, content)
		if name == "" {
			continue
		}

		sig := extractGoSignature(node, chunkType, content)
		body := string(content[node.StartByte():node.EndByte()])
		if len(body) > codeintel.MaxBodyLength {
			body = body[:codeintel.MaxBodyLength]
		}

		chunks = append(chunks, codeintel.RawChunk{
			Name:      name,
			Signature: sig,
			Body:      body,
			ChunkType: chunkType,
			LineStart: int(node.StartPosition().Row) + 1,
			LineEnd:   int(node.EndPosition().Row) + 1,
		})
	}

	return chunks, nil
}

func extractGoName(node *sitter.Node, chunkType codeintel.ChunkType, content []byte) string {
	if chunkType == codeintel.ChunkTypeType {
		for i := uint(0); i < node.ChildCount(); i++ {
			child := node.Child(i)
			if child == nil {
				continue
			}
			if child.Kind() == "type_spec" {
				nameNode := child.ChildByFieldName("name")
				if nameNode != nil {
					return nameNode.Utf8Text(content)
				}
			}
		}
		return ""
	}
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return ""
	}
	return nameNode.Utf8Text(content)
}

func extractGoSignature(node *sitter.Node, chunkType codeintel.ChunkType, content []byte) string {
	if chunkType == codeintel.ChunkTypeType {
		text := string(content[node.StartByte():node.EndByte()])
		if idx := strings.Index(text, "{"); idx != -1 {
			return strings.TrimRight(text[:idx], " \t\n\r")
		}
		if idx := strings.Index(text, "\n"); idx != -1 {
			return text[:idx]
		}
		return text
	}

	bodyNode := node.ChildByFieldName("body")
	if bodyNode == nil {
		return strings.TrimRight(string(content[node.StartByte():node.EndByte()]), " \t\n\r")
	}
	return strings.TrimRight(string(content[node.StartByte():bodyNode.StartByte()]), " \t\n\r")
}

// parseMarkdown splits content on level-2 headers ("## ") into sections.
func parseMarkdown(content []byte) ([]codeintel.RawChunk, error) {
	lines := strings.Split(string(content), "\n")

	var chunks []codeintel.RawChunk
	var currentName string
	var bodyLines []string
	var headerLineStart int
	inSection := false

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1

		if strings.HasPrefix(line, "## ") {
			if inSection && currentName != "" {
				body := strings.Join(bodyLines, "\n")
				if len(body) > codeintel.MaxBodyLength {
					body = body[:codeintel.MaxBodyLength]
				}
				chunks = append(chunks, codeintel.RawChunk{
					Name:      currentName,
					ChunkType: codeintel.ChunkTypeSection,
					Body:      body,
					LineStart: headerLineStart,
					LineEnd:   lineIdx,
				})
			}

			currentName = strings.TrimRight(strings.TrimPrefix(line, "## "), "\r\n")
			bodyLines = nil
			headerLineStart = lineNum
			inSection = true
		} else if inSection {
			bodyLines = append(bodyLines, line)
		}
	}

	if inSection && currentName != "" {
		body := strings.Join(bodyLines, "\n")
		if len(body) > codeintel.MaxBodyLength {
			body = body[:codeintel.MaxBodyLength]
		}
		chunks = append(chunks, codeintel.RawChunk{
			Name:      currentName,
			ChunkType: codeintel.ChunkTypeSection,
			Body:      body,
			LineStart: headerLineStart,
			LineEnd:   len(lines),
		})
	}

	return chunks, nil
}

// parseFallback splits content into overlapping 40-line windows with a 20-line step.
func parseFallback(filePath string, content []byte) []codeintel.RawChunk {
	lines := strings.Split(string(content), "\n")
	var chunks []codeintel.RawChunk

	for start := 0; start < len(lines); start += 20 {
		end := start + 40
		if end > len(lines) {
			end = len(lines)
		}
		body := strings.Join(lines[start:end], "\n")
		if len(body) > codeintel.MaxBodyLength {
			body = body[:codeintel.MaxBodyLength]
		}
		chunks = append(chunks, codeintel.RawChunk{
			Name:      fmt.Sprintf("%s:%d-%d", filePath, start+1, end),
			ChunkType: codeintel.ChunkTypeFallback,
			Body:      body,
			LineStart: start + 1,
			LineEnd:   end,
		})
	}

	return chunks
}
