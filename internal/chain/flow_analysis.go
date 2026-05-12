package chain

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type FlowAnalyzer struct {
	IsSourceWritingRole func(string) bool
	IsDocsImpactingPath func(string) bool
}

type FlowAnalysisInput struct {
	Chain  Chain
	Steps  []Step
	Events []Event
}

type FlowAnalysis struct {
	Warnings []FlowWarning
	Findings FindingLifecycle
}

type FlowWarning struct {
	Code        string
	Message     string
	StepID      string
	SequenceNum int
	Role        string
	FindingID   string
}

type FindingLifecycle struct {
	Findings            []FindingLifecycleEntry
	OpenIDs             []string
	ClosedIDs           []string
	AddressedIDs        []string
	ReopenedIDs         []string
	RepeatedResolverIDs []string
}

type FindingLifecycleEntry struct {
	ID              string
	SourceRole      string
	Status          string
	Open            bool
	LastAction      string
	Severity        string
	Evidence        string
	Summary         string
	RequiredFix     string
	Resolution      string
	FilesChanged    []string
	Validation      []string
	AddressedCount  int
	ClosedCount     int
	ReopenedCount   int
	FirstSeenStep   int
	LastUpdatedStep int
}

type FindingLifecycleFactsPayload struct {
	Role        string                 `json:"role"`
	Verdict     string                 `json:"verdict"`
	ReceiptPath string                 `json:"receipt_path,omitempty"`
	Facts       []FindingLifecycleFact `json:"facts"`
}

type FindingLifecycleFact struct {
	ID           string   `json:"id"`
	SourceRole   string   `json:"source_role,omitempty"`
	Action       string   `json:"action"`
	Status       string   `json:"status,omitempty"`
	Severity     string   `json:"severity,omitempty"`
	Evidence     string   `json:"evidence,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	RequiredFix  string   `json:"required_fix,omitempty"`
	Resolution   string   `json:"resolution,omitempty"`
	FilesChanged []string `json:"files_changed,omitempty"`
	Validation   []string `json:"validation,omitempty"`
}

type receiptFindingsPayload struct {
	Role             string   `json:"role"`
	Verdict          string   `json:"verdict"`
	FindingIDs       []string `json:"finding_ids"`
	OpenFindingIDs   []string `json:"open_finding_ids"`
	ClosedFindingIDs []string `json:"closed_finding_ids"`
	AddressedIDs     []string `json:"addressed_ids"`
	OpenCount        int      `json:"open_count"`
	ClosedCount      int      `json:"closed_count"`
	AddressedCount   int      `json:"addressed_count"`
}

type changedFilesPayload struct {
	Paths []string `json:"paths"`
	Error string   `json:"error"`
}

func AnalyzeFlow(in FlowAnalysisInput) FlowAnalysis {
	return FlowAnalyzer{}.Analyze(in)
}

func (a FlowAnalyzer) Analyze(in FlowAnalysisInput) FlowAnalysis {
	steps := append([]Step(nil), in.Steps...)
	sort.SliceStable(steps, func(i, j int) bool {
		if steps[i].SequenceNum == steps[j].SequenceNum {
			return steps[i].ID < steps[j].ID
		}
		return steps[i].SequenceNum < steps[j].SequenceNum
	})
	eventsByStep := eventsGroupedByStep(in.Events)
	isSourceWriter := a.IsSourceWritingRole
	if isSourceWriter == nil {
		isSourceWriter = defaultSourceWritingRole
	}
	isDocsImpacting := a.IsDocsImpactingPath
	if isDocsImpacting == nil {
		isDocsImpacting = defaultDocsImpactingPath
	}
	launchMode := launchModeFromEvents(in.Events)
	operatorDeclaredSequence := isOperatorDeclaredSequenceMode(launchMode)

	var analysis FlowAnalysis
	states := map[string]*FindingLifecycleEntry{}
	plannerSeen := false
	lastCoderSeq := 0
	lastAuditorAfterCoderSeq := 0
	lastDocsImpactingSeq := 0
	lastDocsArbiterSeq := 0
	repeatedResolverWarned := map[string]bool{}

	for _, step := range steps {
		role := normalizeRole(step.Role)
		switch role {
		case "planner":
			plannerSeen = true
		case "coder":
			if !plannerSeen && !operatorDeclaredSequence {
				analysis.addWarning(FlowWarning{
					Code:        "coder_before_planner",
					Message:     fmt.Sprintf("flow: coder step %d started before planner", step.SequenceNum),
					StepID:      step.ID,
					SequenceNum: step.SequenceNum,
					Role:        step.Role,
				})
			}
			lastCoderSeq = step.SequenceNum
		case "resolver":
			if countOpenFindings(states) == 0 {
				analysis.addWarning(FlowWarning{
					Code:        "resolver_without_open_findings",
					Message:     fmt.Sprintf("flow: resolver step %d ran without open findings", step.SequenceNum),
					StepID:      step.ID,
					SequenceNum: step.SequenceNum,
					Role:        step.Role,
				})
			}
		case "docs-arbiter":
			lastDocsArbiterSeq = step.SequenceNum
		}
		if isAuditorRole(role) && lastCoderSeq > 0 && step.SequenceNum > lastCoderSeq {
			lastAuditorAfterCoderSeq = step.SequenceNum
		}

		hasLifecycleFacts := stepHasLifecycleFacts(eventsByStep[step.ID])
		for _, event := range eventsByStep[step.ID] {
			switch event.EventType {
			case EventFindingLifecycleFacts:
				payload, ok := parseFindingLifecycleFactsPayload(event.EventData)
				if !ok {
					continue
				}
				analysis.applyFindingLifecycleFacts(states, step, payload, repeatedResolverWarned)
			case EventReceiptFindings:
				if hasLifecycleFacts {
					continue
				}
				payload, ok := parseReceiptFindingsPayload(event.EventData)
				if !ok {
					continue
				}
				analysis.applyFindingEvent(states, step, payload, repeatedResolverWarned)
			case EventStepChangedFiles:
				if !isSourceWriter(role) {
					continue
				}
				payload, ok := parseChangedFilesPayload(event.EventData)
				if !ok {
					continue
				}
				for _, path := range payload.Paths {
					if isDocsImpacting(path) {
						lastDocsImpactingSeq = step.SequenceNum
						break
					}
				}
			}
		}
	}
	hasChainLifecycleFacts := stepHasLifecycleFacts(eventsByStep[""])
	for _, event := range eventsByStep[""] {
		switch event.EventType {
		case EventFindingLifecycleFacts:
			payload, ok := parseFindingLifecycleFactsPayload(event.EventData)
			if !ok {
				continue
			}
			analysis.applyFindingLifecycleFacts(states, Step{}, payload, repeatedResolverWarned)
		case EventReceiptFindings:
			if hasChainLifecycleFacts {
				continue
			}
			payload, ok := parseReceiptFindingsPayload(event.EventData)
			if !ok {
				continue
			}
			analysis.applyFindingEvent(states, Step{}, payload, repeatedResolverWarned)
		}
	}

	if in.Chain.Status == "completed" && lastCoderSeq > 0 && lastAuditorAfterCoderSeq < lastCoderSeq && !operatorDeclaredSequence {
		analysis.addWarning(FlowWarning{
			Code:        "completed_without_auditor",
			Message:     fmt.Sprintf("flow: chain completed after coder step %d without later auditor", lastCoderSeq),
			SequenceNum: lastCoderSeq,
			Role:        "coder",
		})
	}
	if isTerminalStatus(in.Chain.Status) && lastDocsImpactingSeq > 0 && lastDocsArbiterSeq < lastDocsImpactingSeq {
		analysis.addWarning(FlowWarning{
			Code:        "docs_arbiter_missing",
			Message:     fmt.Sprintf("flow: docs-arbiter missing after docs-impacting changes in step %d", lastDocsImpactingSeq),
			SequenceNum: lastDocsImpactingSeq,
		})
	}
	analysis.Findings = summarizeFindingLifecycle(states)
	return analysis
}

func (a *FlowAnalysis) addWarning(w FlowWarning) {
	if strings.TrimSpace(w.Message) == "" {
		return
	}
	a.Warnings = append(a.Warnings, w)
}

func (a *FlowAnalysis) applyFindingEvent(states map[string]*FindingLifecycleEntry, step Step, payload receiptFindingsPayload, repeatedResolverWarned map[string]bool) {
	role := normalizeRole(payload.Role)
	if role == "" {
		role = normalizeRole(step.Role)
	}
	switch {
	case role == "resolver":
		for _, id := range uniqueStrings(payload.AddressedIDs) {
			a.applyAddressedFinding(states, step, FindingLifecycleFact{ID: id, Action: "addressed", Status: "addressed"}, repeatedResolverWarned)
		}
	case isAuditorRole(role):
		for _, id := range uniqueStrings(payload.ClosedFindingIDs) {
			a.applyAuditorFinding(states, step, FindingLifecycleFact{ID: id, SourceRole: role, Action: "closed", Status: "closed"})
		}
		for _, id := range uniqueStrings(payload.OpenFindingIDs) {
			a.applyAuditorFinding(states, step, FindingLifecycleFact{ID: id, SourceRole: role, Action: "opened", Status: "open"})
		}
	}
}

func (a *FlowAnalysis) applyFindingLifecycleFacts(states map[string]*FindingLifecycleEntry, step Step, payload FindingLifecycleFactsPayload, repeatedResolverWarned map[string]bool) {
	role := normalizeRole(payload.Role)
	if role == "" {
		role = normalizeRole(step.Role)
	}
	for _, fact := range payload.Facts {
		fact.ID = strings.TrimSpace(fact.ID)
		if fact.ID == "" {
			continue
		}
		fact.SourceRole = normalizeRole(firstNonEmpty(fact.SourceRole, sourceRoleForLifecycleFact(role, fact)))
		fact.Action = normalizeLifecycleAction(fact.Action)
		fact.Status = normalizeLifecycleStatus(fact.Status)
		switch {
		case fact.Action == "addressed" || role == "resolver":
			a.applyAddressedFinding(states, step, fact, repeatedResolverWarned)
		case isAuditorRole(role) || fact.SourceRole != "":
			a.applyAuditorFinding(states, step, fact)
		}
	}
}

func (a *FlowAnalysis) applyAuditorFinding(states map[string]*FindingLifecycleEntry, step Step, fact FindingLifecycleFact) {
	status := normalizeLifecycleStatus(fact.Status)
	action := normalizeLifecycleAction(fact.Action)
	if action == "" {
		action = actionFromLifecycleStatus(status)
	}
	if status == "" {
		status = statusFromLifecycleAction(action)
	}
	if status == "" {
		status = "open"
	}
	state := findingState(states, fact.ID, fact.SourceRole)
	updateFindingMetadata(state, fact)
	if state.FirstSeenStep == 0 {
		state.FirstSeenStep = step.SequenceNum
	}
	switch action {
	case "closed":
		state.ClosedCount++
		state.Status = "closed"
		state.Open = false
	case "reopened":
		state.ReopenedCount++
		state.Status = "open"
		state.Open = true
	case "opened":
		if state.Status == "closed" {
			state.ReopenedCount++
		}
		state.Status = "open"
		state.Open = true
	default:
		if status == "closed" {
			state.ClosedCount++
			state.Status = "closed"
			state.Open = false
		} else {
			if state.Status == "closed" {
				state.ReopenedCount++
			}
			state.Status = "open"
			state.Open = true
		}
	}
	if state.SourceRole == "" {
		state.SourceRole = fact.SourceRole
	}
	state.LastAction = firstNonEmpty(action, status)
	state.LastUpdatedStep = step.SequenceNum
}

func (a *FlowAnalysis) applyAddressedFinding(states map[string]*FindingLifecycleEntry, step Step, fact FindingLifecycleFact, repeatedResolverWarned map[string]bool) {
	targets := findingStatesByID(states, fact.ID)
	if len(targets) == 0 {
		targets = []*FindingLifecycleEntry{findingState(states, fact.ID, fact.SourceRole)}
	}
	for _, state := range targets {
		if state.FirstSeenStep == 0 {
			state.FirstSeenStep = step.SequenceNum
		}
		updateFindingMetadata(state, fact)
		state.AddressedCount++
		state.LastAction = "addressed"
		state.LastUpdatedStep = step.SequenceNum
		if state.Status != "closed" {
			state.Status = "addressed"
			state.Open = true
		}
		if state.AddressedCount > 1 && !repeatedResolverWarned[state.ID] {
			a.addWarning(FlowWarning{
				Code:        "repeated_resolver_loop",
				Message:     fmt.Sprintf("flow: repeated resolver loop for %s (%d resolver receipts)", state.ID, state.AddressedCount),
				StepID:      step.ID,
				SequenceNum: step.SequenceNum,
				Role:        step.Role,
				FindingID:   state.ID,
			})
			repeatedResolverWarned[state.ID] = true
		}
	}
}

func eventsGroupedByStep(events []Event) map[string][]Event {
	byStep := map[string][]Event{}
	for _, event := range events {
		byStep[event.StepID] = append(byStep[event.StepID], event)
	}
	for stepID := range byStep {
		sort.SliceStable(byStep[stepID], func(i, j int) bool {
			if byStep[stepID][i].ID == byStep[stepID][j].ID {
				return byStep[stepID][i].CreatedAt.Before(byStep[stepID][j].CreatedAt)
			}
			return byStep[stepID][i].ID < byStep[stepID][j].ID
		})
	}
	return byStep
}

func parseReceiptFindingsPayload(data string) (receiptFindingsPayload, bool) {
	var payload receiptFindingsPayload
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return receiptFindingsPayload{}, false
	}
	payload.Role = normalizeRole(payload.Role)
	payload.Verdict = strings.TrimSpace(payload.Verdict)
	payload.FindingIDs = uniqueStrings(payload.FindingIDs)
	payload.OpenFindingIDs = uniqueStrings(payload.OpenFindingIDs)
	payload.ClosedFindingIDs = uniqueStrings(payload.ClosedFindingIDs)
	payload.AddressedIDs = uniqueStrings(payload.AddressedIDs)
	if payload.OpenCount == 0 {
		payload.OpenCount = len(payload.OpenFindingIDs)
	}
	if payload.ClosedCount == 0 {
		payload.ClosedCount = len(payload.ClosedFindingIDs)
	}
	if payload.AddressedCount == 0 {
		payload.AddressedCount = len(payload.AddressedIDs)
	}
	return payload, true
}

func parseFindingLifecycleFactsPayload(data string) (FindingLifecycleFactsPayload, bool) {
	var payload FindingLifecycleFactsPayload
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return FindingLifecycleFactsPayload{}, false
	}
	payload.Role = normalizeRole(payload.Role)
	payload.Verdict = strings.TrimSpace(payload.Verdict)
	payload.ReceiptPath = strings.TrimSpace(payload.ReceiptPath)
	facts := make([]FindingLifecycleFact, 0, len(payload.Facts))
	for _, fact := range payload.Facts {
		fact.ID = strings.TrimSpace(fact.ID)
		if fact.ID == "" {
			continue
		}
		fact.SourceRole = normalizeRole(fact.SourceRole)
		fact.Action = normalizeLifecycleAction(fact.Action)
		fact.Status = normalizeLifecycleStatus(fact.Status)
		fact.Severity = strings.TrimSpace(fact.Severity)
		fact.Evidence = strings.TrimSpace(fact.Evidence)
		fact.Summary = strings.TrimSpace(fact.Summary)
		fact.RequiredFix = strings.TrimSpace(fact.RequiredFix)
		fact.Resolution = strings.TrimSpace(fact.Resolution)
		fact.FilesChanged = uniqueStrings(fact.FilesChanged)
		fact.Validation = uniqueStrings(fact.Validation)
		facts = append(facts, fact)
	}
	payload.Facts = facts
	return payload, true
}

func stepHasLifecycleFacts(events []Event) bool {
	for _, event := range events {
		if event.EventType == EventFindingLifecycleFacts {
			return true
		}
	}
	return false
}

func parseChangedFilesPayload(data string) (changedFilesPayload, bool) {
	var payload changedFilesPayload
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return changedFilesPayload{}, false
	}
	payload.Paths = uniqueStrings(payload.Paths)
	payload.Error = strings.TrimSpace(payload.Error)
	return payload, true
}

func launchModeFromEvents(events []Event) string {
	var mode string
	for _, event := range events {
		switch event.EventType {
		case EventChainStarted, EventChainCompleted:
		default:
			continue
		}
		var payload struct {
			Mode string `json:"mode"`
		}
		if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil {
			continue
		}
		if trimmed := strings.TrimSpace(payload.Mode); trimmed != "" {
			mode = trimmed
		}
	}
	return mode
}

func isOperatorDeclaredSequenceMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case "one_step_chain", "manual_roster":
		return true
	default:
		return false
	}
}

func findingState(states map[string]*FindingLifecycleEntry, id string, sourceRole string) *FindingLifecycleEntry {
	id = strings.TrimSpace(id)
	sourceRole = normalizeRole(sourceRole)
	if sourceRole == "" {
		if state := singleFindingStateByID(states, id); state != nil {
			return state
		}
	}
	key := findingStateKey(id, sourceRole)
	state, ok := states[key]
	if !ok {
		if sourceRole != "" {
			unknownKey := findingStateKey(id, "")
			if unknown, found := states[unknownKey]; found {
				delete(states, unknownKey)
				unknown.SourceRole = sourceRole
				states[key] = unknown
				return unknown
			}
		}
		state = &FindingLifecycleEntry{ID: id, SourceRole: sourceRole, Status: "open", Open: true}
		states[key] = state
	}
	if state.SourceRole == "" {
		state.SourceRole = sourceRole
	}
	return state
}

func findingStateKey(id string, sourceRole string) string {
	id = strings.TrimSpace(id)
	sourceRole = normalizeRole(sourceRole)
	if sourceRole == "" {
		return id
	}
	return id + "\x00" + sourceRole
}

func findingStatesByID(states map[string]*FindingLifecycleEntry, id string) []*FindingLifecycleEntry {
	id = strings.TrimSpace(id)
	out := make([]*FindingLifecycleEntry, 0)
	for _, state := range states {
		if state.ID == id {
			out = append(out, state)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].SourceRole < out[j].SourceRole
	})
	return out
}

func singleFindingStateByID(states map[string]*FindingLifecycleEntry, id string) *FindingLifecycleEntry {
	matches := findingStatesByID(states, id)
	if len(matches) == 1 {
		return matches[0]
	}
	return nil
}

func updateFindingMetadata(state *FindingLifecycleEntry, fact FindingLifecycleFact) {
	if state == nil {
		return
	}
	if fact.SourceRole != "" && state.SourceRole == "" {
		state.SourceRole = fact.SourceRole
	}
	state.Severity = firstNonEmpty(fact.Severity, state.Severity)
	state.Evidence = firstNonEmpty(fact.Evidence, state.Evidence)
	state.Summary = firstNonEmpty(fact.Summary, state.Summary)
	state.RequiredFix = firstNonEmpty(fact.RequiredFix, state.RequiredFix)
	state.Resolution = firstNonEmpty(fact.Resolution, state.Resolution)
	if len(fact.FilesChanged) > 0 {
		state.FilesChanged = uniqueStrings(append(state.FilesChanged, fact.FilesChanged...))
	}
	if len(fact.Validation) > 0 {
		state.Validation = uniqueStrings(append(state.Validation, fact.Validation...))
	}
}

func summarizeFindingLifecycle(states map[string]*FindingLifecycleEntry) FindingLifecycle {
	entries := make([]FindingLifecycleEntry, 0, len(states))
	var lifecycle FindingLifecycle
	for _, state := range states {
		if strings.TrimSpace(state.ID) == "" {
			continue
		}
		if state.Status == "" {
			state.Status = "open"
		}
		entry := *state
		entries = append(entries, entry)
		if entry.Open {
			lifecycle.OpenIDs = append(lifecycle.OpenIDs, entry.ID)
		}
		if entry.Status == "closed" {
			lifecycle.ClosedIDs = append(lifecycle.ClosedIDs, entry.ID)
		}
		if entry.AddressedCount > 0 {
			lifecycle.AddressedIDs = append(lifecycle.AddressedIDs, entry.ID)
		}
		if entry.ReopenedCount > 0 {
			lifecycle.ReopenedIDs = append(lifecycle.ReopenedIDs, entry.ID)
		}
		if entry.AddressedCount > 1 {
			lifecycle.RepeatedResolverIDs = append(lifecycle.RepeatedResolverIDs, entry.ID)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].ID == entries[j].ID {
			return entries[i].SourceRole < entries[j].SourceRole
		}
		return entries[i].ID < entries[j].ID
	})
	lifecycle.Findings = entries
	lifecycle.OpenIDs = uniqueStrings(lifecycle.OpenIDs)
	lifecycle.ClosedIDs = uniqueStrings(lifecycle.ClosedIDs)
	lifecycle.AddressedIDs = uniqueStrings(lifecycle.AddressedIDs)
	lifecycle.ReopenedIDs = uniqueStrings(lifecycle.ReopenedIDs)
	lifecycle.RepeatedResolverIDs = uniqueStrings(lifecycle.RepeatedResolverIDs)
	return lifecycle
}

func normalizeLifecycleAction(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), "_"))
	switch value {
	case "open", "opened", "reported":
		return "opened"
	case "reopen", "reopened":
		return "reopened"
	case "close", "closed", "fixed", "resolved":
		return "closed"
	case "address", "addressed", "fixed_by_resolver":
		return "addressed"
	default:
		return value
	}
}

func normalizeLifecycleStatus(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), "_"))
	switch value {
	case "open", "opened":
		return "open"
	case "reopen", "reopened":
		return "reopened"
	case "closed", "fixed", "resolved":
		return "closed"
	case "addressed", "fixed_by_resolver":
		return "addressed"
	default:
		return value
	}
}

func actionFromLifecycleStatus(status string) string {
	switch normalizeLifecycleStatus(status) {
	case "closed":
		return "closed"
	case "reopened":
		return "reopened"
	case "addressed":
		return "addressed"
	case "open":
		return "opened"
	default:
		return ""
	}
}

func statusFromLifecycleAction(action string) string {
	switch normalizeLifecycleAction(action) {
	case "closed":
		return "closed"
	case "reopened", "opened":
		return "open"
	case "addressed":
		return "addressed"
	default:
		return ""
	}
}

func sourceRoleForLifecycleFact(role string, fact FindingLifecycleFact) string {
	if strings.TrimSpace(fact.SourceRole) != "" {
		return fact.SourceRole
	}
	if isAuditorRole(role) {
		return role
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func countOpenFindings(states map[string]*FindingLifecycleEntry) int {
	count := 0
	for _, state := range states {
		if state.Open {
			count++
		}
	}
	return count
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

func defaultSourceWritingRole(role string) bool {
	switch normalizeRole(role) {
	case "coder", "resolver", "test-writer":
		return true
	default:
		return false
	}
}

func isAuditorRole(role string) bool {
	switch normalizeRole(role) {
	case "correctness-auditor", "quality-auditor", "performance-auditor", "security-auditor", "integration-auditor":
		return true
	default:
		return false
	}
}

func defaultDocsImpactingPath(path string) bool {
	path = strings.TrimSpace(filepath.ToSlash(path))
	if path == "" {
		return false
	}
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".md") || strings.HasPrefix(lower, "docs/") || strings.HasPrefix(lower, "agents/")
}

func isTerminalStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "completed", "failed", "cancelled", "dry_run":
		return true
	default:
		return false
	}
}
