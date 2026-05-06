package tui

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/ponchione/sodoryard/internal/operator"
)

type slashCommand struct {
	Name  string
	Args  []string
	Flags map[string][]string
}

func (m Model) handleSlashInput(input string) (tea.Model, tea.Cmd) {
	cmd, err := parseSlashInput(input)
	if err != nil {
		m.appendConsoleEntry(consoleEntryUser, "YOU", input)
		m.appendConsoleEntry(consoleEntryError, "ERROR", err.Error())
		m.err = nil
		return m, m.chatComposer.Focus()
	}
	if cmd.Name == "new" {
		scope := ""
		if len(cmd.Args) > 0 {
			scope = strings.ToLower(cmd.Args[0])
		}
		m.resetConsoleSession(scope)
		return m, m.chatComposer.Focus()
	}

	m.appendConsoleEntry(consoleEntryUser, "YOU", input)
	m.chatEdit = true
	m.err = nil
	switch cmd.Name {
	case "help", "?":
		m.appendConsoleEntry(consoleEntryCommand, "HELP", slashHelpText())
		return m, m.chatComposer.Focus()
	case "status", "doctor":
		m.appendConsoleEntry(consoleEntryCommand, "STATUS", m.renderConsoleStatus())
		return m, m.chatComposer.Focus()
	case "model":
		m.appendConsoleEntry(consoleEntryCommand, "MODEL", m.renderConsoleModel())
		return m, m.chatComposer.Focus()
	case "effort":
		if len(cmd.Args) == 0 {
			m.appendConsoleEntry(consoleEntryCommand, "EFFORT", m.renderConsoleEffort())
			return m, m.chatComposer.Focus()
		}
		effort := strings.ToLower(strings.TrimSpace(cmd.Args[0]))
		m.loading = true
		return m, m.consoleEffortCmd(effort)
	case "chains":
		query := strings.Join(cmd.Args, " ")
		m.appendConsoleEntry(consoleEntryCommand, "CHAINS", m.renderConsoleChains(query))
		return m, m.chatComposer.Focus()
	case "chain":
		chainID := firstArgOrSelectedChain(cmd.Args, m.selectedVisibleChainID())
		if chainID == "" {
			return m.consoleCommandError("chain id is required")
		}
		m.loading = true
		return m, m.consoleChainDetailCmd(chainID)
	case "events", "logs":
		chainID := firstArgOrSelectedChain(cmd.Args, m.selectedVisibleChainID())
		if chainID == "" {
			return m.consoleCommandError("chain id is required")
		}
		limit := parseOptionalLimit(cmd.Args, 1, 20)
		m.loading = true
		return m, m.consoleEventsCmd(chainID, limit)
	case "follow":
		chainID := firstArgOrSelectedChain(cmd.Args, m.selectedVisibleChainID())
		if chainID == "" {
			return m.consoleCommandError("chain id is required")
		}
		m.follow = true
		m.followID = chainID
		m.followLog = initialFollowEvents(m.detail, chainID)
		m.followAfter = maxEventID(m.followLog, 0)
		m.appendConsoleEntry(consoleEntryCommand, "FOLLOW", fmt.Sprintf("following chain %s", chainID))
		return m, m.followCmd()
	case "unfollow":
		if !m.follow {
			m.appendConsoleEntry(consoleEntryCommand, "UNFOLLOW", "no chain is being followed")
			return m, m.chatComposer.Focus()
		}
		chainID := m.followID
		m.follow = false
		m.followID = ""
		m.followAfter = 0
		m.followLog = nil
		m.appendConsoleEntry(consoleEntryCommand, "UNFOLLOW", fmt.Sprintf("stopped following chain %s", chainID))
		return m, m.chatComposer.Focus()
	case "receipt":
		chainID := firstArgOrSelectedChain(cmd.Args, m.selectedVisibleChainID())
		if chainID == "" {
			return m.consoleCommandError("chain id is required")
		}
		step := ""
		if len(cmd.Args) > 1 {
			step = cmd.Args[1]
		}
		m.loading = true
		return m, m.consoleReceiptCmd(chainID, step)
	case "pause", "resume":
		chainID := firstArgOrSelectedChain(cmd.Args, m.selectedVisibleChainID())
		if chainID == "" {
			return m.consoleCommandError("chain id is required")
		}
		m.loading = true
		return m, m.consoleControlCmd(cmd.Name, chainID)
	case "cancel":
		chainID := firstArgOrSelectedChain(cmd.Args, m.selectedVisibleChainID())
		if chainID == "" {
			return m.consoleCommandError("chain id is required")
		}
		m.confirm = pendingConfirmation{Action: "cancel", ChainID: chainID}
		m.appendConsoleEntry(consoleEntryCommand, "CONFIRM", fmt.Sprintf("Cancel chain %s? y/n", chainID))
		return m, m.chatComposer.Focus()
	case "preview":
		req, err := slashLaunchRequest(cmd)
		if err != nil {
			return m.consoleCommandError(err.Error())
		}
		m.loading = true
		return m, m.consoleLaunchPreviewCmd(req)
	case "start":
		req, err := slashLaunchRequest(cmd)
		if err != nil {
			return m.consoleCommandError(err.Error())
		}
		m.loading = true
		return m, m.consoleLaunchStartCmd(req)
	case "web":
		chainID := firstArgOrSelectedChain(cmd.Args, m.selectedVisibleChainID())
		if chainID == "" {
			return m.consoleCommandError("chain id is required")
		}
		target := fmt.Sprintf("%s/chains/%s", m.webBaseURL(), url.PathEscape(chainID))
		m.appendConsoleEntry(consoleEntryCommand, "WEB", fmt.Sprintf("run yard serve, then open %s", target))
		return m, m.chatComposer.Focus()
	default:
		return m.consoleCommandError(fmt.Sprintf("unknown command /%s; try /help", cmd.Name))
	}
}

func (m Model) consoleCommandError(message string) (tea.Model, tea.Cmd) {
	m.appendConsoleEntry(consoleEntryError, "ERROR", message)
	m.err = nil
	return m, m.chatComposer.Focus()
}

func (m Model) consoleChainDetailCmd(chainID string) tea.Cmd {
	return func() tea.Msg {
		if m.svc == nil {
			return consoleCommandMsg{Err: fmt.Errorf("operator service is not configured")}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		detail, err := m.svc.GetChainDetail(ctx, chainID)
		if err != nil {
			return consoleCommandMsg{Err: err}
		}
		return consoleCommandMsg{Entry: consoleEntry{Kind: consoleEntryCommand, Title: "CHAIN " + chainID, Body: m.renderConsoleChainDetail(detail)}, Refresh: true}
	}
}

func (m Model) consoleEventsCmd(chainID string, limit int) tea.Cmd {
	return func() tea.Msg {
		if m.svc == nil {
			return consoleCommandMsg{Err: fmt.Errorf("operator service is not configured")}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		events, err := m.svc.ListEventsSince(ctx, chainID, 0)
		if err != nil {
			return consoleCommandMsg{Err: err}
		}
		return consoleCommandMsg{Entry: consoleEntry{Kind: consoleEntryCommand, Title: "EVENTS " + chainID, Body: renderConsoleEvents(events, limit)}}
	}
}

func (m Model) consoleEffortCmd(effort string) tea.Cmd {
	return func() tea.Msg {
		if m.svc == nil {
			return consoleCommandMsg{Err: fmt.Errorf("operator service is not configured")}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		status, err := m.svc.SetReasoningEffort(ctx, effort)
		if err != nil {
			return consoleCommandMsg{Err: err}
		}
		return consoleCommandMsg{
			Status:  &status,
			Entry:   consoleEntry{Kind: consoleEntryCommand, Title: "EFFORT", Body: fmt.Sprintf("reasoning effort set to %s", valueOrUnknown(status.ReasoningEffort))},
			Refresh: true,
		}
	}
}

func (m Model) consoleReceiptCmd(chainID string, step string) tea.Cmd {
	return func() tea.Msg {
		if m.svc == nil {
			return consoleCommandMsg{Err: fmt.Errorf("operator service is not configured")}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		receipt, err := m.svc.ReadReceipt(ctx, chainID, step)
		if err != nil {
			return consoleCommandMsg{Err: err}
		}
		title := "RECEIPT " + chainID
		if strings.TrimSpace(step) != "" {
			title += " step " + step
		}
		body := fmt.Sprintf("path: %s\n\n%s", receipt.Path, renderReceiptViewportContent(m.styles, &receipt, maxInt(40, m.contentWidth()-4)))
		return consoleCommandMsg{Entry: consoleEntry{Kind: consoleEntryCommand, Title: title, Body: body}}
	}
}

func (m Model) consoleLaunchPreviewCmd(req operator.LaunchRequest) tea.Cmd {
	return func() tea.Msg {
		if m.svc == nil {
			return consoleCommandMsg{Err: fmt.Errorf("operator service is not configured")}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		preview, err := m.svc.ValidateLaunch(ctx, req)
		if err != nil {
			return consoleCommandMsg{Err: err}
		}
		return consoleCommandMsg{Entry: consoleEntry{Kind: consoleEntryCommand, Title: "PREVIEW", Body: renderConsoleLaunchPreview(preview)}}
	}
}

func (m Model) consoleLaunchStartCmd(req operator.LaunchRequest) tea.Cmd {
	return func() tea.Msg {
		if m.svc == nil {
			return consoleCommandMsg{Err: fmt.Errorf("operator service is not configured")}
		}
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		result, err := m.svc.StartChain(ctx, req)
		if err != nil {
			return consoleCommandMsg{Err: err}
		}
		body := fmt.Sprintf("chain: %s\nstatus: %s\nsummary: %s", result.ChainID, result.Status, result.Preview.Summary)
		return consoleCommandMsg{Entry: consoleEntry{Kind: consoleEntryCommand, Title: "STARTED", Body: body}, FollowChainID: result.ChainID, Refresh: true}
	}
}

func (m Model) consoleControlCmd(action string, chainID string) tea.Cmd {
	return func() tea.Msg {
		if m.svc == nil {
			return consoleCommandMsg{Err: fmt.Errorf("operator service is not configured")}
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
		case "resume":
			result, err = m.svc.ResumeChain(ctx, chainID)
		default:
			err = fmt.Errorf("unsupported control action %s", action)
		}
		if err != nil {
			return consoleCommandMsg{Err: err}
		}
		return consoleCommandMsg{Entry: consoleEntry{Kind: consoleEntryCommand, Title: strings.ToUpper(action), Body: renderControlResult(result)}, Refresh: true}
	}
}

func parseSlashInput(input string) (slashCommand, error) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return slashCommand{}, fmt.Errorf("command must start with /")
	}
	fields, err := shellFields(strings.TrimSpace(strings.TrimPrefix(input, "/")))
	if err != nil {
		return slashCommand{}, err
	}
	if len(fields) == 0 {
		return slashCommand{}, fmt.Errorf("empty command")
	}
	cmd := slashCommand{Name: strings.ToLower(fields[0]), Flags: map[string][]string{}}
	for i := 1; i < len(fields); i++ {
		field := fields[i]
		if strings.HasPrefix(field, "--") && len(field) > 2 {
			keyValue := strings.TrimPrefix(field, "--")
			key, value, ok := strings.Cut(keyValue, "=")
			key = strings.ToLower(strings.TrimSpace(key))
			if key == "" {
				return slashCommand{}, fmt.Errorf("empty flag in %q", field)
			}
			if !ok {
				value = "true"
				if i+1 < len(fields) && !strings.HasPrefix(fields[i+1], "--") {
					value = fields[i+1]
					i++
				}
			}
			cmd.Flags[key] = append(cmd.Flags[key], value)
			continue
		}
		cmd.Args = append(cmd.Args, field)
	}
	return cmd, nil
}

func shellFields(input string) ([]string, error) {
	var fields []string
	var current strings.Builder
	var quote rune
	escaped := false
	for _, r := range input {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if current.Len() > 0 {
				fields = append(fields, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if escaped {
		current.WriteRune('\\')
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if current.Len() > 0 {
		fields = append(fields, current.String())
	}
	return fields, nil
}

func slashLaunchRequest(cmd slashCommand) (operator.LaunchRequest, error) {
	req := operator.LaunchRequest{
		Role:       firstFlag(cmd, "role"),
		SourceTask: firstFlag(cmd, "task"),
	}
	if req.SourceTask == "" {
		req.SourceTask = strings.Join(cmd.Args, " ")
	}
	if mode := firstFlag(cmd, "mode"); mode != "" {
		normalized, err := normalizeSlashLaunchMode(mode)
		if err != nil {
			return operator.LaunchRequest{}, err
		}
		req.Mode = normalized
	}
	req.SourceSpecs = parseLaunchSpecs(strings.Join(flagValues(cmd, "spec", "specs"), ","))
	req.Roster = splitRoleList(flagValues(cmd, "roster"))
	req.AllowedRoles = splitRoleList(flagValues(cmd, "allowed", "roles"))
	if maxSteps, err := intFlag(cmd, "max-steps"); err != nil {
		return operator.LaunchRequest{}, err
	} else {
		req.MaxSteps = maxSteps
	}
	if tokenBudget, err := intFlag(cmd, "token-budget"); err != nil {
		return operator.LaunchRequest{}, err
	} else {
		req.TokenBudget = tokenBudget
	}
	if maxDuration := firstFlag(cmd, "max-duration"); maxDuration != "" {
		duration, err := time.ParseDuration(maxDuration)
		if err != nil {
			return operator.LaunchRequest{}, fmt.Errorf("invalid --max-duration: %w", err)
		}
		req.MaxDuration = duration
	}
	if maxResolverLoops, err := intFlag(cmd, "max-resolver-loops"); err != nil {
		return operator.LaunchRequest{}, err
	} else {
		req.MaxResolverLoops = maxResolverLoops
	}
	return req, nil
}

func normalizeSlashLaunchMode(mode string) (operator.LaunchMode, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto", "orchestrator", "sir_topham", "sir_topham_decides":
		return operator.LaunchModeOrchestrator, nil
	case "one", "one_step", "one-step", "one_step_chain":
		return operator.LaunchModeOneStep, nil
	case "manual", "manual_roster", "roster":
		return operator.LaunchModeManualRoster, nil
	case "constrained", "constrained_orchestration":
		return operator.LaunchModeConstrained, nil
	default:
		return "", fmt.Errorf("unsupported launch mode %s", mode)
	}
}

func firstFlag(cmd slashCommand, names ...string) string {
	for _, name := range names {
		values := cmd.Flags[strings.ToLower(name)]
		for _, value := range values {
			if strings.TrimSpace(value) != "" && value != "true" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func flagValues(cmd slashCommand, names ...string) []string {
	var values []string
	for _, name := range names {
		for _, value := range cmd.Flags[strings.ToLower(name)] {
			value = strings.TrimSpace(value)
			if value != "" && value != "true" {
				values = append(values, value)
			}
		}
	}
	return values
}

func intFlag(cmd slashCommand, name string) (int, error) {
	value := firstFlag(cmd, name)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid --%s: %w", name, err)
	}
	return parsed, nil
}

func splitRoleList(values []string) []string {
	var roles []string
	for _, value := range values {
		for _, role := range strings.Split(value, ",") {
			role = strings.TrimSpace(role)
			if role != "" {
				roles = append(roles, role)
			}
		}
	}
	return roles
}

func firstArgOrSelectedChain(args []string, selected string) string {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return strings.TrimSpace(args[0])
	}
	return strings.TrimSpace(selected)
}

func parseOptionalLimit(args []string, index int, fallback int) int {
	if len(args) <= index {
		return fallback
	}
	limit, err := strconv.Atoi(args[index])
	if err != nil || limit <= 0 {
		return fallback
	}
	return limit
}

func slashHelpText() string {
	return strings.Join([]string{
		"Core commands:",
		"/new                         clear this console session",
		"/status                      show Yard readiness",
		"/model                       show configured provider/model/effort",
		"/effort [low|medium|high|xhigh]  show or set Codex reasoning effort",
		"/chains [filter]             list recent chains",
		"/chain <chain-id>            show chain detail",
		"/events <chain-id> [limit]   show recent events",
		"/follow <chain-id>           append live events here",
		"/unfollow                    stop live event follow",
		"/receipt <chain-id> [step]   show receipt content",
		"/preview [launch flags]      validate a launch",
		"/start [launch flags]        start a chain and follow it",
		"/pause <chain-id>            request pause",
		"/resume <chain-id>           resume paused chain",
		"/cancel <chain-id>           confirm and request cancel",
		"/web <chain-id>              show web inspector target",
		"",
		"Launch flags:",
		"--task \"text\"  --role coder  --mode one_step_chain|manual_roster|constrained_orchestration|sir_topham_decides",
		"--roster planner,coder  --allowed coder,planner  --spec docs/specs/foo.md",
	}, "\n")
}

func (m Model) renderConsoleModel() string {
	lines := []string{
		"provider: " + valueOrUnknown(m.status.Provider),
		"model: " + valueOrUnknown(m.status.Model),
		"reasoning effort: " + valueOrUnknown(m.status.ReasoningEffort),
		"auth: " + valueOrUnknown(m.status.AuthStatus),
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderConsoleEffort() string {
	return strings.Join([]string{
		"current: " + valueOrUnknown(m.status.ReasoningEffort),
		"allowed: low, medium, high, xhigh",
		"set with: /effort xhigh",
	}, "\n")
}

func (m Model) renderConsoleStatus() string {
	lines := []string{
		fmt.Sprintf("readiness: %s", runtimeReadinessSummary(m.status)),
		fmt.Sprintf("project: %s", valueOrUnknown(m.status.ProjectName)),
		fmt.Sprintf("root: %s", valueOrUnknown(m.status.ProjectRoot)),
		fmt.Sprintf("provider: %s", valueOrUnknown(m.status.Provider)),
		fmt.Sprintf("model: %s", valueOrUnknown(m.status.Model)),
		fmt.Sprintf("reasoning effort: %s", valueOrUnknown(m.status.ReasoningEffort)),
		fmt.Sprintf("auth: %s", valueOrUnknown(m.status.AuthStatus)),
		fmt.Sprintf("code index: %s", renderIndexStatus(m.status.CodeIndex)),
		fmt.Sprintf("brain index: %s", renderIndexStatus(m.status.BrainIndex)),
		fmt.Sprintf("local services: %s", valueOrUnknown(m.status.LocalServicesStatus)),
		fmt.Sprintf("active chains: %d", m.status.ActiveChains),
		"",
		"Readiness:",
	}
	lines = append(lines, renderReadinessRows(m.styles, runtimeReadinessRows(m.status))...)
	if actions := dashboardActionLines(m.status); len(actions) > 0 {
		lines = append(lines, "", "Next actions:")
		for _, action := range actions {
			lines = append(lines, "- "+action)
		}
	}
	if len(m.status.Warnings) > 0 {
		lines = append(lines, "", "Warnings:")
		for _, warning := range m.status.Warnings {
			lines = append(lines, "- "+warning.Message)
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderConsoleChains(query string) string {
	chains := m.chains
	if strings.TrimSpace(query) != "" {
		chains = filterChains(chains, query)
	}
	if len(chains) == 0 {
		if strings.TrimSpace(query) != "" {
			return "No chains match filter."
		}
		return "No chains found."
	}
	lines := []string{}
	for i, ch := range chains {
		if i >= m.chainLimit {
			break
		}
		current := "idle"
		if ch.CurrentStep != nil {
			current = fmt.Sprintf("%s/%s", valueOrUnknown(ch.CurrentStep.Role), valueOrUnknown(ch.CurrentStep.Status))
		}
		task := ch.SourceTask
		if task == "" && len(ch.SourceSpecs) > 0 {
			task = strings.Join(ch.SourceSpecs, ", ")
		}
		lines = append(lines, fmt.Sprintf("%s  %-14s steps=%d tokens=%d current=%s  %s", ch.ID, ch.Status, ch.TotalSteps, ch.TotalTokens, current, trimOneLine(task, 72)))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderConsoleChainDetail(detail operator.ChainDetail) string {
	lines := []string{
		fmt.Sprintf("chain: %s", detail.Chain.ID),
		fmt.Sprintf("status: %s", valueOrUnknown(detail.Chain.Status)),
		fmt.Sprintf("health: %s", chainDetailHealth(&detail)),
		fmt.Sprintf("budgets: %s", renderChainBudgetLine(detail.Chain, len(detail.Steps))),
		fmt.Sprintf("current: %s", renderCurrentStep(currentStepSummary(detail.Steps))),
		fmt.Sprintf("summary: %s", trimOneLine(detail.Chain.Summary, 120)),
	}
	if len(detail.Chain.SourceSpecs) > 0 {
		lines = append(lines, "specs: "+strings.Join(detail.Chain.SourceSpecs, ", "))
	}
	lines = append(lines, "", "Steps:")
	if len(detail.Steps) == 0 {
		lines = append(lines, "No steps recorded.")
	} else {
		for _, step := range detail.Steps {
			lines = append(lines, renderStepLine(step))
		}
	}
	lines = append(lines, "", "Recent events:")
	lines = append(lines, renderEventLines(detail.RecentEvents, 8)...)
	return strings.Join(lines, "\n")
}

func renderConsoleEvents(events []chain.Event, limit int) string {
	return strings.Join(renderEventLines(events, limit), "\n")
}

func renderConsoleLaunchPreview(preview operator.LaunchPreview) string {
	lines := []string{
		"summary: " + preview.Summary,
		"mode: " + string(preview.Mode),
		"role: " + valueOrUnknown(preview.Role),
	}
	if len(preview.Roster) > 0 {
		lines = append(lines, "roster: "+strings.Join(preview.Roster, " -> "))
	}
	if len(preview.AllowedRoles) > 0 {
		lines = append(lines, "allowed: "+strings.Join(preview.AllowedRoles, ", "))
	}
	if strings.TrimSpace(preview.CompiledTask) != "" {
		lines = append(lines, "", "Compiled task:", trimOneLine(preview.CompiledTask, 160))
	}
	if len(preview.Warnings) > 0 {
		lines = append(lines, "", "Warnings:")
		for _, warning := range preview.Warnings {
			lines = append(lines, "- "+warning.Message)
		}
	}
	return strings.Join(lines, "\n")
}

func renderControlResult(result operator.ControlResult) string {
	lines := []string{
		"chain: " + result.ChainID,
		"status: " + valueOrUnknown(result.Status),
		"message: " + valueOrUnknown(result.Message),
	}
	if result.PreviousStatus != "" || result.TargetStatus != "" {
		lines = append(lines, fmt.Sprintf("transition: %s -> %s", valueOrUnknown(result.PreviousStatus), valueOrUnknown(result.TargetStatus)))
	}
	if len(result.SignaledPIDs) > 0 {
		pids := make([]string, 0, len(result.SignaledPIDs))
		for _, pid := range result.SignaledPIDs {
			pids = append(pids, strconv.Itoa(pid))
		}
		lines = append(lines, "signaled pids: "+strings.Join(pids, ", "))
	}
	if len(result.Warnings) > 0 {
		lines = append(lines, "", "Warnings:")
		for _, warning := range result.Warnings {
			lines = append(lines, "- "+warning.Message)
		}
	}
	return strings.Join(lines, "\n")
}
