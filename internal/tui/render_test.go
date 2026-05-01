package tui

import (
	"strings"
	"testing"
)

func TestDashboardRenderIncludesStableFragments(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	updated, _ := model.Update(model.refreshCmd()())
	got := updated.(Model)

	view := got.View()
	for _, want := range []string{"Dashboard", "project: project", "provider: codex", "chain-1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("dashboard view missing %q:\n%s", want, view)
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
