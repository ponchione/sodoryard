package chain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type StepBriefingInput struct {
	Chain               Chain
	Steps               []Step
	Events              []Event
	CurrentStepSequence int
	CurrentRole         string
	ReceiptPath         string
}

func BuildStepBriefing(in StepBriefingInput) string {
	var b strings.Builder
	writeBriefingLine(&b, "Chain ID", in.Chain.ID)
	writeBriefingLine(&b, "Chain status", in.Chain.Status)
	writeBriefingLine(&b, "Launch task", in.Chain.SourceTask)
	writeBriefingLine(&b, "Current step", fmt.Sprintf("%d %s", in.CurrentStepSequence, strings.TrimSpace(in.CurrentRole)))
	writeBriefingLine(&b, "Receipt path", in.ReceiptPath)
	writeBriefingLine(&b, "Budgets", fmt.Sprintf("steps %d/%d, tokens %d/%d, duration %ds/%ds, resolver loops %d/%d",
		in.Chain.TotalSteps, in.Chain.MaxSteps,
		in.Chain.TotalTokens, in.Chain.TokenBudget,
		in.Chain.TotalDurationSecs, in.Chain.MaxDurationSecs,
		in.Chain.ResolverLoops, in.Chain.MaxResolverLoops,
	))

	b.WriteString("\nPrevious step receipts:\n")
	receiptLines := previousReceiptLines(in.Steps)
	if len(receiptLines) == 0 {
		b.WriteString("- none yet\n")
	} else {
		for _, line := range receiptLines {
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	b.WriteString("\nLatest changed-file manifest:\n")
	paths, manifestErr := latestChangedFiles(in.Events)
	if manifestErr != "" {
		b.WriteString("- capture error: ")
		b.WriteString(manifestErr)
		b.WriteByte('\n')
	} else if len(paths) == 0 {
		b.WriteString("- none recorded\n")
	} else {
		for _, path := range paths {
			b.WriteString("- ")
			b.WriteString(path)
			b.WriteByte('\n')
		}
	}

	warnings := receiptValidationWarnings(in.Events)
	if len(warnings) > 0 {
		b.WriteString("\nReceipt validation warnings:\n")
		for _, warning := range warnings {
			b.WriteString("- ")
			b.WriteString(warning)
			b.WriteByte('\n')
		}
	}

	openFindings := openFindingLines(in.Events)
	if len(openFindings) > 0 {
		b.WriteString("\nOpen audit findings:\n")
		for _, finding := range openFindings {
			b.WriteString("- ")
			b.WriteString(finding)
			b.WriteByte('\n')
		}
	}

	return strings.TrimSpace(b.String())
}

func writeBriefingLine(b *strings.Builder, label string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "<unset>"
	}
	b.WriteString("- ")
	b.WriteString(label)
	b.WriteString(": ")
	b.WriteString(value)
	b.WriteByte('\n')
}

func previousReceiptLines(steps []Step) []string {
	lines := make([]string, 0, len(steps))
	for _, step := range steps {
		if strings.TrimSpace(step.ReceiptPath) == "" {
			continue
		}
		verdict := strings.TrimSpace(step.Verdict)
		if verdict == "" {
			verdict = "<unset>"
		}
		status := strings.TrimSpace(step.Status)
		if status == "" {
			status = "<unset>"
		}
		lines = append(lines, fmt.Sprintf("step %d %s status=%s verdict=%s receipt=%s", step.SequenceNum, step.Role, status, verdict, step.ReceiptPath))
	}
	sort.Strings(lines)
	return lines
}

func latestChangedFiles(events []Event) ([]string, string) {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].EventType != EventStepChangedFiles {
			continue
		}
		var payload struct {
			Paths []string `json:"paths"`
			Error string   `json:"error"`
		}
		if err := json.Unmarshal([]byte(events[i].EventData), &payload); err != nil {
			return nil, err.Error()
		}
		paths := append([]string(nil), payload.Paths...)
		sort.Strings(paths)
		return paths, strings.TrimSpace(payload.Error)
	}
	return nil, ""
}

func receiptValidationWarnings(events []Event) []string {
	warnings := make([]string, 0)
	for _, event := range events {
		if event.EventType != EventReceiptValidation {
			continue
		}
		var payload struct {
			Role        string `json:"role"`
			ReceiptPath string `json:"receipt_path"`
			Warning     string `json:"warning"`
			Error       string `json:"error"`
		}
		if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		warning := strings.TrimSpace(payload.Warning)
		if warning == "" {
			warning = strings.TrimSpace(payload.Error)
		}
		if warning == "" {
			continue
		}
		prefix := strings.TrimSpace(payload.Role)
		if payload.ReceiptPath != "" {
			if prefix != "" {
				prefix += " "
			}
			prefix += payload.ReceiptPath
		}
		if prefix != "" {
			warning = prefix + ": " + warning
		}
		warnings = append(warnings, warning)
	}
	return warnings
}

func openFindingLines(events []Event) []string {
	open := map[string]string{}
	for _, event := range events {
		if event.EventType != EventReceiptFindings {
			continue
		}
		var payload struct {
			Role             string   `json:"role"`
			OpenFindingIDs   []string `json:"open_finding_ids"`
			ClosedFindingIDs []string `json:"closed_finding_ids"`
		}
		if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil {
			continue
		}
		for _, id := range payload.ClosedFindingIDs {
			delete(open, strings.TrimSpace(id))
		}
		for _, id := range payload.OpenFindingIDs {
			id = strings.TrimSpace(id)
			if id != "" {
				open[id] = strings.TrimSpace(payload.Role)
			}
		}
	}
	lines := make([]string, 0, len(open))
	for id, role := range open {
		if role == "" {
			lines = append(lines, id)
		} else {
			lines = append(lines, fmt.Sprintf("%s from %s", id, role))
		}
	}
	sort.Strings(lines)
	return lines
}
