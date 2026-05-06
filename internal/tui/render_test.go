package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/operator"
)

func TestDashboardRenderIncludesStableFragments(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	model.screen = screenDashboard
	updated, _ := model.Update(model.refreshCmd()())
	got := updated.(Model)

	view := got.View()
	for _, want := range []string{"Dashboard", "project: project", "provider: codex", "auth: ready (oauth, private_store)", "code index: indexed at 2026-05-01T12:00:00Z commit abc123", "brain index: disabled", "local services: disabled", "chain-1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("dashboard view missing %q:\n%s", want, view)
		}
	}
}

func TestDashboardRenderShowsReadinessActions(t *testing.T) {
	fake := newFakeOperator()
	fake.status.AuthStatus = "missing credentials"
	fake.status.CodeIndex = operator.RuntimeIndexStatus{Status: "never_indexed"}
	fake.status.BrainIndex = operator.RuntimeIndexStatus{Status: "stale", StaleReason: "brain_update"}
	fake.status.Warnings = []operator.RuntimeWarning{
		{Message: "code index has not been built; run `yard index` before retrieval/runtime validation"},
		{Message: "brain index is stale; run `yard brain index`"},
	}
	model := NewModel(fake, Options{RefreshInterval: -1})
	model.screen = screenDashboard
	updated, _ := model.Update(model.refreshCmd()())
	got := updated.(Model)

	view := got.View()
	for _, want := range []string{"readiness:", "Readiness", "FAIL", "code index", "fix: yard index", "fix: yard brain index", "Next actions", "yard auth status", "yard doctor"} {
		if !strings.Contains(view, want) {
			t.Fatalf("dashboard readiness view missing %q:\n%s", want, view)
		}
	}
}

func TestStatusLineIncludesWarningCount(t *testing.T) {
	fake := newFakeOperator()
	fake.status.Warnings = []operator.RuntimeWarning{{Message: "degraded runtime"}, {Message: "index stale"}}
	model := NewModel(fake, Options{RefreshInterval: -1})
	updated, _ := model.Update(model.refreshCmd()())
	got := updated.(Model)

	view := got.View()
	if !strings.Contains(view, "warnings:2") {
		t.Fatalf("status line missing warning count:\n%s", view)
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
	for _, want := range []string{"Yard Console", "runtime codex:test-model", "YOU", "draft a spec", "ASSISTANT", "Here is a spec outline.", "next step"} {
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

func TestChatRenderIncludesRunningAndUsageStatus(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	model.chatRunning = true
	view := model.View()
	if !strings.Contains(view, "generating response") || !strings.Contains(view, "ctrl+g cancels") {
		t.Fatalf("chat running view missing cancel affordance:\n%s", view)
	}

	model.chatRunning = false
	model.chatInputTokens = 12
	model.chatOutputTokens = 34
	model.chatStopReason = "stop"
	view = model.View()
	for _, want := range []string{"last turn tokens in:12 out:34", "stop:stop"} {
		if !strings.Contains(view, want) {
			t.Fatalf("chat usage view missing %q:\n%s", want, view)
		}
	}
}

func TestChatRenderWrapsLongWordsWithinWidth(t *testing.T) {
	styles := newStyles()
	lines := renderChatContent(styles, strings.Repeat("x", 45), 20, styles.chatAgent)
	if len(lines) < 2 {
		t.Fatalf("rendered long word into %d lines, want wrapping", len(lines))
	}
	for _, line := range lines {
		if width := lipgloss.Width(line); width > 20 {
			t.Fatalf("rendered line width = %d, want <= 20: %q", width, line)
		}
	}
}

func TestFooterHelpIsScreenSpecific(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	model.screen = screenLaunch
	view := model.View()
	if !strings.Contains(view, "v preview") || strings.Contains(view, "ctrl+g cancel chat") {
		t.Fatalf("launch footer is not screen-specific:\n%s", view)
	}
}

func TestChainRenderShowsHealthBudgetsAndCurrentStep(t *testing.T) {
	fake := newFakeOperator()
	fake.details["chain-1"] = operator.ChainDetail{
		Chain: chain.Chain{ID: "chain-1", Status: "running", SourceTask: "first task", TotalSteps: 1, TotalTokens: 85, TotalDurationSecs: 9, MaxSteps: 2, TokenBudget: 100, MaxDurationSecs: 20, MaxResolverLoops: 2},
		Steps: []chain.Step{{SequenceNum: 1, Role: "coder", Status: "completed", Verdict: "completed", ReceiptPath: "receipts/coder/chain-1-step-001.md", TokensUsed: 85, TurnsUsed: 3, DurationSecs: 9}},
		Receipts: []operator.ReceiptSummary{
			{Label: "step 1 coder", Step: "1", Path: "receipts/coder/chain-1-step-001.md"},
		},
	}
	model := NewModel(fake, Options{RefreshInterval: -1})
	model.screen = screenChains
	updated, _ := model.Update(model.refreshCmd()())
	got := updated.(Model)

	view := got.View()
	for _, want := range []string{"health: attention", "budgets: steps 1/2 (50%)", "tokens 85/100 (85%)", "current: #1 coder completed verdict=completed", "turns=3", "duration=9s"} {
		if !strings.Contains(view, want) {
			t.Fatalf("chain polish view missing %q:\n%s", want, view)
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

func TestReceiptRenderParsesFrontmatter(t *testing.T) {
	fake := newFakeOperator()
	fake.receipts["chain-1:"] = operator.ReceiptView{
		ChainID: "chain-1",
		Path:    "receipts/orchestrator/chain-1.md",
		Content: `---
agent: coder
chain_id: chain-1
step: 1
verdict: completed
timestamp: 2026-05-01T12:00:00Z
turns_used: 3
tokens_used: 99
duration_seconds: 7
---

# Summary

- Changed code.
`,
	}
	model := NewModel(fake, Options{RefreshInterval: -1})
	model.screen = screenReceipts
	updated, _ := model.Update(model.refreshCmd()())
	got := updated.(Model)

	view := got.View()
	for _, want := range []string{"Metadata", "agent: coder  verdict: completed  step: 1", "turns: 3  tokens: 99  duration: 7s", "Body", "Summary", "- Changed code."} {
		if !strings.Contains(view, want) {
			t.Fatalf("receipt parsed view missing %q:\n%s", want, view)
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
