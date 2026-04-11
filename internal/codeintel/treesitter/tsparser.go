package treesitter

import (
	"fmt"
	"log/slog"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"

	"github.com/ponchione/sodoryard/internal/codeintel"
)

// parseTypeScript uses tree-sitter to extract top-level declarations from
// TypeScript or TSX source code.
func parseTypeScript(content []byte, isTSX bool) ([]codeintel.RawChunk, error) {
	parser := sitter.NewParser()
	defer parser.Close()

	var lang *sitter.Language
	if isTSX {
		lang = sitter.NewLanguage(typescript.LanguageTSX())
	} else {
		lang = sitter.NewLanguage(typescript.LanguageTypescript())
	}

	if err := parser.SetLanguage(lang); err != nil {
		return nil, fmt.Errorf("tree-sitter set language: %w", err)
	}

	tree := parser.Parse(content, nil)
	if tree == nil {
		slog.Warn("tree-sitter returned nil tree", "language", "typescript")
		return nil, nil
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
		chunks = append(chunks, extractTSNode(node, content)...)
	}

	return chunks, nil
}

func extractTSNode(node *sitter.Node, content []byte) []codeintel.RawChunk {
	switch node.Kind() {
	case "function_declaration":
		return extractNamedDecl(node, content, codeintel.ChunkTypeFunction)
	case "class_declaration":
		return extractNamedDecl(node, content, codeintel.ChunkTypeClass)
	case "interface_declaration":
		return extractNamedDecl(node, content, codeintel.ChunkTypeInterface)
	case "type_alias_declaration":
		return extractTypeAlias(node, content)
	case "enum_declaration":
		return extractNamedDecl(node, content, codeintel.ChunkTypeEnum)
	case "export_statement":
		for j := uint(0); j < node.ChildCount(); j++ {
			child := node.Child(j)
			if child == nil {
				continue
			}
			if chunks := extractTSNode(child, content); len(chunks) > 0 {
				for k := range chunks {
					chunks[k].LineStart = int(node.StartPosition().Row) + 1
					chunks[k].LineEnd = int(node.EndPosition().Row) + 1
				}
				return chunks
			}
		}
		return nil
	case "lexical_declaration":
		return extractLexicalDecl(node, content)
	default:
		return nil
	}
}

func extractNamedDecl(node *sitter.Node, content []byte, chunkType codeintel.ChunkType) []codeintel.RawChunk {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nameNode.Utf8Text(content)
	if name == "" {
		return nil
	}

	text := string(content[node.StartByte():node.EndByte()])
	sig := extractTSSignature(text)

	body := codeintel.TruncateUTF8(text, codeintel.MaxBodyLength)

	return []codeintel.RawChunk{{
		Name:      name,
		Signature: sig,
		Body:      body,
		ChunkType: chunkType,
		LineStart: int(node.StartPosition().Row) + 1,
		LineEnd:   int(node.EndPosition().Row) + 1,
	}}
}

func extractTypeAlias(node *sitter.Node, content []byte) []codeintel.RawChunk {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nameNode.Utf8Text(content)
	if name == "" {
		return nil
	}

	text := codeintel.TruncateUTF8(string(content[node.StartByte():node.EndByte()]), codeintel.MaxBodyLength)

	return []codeintel.RawChunk{{
		Name:      name,
		Signature: text,
		Body:      text,
		ChunkType: codeintel.ChunkTypeType,
		LineStart: int(node.StartPosition().Row) + 1,
		LineEnd:   int(node.EndPosition().Row) + 1,
	}}
}

func extractLexicalDecl(node *sitter.Node, content []byte) []codeintel.RawChunk {
	var chunks []codeintel.RawChunk

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil || child.Kind() != "variable_declarator" {
			continue
		}

		nameNode := child.ChildByFieldName("name")
		valueNode := child.ChildByFieldName("value")
		if nameNode == nil || valueNode == nil {
			continue
		}

		valKind := valueNode.Kind()
		if valKind != "arrow_function" && valKind != "function" {
			continue
		}

		name := nameNode.Utf8Text(content)
		if name == "" {
			continue
		}

		text := string(content[node.StartByte():node.EndByte()])
		sig := extractTSSignature(text)

		body := codeintel.TruncateUTF8(text, codeintel.MaxBodyLength)

		chunks = append(chunks, codeintel.RawChunk{
			Name:      name,
			Signature: sig,
			Body:      body,
			ChunkType: codeintel.ChunkTypeFunction,
			LineStart: int(node.StartPosition().Row) + 1,
			LineEnd:   int(node.EndPosition().Row) + 1,
		})
	}

	return chunks
}

func extractTSSignature(text string) string {
	if idx := strings.Index(text, "{"); idx != -1 {
		return strings.TrimRight(text[:idx], " \t\n\r")
	}
	if idx := strings.Index(text, "\n"); idx != -1 {
		return text[:idx]
	}
	return text
}
