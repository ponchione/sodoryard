package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
	"github.com/ponchione/sodoryard/internal/brain/analysis"
	"github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/provider"
)

type BrainLint struct {
	client brain.Backend
	config config.BrainConfig
	llm    provider.Provider
}

func NewBrainLint(client brain.Backend, cfg config.BrainConfig) *BrainLint {
	return NewBrainLintWithProvider(client, cfg, nil)
}

func NewBrainLintWithProvider(client brain.Backend, cfg config.BrainConfig, llm provider.Provider) *BrainLint {
	return &BrainLint{client: client, config: cfg, llm: llm}
}

type brainLintInput struct {
	Scope           string   `json:"scope"`
	Checks          []string `json:"checks"`
	AllowModelCalls bool     `json:"allow_model_calls,omitempty"`
}

func (b *BrainLint) Name() string { return "brain_lint" }
func (b *BrainLint) Description() string {
	return "Run deterministic health checks over brain documents in the Obsidian vault"
}
func (b *BrainLint) ToolPurity() Purity { return Mutating }

func (b *BrainLint) Schema() json.RawMessage {
	return json.RawMessage(`{
		"name": "brain_lint",
		"description": "Run deterministic lint checks over the project brain (Obsidian vault). Supports full-vault, path-prefix, #tag, or combined path+tag scope like 'notes/+#architecture', plus optional check subsets. The contradictions check is model-assisted and only runs when explicitly opted in.",
		"input_schema": {
			"type": "object",
			"properties": {
				"scope": {
					"type": "string",
					"description": "Lint scope: 'full' (default), a path-prefix scope like 'architecture/', a tag scope like '#architecture', or a combined scope like 'notes/+#architecture'"
				},
				"checks": {
					"type": "array",
					"items": {"type": "string"},
					"description": "Subset of checks to run: orphans, dead_links, stale_references, missing_pages, contradictions, tag_hygiene"
				},
				"allow_model_calls": {
					"type": "boolean",
					"description": "Required when using the contradictions check. Set true to explicitly allow model-assisted contradiction analysis."
				}
			}
		}
	}`)
}

func (b *BrainLint) Execute(ctx context.Context, projectRoot string, input json.RawMessage) (*ToolResult, error) {
	if !b.config.Enabled {
		return &ToolResult{Success: false, Content: "Project brain is not configured. See the project's YAML config brain section."}, nil
	}
	if b.client == nil {
		return &ToolResult{Success: false, Content: "Project brain backend is not configured."}, nil
	}

	var params brainLintInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &params); err != nil {
			return &ToolResult{Success: false, Content: fmt.Sprintf("Invalid input: %v", err), Error: err.Error()}, nil
		}
	}
	if strings.TrimSpace(params.Scope) == "" {
		params.Scope = "full"
	}
	if err := analysis.ValidateScope(params.Scope); err != nil {
		return &ToolResult{Success: false, Content: fmt.Sprintf("Invalid input: %v", err), Error: err.Error()}, nil
	}
	if err := analysis.ValidateChecks(params.Checks); err != nil {
		return &ToolResult{Success: false, Content: fmt.Sprintf("Invalid input: %v", err), Error: err.Error()}, nil
	}
	if hasCheck(params.Checks, analysis.ContradictionsCheck) && !params.AllowModelCalls {
		return &ToolResult{Success: false, Content: "Invalid input: contradictions requires allow_model_calls=true", Error: "contradictions requires explicit opt-in"}, nil
	}

	allDocs, err := analysis.LoadDocuments(ctx, b.client, "full")
	if err != nil {
		return &ToolResult{Success: false, Content: fmt.Sprintf("Failed to load brain documents: %v", err), Error: err.Error()}, nil
	}
	docs, err := analysis.LoadDocuments(ctx, b.client, params.Scope)
	if err != nil {
		return &ToolResult{Success: false, Content: fmt.Sprintf("Failed to load brain documents: %v", err), Error: err.Error()}, nil
	}

	report := analysis.RunLint(docs, analysis.LintOptions{
		Scope:           params.Scope,
		Checks:          params.Checks,
		StaleAfter:      time.Duration(b.config.LintStaleDays) * 24 * time.Hour,
		OrphanAllowlist: b.config.LintOrphanAllowlist,
		Universe:        allDocs,
	})
	if slices.Contains(report.Checks, analysis.ContradictionsCheck) {
		findings, examinedPairs, err := b.detectContradictions(ctx, docs)
		if err != nil {
			return &ToolResult{Success: false, Content: fmt.Sprintf("Contradiction analysis failed: %v", err), Error: err.Error()}, nil
		}
		report.Findings.Contradictions = findings
		report.Summary.Contradictions = len(findings)
		report.Summary.ContradictionPairsExamined = examinedPairs
	}
	content := formatBrainLintReport(report)

	if b.config.LogBrainOperations {
		summaryParts := []string{
			fmt.Sprintf("%d orphan documents", report.Summary.Orphans),
			fmt.Sprintf("%d dead links", report.Summary.DeadLinks),
			fmt.Sprintf("%d stale references", report.Summary.StaleReferences),
		}
		if slices.Contains(report.Checks, "missing_pages") {
			summaryParts = append(summaryParts, fmt.Sprintf("%d missing pages", report.Summary.MissingPages))
		}
		if slices.Contains(report.Checks, analysis.ContradictionsCheck) {
			summaryParts = append(summaryParts, fmt.Sprintf("%d contradictions across %d examined pairs", report.Summary.Contradictions, report.Summary.ContradictionPairsExamined))
		}
		summary := "Found " + strings.Join(summaryParts, ", ") + "."
		if err := appendBrainLog(ctx, b.client, BrainLogEntry{
			Timestamp: time.Now().UTC(),
			Operation: "lint",
			Target:    params.Scope,
			Summary:   summary,
			Session:   sessionIDFromContext(ctx),
		}); err != nil {
			return &ToolResult{Success: false, Content: fmt.Sprintf("Lint completed but failed to append operation log: %v", err), Error: err.Error()}, nil
		}
	}

	return &ToolResult{Success: true, Content: content}, nil
}

func (b *BrainLint) detectContradictions(ctx context.Context, docs []analysis.Document) ([]analysis.ContradictionFinding, int, error) {
	if b.llm == nil {
		return nil, 0, fmt.Errorf("model-assisted contradictions check requires a configured provider")
	}
	candidates := analysis.FindContradictionCandidates(docs)
	examinedPairs := len(candidates)
	if len(candidates) == 0 {
		return nil, 0, nil
	}
	if len(candidates) > 8 {
		candidates = candidates[:8]
		examinedPairs = len(candidates)
	}

	response, err := b.llm.Complete(ctx, &provider.Request{
		Messages:  []provider.Message{provider.NewUserMessage(buildContradictionPrompt(candidates))},
		MaxTokens: 800,
		Purpose:   "brain_lint",
	})
	if err != nil {
		return nil, examinedPairs, err
	}
	findings, err := parseContradictionResponse(response)
	if err != nil {
		return nil, examinedPairs, err
	}
	return findings, examinedPairs, nil
}

type contradictionResponse struct {
	Contradictions []analysis.ContradictionFinding `json:"contradictions"`
}

var allowedContradictionConfidence = map[string]struct{}{
	"low":    {},
	"medium": {},
	"high":   {},
}

func parseContradictionResponse(response *provider.Response) ([]analysis.ContradictionFinding, error) {
	text := extractProviderText(response)
	if text == "" {
		return nil, fmt.Errorf("contradiction response contained no text")
	}
	var parsed contradictionResponse
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("parse contradiction response: %w", err)
	}
	unique := map[string]analysis.ContradictionFinding{}
	for _, finding := range parsed.Contradictions {
		left := strings.TrimSpace(finding.Left)
		right := strings.TrimSpace(finding.Right)
		summary := strings.TrimSpace(finding.Summary)
		confidence := strings.ToLower(strings.TrimSpace(finding.Confidence))
		if left == "" || right == "" || summary == "" {
			continue
		}
		if strings.Compare(left, right) > 0 {
			left, right = right, left
		}
		if _, ok := allowedContradictionConfidence[confidence]; !ok {
			confidence = ""
		}
		key := left + "\x00" + right + "\x00" + summary
		if existing, ok := unique[key]; ok {
			if contradictionConfidenceRank(confidence) > contradictionConfidenceRank(existing.Confidence) {
				existing.Confidence = confidence
				unique[key] = existing
			}
			continue
		}
		unique[key] = analysis.ContradictionFinding{Left: left, Right: right, Summary: summary, Confidence: confidence}
	}
	findings := make([]analysis.ContradictionFinding, 0, len(unique))
	for _, finding := range unique {
		findings = append(findings, finding)
	}
	slices.SortFunc(findings, func(a, b analysis.ContradictionFinding) int {
		if cmp := strings.Compare(a.Left, b.Left); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.Right, b.Right); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.Summary, b.Summary); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Confidence, b.Confidence)
	})
	return findings, nil
}

func contradictionConfidenceRank(confidence string) int {
	switch confidence {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func extractProviderText(response *provider.Response) string {
	if response == nil {
		return ""
	}
	parts := make([]string, 0, len(response.Content))
	for _, block := range response.Content {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, strings.TrimSpace(block.Text))
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func buildContradictionPrompt(candidates []analysis.ContradictionCandidate) string {
	var b strings.Builder
	b.WriteString("You are checking project brain notes for likely contradictions. Return JSON only with shape {\"contradictions\":[{\"left\":string,\"right\":string,\"summary\":string,\"confidence\":\"low|medium|high\"}]}. ")
	b.WriteString("Report only clear factual contradictions. Ignore tone differences, incomplete overlap, and ordinary evolution unless the statements directly conflict. If none, return {\"contradictions\":[]}.\n\n")
	for i, candidate := range candidates {
		fmt.Fprintf(&b, "Pair %d\n", i+1)
		fmt.Fprintf(&b, "left_path: %s\n", candidate.Left.Path)
		fmt.Fprintf(&b, "left_title: %s\n", candidate.Left.Title)
		fmt.Fprintf(&b, "left_tags: %s\n", strings.Join(candidate.Left.Tags, ", "))
		fmt.Fprintf(&b, "left_excerpt: %s\n", contradictionExcerpt(candidate.Left.Content))
		fmt.Fprintf(&b, "right_path: %s\n", candidate.Right.Path)
		fmt.Fprintf(&b, "right_title: %s\n", candidate.Right.Title)
		fmt.Fprintf(&b, "right_tags: %s\n", strings.Join(candidate.Right.Tags, ", "))
		fmt.Fprintf(&b, "right_excerpt: %s\n\n", contradictionExcerpt(candidate.Right.Content))
	}
	return b.String()
}

func contradictionExcerpt(content string) string {
	content = strings.TrimSpace(content)
	content = strings.Join(strings.Fields(content), " ")
	if len(content) > 500 {
		return content[:500]
	}
	return content
}

func hasCheck(checks []string, want string) bool {
	want = strings.TrimSpace(strings.ToLower(want))
	for _, check := range checks {
		if strings.TrimSpace(strings.ToLower(check)) == want {
			return true
		}
	}
	return false
}

func formatBrainLintReport(report analysis.LintReport) string {
	var lines []string
	lines = append(lines,
		fmt.Sprintf("Brain lint report (%s)", report.Scope),
		fmt.Sprintf("- documents: %d", report.Summary.Documents),
		fmt.Sprintf("- orphans: %d", report.Summary.Orphans),
		fmt.Sprintf("- dead links: %d", report.Summary.DeadLinks),
		fmt.Sprintf("- stale references: %d", report.Summary.StaleReferences),
	)
	if slices.Contains(report.Checks, "missing_pages") {
		lines = append(lines, fmt.Sprintf("- missing pages: %d", report.Summary.MissingPages))
	}
	if slices.Contains(report.Checks, analysis.ContradictionsCheck) {
		lines = append(lines,
			fmt.Sprintf("- contradictions: %d", report.Summary.Contradictions),
			fmt.Sprintf("- contradiction pairs examined: %d", report.Summary.ContradictionPairsExamined),
		)
	}
	lines = append(lines,
		fmt.Sprintf("- singleton tags: %d", report.Summary.SingletonTags),
		fmt.Sprintf("- similar tag pairs: %d", report.Summary.SimilarTagPairs),
		fmt.Sprintf("- untagged documents: %d", report.Summary.UntaggedDocuments),
	)
	if len(report.Findings.DeadLinks) > 0 {
		lines = append(lines, "", "Dead links:")
		for _, finding := range report.Findings.DeadLinks {
			lines = append(lines, fmt.Sprintf("- %s -> %s", finding.Source, finding.Target))
		}
	}
	if len(report.Findings.Orphans) > 0 {
		lines = append(lines, "", "Orphans:")
		for _, finding := range report.Findings.Orphans {
			lines = append(lines, fmt.Sprintf("- %s", finding.Path))
		}
	}
	if len(report.Findings.StaleReferences) > 0 {
		lines = append(lines, "", "Stale references:")
		for _, finding := range report.Findings.StaleReferences {
			lines = append(lines, fmt.Sprintf("- %s is older than %s", finding.Source, finding.Target))
		}
	}
	if len(report.Findings.MissingPages) > 0 {
		lines = append(lines, "", "Missing page suggestions:")
		for _, finding := range report.Findings.MissingPages {
			lines = append(lines, fmt.Sprintf("- %s (referenced %d times)", finding.Target, finding.Count))
		}
	}
	if len(report.Findings.Contradictions) > 0 {
		lines = append(lines, "", "Potential contradictions:")
		for _, finding := range report.Findings.Contradictions {
			line := fmt.Sprintf("- %s <> %s: %s", finding.Left, finding.Right, finding.Summary)
			if finding.Confidence != "" {
				line += fmt.Sprintf(" [%s]", finding.Confidence)
			}
			lines = append(lines, line)
		}
	}
	if len(report.Findings.TagHygiene.SingletonTags) > 0 {
		lines = append(lines, "", "Singleton tags:")
		for _, finding := range report.Findings.TagHygiene.SingletonTags {
			lines = append(lines, fmt.Sprintf("- #%s (%s)", finding.Tag, strings.Join(finding.Paths, ", ")))
		}
	}
	if len(report.Findings.TagHygiene.SimilarTagPairs) > 0 {
		lines = append(lines, "", "Similar tag pairs:")
		for _, finding := range report.Findings.TagHygiene.SimilarTagPairs {
			lines = append(lines, fmt.Sprintf("- #%s ~ #%s", finding.Left, finding.Right))
		}
	}
	if len(report.Findings.TagHygiene.UntaggedDocuments) > 0 {
		lines = append(lines, "", "Untagged documents:")
		for _, finding := range report.Findings.TagHygiene.UntaggedDocuments {
			lines = append(lines, fmt.Sprintf("- %s", finding.Path))
		}
	}
	return strings.Join(lines, "\n")
}
