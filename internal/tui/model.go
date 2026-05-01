package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/operator"
)

type appScreen int

const (
	screenDashboard appScreen = iota
	screenLaunch
	screenChains
	screenReceipts
	screenHelp
)

type receiptItem struct {
	Label string
	Step  string
	Path  string
}

type launchField int

const (
	launchFieldTask launchField = iota
	launchFieldSpecs
	launchFieldMode
	launchFieldRole
)

type launchDraft struct {
	Mode       operator.LaunchMode
	Role       string
	SourceTask string
	SpecsText  string
}

type Model struct {
	ctx             context.Context
	svc             Operator
	screen          appScreen
	previousScreen  appScreen
	width           int
	height          int
	refreshInterval time.Duration
	followInterval  time.Duration
	chainLimit      int
	receiptOpener   ReceiptOpener

	status        operator.RuntimeStatus
	roles         []operator.AgentRoleSummary
	chains        []operator.ChainSummary
	chainCursor   int
	detail        *operator.ChainDetail
	receiptItems  []receiptItem
	receiptCursor int
	receipt       *operator.ReceiptView
	viewport      viewport.Model

	loading      bool
	err          error
	notice       string
	confirm      pendingConfirmation
	pendingChain string
	launch       launchDraft
	launchField  launchField
	launchEdit   bool
	preview      *operator.LaunchPreview
	previewReq   *operator.LaunchRequest
	follow       bool
	followID     string
	followAfter  int64
	followLog    []chain.Event
	lastUpdated  time.Time
	styles       styles
}

type pendingConfirmation struct {
	Action        string
	ChainID       string
	LaunchRequest operator.LaunchRequest
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), m.tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewport()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case dataLoadedMsg:
		m.loading = false
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.err = nil
		m.status = msg.Status
		m.roles = msg.Roles
		m.ensureLaunchDefaults()
		m.chains = msg.Chains
		m.chainCursor = chainIndexByID(m.chains, msg.SelectedChainID, m.chainCursor)
		m.pendingChain = ""
		m.detail = msg.Detail
		m.receiptItems = buildReceiptItems(m.detail)
		m.receiptCursor = clampCursor(m.receiptCursor, len(m.receiptItems))
		m.receipt = msg.Receipt
		m.lastUpdated = time.Now()
		m.updateReceiptViewport()
		if m.follow && m.followID == msg.SelectedChainID && m.detail != nil && !followStatusActive(m.detail.Chain.Status) {
			m.stopFollowingCompletedChain(msg.SelectedChainID, m.detail.Chain.Status)
		}
		m.invalidateStaleConfirmation()
		return m, nil
	case controlMsg:
		m.loading = false
		if msg.Err != nil {
			m.err = msg.Err
			m.notice = ""
			return m, nil
		}
		m.err = nil
		m.confirm = pendingConfirmation{}
		m.notice = fmt.Sprintf("chain %s %s", msg.Result.ChainID, msg.Result.Message)
		m.loading = true
		return m, m.refreshCmd()
	case followEventsMsg:
		if !m.follow || msg.ChainID != m.followID {
			return m, nil
		}
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.err = nil
		m.followLog = append(m.followLog, msg.Events...)
		m.followLog = trimEvents(m.followLog, 200)
		m.followAfter = maxEventID(m.followLog, m.followAfter)
		if msg.Detail != nil {
			m.applyFollowDetail(*msg.Detail)
		}
		if !followStatusActive(msg.Status) {
			m.stopFollowingCompletedChain(msg.ChainID, msg.Status)
			return m, nil
		}
		return m, m.followTickCmd()
	case receiptOpenedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			m.notice = ""
			return m, nil
		}
		m.err = nil
		m.notice = fmt.Sprintf("opened %s in %s", msg.Path, msg.Mode)
		return m, nil
	case launchPreviewMsg:
		if msg.Err != nil {
			m.err = msg.Err
			m.preview = nil
			m.previewReq = nil
			m.notice = ""
			return m, nil
		}
		m.err = nil
		preview := msg.Preview
		req := msg.Request
		m.preview = &preview
		m.previewReq = &req
		m.notice = "launch preview ready"
		return m, nil
	case launchStartedMsg:
		m.loading = false
		if msg.Err != nil {
			m.err = msg.Err
			m.notice = ""
			return m, nil
		}
		m.err = nil
		m.confirm = pendingConfirmation{}
		m.pendingChain = msg.Result.ChainID
		m.follow = true
		m.followID = msg.Result.ChainID
		m.followAfter = 0
		m.followLog = nil
		m.screen = screenChains
		m.notice = fmt.Sprintf("chain %s started", msg.Result.ChainID)
		m.loading = true
		return m, tea.Batch(m.refreshCmd(), m.followCmd())
	case tickMsg:
		m.loading = true
		return m, tea.Batch(m.refreshCmd(), m.tickCmd())
	case followTickMsg:
		if !m.follow {
			return m, nil
		}
		return m, m.followCmd()
	default:
		if m.screen == screenReceipts {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		return m, nil
	}
}

func (m Model) View() string {
	if m.screen == screenHelp {
		return m.renderFrame(m.renderHelp())
	}
	var body string
	switch m.screen {
	case screenLaunch:
		body = m.renderLaunch()
	case screenChains:
		body = m.renderChains()
	case screenReceipts:
		body = m.renderReceipts()
	default:
		body = m.renderDashboard()
	}
	return m.renderFrame(body)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		if !m.launchEdit {
			return m, tea.Quit
		}
	}
	if m.confirm.Action != "" {
		return m.handleConfirmationKey(msg)
	}
	if m.launchEdit {
		return m.handleLaunchEditKey(msg)
	}
	switch msg.String() {
	case "?":
		if m.screen == screenHelp {
			m.screen = m.previousScreen
		} else {
			m.previousScreen = m.screen
			m.screen = screenHelp
		}
		return m, nil
	case "tab":
		if m.screen == screenHelp {
			m.screen = m.previousScreen
		} else {
			m.screen = nextScreen(m.screen)
			m.receiptCursor = clampCursor(m.receiptCursor, len(m.receiptItems))
			m.updateReceiptViewport()
		}
		return m, nil
	case "esc":
		if m.screen == screenHelp {
			m.screen = m.previousScreen
		} else if m.screen == screenReceipts {
			m.screen = screenChains
		} else {
			m.screen = screenDashboard
		}
		return m, nil
	case "r":
		m.loading = true
		return m, m.refreshCmd()
	case "v":
		if m.screen == screenLaunch {
			return m, m.launchPreviewCmd()
		}
	case "S":
		if m.screen == screenLaunch {
			return m.confirmLaunch()
		}
	case "m":
		if m.screen == screenLaunch {
			m.toggleLaunchMode()
			return m, nil
		}
	case "n":
		if m.screen == screenLaunch {
			m.nextLaunchRole()
			return m, nil
		}
	case "i":
		if m.screen == screenLaunch {
			if !m.launchFieldEditable() {
				m.notice = "selected launch field is not editable"
				return m, nil
			}
			m.launchEdit = true
			m.notice = "editing launch " + m.launchFieldLabel()
			return m, nil
		}
	case "F":
		if m.screen == screenChains {
			return m.toggleFollowSelectedChain()
		}
	case "R":
		if m.screen == screenChains {
			return m.showResumeCommand()
		}
	case "P":
		if m.screen == screenChains {
			return m.pauseSelectedChain()
		}
	case "X":
		if m.screen == screenChains {
			return m.confirmCancelSelectedChain()
		}
	case "enter":
		if m.screen == screenDashboard {
			m.screen = screenChains
			return m, nil
		}
		if m.screen == screenLaunch {
			if m.launchFieldEditable() {
				m.launchEdit = true
				m.notice = "editing launch " + m.launchFieldLabel()
				return m, nil
			}
			return m, m.launchPreviewCmd()
		}
		if m.screen == screenChains {
			m.screen = screenReceipts
			m.receiptCursor = 0
			m.loading = true
			return m, m.refreshCmd()
		}
		return m, nil
	case "j", "down":
		if m.screen == screenLaunch {
			m.moveLaunchField(1)
			return m, nil
		}
		return m.moveSelection(1)
	case "k", "up":
		if m.screen == screenLaunch {
			m.moveLaunchField(-1)
			return m, nil
		}
		return m.moveSelection(-1)
	case "l":
		m.screen = screenLaunch
	case "d":
		m.screen = screenDashboard
	case "c":
		m.screen = screenChains
	case "p":
		m.screen = screenReceipts
		m.loading = true
		return m, m.refreshCmd()
	case "o":
		if m.screen == screenReceipts {
			return m.openSelectedReceipt(ReceiptOpenPager)
		}
	case "E":
		if m.screen == screenReceipts {
			return m.openSelectedReceipt(ReceiptOpenEditor)
		}
	}
	if m.screen == screenReceipts {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleLaunchEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.launchEdit = false
		m.notice = "launch task edit stopped"
		return m, nil
	case tea.KeyEnter:
		m.launchEdit = false
		return m, m.launchPreviewCmd()
	case tea.KeyBackspace, tea.KeyCtrlH:
		m.setLaunchFieldText(dropLastRune(m.launchFieldText()))
		m.clearLaunchPreview()
		m.err = nil
		return m, nil
	case tea.KeyCtrlU:
		m.setLaunchFieldText("")
		m.clearLaunchPreview()
		m.err = nil
		return m, nil
	case tea.KeySpace:
		m.setLaunchFieldText(m.launchFieldText() + " ")
		m.clearLaunchPreview()
		m.err = nil
		return m, nil
	case tea.KeyRunes:
		m.setLaunchFieldText(m.launchFieldText() + string(msg.Runes))
		m.clearLaunchPreview()
		m.err = nil
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) handleConfirmationKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		confirm := m.confirm
		m.confirm = pendingConfirmation{}
		switch confirm.Action {
		case "cancel":
			m.loading = true
			return m, m.controlCmd("cancel", confirm.ChainID)
		case "launch":
			m.loading = true
			return m, m.launchStartCmd(confirm.LaunchRequest)
		default:
			m.notice = fmt.Sprintf("unsupported confirmation action %s", confirm.Action)
			return m, nil
		}
	case "n", "N", "esc":
		m.notice = "cancel aborted"
		m.confirm = pendingConfirmation{}
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) pauseSelectedChain() (tea.Model, tea.Cmd) {
	chainID := selectedChainID(m.chains, m.chainCursor)
	if chainID == "" {
		m.notice = "no chain selected"
		return m, nil
	}
	status := m.selectedChainStatus()
	if !canPauseChain(status) {
		m.notice = fmt.Sprintf("chain %s is %s and cannot be paused here", chainID, status)
		return m, nil
	}
	m.loading = true
	return m, m.controlCmd("pause", chainID)
}

func (m Model) confirmCancelSelectedChain() (tea.Model, tea.Cmd) {
	chainID := selectedChainID(m.chains, m.chainCursor)
	if chainID == "" {
		m.notice = "no chain selected"
		return m, nil
	}
	status := m.selectedChainStatus()
	if !canCancelChain(status) {
		m.notice = fmt.Sprintf("chain %s is %s and cannot be cancelled here", chainID, status)
		return m, nil
	}
	m.confirm = pendingConfirmation{Action: "cancel", ChainID: chainID}
	m.notice = fmt.Sprintf("cancel chain %s? y/n", chainID)
	return m, nil
}

func (m Model) showResumeCommand() (tea.Model, tea.Cmd) {
	chainID := selectedChainID(m.chains, m.chainCursor)
	if chainID == "" {
		m.notice = "no chain selected"
		return m, nil
	}
	status := m.selectedChainStatus()
	if status != "paused" {
		m.notice = fmt.Sprintf("chain %s is %s and cannot be resumed here", chainID, status)
		return m, nil
	}
	m.notice = fmt.Sprintf("resume in a foreground shell: yard chain resume %s", chainID)
	return m, nil
}

func (m Model) toggleFollowSelectedChain() (tea.Model, tea.Cmd) {
	chainID := selectedChainID(m.chains, m.chainCursor)
	if chainID == "" {
		m.notice = "no chain selected"
		return m, nil
	}
	if m.follow && m.followID == chainID {
		m.follow = false
		m.followID = ""
		m.followAfter = 0
		m.followLog = nil
		m.notice = fmt.Sprintf("stopped following chain %s", chainID)
		return m, nil
	}
	m.follow = true
	m.followID = chainID
	m.followLog = initialFollowEvents(m.detail, chainID)
	m.followAfter = maxEventID(m.followLog, 0)
	m.notice = fmt.Sprintf("following chain %s", chainID)
	return m, m.followCmd()
}

func (m Model) confirmLaunch() (tea.Model, tea.Cmd) {
	if m.preview == nil || m.previewReq == nil {
		m.notice = "preview launch before starting"
		return m, m.launchPreviewCmd()
	}
	current := m.launchRequest()
	if !sameLaunchRequest(*m.previewReq, current) {
		m.clearLaunchPreview()
		m.notice = "preview launch before starting"
		return m, m.launchPreviewCmd()
	}
	m.confirm = pendingConfirmation{Action: "launch", LaunchRequest: *m.previewReq}
	m.notice = "start launch? y/n"
	return m, nil
}

func (m Model) openSelectedReceipt(mode ReceiptOpenMode) (tea.Model, tea.Cmd) {
	if m.receipt == nil {
		m.notice = "no receipt selected"
		return m, nil
	}
	if strings.TrimSpace(m.receipt.Content) == "" {
		m.notice = "selected receipt is empty"
		return m, nil
	}
	if m.receiptOpener == nil {
		m.notice = "receipt opener is not configured"
		return m, nil
	}
	m.notice = fmt.Sprintf("opening %s in %s", m.receipt.Path, mode)
	return m, m.receiptOpener(ReceiptOpenRequest{Mode: mode, Path: m.receipt.Path, Content: m.receipt.Content})
}

func (m Model) moveSelection(delta int) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenChains:
		next := clampCursor(m.chainCursor+delta, len(m.chains))
		if next == m.chainCursor {
			return m, nil
		}
		m.chainCursor = next
		m.loading = true
		return m, m.refreshCmd()
	case screenReceipts:
		next := clampCursor(m.receiptCursor+delta, len(m.receiptItems))
		if next == m.receiptCursor {
			return m, nil
		}
		m.receiptCursor = next
		m.loading = true
		return m, m.refreshCmd()
	default:
		return m, nil
	}
}

func (m Model) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		if m.svc == nil {
			return dataLoadedMsg{Err: fmt.Errorf("operator service is not configured")}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()

		status, err := m.svc.RuntimeStatus(ctx)
		if err != nil {
			return dataLoadedMsg{Err: err}
		}
		chains, err := m.svc.ListChains(ctx, m.chainLimit)
		if err != nil {
			return dataLoadedMsg{Err: err}
		}
		roles, err := m.svc.ListAgentRoles(ctx)
		if err != nil {
			return dataLoadedMsg{Err: err}
		}
		selectedID := selectedChainID(chains, m.chainCursor)
		if m.pendingChain != "" {
			selectedID = m.pendingChain
		}
		var detail *operator.ChainDetail
		if selectedID != "" {
			loaded, err := m.svc.GetChainDetail(ctx, selectedID)
			if err != nil {
				return dataLoadedMsg{Err: err}
			}
			detail = &loaded
		}
		items := buildReceiptItems(detail)
		var receipt *operator.ReceiptView
		if m.screen == screenReceipts && selectedID != "" && len(items) > 0 {
			item := items[clampCursor(m.receiptCursor, len(items))]
			loaded, err := m.svc.ReadReceipt(ctx, selectedID, item.Step)
			if err != nil {
				return dataLoadedMsg{Err: err}
			}
			receipt = &loaded
		}
		return dataLoadedMsg{Status: status, Roles: roles, Chains: chains, Detail: detail, Receipt: receipt, SelectedChainID: selectedID}
	}
}

func (m Model) launchPreviewCmd() tea.Cmd {
	return func() tea.Msg {
		if m.svc == nil {
			return launchPreviewMsg{Err: fmt.Errorf("operator service is not configured")}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		req := m.launchRequest()
		preview, err := m.svc.ValidateLaunch(ctx, req)
		return launchPreviewMsg{Request: req, Preview: preview, Err: err}
	}
}

func (m Model) launchStartCmd(req operator.LaunchRequest) tea.Cmd {
	return func() tea.Msg {
		if m.svc == nil {
			return launchStartedMsg{Err: fmt.Errorf("operator service is not configured")}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		result, err := m.svc.StartChain(ctx, req)
		return launchStartedMsg{Result: result, Err: err}
	}
}

func (m Model) controlCmd(action string, chainID string) tea.Cmd {
	return func() tea.Msg {
		if m.svc == nil {
			return controlMsg{Action: action, Err: fmt.Errorf("operator service is not configured")}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()

		var (
			result operator.ControlResult
			err    error
		)
		switch action {
		case "pause":
			result, err = m.svc.PauseChain(ctx, chainID)
		case "cancel":
			result, err = m.svc.CancelChain(ctx, chainID)
		default:
			err = fmt.Errorf("unsupported chain action %s", action)
		}
		return controlMsg{Action: action, Result: result, Err: err}
	}
}

func (m Model) followCmd() tea.Cmd {
	return func() tea.Msg {
		if m.svc == nil {
			return followEventsMsg{ChainID: m.followID, Err: fmt.Errorf("operator service is not configured")}
		}
		chainID := m.followID
		afterID := m.followAfter
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		events, err := m.svc.ListEventsSince(ctx, chainID, afterID)
		if err != nil {
			return followEventsMsg{ChainID: chainID, Err: err}
		}
		detail, err := m.svc.GetChainDetail(ctx, chainID)
		if err != nil {
			return followEventsMsg{ChainID: chainID, Err: err}
		}
		return followEventsMsg{ChainID: chainID, Status: detail.Chain.Status, Detail: &detail, Events: events}
	}
}

func (m Model) tickCmd() tea.Cmd {
	if m.refreshInterval <= 0 {
		return nil
	}
	return tea.Tick(m.refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) followTickCmd() tea.Cmd {
	if m.followInterval <= 0 {
		return nil
	}
	return tea.Tick(m.followInterval, func(t time.Time) tea.Msg {
		return followTickMsg(t)
	})
}

func (m *Model) resizeViewport() {
	width := maxInt(20, m.contentWidth()-2)
	height := maxInt(4, m.height-10)
	m.viewport.Width = width
	m.viewport.Height = height
	m.updateReceiptViewport()
}

func (m *Model) updateReceiptViewport() {
	if m.viewport.Width == 0 {
		m.viewport = viewport.New(maxInt(20, m.contentWidth()-2), maxInt(4, m.height-10))
	}
	content := ""
	if m.receipt != nil {
		content = m.receipt.Content
	} else if m.screen == screenReceipts {
		content = "No receipt loaded."
	}
	m.viewport.SetContent(content)
}

func (m Model) renderFrame(body string) string {
	width := maxInt(80, m.width)
	top := m.styles.status.Width(width).Render(m.statusLine())
	navAndBody := lipgloss.JoinHorizontal(lipgloss.Top, m.renderNav(), m.styles.panel.Width(m.contentWidth()).Render(body))
	footer := m.styles.footer.Width(width).Render(footerHelp)
	return lipgloss.JoinVertical(lipgloss.Left, top, navAndBody, footer)
}

func (m Model) statusLine() string {
	project := m.status.ProjectName
	if strings.TrimSpace(project) == "" {
		project = "unknown project"
	}
	provider := strings.TrimSpace(m.status.Provider + ":" + m.status.Model)
	if provider == ":" {
		provider = "provider:model unknown"
	}
	updated := ""
	if !m.lastUpdated.IsZero() {
		updated = " updated " + m.lastUpdated.Format("15:04:05")
	}
	if m.loading {
		updated = " loading"
	}
	return fmt.Sprintf("Yard %s | %s | active chains %d%s", project, provider, m.status.ActiveChains, updated)
}

func (m Model) renderNav() string {
	lines := []string{"Nav", navLine("Dashboard", m.screen == screenDashboard), navLine("Launch", m.screen == screenLaunch), navLine("Chains", m.screen == screenChains), navLine("Receipts", m.screen == screenReceipts)}
	return m.styles.nav.Width(14).Render(strings.Join(lines, "\n"))
}

func navLine(label string, active bool) string {
	if active {
		return "> " + label
	}
	return "  " + label
}

func (m Model) contentWidth() int {
	return maxInt(40, m.width-18)
}

func selectedChainID(chains []operator.ChainSummary, cursor int) string {
	if len(chains) == 0 {
		return ""
	}
	return chains[clampCursor(cursor, len(chains))].ID
}

func chainIndexByID(chains []operator.ChainSummary, chainID string, fallback int) int {
	if chainID != "" {
		for i, ch := range chains {
			if ch.ID == chainID {
				return i
			}
		}
	}
	return clampCursor(fallback, len(chains))
}

func buildReceiptItems(detail *operator.ChainDetail) []receiptItem {
	if detail == nil || detail.Chain.ID == "" {
		return nil
	}
	items := []receiptItem{{
		Label: "orchestrator",
		Step:  "",
		Path:  fmt.Sprintf("receipts/orchestrator/%s.md", detail.Chain.ID),
	}}
	for _, step := range detail.Steps {
		if strings.TrimSpace(step.ReceiptPath) == "" {
			continue
		}
		items = append(items, receiptItem{
			Label: fmt.Sprintf("step %d %s", step.SequenceNum, step.Role),
			Step:  strconv.Itoa(step.SequenceNum),
			Path:  step.ReceiptPath,
		})
	}
	return items
}

func (m Model) selectedChainStatus() string {
	chainID := selectedChainID(m.chains, m.chainCursor)
	if chainID == "" {
		return ""
	}
	status, _ := m.chainStatusByID(chainID)
	return status
}

func (m Model) chainStatusByID(chainID string) (string, bool) {
	if m.detail != nil && m.detail.Chain.ID == chainID {
		return m.detail.Chain.Status, true
	}
	for _, ch := range m.chains {
		if ch.ID == chainID {
			return ch.Status, true
		}
	}
	return "", false
}

func (m *Model) invalidateStaleConfirmation() {
	if m.confirm.Action != "cancel" {
		return
	}
	chainID := m.confirm.ChainID
	status, ok := m.chainStatusByID(chainID)
	if !ok {
		m.confirm = pendingConfirmation{}
		m.notice = fmt.Sprintf("chain %s is no longer available; cancel aborted", chainID)
		return
	}
	if !canCancelChain(status) {
		m.confirm = pendingConfirmation{}
		m.notice = fmt.Sprintf("chain %s is %s; cancel aborted", chainID, status)
	}
}

func canPauseChain(status string) bool {
	return status == "running" || status == "pause_requested"
}

func canCancelChain(status string) bool {
	switch status {
	case "running", "pause_requested", "cancel_requested", "paused":
		return true
	default:
		return false
	}
}

func followStatusActive(status string) bool {
	return status == "running" || status == "pause_requested" || status == "cancel_requested"
}

func (m *Model) applyFollowDetail(detail operator.ChainDetail) {
	if selectedChainID(m.chains, m.chainCursor) != detail.Chain.ID {
		return
	}
	m.detail = &detail
	m.receiptItems = buildReceiptItems(m.detail)
	m.receiptCursor = clampCursor(m.receiptCursor, len(m.receiptItems))
	for i := range m.chains {
		if m.chains[i].ID != detail.Chain.ID {
			continue
		}
		m.chains[i].Status = detail.Chain.Status
		m.chains[i].TotalSteps = detail.Chain.TotalSteps
		m.chains[i].TotalTokens = detail.Chain.TotalTokens
		m.chains[i].UpdatedAt = detail.Chain.UpdatedAt
		break
	}
}

func (m *Model) stopFollowingCompletedChain(chainID string, status string) {
	m.follow = false
	m.followID = ""
	m.followAfter = 0
	m.followLog = nil
	if strings.TrimSpace(status) == "" {
		m.notice = fmt.Sprintf("stopped following chain %s", chainID)
		return
	}
	m.notice = fmt.Sprintf("chain %s is %s; stopped following", chainID, status)
}

func initialFollowEvents(detail *operator.ChainDetail, chainID string) []chain.Event {
	if detail == nil || detail.Chain.ID != chainID {
		return nil
	}
	return append([]chain.Event(nil), detail.RecentEvents...)
}

func trimEvents(events []chain.Event, limit int) []chain.Event {
	if limit <= 0 || len(events) <= limit {
		return events
	}
	return events[len(events)-limit:]
}

func maxEventID(events []chain.Event, fallback int64) int64 {
	maxID := fallback
	for _, event := range events {
		if event.ID > maxID {
			maxID = event.ID
		}
	}
	return maxID
}

func clampCursor(cursor int, count int) int {
	if count <= 0 {
		return 0
	}
	if cursor < 0 {
		return 0
	}
	if cursor >= count {
		return count - 1
	}
	return cursor
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func trimOneLine(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if max <= 0 || len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}
