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
	analysis := AnalyzeFlow(FlowAnalysisInput{Chain: in.Chain, Steps: in.Steps, Events: in.Events})
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

	if len(analysis.Warnings) > 0 {
		b.WriteString("\nChain flow warnings:\n")
		for _, warning := range analysis.Warnings {
			b.WriteString("- ")
			b.WriteString(warning.Message)
			b.WriteByte('\n')
		}
	}

	guardrailFacts := latestGuardrailFacts(in.Events)
	if len(guardrailFacts) > 0 {
		b.WriteString("\nLatest post-step guardrail facts:\n")
		for _, fact := range guardrailFacts {
			b.WriteString("- ")
			b.WriteString(fact)
			b.WriteByte('\n')
		}
	}

	openFindings := findingLines(analysis.Findings.OpenIDs, analysis.Findings, true)
	if len(openFindings) > 0 {
		b.WriteString("\nOpen audit findings:\n")
		for _, finding := range openFindings {
			b.WriteString("- ")
			b.WriteString(finding)
			b.WriteByte('\n')
		}
	}

	addressedFindings := findingLines(analysis.Findings.AddressedIDs, analysis.Findings, true)
	if len(addressedFindings) > 0 {
		b.WriteString("\nAddressed audit findings:\n")
		for _, finding := range addressedFindings {
			b.WriteString("- ")
			b.WriteString(finding)
			b.WriteByte('\n')
		}
	}

	reopenedFindings := findingLines(analysis.Findings.ReopenedIDs, analysis.Findings, false)
	if len(reopenedFindings) > 0 {
		b.WriteString("\nReopened audit findings:\n")
		for _, finding := range reopenedFindings {
			b.WriteString("- ")
			b.WriteString(finding)
			b.WriteByte('\n')
		}
	}

	repeatedResolverFindings := findingLines(analysis.Findings.RepeatedResolverIDs, analysis.Findings, false)
	if len(repeatedResolverFindings) > 0 {
		b.WriteString("\nRepeated resolver loops:\n")
		for _, finding := range repeatedResolverFindings {
			b.WriteString("- ")
			b.WriteString(finding)
			b.WriteByte('\n')
		}
	}

	closedFindings := findingLines(analysis.Findings.ClosedIDs, analysis.Findings, true)
	if len(closedFindings) > 0 {
		b.WriteString("\nClosed audit findings:\n")
		for _, finding := range closedFindings {
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

func latestGuardrailFacts(events []Event) []string {
	out := make([]string, 0)
	for i := len(events) - 1; i >= 0 && len(out) < 3; i-- {
		if events[i].EventType != EventStepGuardrailFacts {
			continue
		}
		var payload struct {
			Role                             string   `json:"role"`
			Sequence                         int      `json:"sequence"`
			ReceiptValid                     bool     `json:"receipt_valid"`
			ReceiptError                     string   `json:"receipt_error"`
			ChangedFileClaimPresent          bool     `json:"changed_file_claim_present"`
			ClaimedChangedFiles              []string `json:"claimed_changed_files"`
			ChangedFileClaimMatchesManifest  bool     `json:"changed_file_claim_matches_manifest"`
			ChangedFileClaimExtra            []string `json:"changed_file_claim_extra"`
			ChangedFileManifestUnclaimed     []string `json:"changed_file_manifest_unclaimed"`
			ChangedFileManifestPresent       bool     `json:"changed_file_manifest_present"`
			ChangedFileCount                 int      `json:"changed_file_count"`
			SourceWriterLockReleased         bool     `json:"source_writer_lock_released"`
			SourceWriterLockReleaseAttempted bool     `json:"source_writer_lock_release_attempted"`
			OpenFindingIDs                   []string `json:"open_finding_ids"`
			AddressedIDs                     []string `json:"addressed_ids"`
		}
		if err := json.Unmarshal([]byte(events[i].EventData), &payload); err != nil {
			out = append(out, "decode error: "+err.Error())
			continue
		}
		parts := []string{
			fmt.Sprintf("step %d", payload.Sequence),
			briefingKV("role", payload.Role),
			fmt.Sprintf("receipt_valid=%t", payload.ReceiptValid),
			fmt.Sprintf("manifest=%t", payload.ChangedFileManifestPresent),
			fmt.Sprintf("changed=%d", payload.ChangedFileCount),
		}
		if payload.ChangedFileClaimPresent || len(payload.ClaimedChangedFiles) > 0 || len(payload.ChangedFileClaimExtra) > 0 || len(payload.ChangedFileManifestUnclaimed) > 0 {
			parts = append(parts,
				fmt.Sprintf("claim_present=%t", payload.ChangedFileClaimPresent),
				fmt.Sprintf("claim_matches=%t", payload.ChangedFileClaimMatchesManifest),
			)
			if len(payload.ClaimedChangedFiles) > 0 {
				parts = append(parts, briefingKV("claimed", strings.Join(compactBriefingStrings(payload.ClaimedChangedFiles), ",")))
			}
			if len(payload.ChangedFileClaimExtra) > 0 {
				parts = append(parts, briefingKV("claim_extra", strings.Join(compactBriefingStrings(payload.ChangedFileClaimExtra), ",")))
			}
			if len(payload.ChangedFileManifestUnclaimed) > 0 {
				parts = append(parts, briefingKV("manifest_unclaimed", strings.Join(compactBriefingStrings(payload.ChangedFileManifestUnclaimed), ",")))
			}
		}
		parts = append(parts,
			fmt.Sprintf("lock_release_attempted=%t", payload.SourceWriterLockReleaseAttempted),
			fmt.Sprintf("lock_released=%t", payload.SourceWriterLockReleased),
		)
		if len(payload.OpenFindingIDs) > 0 {
			parts = append(parts, briefingKV("open", strings.Join(compactBriefingStrings(payload.OpenFindingIDs), ",")))
		}
		if len(payload.AddressedIDs) > 0 {
			parts = append(parts, briefingKV("addressed", strings.Join(compactBriefingStrings(payload.AddressedIDs), ",")))
		}
		if strings.TrimSpace(payload.ReceiptError) != "" {
			parts = append(parts, briefingKV("receipt_error", payload.ReceiptError))
		}
		out = append(out, strings.Join(compactBriefingStrings(parts), " "))
	}
	sort.Strings(out)
	return out
}

func findingLines(ids []string, lifecycle FindingLifecycle, includeEvidence bool) []string {
	idSet := map[string]struct{}{}
	for _, id := range ids {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			idSet[trimmed] = struct{}{}
		}
	}
	for _, finding := range lifecycle.Findings {
		delete(idSet, finding.ID)
	}
	lines := make([]string, 0, len(ids))
	for _, finding := range lifecycle.Findings {
		if _, ok := idSet[finding.ID]; ok {
			continue
		}
		if !containsBriefingString(ids, finding.ID) {
			continue
		}
		lines = append(lines, formatFindingLine(finding, includeEvidence))
	}
	for id := range idSet {
		lines = append(lines, id)
	}
	sort.Strings(lines)
	return lines
}

func formatFindingLine(finding FindingLifecycleEntry, includeEvidence bool) string {
	parts := []string{finding.ID}
	if strings.TrimSpace(finding.SourceRole) != "" {
		parts = append(parts, "from "+finding.SourceRole)
	}
	if strings.TrimSpace(finding.Status) != "" {
		parts = append(parts, briefingKV("status", finding.Status))
	}
	if finding.AddressedCount > 0 {
		parts = append(parts, fmt.Sprintf("addressed=%d", finding.AddressedCount))
	}
	if finding.ClosedCount > 0 {
		parts = append(parts, fmt.Sprintf("closed=%d", finding.ClosedCount))
	}
	if finding.ReopenedCount > 0 {
		parts = append(parts, fmt.Sprintf("reopened=%d", finding.ReopenedCount))
	}
	if includeEvidence {
		parts = append(parts,
			briefingKV("severity", finding.Severity),
			briefingKV("evidence", finding.Evidence),
			briefingKV("summary", finding.Summary),
			briefingKV("required_fix", finding.RequiredFix),
			briefingKV("resolution", finding.Resolution),
			briefingKV("files", strings.Join(finding.FilesChanged, ",")),
			briefingKV("validation", strings.Join(finding.Validation, ",")),
		)
	}
	return strings.Join(compactBriefingStrings(parts), " ")
}

func briefingKV(key string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return key + "=" + value
}

func containsBriefingString(values []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func compactBriefingStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
