package treesitter

import (
	"fmt"
	"log/slog"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	python "github.com/tree-sitter/tree-sitter-python/bindings/go"

	"github.com/ponchione/sirtopham/internal/codeintel"
)

// parsePython uses tree-sitter to extract top-level functions, classes, and
// methods from Python source code.
func parsePython(content []byte) ([]codeintel.RawChunk, error) {
	parser := sitter.NewParser()
	defer parser.Close()

	if err := parser.SetLanguage(sitter.NewLanguage(python.Language())); err != nil {
		return nil, fmt.Errorf("tree-sitter set language: %w", err)
	}

	tree := parser.Parse(content, nil)
	if tree == nil {
		slog.Warn("tree-sitter returned nil tree", "language", "python")
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
		chunks = append(chunks, extractPyNode(node, content)...)
	}

	return chunks, nil
}

func extractPyNode(node *sitter.Node, content []byte) []codeintel.RawChunk {
	switch node.Kind() {
	case "function_definition":
		return extractPyFunc(node, node, content, codeintel.ChunkTypeFunction)
	case "class_definition":
		return extractPyClass(node, node, content)
	case "decorated_definition":
		return extractPyDecorated(node, content)
	default:
		return nil
	}
}

func extractPyDecorated(outer *sitter.Node, content []byte) []codeintel.RawChunk {
	for i := uint(0); i < outer.ChildCount(); i++ {
		child := outer.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "function_definition":
			return extractPyFunc(outer, child, content, codeintel.ChunkTypeFunction)
		case "class_definition":
			return extractPyClass(outer, child, content)
		}
	}
	return nil
}

func extractPyFunc(spanNode, defNode *sitter.Node, content []byte, chunkType codeintel.ChunkType) []codeintel.RawChunk {
	nameNode := defNode.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nameNode.Utf8Text(content)
	if name == "" {
		return nil
	}

	bodyField := defNode.ChildByFieldName("body")
	var sig string
	if bodyField != nil {
		sig = strings.TrimRight(string(content[spanNode.StartByte():bodyField.StartByte()]), " \t\n\r")
	} else {
		sig = strings.TrimRight(string(content[spanNode.StartByte():spanNode.EndByte()]), " \t\n\r")
	}

	body := string(content[spanNode.StartByte():spanNode.EndByte()])
	if len(body) > codeintel.MaxBodyLength {
		body = body[:codeintel.MaxBodyLength]
	}

	return []codeintel.RawChunk{{
		Name:      name,
		Signature: sig,
		Body:      body,
		ChunkType: chunkType,
		LineStart: int(spanNode.StartPosition().Row) + 1,
		LineEnd:   int(spanNode.EndPosition().Row) + 1,
	}}
}

func extractPyClass(spanNode, defNode *sitter.Node, content []byte) []codeintel.RawChunk {
	nameNode := defNode.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nameNode.Utf8Text(content)
	if name == "" {
		return nil
	}

	bodyField := defNode.ChildByFieldName("body")
	var sig string
	if bodyField != nil {
		sig = strings.TrimRight(string(content[spanNode.StartByte():bodyField.StartByte()]), " \t\n\r")
	} else {
		sig = strings.TrimRight(string(content[spanNode.StartByte():spanNode.EndByte()]), " \t\n\r")
	}

	classBody := string(content[spanNode.StartByte():spanNode.EndByte()])
	if len(classBody) > codeintel.MaxBodyLength {
		classBody = classBody[:codeintel.MaxBodyLength]
	}

	chunks := []codeintel.RawChunk{{
		Name:      name,
		Signature: sig,
		Body:      classBody,
		ChunkType: codeintel.ChunkTypeClass,
		LineStart: int(spanNode.StartPosition().Row) + 1,
		LineEnd:   int(spanNode.EndPosition().Row) + 1,
	}}

	if bodyField != nil {
		for i := uint(0); i < bodyField.ChildCount(); i++ {
			child := bodyField.Child(i)
			if child == nil {
				continue
			}
			switch child.Kind() {
			case "function_definition":
				chunks = append(chunks, extractPyFunc(child, child, content, codeintel.ChunkTypeMethod)...)
			case "decorated_definition":
				for j := uint(0); j < child.ChildCount(); j++ {
					inner := child.Child(j)
					if inner != nil && inner.Kind() == "function_definition" {
						chunks = append(chunks, extractPyFunc(child, inner, content, codeintel.ChunkTypeMethod)...)
						break
					}
				}
			}
		}
	}

	return chunks
}
