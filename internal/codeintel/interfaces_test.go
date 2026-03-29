package codeintel

import "testing"

type parserStub struct{}

func (parserStub) Parse(filePath string, content []byte) ([]RawChunk, error) {
	return []RawChunk{}, nil
}

func TestParserInterfaceIsSatisfied(t *testing.T) {
	var parser Parser = parserStub{}

	chunks, err := parser.Parse("main.go", []byte("package main"))
	if err != nil {
		t.Fatalf("Parse returned unexpected error: %v", err)
	}
	if chunks == nil {
		t.Fatal("Parse returned nil slice, want empty slice")
	}
}
