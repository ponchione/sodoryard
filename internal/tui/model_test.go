package tui

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/operator"
)

type fakeOperator struct {
	status         operator.RuntimeStatus
	roles          []operator.AgentRoleSummary
	chains         []operator.ChainSummary
	details        map[string]operator.ChainDetail
	receipts       map[string]operator.ReceiptView
	eventsSince    map[string][]chain.Event
	launchRequest  operator.LaunchRequest
	startRequest   operator.LaunchRequest
	savedDraft     operator.LaunchDraft
	loadDraft      operator.LaunchDraft
	loadDraftFound bool
	customPresets  []operator.LaunchPreset
	savedPreset    operator.LaunchPreset
	pausedChain    string
	cancelledChain string
}

func newFakeOperator() *fakeOperator {
	started := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	return &fakeOperator{
		status: operator.RuntimeStatus{
			ProjectRoot: "/tmp/project",
			ProjectName: "project",
			Provider:    "codex",
			Model:       "test-model",
			AuthStatus:  "not checked",
			CodeIndex: operator.RuntimeIndexStatus{
				Status:            "indexed",
				LastIndexedAt:     "2026-05-01T12:00:00Z",
				LastIndexedCommit: "abc123",
			},
			BrainIndex:          operator.RuntimeIndexStatus{Status: "disabled"},
			LocalServicesStatus: "disabled",
			ActiveChains:        1,
		},
		roles: []operator.AgentRoleSummary{{Name: "coder"}, {Name: "orchestrator"}, {Name: "planner"}},
		chains: []operator.ChainSummary{
			{ID: "chain-1", Status: "running", SourceTask: "first task", SourceSpecs: []string{"docs/specs/first.md"}, TotalSteps: 1, TotalTokens: 12, StartedAt: started, UpdatedAt: started, CurrentStep: &operator.StepSummary{SequenceNum: 1, Role: "coder", Status: "completed", ReceiptPath: "receipts/coder/chain-1-step-001.md"}},
			{ID: "chain-2", Status: "completed", SourceTask: "second task", SourceSpecs: []string{"docs/specs/second.md"}, TotalSteps: 2, TotalTokens: 34, StartedAt: started, UpdatedAt: started, CurrentStep: &operator.StepSummary{SequenceNum: 1, Role: "planner", Status: "completed", ReceiptPath: "receipts/planner/chain-2-step-001.md"}},
		},
		details: map[string]operator.ChainDetail{
			"chain-1": {
				Chain: chain.Chain{ID: "chain-1", Status: "running", SourceTask: "first task"},
				Steps: []chain.Step{{SequenceNum: 1, Role: "coder", Status: "completed", Verdict: "accepted", ReceiptPath: "receipts/coder/chain-1-step-001.md"}},
				Receipts: []operator.ReceiptSummary{
					{Label: "orchestrator", Path: "receipts/orchestrator/chain-1.md"},
					{Label: "step 1 coder", Step: "1", Path: "receipts/coder/chain-1-step-001.md"},
				},
				RecentEvents: []chain.Event{{ID: 1, EventType: chain.EventStepStarted, EventData: `{"role":"coder"}`, CreatedAt: started}},
			},
			"chain-2": {
				Chain:    chain.Chain{ID: "chain-2", Status: "completed", SourceTask: "second task"},
				Steps:    []chain.Step{{SequenceNum: 1, Role: "planner", Status: "completed", ReceiptPath: "receipts/planner/chain-2-step-001.md"}},
				Receipts: []operator.ReceiptSummary{{Label: "orchestrator", Path: "receipts/orchestrator/chain-2.md"}},
			},
		},
		receipts: map[string]operator.ReceiptView{
			"chain-1:":  {ChainID: "chain-1", Path: "receipts/orchestrator/chain-1.md", Content: "orchestrator receipt"},
			"chain-1:1": {ChainID: "chain-1", Step: "1", Path: "receipts/coder/chain-1-step-001.md", Content: "step receipt"},
			"chain-2:":  {ChainID: "chain-2", Path: "receipts/orchestrator/chain-2.md", Content: "chain 2 receipt"},
		},
		eventsSince: map[string][]chain.Event{},
	}
}

func (f *fakeOperator) RuntimeStatus(context.Context) (operator.RuntimeStatus, error) {
	return f.status, nil
}

func (f *fakeOperator) ListAgentRoles(context.Context) ([]operator.AgentRoleSummary, error) {
	return append([]operator.AgentRoleSummary(nil), f.roles...), nil
}

func (f *fakeOperator) ListChains(context.Context, int) ([]operator.ChainSummary, error) {
	return append([]operator.ChainSummary(nil), f.chains...), nil
}

func (f *fakeOperator) GetChainDetail(_ context.Context, chainID string) (operator.ChainDetail, error) {
	detail, ok := f.details[chainID]
	if !ok {
		return operator.ChainDetail{}, fmt.Errorf("missing detail %s", chainID)
	}
	return detail, nil
}

func (f *fakeOperator) ReadReceipt(_ context.Context, chainID string, step string) (operator.ReceiptView, error) {
	receipt, ok := f.receipts[chainID+":"+step]
	if !ok {
		return operator.ReceiptView{}, fmt.Errorf("missing receipt %s:%s", chainID, step)
	}
	return receipt, nil
}

func (f *fakeOperator) ListEventsSince(_ context.Context, chainID string, afterID int64) ([]chain.Event, error) {
	var events []chain.Event
	if detail, ok := f.details[chainID]; ok {
		events = append(events, detail.RecentEvents...)
	}
	events = append(events, f.eventsSince[chainID]...)
	filtered := make([]chain.Event, 0, len(events))
	for _, event := range events {
		if event.ID > afterID {
			filtered = append(filtered, event)
		}
	}
	return filtered, nil
}

func (f *fakeOperator) PauseChain(_ context.Context, chainID string) (operator.ControlResult, error) {
	f.pausedChain = chainID
	return operator.ControlResult{ChainID: chainID, Message: "pause requested"}, nil
}

func (f *fakeOperator) CancelChain(_ context.Context, chainID string) (operator.ControlResult, error) {
	f.cancelledChain = chainID
	return operator.ControlResult{ChainID: chainID, Message: "cancel requested"}, nil
}

func (f *fakeOperator) ValidateLaunch(_ context.Context, req operator.LaunchRequest) (operator.LaunchPreview, error) {
	f.launchRequest = req
	if strings.TrimSpace(req.SourceTask) == "" && len(req.SourceSpecs) == 0 {
		return operator.LaunchPreview{}, fmt.Errorf("one of task or specs is required")
	}
	role := req.Role
	roster := append([]string(nil), req.Roster...)
	allowedRoles := append([]string(nil), req.AllowedRoles...)
	summary := "preview " + role
	if req.Mode == operator.LaunchModeManualRoster {
		if len(roster) == 0 && role != "" {
			roster = strings.Split(role, ",")
		}
		if len(roster) == 0 {
			return operator.LaunchPreview{}, fmt.Errorf("manual roster requires at least one role")
		}
		role = strings.Join(roster, ",")
		summary = "preview manual roster"
	}
	if req.Mode == operator.LaunchModeConstrained {
		if len(allowedRoles) == 0 && role != "" {
			allowedRoles = strings.Split(role, ",")
		}
		if len(allowedRoles) == 0 {
			return operator.LaunchPreview{}, fmt.Errorf("constrained orchestration requires at least one allowed role")
		}
		role = "orchestrator"
		summary = "preview constrained orchestration"
	}
	if role == "" {
		role = "orchestrator"
		summary = "preview " + role
	}
	compiled := req.SourceTask
	if len(req.SourceSpecs) > 0 {
		if compiled != "" {
			compiled += "\n\n"
		}
		compiled += "Specs: " + strings.Join(req.SourceSpecs, ", ")
	}
	if len(allowedRoles) > 0 {
		if compiled != "" {
			compiled += "\n\n"
		}
		compiled += "Allowed roles: " + strings.Join(allowedRoles, ", ")
	}
	return operator.LaunchPreview{
		Mode:         req.Mode,
		Role:         role,
		AllowedRoles: allowedRoles,
		Roster:       roster,
		Summary:      summary,
		CompiledTask: compiled,
	}, nil
}

func (f *fakeOperator) StartChain(_ context.Context, req operator.LaunchRequest) (operator.StartResult, error) {
	f.startRequest = req
	started := time.Date(2026, 5, 1, 12, 1, 0, 0, time.UTC)
	ch := operator.ChainSummary{ID: "chain-started", Status: "running", SourceTask: req.SourceTask, StartedAt: started, UpdatedAt: started}
	f.chains = append([]operator.ChainSummary{ch}, f.chains...)
	f.details["chain-started"] = operator.ChainDetail{Chain: chain.Chain{ID: "chain-started", Status: "running", SourceTask: req.SourceTask}}
	return operator.StartResult{ChainID: "chain-started", Status: "running", Preview: operator.LaunchPreview{Mode: req.Mode, Role: req.Role, Summary: "started"}}, nil
}

func (f *fakeOperator) SaveLaunchDraft(_ context.Context, req operator.LaunchRequest) (operator.LaunchDraft, error) {
	f.savedDraft = operator.LaunchDraft{ID: "current", Request: req, UpdatedAt: "2026-05-01T12:02:00Z"}
	return f.savedDraft, nil
}

func (f *fakeOperator) LoadLaunchDraft(context.Context) (operator.LaunchDraft, bool, error) {
	return f.loadDraft, f.loadDraftFound, nil
}

func (f *fakeOperator) ListLaunchPresets(context.Context) ([]operator.LaunchPreset, error) {
	return append([]operator.LaunchPreset(nil), f.customPresets...), nil
}

func (f *fakeOperator) SaveLaunchPreset(_ context.Context, name string, req operator.LaunchRequest) (operator.LaunchPreset, error) {
	req.SourceTask = ""
	req.SourceSpecs = nil
	preset := operator.LaunchPreset{ID: "custom:" + name, Name: name, Request: req, UpdatedAt: "2026-05-01T12:04:00Z"}
	f.savedPreset = preset
	f.customPresets = append(f.customPresets, preset)
	return preset, nil
}

func TestModelRefreshLoadsOperatorData(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	updated, _ := model.Update(model.refreshCmd()())
	got := updated.(Model)

	if got.status.ProjectName != "project" {
		t.Fatalf("ProjectName = %q, want project", got.status.ProjectName)
	}
	if len(got.chains) != 2 {
		t.Fatalf("chain count = %d, want 2", len(got.chains))
	}
	if got.detail == nil || got.detail.Chain.ID != "chain-1" {
		t.Fatalf("detail = %+v, want chain-1", got.detail)
	}
	if len(got.receiptItems) != 2 {
		t.Fatalf("receipt item count = %d, want orchestrator plus step", len(got.receiptItems))
	}
	if got.launch.Role != "coder" || got.launch.Mode != operator.LaunchModeOneStep || got.activeLaunchPresetName() != "solo coder" {
		t.Fatalf("launch defaults = %+v, want coder one-step", got.launch)
	}
}

func TestModelMovesChainSelectionAndReloadsDetail(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenChains

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	got = updated.(Model)
	if got.chainCursor != 1 {
		t.Fatalf("chainCursor = %d, want 1", got.chainCursor)
	}
	if cmd == nil {
		t.Fatal("move selection returned nil refresh command")
	}

	updated, _ = got.Update(cmd())
	got = updated.(Model)
	if got.detail == nil || got.detail.Chain.ID != "chain-2" {
		t.Fatalf("detail = %+v, want chain-2", got.detail)
	}
}

func TestModelLoadsSelectedReceipt(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	model.screen = screenReceipts
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	if got.receipt == nil || got.receipt.Content != "orchestrator receipt" {
		t.Fatalf("initial receipt = %+v, want orchestrator receipt", got.receipt)
	}

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	got = updated.(Model)
	if got.receiptCursor != 1 {
		t.Fatalf("receiptCursor = %d, want 1", got.receiptCursor)
	}
	if cmd == nil {
		t.Fatal("receipt move returned nil refresh command")
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)
	if got.receipt == nil || got.receipt.Content != "step receipt" {
		t.Fatalf("selected receipt = %+v, want step receipt", got.receipt)
	}
}

func TestModelEntersAndExitsFilterEditMode(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenChains

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("starting filter edit returned command")
	}
	if !got.filterEdit || got.filterScreen != screenChains {
		t.Fatalf("filter edit state = edit %t screen %v, want chains edit", got.filterEdit, got.filterScreen)
	}

	for _, r := range []rune("ru") {
		updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		got = updated.(Model)
		if cmd == nil {
			t.Fatalf("typing %q did not request filtered selection refresh", string(r))
		}
	}
	if got.chainFilter != "ru" {
		t.Fatalf("chainFilter = %q, want ru", got.chainFilter)
	}
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	got = updated.(Model)
	if got.chainFilter != "r" {
		t.Fatalf("chainFilter after backspace = %q, want r", got.chainFilter)
	}
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	got = updated.(Model)
	if got.chainFilter != "" {
		t.Fatalf("chainFilter after ctrl+u = %q, want empty", got.chainFilter)
	}
	for _, r := range []rune("chain-2") {
		updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		got = updated.(Model)
	}
	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("escaping filter edit returned command")
	}
	if got.filterEdit || got.chainFilter != "chain-2" {
		t.Fatalf("filter state after esc = edit %t query %q, want kept chain-2", got.filterEdit, got.chainFilter)
	}
	if got.notice != "chain filter kept: chain-2" {
		t.Fatalf("notice = %q, want explicit keep notice", got.notice)
	}
}

func TestChainFilterMatching(t *testing.T) {
	chains := []operator.ChainSummary{
		{
			ID:          "chain-alpha",
			Status:      "running",
			SourceTask:  "repair auth flow",
			SourceSpecs: []string{"docs/specs/auth.md"},
			CurrentStep: &operator.StepSummary{SequenceNum: 2, Role: "coder", Status: "pending"},
		},
		{
			ID:          "chain-beta",
			Status:      "completed",
			SourceTask:  "write docs",
			SourceSpecs: []string{"docs/specs/docs.md"},
			CurrentStep: &operator.StepSummary{SequenceNum: 1, Role: "planner", Status: "completed"},
		},
	}
	tests := map[string][]string{
		"alpha":       {"chain-alpha"},
		"completed":   {"chain-beta"},
		"auth":        {"chain-alpha"},
		"docs.md":     {"chain-beta"},
		"coder":       {"chain-alpha"},
		"planner":     {"chain-beta"},
		"pending":     {"chain-alpha"},
		"repair flow": {"chain-alpha"},
	}
	for query, want := range tests {
		if got := chainIDs(filterChains(chains, query)); !reflect.DeepEqual(got, want) {
			t.Fatalf("filterChains(%q) = %v, want %v", query, got, want)
		}
	}
}

func TestReceiptFilterMatching(t *testing.T) {
	items := []receiptItem{
		{Label: "orchestrator", Path: "receipts/orchestrator/chain-1.md"},
		{Label: "step 2 coder", Step: "2", Path: "receipts/coder/chain-1-step-002.md"},
	}
	loaded := &operator.ReceiptView{Step: "2", Path: "receipts/coder/chain-1-step-002.md", Content: "visible receipt content with retry details"}
	tests := map[string][]string{
		"orchestrator":  {"orchestrator"},
		"2":             {"step 2 coder"},
		"coder":         {"step 2 coder"},
		"step-002":      {"step 2 coder"},
		"retry details": {"step 2 coder"},
	}
	for query, want := range tests {
		if got := receiptLabels(filterReceiptItems(items, loaded, query)); !reflect.DeepEqual(got, want) {
			t.Fatalf("filterReceiptItems(%q) = %v, want %v", query, got, want)
		}
	}
	if got := receiptLabels(filterReceiptItems(items, nil, "retry details")); len(got) != 0 {
		t.Fatalf("filterReceiptItems without loaded content = %v, want no content match", got)
	}
}

func TestModelClampsCursorAfterFilterChanges(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenChains
	got.chainCursor = 1
	loaded, _ = got.Update(got.refreshCmd()())
	got = loaded.(Model)
	if got.detail == nil || got.detail.Chain.ID != "chain-2" {
		t.Fatalf("detail before filter = %+v, want chain-2", got.detail)
	}

	got.filterEdit = true
	got.filterScreen = screenChains
	updated, cmd := got.updateCurrentFilter("chain-1")
	got = updated.(Model)
	if got.chainCursor != 0 || got.selectedVisibleChainID() != "chain-1" {
		t.Fatalf("filtered selection = cursor %d id %q, want chain-1 at 0", got.chainCursor, got.selectedVisibleChainID())
	}
	if cmd == nil {
		t.Fatal("chain filter change did not request selected detail refresh")
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)
	if got.detail == nil || got.detail.Chain.ID != "chain-1" {
		t.Fatalf("detail after filter = %+v, want chain-1", got.detail)
	}

	got.screen = screenReceipts
	loaded, _ = got.Update(got.refreshCmd()())
	got = loaded.(Model)
	got.receiptCursor = 1
	loaded, _ = got.Update(got.refreshCmd()())
	got = loaded.(Model)
	if got.receipt == nil || got.receipt.Step != "1" {
		t.Fatalf("receipt before filter = %+v, want step 1", got.receipt)
	}
	got.filterEdit = true
	got.filterScreen = screenReceipts
	updated, cmd = got.updateCurrentFilter("orchestrator")
	got = updated.(Model)
	if got.receiptCursor != 0 {
		t.Fatalf("receiptCursor after filter = %d, want 0", got.receiptCursor)
	}
	if cmd == nil {
		t.Fatal("receipt filter change did not request selected receipt refresh")
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)
	if got.receipt == nil || got.receipt.Path != "receipts/orchestrator/chain-1.md" {
		t.Fatalf("receipt after filter = %+v, want orchestrator receipt", got.receipt)
	}
}

func TestModelReceiptListDoesNotInventOrchestratorReceipt(t *testing.T) {
	fake := newFakeOperator()
	fake.chains = []operator.ChainSummary{{ID: "one-step", Status: "completed", SourceTask: "one step"}}
	fake.details = map[string]operator.ChainDetail{
		"one-step": {
			Chain: chain.Chain{ID: "one-step", Status: "completed", SourceTask: "one step"},
			Steps: []chain.Step{{SequenceNum: 1, Role: "coder", Status: "completed", ReceiptPath: "receipts/coder/one-step-step-001.md"}},
			Receipts: []operator.ReceiptSummary{
				{Label: "step 1 coder", Step: "1", Path: "receipts/coder/one-step-step-001.md"},
			},
		},
	}
	fake.receipts = map[string]operator.ReceiptView{
		"one-step:1": {ChainID: "one-step", Step: "1", Path: "receipts/coder/one-step-step-001.md", Content: "one-step receipt"},
	}
	model := NewModel(fake, Options{RefreshInterval: -1})
	model.screen = screenReceipts

	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	if len(got.receiptItems) != 1 {
		t.Fatalf("receipt item count = %d, want one step receipt", len(got.receiptItems))
	}
	if got.receiptItems[0].Label != "step 1 coder" || got.receiptItems[0].Step != "1" {
		t.Fatalf("receipt item = %+v, want step receipt item", got.receiptItems[0])
	}
	view := got.View()
	if strings.Contains(view, "orchestrator") {
		t.Fatalf("receipt view invented orchestrator receipt:\n%s", view)
	}
	if got.receipt == nil || got.receipt.Path != "receipts/coder/one-step-step-001.md" {
		t.Fatalf("receipt = %+v, want one-step step receipt", got.receipt)
	}
}

func chainIDs(chains []operator.ChainSummary) []string {
	ids := make([]string, 0, len(chains))
	for _, ch := range chains {
		ids = append(ids, ch.ID)
	}
	return ids
}

func receiptLabels(items []receiptItem) []string {
	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	return labels
}

func TestModelLaunchDraftEditsAndPreviews(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenLaunch

	updated, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	got = updated.(Model)
	if !got.launchEdit {
		t.Fatal("launch edit mode not enabled")
	}
	for _, r := range []rune("fix tests") {
		updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		got = updated.(Model)
	}
	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(Model)
	if got.launchEdit {
		t.Fatal("enter did not leave launch edit mode")
	}
	if cmd == nil {
		t.Fatal("launch preview returned nil command")
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)
	if fake.launchRequest.SourceTask != "fix tests" || fake.launchRequest.Role != "coder" || fake.launchRequest.Mode != operator.LaunchModeOneStep {
		t.Fatalf("launch request = %+v, want one-step coder task", fake.launchRequest)
	}
	if got.preview == nil || got.preview.Summary != "preview coder" {
		t.Fatalf("preview = %+v, want preview coder", got.preview)
	}
}

func TestModelLaunchSpecsEditAndPreview(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenLaunch

	updated, _ := got.Update(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(Model)
	if got.launchField != launchFieldSpecs {
		t.Fatalf("launchField = %v, want specs", got.launchField)
	}
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(Model)
	if !got.launchEdit {
		t.Fatal("specs edit mode not enabled")
	}
	for _, r := range []rune("specs/a.md, specs/b.md, specs/a.md") {
		updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		got = updated.(Model)
	}
	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("specs edit did not request preview")
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)
	wantSpecs := []string{"specs/a.md", "specs/b.md"}
	if !reflect.DeepEqual(fake.launchRequest.SourceSpecs, wantSpecs) {
		t.Fatalf("SourceSpecs = %v, want %v", fake.launchRequest.SourceSpecs, wantSpecs)
	}
	if got.preview == nil || !strings.Contains(got.preview.CompiledTask, "Specs: specs/a.md, specs/b.md") {
		t.Fatalf("preview = %+v, want compiled specs", got.preview)
	}
}

func TestModelLaunchModeAndRoleControls(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenLaunch

	updated, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	got = updated.(Model)
	if got.launch.Mode != operator.LaunchModeOrchestrator {
		t.Fatalf("mode = %s, want orchestrator", got.launch.Mode)
	}
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got = updated.(Model)
	if got.launch.Role != "orchestrator" {
		t.Fatalf("role = %s, want orchestrator after cycling", got.launch.Role)
	}
}

func TestModelLaunchManualRosterControls(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenLaunch
	got.launch.SourceTask = "ship roster"

	updated, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	got = updated.(Model)
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	got = updated.(Model)
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	got = updated.(Model)
	if got.launch.Mode != operator.LaunchModeManualRoster || !reflect.DeepEqual(got.launch.Roster, []string{"coder"}) {
		t.Fatalf("manual launch state = %+v, want coder roster", got.launch)
	}
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got = updated.(Model)
	if !reflect.DeepEqual(got.launch.Roster, []string{"coder", "orchestrator"}) {
		t.Fatalf("roster = %v, want coder then orchestrator", got.launch.Roster)
	}
	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("manual roster preview returned nil command")
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)
	if fake.launchRequest.Mode != operator.LaunchModeManualRoster || !reflect.DeepEqual(fake.launchRequest.Roster, []string{"coder", "orchestrator"}) {
		t.Fatalf("launch request = %+v, want manual roster", fake.launchRequest)
	}
	view := got.View()
	if !strings.Contains(view, "roster: coder -> orchestrator") || !strings.Contains(view, "preview manual roster") {
		t.Fatalf("manual roster view missing preview fragments:\n%s", view)
	}
}

func TestModelLaunchRoleListRemovalControls(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenLaunch
	got.launch.SourceTask = "ship roster controls"

	for i := 0; i < 3; i++ {
		updated, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
		got = updated.(Model)
	}
	updated, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got = updated.(Model)
	if !reflect.DeepEqual(got.launch.Roster, []string{"coder", "orchestrator"}) {
		t.Fatalf("initial roster = %v, want coder then orchestrator", got.launch.Roster)
	}
	got.preview = &operator.LaunchPreview{Summary: "stale"}
	got.previewReq = &operator.LaunchRequest{SourceTask: "stale"}

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("remove role returned command")
	}
	if !reflect.DeepEqual(got.launch.Roster, []string{"coder"}) || got.launch.Role != "coder" {
		t.Fatalf("roster after remove = %+v, want coder only", got.launch)
	}
	if got.preview != nil || got.previewReq != nil {
		t.Fatalf("preview after remove = %+v/%+v, want cleared", got.preview, got.previewReq)
	}
	if got.notice != "removed orchestrator from manual roster" {
		t.Fatalf("notice = %q, want remove notice", got.notice)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("clear roster returned command")
	}
	if len(got.launch.Roster) != 0 || got.launch.Role != "" {
		t.Fatalf("roster after clear = %+v, want empty role list", got.launch)
	}
	if got.notice != "manual roster cleared" {
		t.Fatalf("notice = %q, want clear notice", got.notice)
	}

	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got = updated.(Model)
	if !reflect.DeepEqual(got.launch.Roster, []string{"coder"}) || got.launch.Role != "coder" {
		t.Fatalf("roster after re-add = %+v, want coder", got.launch)
	}
}

func TestModelLaunchConstrainedOrchestrationControls(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenLaunch
	got.launch.SourceTask = "ship constrained"

	updated, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	got = updated.(Model)
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	got = updated.(Model)
	if got.launch.Mode != operator.LaunchModeConstrained || !reflect.DeepEqual(got.launch.AllowedRoles, []string{"coder"}) {
		t.Fatalf("constrained launch state = %+v, want coder allowed role", got.launch)
	}
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got = updated.(Model)
	if !reflect.DeepEqual(got.launch.AllowedRoles, []string{"coder", "planner"}) {
		t.Fatalf("allowed roles = %v, want coder then planner", got.launch.AllowedRoles)
	}
	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("constrained preview returned nil command")
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)
	if fake.launchRequest.Mode != operator.LaunchModeConstrained || !reflect.DeepEqual(fake.launchRequest.AllowedRoles, []string{"coder", "planner"}) {
		t.Fatalf("launch request = %+v, want constrained allowed roles", fake.launchRequest)
	}
	view := got.View()
	for _, want := range []string{"allowed: coder, planner", "preview constrained orchestration", "Allowed roles: coder, planner"} {
		if !strings.Contains(view, want) {
			t.Fatalf("constrained view missing %q:\n%s", want, view)
		}
	}
}

func TestModelLaunchConstrainedRoleListRemovalControls(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenLaunch

	updated, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	got = updated.(Model)
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	got = updated.(Model)
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got = updated.(Model)
	if !reflect.DeepEqual(got.launch.AllowedRoles, []string{"coder", "planner"}) {
		t.Fatalf("initial allowed roles = %v, want coder then planner", got.launch.AllowedRoles)
	}

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("remove allowed role returned command")
	}
	if !reflect.DeepEqual(got.launch.AllowedRoles, []string{"coder"}) || got.launch.Role != "coder" {
		t.Fatalf("allowed roles after remove = %+v, want coder only", got.launch)
	}
	if got.notice != "removed planner from constrained allowed roles" {
		t.Fatalf("notice = %q, want constrained remove notice", got.notice)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("clear allowed roles returned command")
	}
	if len(got.launch.AllowedRoles) != 0 || got.launch.Role != "" {
		t.Fatalf("allowed roles after clear = %+v, want empty role list", got.launch)
	}
	if got.notice != "constrained allowed roles cleared" {
		t.Fatalf("notice = %q, want constrained clear notice", got.notice)
	}

	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got = updated.(Model)
	if !reflect.DeepEqual(got.launch.AllowedRoles, []string{"coder"}) || got.launch.Role != "coder" {
		t.Fatalf("allowed roles after re-add = %+v, want coder", got.launch)
	}
}

func TestModelCyclesBuiltInLaunchPresets(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenLaunch
	got.launch.SourceTask = "preserved task"
	got.launch.SpecsText = "specs/preserved.md"

	updated, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	got = updated.(Model)
	if got.launch.Mode != operator.LaunchModeOrchestrator || got.launch.Role != "orchestrator" {
		t.Fatalf("first preset = %+v, want sir topham orchestrator", got.launch)
	}
	if got.launch.SourceTask != "preserved task" || got.launch.SpecsText != "specs/preserved.md" {
		t.Fatalf("draft text/specs = %q/%q, want preserved", got.launch.SourceTask, got.launch.SpecsText)
	}
	if got.notice != "launch preset set to sir topham decides" {
		t.Fatalf("notice = %q, want sir topham preset notice", got.notice)
	}

	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	got = updated.(Model)
	if got.launch.Mode != operator.LaunchModeManualRoster || !reflect.DeepEqual(got.launch.Roster, []string{"planner", "coder"}) {
		t.Fatalf("second preset = %+v, want plan then code roster", got.launch)
	}

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("preset preview returned nil command")
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)
	if fake.launchRequest.Mode != operator.LaunchModeManualRoster || !reflect.DeepEqual(fake.launchRequest.Roster, []string{"planner", "coder"}) || fake.launchRequest.SourceTask != "preserved task" {
		t.Fatalf("launch request = %+v, want plan then code preset preserving task", fake.launchRequest)
	}
	view := got.View()
	if !strings.Contains(view, "preset: plan then code") || !strings.Contains(view, "roster: planner -> coder") {
		t.Fatalf("preset view missing fragments:\n%s", view)
	}
}

func TestModelSavesLaunchDraft(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenLaunch
	got.launch.SourceTask = "persist me"
	got.launch.SpecsText = "specs/a.md, specs/b.md"
	got.launch.Mode = operator.LaunchModeConstrained
	got.launch.Role = "coder"
	got.launch.AllowedRoles = []string{"coder", "planner"}

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("save launch draft returned nil command")
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)

	if fake.savedDraft.Request.SourceTask != "persist me" || !reflect.DeepEqual(fake.savedDraft.Request.SourceSpecs, []string{"specs/a.md", "specs/b.md"}) {
		t.Fatalf("saved draft request = %+v, want task and specs", fake.savedDraft.Request)
	}
	if fake.savedDraft.Request.Mode != operator.LaunchModeConstrained || !reflect.DeepEqual(fake.savedDraft.Request.AllowedRoles, []string{"coder", "planner"}) {
		t.Fatalf("saved draft launch shape = %+v, want constrained coder/planner", fake.savedDraft.Request)
	}
	if got.loading || got.err != nil || got.notice != "launch draft saved" {
		t.Fatalf("post-save state loading=%t err=%v notice=%q", got.loading, got.err, got.notice)
	}
}

func TestModelLoadsLaunchDraft(t *testing.T) {
	fake := newFakeOperator()
	fake.loadDraftFound = true
	fake.loadDraft = operator.LaunchDraft{
		ID: "current",
		Request: operator.LaunchRequest{
			Mode:        operator.LaunchModeManualRoster,
			Role:        "coder",
			Roster:      []string{"planner", "coder"},
			SourceTask:  "loaded draft",
			SourceSpecs: []string{"specs/loaded.md"},
		},
		UpdatedAt: "2026-05-01T12:03:00Z",
	}
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenLaunch
	got.launch.SourceTask = "old draft"
	got.preview = &operator.LaunchPreview{Summary: "stale preview"}
	got.previewReq = &operator.LaunchRequest{SourceTask: "old draft"}

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("load launch draft returned nil command")
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)

	if got.launch.SourceTask != "loaded draft" || got.launch.SpecsText != "specs/loaded.md" {
		t.Fatalf("loaded draft text/specs = %q/%q, want loaded values", got.launch.SourceTask, got.launch.SpecsText)
	}
	if got.launch.Mode != operator.LaunchModeManualRoster || !reflect.DeepEqual(got.launch.Roster, []string{"planner", "coder"}) {
		t.Fatalf("loaded launch state = %+v, want manual roster", got.launch)
	}
	if got.preview != nil || got.previewReq != nil {
		t.Fatalf("preview = %+v/%+v, want cleared after load", got.preview, got.previewReq)
	}
	if got.loading || got.err != nil || got.notice != "launch draft loaded" {
		t.Fatalf("post-load state loading=%t err=%v notice=%q", got.loading, got.err, got.notice)
	}
}

func TestModelSavesCustomLaunchPreset(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenLaunch
	got.launch.SourceTask = "do not store this"
	got.launch.SpecsText = "specs/not-a-preset.md"
	got.launch.Mode = operator.LaunchModeManualRoster
	got.launch.Role = "coder"
	got.launch.Roster = []string{"planner", "coder"}

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("save custom preset returned nil command")
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)

	if fake.savedPreset.Name != "custom roster planner -> coder" {
		t.Fatalf("saved preset name = %q, want custom roster name", fake.savedPreset.Name)
	}
	if fake.savedPreset.Request.SourceTask != "" || len(fake.savedPreset.Request.SourceSpecs) != 0 {
		t.Fatalf("saved preset request = %+v, want no task/specs", fake.savedPreset.Request)
	}
	if !reflect.DeepEqual(fake.savedPreset.Request.Roster, []string{"planner", "coder"}) {
		t.Fatalf("saved preset roster = %v, want planner/coder", fake.savedPreset.Request.Roster)
	}
	if got.notice != "launch preset saved: custom roster planner -> coder" || len(got.customPresets) != 1 {
		t.Fatalf("post-save notice/customPresets = %q/%d", got.notice, len(got.customPresets))
	}
}

func TestModelCyclesCustomLaunchPreset(t *testing.T) {
	fake := newFakeOperator()
	fake.customPresets = []operator.LaunchPreset{
		{
			ID:   "custom:custom roster orchestrator -> coder",
			Name: "custom roster orchestrator -> coder",
			Request: operator.LaunchRequest{
				Mode:   operator.LaunchModeManualRoster,
				Role:   "orchestrator,coder",
				Roster: []string{"orchestrator", "coder"},
			},
		},
	}
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenLaunch
	got.launch.SourceTask = "preserved"
	got.launch.SpecsText = "specs/preserved.md"

	for i := 0; i < 4; i++ {
		updated, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
		got = updated.(Model)
	}

	if got.activeLaunchPresetName() != "custom roster orchestrator -> coder" {
		t.Fatalf("active preset = %q, want custom preset", got.activeLaunchPresetName())
	}
	if !reflect.DeepEqual(got.launch.Roster, []string{"orchestrator", "coder"}) {
		t.Fatalf("custom preset roster = %v, want orchestrator/coder", got.launch.Roster)
	}
	if got.launch.SourceTask != "preserved" || got.launch.SpecsText != "specs/preserved.md" {
		t.Fatalf("draft text/specs = %q/%q, want preserved", got.launch.SourceTask, got.launch.SpecsText)
	}
}

func TestModelLaunchPreviewValidationError(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenLaunch

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("launch preview returned nil command")
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)
	if got.err == nil || !strings.Contains(got.err.Error(), "one of task or specs is required") {
		t.Fatalf("err = %v, want missing task/spec error", got.err)
	}
}

func TestModelStartsPreviewedLaunchAfterConfirmation(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1, FollowInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenLaunch
	got.launch.SourceTask = "ship launch"
	got.launch.SpecsText = "specs/launch.md"

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("preview returned nil command")
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)
	if got.preview == nil {
		t.Fatal("preview not loaded")
	}
	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("start confirmation returned command before confirmation")
	}
	if got.confirm.Action != "launch" {
		t.Fatalf("confirm action = %q, want launch", got.confirm.Action)
	}
	got.launch.SourceTask = "changed after confirmation"
	got.launch.SpecsText = "specs/changed.md"
	got.launch.Mode = operator.LaunchModeOrchestrator
	got.launch.Role = "planner"
	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("confirmed launch returned nil command")
	}
	updated, cmd = got.Update(cmd())
	got = updated.(Model)
	if fake.startRequest.SourceTask != "ship launch" || fake.startRequest.Role != "coder" || fake.startRequest.Mode != operator.LaunchModeOneStep || !reflect.DeepEqual(fake.startRequest.SourceSpecs, []string{"specs/launch.md"}) {
		t.Fatalf("start request = %+v, want one-step coder launch", fake.startRequest)
	}
	if got.screen != screenChains || got.followID != "chain-started" || !got.follow {
		t.Fatalf("post-start state = screen %v follow %t id %q, want chains following chain-started", got.screen, got.follow, got.followID)
	}
	if got.notice != "chain chain-started started" {
		t.Fatalf("notice = %q, want chain started", got.notice)
	}
	if cmd == nil {
		t.Fatal("launch start did not trigger refresh/follow batch")
	}
}

func TestModelStartWithoutPreviewRunsPreviewOnly(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenLaunch
	got.launch.SourceTask = "needs preview"

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("start without preview should trigger preview command")
	}
	if got.confirm.Action != "" {
		t.Fatalf("confirm = %+v, want no confirmation before preview", got.confirm)
	}
	if fake.startRequest.SourceTask != "" {
		t.Fatalf("start request = %+v before preview, want empty", fake.startRequest)
	}
}

func TestModelOpensSelectedReceiptInPager(t *testing.T) {
	var opened ReceiptOpenRequest
	opener := func(req ReceiptOpenRequest) tea.Cmd {
		opened = req
		return func() tea.Msg {
			return receiptOpenedMsg{Mode: req.Mode, Path: req.Path}
		}
	}
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1, ReceiptOpener: opener})
	model.screen = screenReceipts
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("open receipt returned nil command")
	}
	if opened.Mode != ReceiptOpenPager || opened.Path != "receipts/orchestrator/chain-1.md" || opened.Content != "orchestrator receipt" {
		t.Fatalf("opened = %+v, want orchestrator receipt in pager", opened)
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)
	if got.notice != "opened receipts/orchestrator/chain-1.md in pager" {
		t.Fatalf("notice = %q, want opened pager notice", got.notice)
	}
}

func TestModelOpensSelectedReceiptInEditor(t *testing.T) {
	var opened ReceiptOpenRequest
	opener := func(req ReceiptOpenRequest) tea.Cmd {
		opened = req
		return func() tea.Msg {
			return receiptOpenedMsg{Mode: req.Mode, Path: req.Path}
		}
	}
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1, ReceiptOpener: opener})
	model.screen = screenReceipts
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	_ = updated.(Model)
	if cmd == nil {
		t.Fatal("open editor returned nil command")
	}
	if opened.Mode != ReceiptOpenEditor || opened.Path != "receipts/orchestrator/chain-1.md" {
		t.Fatalf("opened = %+v, want orchestrator receipt in editor", opened)
	}
}

func TestModelDoesNotOpenMissingReceipt(t *testing.T) {
	var opened bool
	opener := func(req ReceiptOpenRequest) tea.Cmd {
		opened = true
		return func() tea.Msg {
			return receiptOpenedMsg{Mode: req.Mode, Path: req.Path}
		}
	}
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1, ReceiptOpener: opener})
	got := model
	got.screen = screenReceipts

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("missing receipt returned command")
	}
	if opened {
		t.Fatal("opener was called for missing receipt")
	}
	if got.notice != "no receipt selected" {
		t.Fatalf("notice = %q, want no receipt selected", got.notice)
	}
}

func TestModelShowsWebInspectorTargetForSelectedChain(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1, WebBaseURL: "http://127.0.0.1:7777"})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenChains

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("web inspector handoff returned command")
	}
	for _, want := range []string{"web inspector target for chain chain-1", "run yard serve", "http://127.0.0.1:7777/chains/chain-1"} {
		if !strings.Contains(got.notice, want) {
			t.Fatalf("notice = %q, want fragment %q", got.notice, want)
		}
	}
}

func TestModelShowsWebInspectorTargetForSelectedReceipt(t *testing.T) {
	model := NewModel(newFakeOperator(), Options{RefreshInterval: -1})
	model.screen = screenReceipts
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("receipt web inspector handoff returned command")
	}
	for _, want := range []string{"web inspector target for receipt receipts/orchestrator/chain-1.md", "run yard serve", "http://localhost:8090/chains/chain-1?receipt=receipts%2Forchestrator%2Fchain-1.md"} {
		if !strings.Contains(got.notice, want) {
			t.Fatalf("notice = %q, want fragment %q", got.notice, want)
		}
	}
}

func TestModelPausesSelectedChain(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenChains

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("pause returned nil command")
	}
	updated, cmd = got.Update(cmd())
	got = updated.(Model)
	if fake.pausedChain != "chain-1" {
		t.Fatalf("pausedChain = %q, want chain-1", fake.pausedChain)
	}
	if got.notice != "chain chain-1 pause requested" {
		t.Fatalf("notice = %q, want pause requested notice", got.notice)
	}
	if cmd == nil {
		t.Fatal("pause result did not trigger refresh")
	}
}

func TestModelCancelsSelectedChain(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenChains

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("cancel confirmation returned command before confirmation")
	}
	if got.confirm.Action != "cancel" || got.confirm.ChainID != "chain-1" {
		t.Fatalf("confirm = %+v, want cancel chain-1", got.confirm)
	}
	if fake.cancelledChain != "" {
		t.Fatalf("cancelledChain = %q before confirmation, want empty", fake.cancelledChain)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("cancel returned nil command")
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)
	if fake.cancelledChain != "chain-1" {
		t.Fatalf("cancelledChain = %q, want chain-1", fake.cancelledChain)
	}
	if got.notice != "chain chain-1 cancel requested" {
		t.Fatalf("notice = %q, want cancel requested notice", got.notice)
	}
}

func TestModelClearsStaleCancelConfirmation(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenChains

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("cancel confirmation returned command before confirmation")
	}
	if got.confirm.Action != "cancel" || got.confirm.ChainID != "chain-1" {
		t.Fatalf("confirm = %+v, want cancel chain-1", got.confirm)
	}

	fake.chains[0].Status = "completed"
	fake.details["chain-1"] = operator.ChainDetail{
		Chain: chain.Chain{ID: "chain-1", Status: "completed", SourceTask: "first task"},
		Steps: []chain.Step{{SequenceNum: 1, Role: "coder", Status: "completed", Verdict: "accepted", ReceiptPath: "receipts/coder/chain-1-step-001.md"}},
	}
	updated, _ = got.Update(got.refreshCmd()())
	got = updated.(Model)
	if got.confirm.Action != "" {
		t.Fatalf("confirm = %+v, want cleared stale cancel confirmation", got.confirm)
	}
	if got.notice != "chain chain-1 is completed; cancel aborted" {
		t.Fatalf("notice = %q, want stale cancel notice", got.notice)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("stale confirm y returned command")
	}
	if fake.cancelledChain != "" {
		t.Fatalf("cancelledChain = %q after stale confirmation, want empty", fake.cancelledChain)
	}
}

func TestModelShowsResumeCommandForPausedChain(t *testing.T) {
	fake := newFakeOperator()
	fake.chains[1].Status = "paused"
	fake.details["chain-2"] = operator.ChainDetail{
		Chain: chain.Chain{ID: "chain-2", Status: "paused", SourceTask: "second task"},
		Steps: []chain.Step{{SequenceNum: 1, Role: "planner", Status: "completed", ReceiptPath: "receipts/planner/chain-2-step-001.md"}},
	}
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenChains
	got.chainCursor = 1
	loaded, _ = got.Update(got.refreshCmd()())
	got = loaded.(Model)

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("resume guidance returned command")
	}
	want := "resume in a foreground shell: yard chain resume chain-2"
	if got.notice != want {
		t.Fatalf("notice = %q, want %q", got.notice, want)
	}
}

func TestModelRejectsTerminalChainControls(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenChains
	got.chainCursor = 1
	loaded, _ = got.Update(got.refreshCmd()())
	got = loaded.(Model)

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("terminal cancel returned command")
	}
	if got.confirm.Action != "" {
		t.Fatalf("confirm = %+v, want none", got.confirm)
	}
	if got.notice != "chain chain-2 is completed and cannot be cancelled here" {
		t.Fatalf("notice = %q, want terminal-chain notice", got.notice)
	}
}

func TestModelFollowsSelectedChainEvents(t *testing.T) {
	fake := newFakeOperator()
	started := time.Date(2026, 5, 1, 12, 0, 1, 0, time.UTC)
	fake.eventsSince["chain-1"] = []chain.Event{
		{ID: 2, EventType: chain.EventStepOutput, EventData: `{"stream":"stderr","line":"status: waiting_for_llm"}`, CreatedAt: started},
		{ID: 3, EventType: chain.EventStepCompleted, EventData: `{"role":"coder","verdict":"accepted"}`, CreatedAt: started},
	}
	model := NewModel(fake, Options{RefreshInterval: -1, FollowInterval: -1})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenChains

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	got = updated.(Model)
	if !got.follow || got.followID != "chain-1" || got.followAfter != 1 {
		t.Fatalf("follow state = follow %t id %q after %d, want chain-1 after 1", got.follow, got.followID, got.followAfter)
	}
	if cmd == nil {
		t.Fatal("follow returned nil command")
	}
	updated, _ = got.Update(cmd())
	got = updated.(Model)
	if got.followAfter != 3 {
		t.Fatalf("followAfter = %d, want 3", got.followAfter)
	}
	if len(got.followLog) != 3 {
		t.Fatalf("followLog count = %d, want initial plus two new events", len(got.followLog))
	}
	view := got.View()
	if !strings.Contains(view, "Following chain-1") || strings.Contains(view, "waiting_for_llm") || !strings.Contains(view, "step_completed") {
		t.Fatalf("follow view did not render filtered live events:\n%s", view)
	}
}

func TestModelStopsFollowingCompletedChain(t *testing.T) {
	fake := newFakeOperator()
	model := NewModel(fake, Options{RefreshInterval: -1, FollowInterval: time.Second})
	loaded, _ := model.Update(model.refreshCmd()())
	got := loaded.(Model)
	got.screen = screenChains

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("follow returned nil command")
	}

	completedAt := time.Date(2026, 5, 1, 12, 0, 2, 0, time.UTC)
	fake.details["chain-1"] = operator.ChainDetail{
		Chain: chain.Chain{ID: "chain-1", Status: "completed", SourceTask: "first task", TotalSteps: 1, TotalTokens: 12, UpdatedAt: completedAt},
		Steps: []chain.Step{{SequenceNum: 1, Role: "coder", Status: "completed", Verdict: "accepted", ReceiptPath: "receipts/coder/chain-1-step-001.md"}},
		RecentEvents: []chain.Event{
			{ID: 1, EventType: chain.EventStepStarted, EventData: `{"role":"coder"}`, CreatedAt: completedAt},
			{ID: 2, EventType: chain.EventChainCompleted, EventData: `{"status":"completed"}`, CreatedAt: completedAt},
		},
	}

	updated, next := got.Update(cmd())
	got = updated.(Model)
	if next != nil {
		t.Fatal("completed follow scheduled another tick")
	}
	if got.follow || got.followID != "" || got.followAfter != 0 {
		t.Fatalf("follow state = follow %t id %q after %d, want stopped", got.follow, got.followID, got.followAfter)
	}
	if got.detail == nil || got.detail.Chain.Status != "completed" {
		t.Fatalf("detail status = %+v, want completed", got.detail)
	}
	if got.chains[0].Status != "completed" {
		t.Fatalf("chain list status = %q, want completed", got.chains[0].Status)
	}
	if got.notice != "chain chain-1 is completed; stopped following" {
		t.Fatalf("notice = %q, want completed follow notice", got.notice)
	}
}
