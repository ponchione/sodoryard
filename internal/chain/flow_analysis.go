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
	AddressedCount  int
	ClosedCount     int
	ReopenedCount   int
	FirstSeenStep   int
	LastUpdatedStep int
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
			if !plannerSeen {
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

		for _, event := range eventsByStep[step.ID] {
			switch event.EventType {
			case EventReceiptFindings:
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
	for _, event := range eventsByStep[""] {
		if event.EventType != EventReceiptFindings {
			continue
		}
		payload, ok := parseReceiptFindingsPayload(event.EventData)
		if !ok {
			continue
		}
		analysis.applyFindingEvent(states, Step{}, payload, repeatedResolverWarned)
	}

	if in.Chain.Status == "completed" && lastCoderSeq > 0 && lastAuditorAfterCoderSeq < lastCoderSeq {
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
			state := findingState(states, id, "")
			if state.FirstSeenStep == 0 {
				state.FirstSeenStep = step.SequenceNum
			}
			state.AddressedCount++
			state.LastUpdatedStep = step.SequenceNum
			if state.Status != "closed" {
				state.Status = "addressed"
				state.Open = true
			}
			if state.AddressedCount > 1 && !repeatedResolverWarned[id] {
				a.addWarning(FlowWarning{
					Code:        "repeated_resolver_loop",
					Message:     fmt.Sprintf("flow: repeated resolver loop for %s (%d resolver receipts)", id, state.AddressedCount),
					StepID:      step.ID,
					SequenceNum: step.SequenceNum,
					Role:        step.Role,
					FindingID:   id,
				})
				repeatedResolverWarned[id] = true
			}
		}
	case isAuditorRole(role):
		for _, id := range uniqueStrings(payload.ClosedFindingIDs) {
			state := findingState(states, id, role)
			if state.FirstSeenStep == 0 {
				state.FirstSeenStep = step.SequenceNum
			}
			state.ClosedCount++
			state.Status = "closed"
			state.Open = false
			state.LastUpdatedStep = step.SequenceNum
			if state.SourceRole == "" {
				state.SourceRole = role
			}
		}
		for _, id := range uniqueStrings(payload.OpenFindingIDs) {
			state := findingState(states, id, role)
			if state.FirstSeenStep == 0 {
				state.FirstSeenStep = step.SequenceNum
			}
			if state.Status == "closed" {
				state.ReopenedCount++
			}
			state.Status = "open"
			state.Open = true
			state.LastUpdatedStep = step.SequenceNum
			if state.SourceRole == "" {
				state.SourceRole = role
			}
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

func parseChangedFilesPayload(data string) (changedFilesPayload, bool) {
	var payload changedFilesPayload
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return changedFilesPayload{}, false
	}
	payload.Paths = uniqueStrings(payload.Paths)
	payload.Error = strings.TrimSpace(payload.Error)
	return payload, true
}

func findingState(states map[string]*FindingLifecycleEntry, id string, sourceRole string) *FindingLifecycleEntry {
	id = strings.TrimSpace(id)
	state, ok := states[id]
	if !ok {
		state = &FindingLifecycleEntry{ID: id, SourceRole: normalizeRole(sourceRole), Status: "open", Open: true}
		states[id] = state
	}
	if state.SourceRole == "" {
		state.SourceRole = normalizeRole(sourceRole)
	}
	return state
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
	sort.Strings(lifecycle.OpenIDs)
	sort.Strings(lifecycle.ClosedIDs)
	sort.Strings(lifecycle.AddressedIDs)
	sort.Strings(lifecycle.ReopenedIDs)
	sort.Strings(lifecycle.RepeatedResolverIDs)
	return lifecycle
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
