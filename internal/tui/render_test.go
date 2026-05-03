package tui

import (
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/operator"
)

func TestDashboardRenderIncludesStableFragments(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	model.screen = screenDashboard
	updated, _ := model.Update(model.refreshCmd()())
	got := updated.(Model)

	view := got.View()
	for _, want := range []string{"Dashboard", "project: project", "provider: codex", "auth: not checked", "code index: indexed at 2026-05-01T12:00:00Z commit abc123", "brain index: disabled", "local services: disabled", "chain-1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("dashboard view missing %q:\n%s", want, view)
		}
	}
}

func TestChatRenderIncludesTranscriptAndComposer(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	model.chatConversationID = "chat-1"
	model.chatMessages = []operator.ChatMessage{
		{Role: "user", Content: "draft a spec"},
		{Role: "assistant", Content: "Here is a spec outline."},
	}
	model.chatComposer.SetValue("next step")
	model.chatEdit = true
	updated, _ := model.Update(model.refreshCmd()())
	got := updated.(Model)

	view := got.View()
	for _, want := range []string{"Chat", "runtime codex:test-model", "YOU", "draft a spec", "ASSISTANT", "Here is a spec outline.", "next step"} {
		if !strings.Contains(view, want) {
			t.Fatalf("chat view missing %q:\n%s", want, view)
		}
	}
}

func TestChatRenderFormatsMarkdownBasics(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	model.chatMessages = []operator.ChatMessage{
		{Role: "assistant", Content: "# Plan\n- write spec\n1. review it\n```go\nfunc main() {}\n```"},
	}
	updated, _ := model.Update(model.refreshCmd()())
	got := updated.(Model)

	view := got.View()
	for _, want := range []string{"Plan", "- write spec", "1. review it", "code go", "func main() {}"} {
		if !strings.Contains(view, want) {
			t.Fatalf("chat markdown render missing %q:\n%s", want, view)
		}
	}
}

func TestReceiptRenderIncludesContent(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	model.screen = screenReceipts
	updated, _ := model.Update(model.refreshCmd()())
	got := updated.(Model)

	view := got.View()
	for _, want := range []string{"Receipts", "chain: chain-1", "orchestrator", "orchestrator receipt"} {
		if !strings.Contains(view, want) {
			t.Fatalf("receipt view missing %q:\n%s", want, view)
		}
	}
}

func TestRenderShowsActiveFilter(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	model.screen = screenChains
	model.chainFilter = "coder"
	updated, _ := model.Update(model.refreshCmd()())
	got := updated.(Model)

	view := got.View()
	if !strings.Contains(view, "filter: coder (1/2 chains)") {
		t.Fatalf("chain view missing active filter:\n%s", view)
	}
	if strings.Contains(view, "chain-2") {
		t.Fatalf("chain view rendered filtered-out chain:\n%s", view)
	}
}

func TestLaunchRenderShowsRoleListControls(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	updated, _ := model.Update(model.refreshCmd()())
	got := updated.(Model)
	got.screen = screenLaunch

	view := got.View()
	for _, want := range []string{"n add role", "- ", "remove role", "ctrl+u clear roles"} {
		if !strings.Contains(view, want) {
			t.Fatalf("launch view missing %q:\n%s", want, view)
		}
	}
}
