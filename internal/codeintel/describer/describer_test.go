package describer

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/codeintel"
)

type mockLLM struct {
	systemPrompt string
	userMessage  string
	response     string
}

func (m *mockLLM) Complete(_ context.Context, sys, user string) (string, error) {
	m.systemPrompt = sys
	m.userMessage = user
	return m.response, nil
}

func TestDescribeFile_BasicJSON(t *testing.T) {
	mock := &mockLLM{
		response: `[{"name":"Login","description":"Authenticates user and creates session"}]`,
	}
	d := New(mock, "test system prompt")

	descs, err := d.DescribeFile(context.Background(), "func Login() {}", "")
	if err != nil {
		t.Fatalf("DescribeFile: %v", err)
	}
	if len(descs) != 1 {
		t.Fatalf("got %d descriptions, want 1", len(descs))
	}
	if descs[0].Name != "Login" {
		t.Errorf("Name = %q, want Login", descs[0].Name)
	}
	if descs[0].Description != "Authenticates user and creates session" {
		t.Errorf("Description = %q", descs[0].Description)
	}
}

func TestDescribeFile_WithCodeFence(t *testing.T) {
	mock := &mockLLM{
		response: "```json\n[{\"name\":\"Foo\",\"description\":\"Does foo\"}]\n```",
	}
	d := New(mock, "prompt")

	descs, err := d.DescribeFile(context.Background(), "func Foo() {}", "")
	if err != nil {
		t.Fatalf("DescribeFile: %v", err)
	}
	if len(descs) != 1 || descs[0].Name != "Foo" {
		t.Errorf("expected Foo, got %#v", descs)
	}
}

func TestDescribeFile_WithRelationshipContext(t *testing.T) {
	mock := &mockLLM{
		response: `[{"name":"Login","description":"Auth handler"}]`,
	}
	d := New(mock, "prompt")

	relCtx := "=== RELATIONSHIP CONTEXT ===\nFunction: Login\n  Calls: Validate (auth)\n"
	_, err := d.DescribeFile(context.Background(), "func Login() {}", relCtx)
	if err != nil {
		t.Fatalf("DescribeFile: %v", err)
	}
	if !strings.Contains(mock.userMessage, "RELATIONSHIP CONTEXT") {
		t.Error("user message should include relationship context")
	}
}

func TestDescribeFile_InvalidJSON(t *testing.T) {
	mock := &mockLLM{response: "not json"}
	d := New(mock, "prompt")

	descs, err := d.DescribeFile(context.Background(), "code", "")
	if err != nil {
		t.Fatalf("DescribeFile should not return error for invalid JSON, got: %v", err)
	}
	if len(descs) != 0 {
		t.Errorf("expected empty slice for invalid JSON, got %d", len(descs))
	}
}

func TestDescribeFile_TruncatesContent(t *testing.T) {
	mock := &mockLLM{response: `[]`}
	d := New(mock, "prompt")

	longContent := strings.Repeat("x", MaxDescriptionFileLength+1000)
	_, err := d.DescribeFile(context.Background(), longContent, "")
	if err != nil {
		t.Fatalf("DescribeFile: %v", err)
	}
	// User message should be truncated.
	if len(mock.userMessage) > MaxDescriptionFileLength+200 {
		t.Errorf("user message too long: %d", len(mock.userMessage))
	}
}

func TestFormatRelationshipContext(t *testing.T) {
	chunks := []codeintel.Chunk{
		{
			Name:  "Login",
			Calls: []codeintel.FuncRef{{Name: "ValidateToken", Package: "auth"}},
			CalledBy: []codeintel.FuncRef{{Name: "HandleLogin", Package: "handler"}},
			TypesUsed:        []string{"LoginRequest"},
			ImplementsIfaces: []string{"Authenticator"},
		},
	}

	got := FormatRelationshipContext(chunks)

	for _, want := range []string{
		"=== RELATIONSHIP CONTEXT ===",
		"Function: Login",
		"Calls: ValidateToken (auth)",
		"Called by: HandleLogin (handler)",
		"Types used: LoginRequest",
		"Implements: Authenticator",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatRelationshipContext_Empty(t *testing.T) {
	got := FormatRelationshipContext([]codeintel.Chunk{{Name: "Foo"}})
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestDescriberImplementsInterface(t *testing.T) {
	var _ codeintel.Describer = (*Describer)(nil)
}

type errorLLM struct{}

func (e *errorLLM) Complete(_ context.Context, _, _ string) (string, error) {
	return "", fmt.Errorf("connection refused")
}

func TestDescribeFile_LLMError_ReturnsNilNil(t *testing.T) {
	d := New(&errorLLM{}, "system prompt")
	descs, err := d.DescribeFile(context.Background(), "func main() {}", "")
	if err != nil {
		t.Fatalf("expected nil error for LLM failure, got: %v", err)
	}
	if descs != nil {
		t.Errorf("expected nil descriptions, got %v", descs)
	}
}

func TestDescribeFile_ContextCancelled_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before calling

	d := New(&mockLLM{response: `[{"name":"Foo","description":"bar"}]`}, "system prompt")
	_, err := d.DescribeFile(ctx, "func main() {}", "")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
