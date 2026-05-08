package receipt

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	ErrMissingFrontmatter = errors.New("receipt: missing or malformed YAML frontmatter")
	ErrMissingField       = errors.New("receipt: missing required field")
	ErrInvalidField       = errors.New("receipt: invalid field")
	ErrInvalidVerdict     = errors.New("receipt: invalid verdict")
	ErrMissingSection     = errors.New("receipt: missing required section")
)

func Parse(content []byte) (Receipt, error) {
	frontmatter, body, ok := splitFrontmatter(content)
	if !ok {
		return Receipt{}, ErrMissingFrontmatter
	}

	var receipt Receipt
	if err := yaml.Unmarshal(frontmatter, &receipt); err != nil {
		return Receipt{}, fmt.Errorf("receipt: decode yaml: %w", err)
	}
	receipt.normalizeMetadataAliases()
	receipt.RawBody = string(body)
	if err := receipt.validate(); err != nil {
		return Receipt{}, err
	}
	receipt.SchemaWarnings = receipt.schemaMetadataWarnings()
	return receipt, nil
}

func RewriteUsageMetrics(content []byte, usage UsageMetrics) ([]byte, Receipt, bool, error) {
	frontmatter, body, ok := splitFrontmatter(content)
	if !ok {
		return nil, Receipt{}, false, ErrMissingFrontmatter
	}
	parsed, err := Parse(content)
	if err != nil {
		return nil, Receipt{}, false, err
	}
	changed := false
	if usage.TurnsUsed > 0 && parsed.TurnsUsed != usage.TurnsUsed {
		parsed.TurnsUsed = usage.TurnsUsed
		changed = true
	}
	if usage.TokensUsed > 0 && parsed.TokensUsed != usage.TokensUsed {
		parsed.TokensUsed = usage.TokensUsed
		changed = true
	}
	if usage.DurationSeconds > 0 && parsed.DurationSeconds != usage.DurationSeconds {
		parsed.DurationSeconds = usage.DurationSeconds
		changed = true
	}
	if !changed {
		return append([]byte(nil), content...), parsed, false, nil
	}
	frontmatter, err = rewriteUsageFrontmatter(frontmatter, parsed)
	if err != nil {
		return nil, Receipt{}, false, err
	}
	updated := bytes.NewBuffer(nil)
	updated.WriteString("---\n")
	updated.Write(frontmatter)
	updated.WriteString("---\n")
	updated.Write(body)
	reparsed, err := Parse(updated.Bytes())
	if err != nil {
		return nil, Receipt{}, false, err
	}
	return updated.Bytes(), reparsed, true, nil
}

func rewriteUsageFrontmatter(frontmatter []byte, usage Receipt) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(frontmatter, &root); err != nil {
		return nil, fmt.Errorf("receipt: decode yaml: %w", err)
	}
	node := &root
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		node = root.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil, ErrMissingFrontmatter
	}
	setYAMLScalar(node, "turns_used", strconv.Itoa(usage.TurnsUsed))
	setYAMLScalar(node, "tokens_used", strconv.Itoa(usage.TokensUsed))
	setYAMLScalar(node, "duration_seconds", strconv.Itoa(usage.DurationSeconds))
	if hasYAMLKey(node, "schema_version") || hasYAMLKey(node, "metrics") {
		setYAMLNestedScalar(node, "metrics", "turns", strconv.Itoa(usage.TurnsUsed))
		setYAMLNestedScalar(node, "metrics", "tokens", strconv.Itoa(usage.TokensUsed))
		setYAMLNestedScalar(node, "metrics", "duration_seconds", strconv.Itoa(usage.DurationSeconds))
	}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&root); err != nil {
		_ = encoder.Close()
		return nil, fmt.Errorf("receipt: encode yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("receipt: encode yaml: %w", err)
	}
	return out.Bytes(), nil
}

func hasYAMLKey(mapping *yaml.Node, key string) bool {
	return yamlMappingValue(mapping, key) != nil
}

func yamlMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func setYAMLScalar(mapping *yaml.Node, key string, value string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1].Kind = yaml.ScalarNode
			mapping.Content[i+1].Tag = "!!int"
			mapping.Content[i+1].Value = value
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: value},
	)
}

func setYAMLNestedScalar(mapping *yaml.Node, parentKey string, key string, value string) {
	parent := yamlMappingValue(mapping, parentKey)
	if parent == nil {
		parent = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: parentKey},
			parent,
		)
	}
	if parent.Kind != yaml.MappingNode {
		parent.Kind = yaml.MappingNode
		parent.Tag = "!!map"
		parent.Value = ""
		parent.Content = nil
	}
	setYAMLScalar(parent, key, value)
}

func (r *Receipt) normalizeMetadataAliases() {
	r.Agent = strings.TrimSpace(r.Agent)
	r.Role = strings.TrimSpace(r.Role)
	if r.Agent == "" && r.Role != "" {
		r.Agent = r.Role
	}
	if r.Role == "" && r.Agent != "" {
		r.Role = r.Agent
	}
	r.SchemaVersion = strings.TrimSpace(r.SchemaVersion)
	r.ChainID = strings.TrimSpace(r.ChainID)
	r.StepID = strings.TrimSpace(r.StepID)
	r.ChangedFiles = compactStrings(r.ChangedFiles)
	r.Followups = compactStrings(r.Followups)
	for i := range r.Findings {
		r.Findings[i].ID = strings.TrimSpace(r.Findings[i].ID)
		r.Findings[i].Status = normalizeFindingStatus(r.Findings[i].Status)
		r.Findings[i].Severity = strings.TrimSpace(r.Findings[i].Severity)
		r.Findings[i].Category = strings.TrimSpace(r.Findings[i].Category)
		r.Findings[i].File = strings.TrimSpace(r.Findings[i].File)
		r.Findings[i].Summary = strings.TrimSpace(r.Findings[i].Summary)
		r.Findings[i].Evidence = strings.TrimSpace(r.Findings[i].Evidence)
		r.Findings[i].Recommendation = strings.TrimSpace(r.Findings[i].Recommendation)
		r.Findings[i].RequiredFix = strings.TrimSpace(r.Findings[i].RequiredFix)
		r.Findings[i].Resolution = strings.TrimSpace(r.Findings[i].Resolution)
		r.Findings[i].AddressedByStep = strings.TrimSpace(r.Findings[i].AddressedByStep)
		r.Findings[i].FilesChanged = compactStrings(r.Findings[i].FilesChanged)
		r.Findings[i].Validation = compactStrings(r.Findings[i].Validation)
	}
	if r.Metrics.Turns > 0 && r.TurnsUsed == 0 {
		r.TurnsUsed = r.Metrics.Turns
	}
	if r.TokensUsed == 0 {
		if r.Metrics.Tokens > 0 {
			r.TokensUsed = r.Metrics.Tokens
		} else if r.Metrics.InputTokens > 0 || r.Metrics.OutputTokens > 0 {
			r.TokensUsed = r.Metrics.InputTokens + r.Metrics.OutputTokens
		}
	}
	if r.Metrics.DurationSeconds > 0 && r.DurationSeconds == 0 {
		r.DurationSeconds = r.Metrics.DurationSeconds
	}
}

func (r Receipt) schemaMetadataWarnings() []string {
	warnings := make([]string, 0)
	if r.SchemaVersion == "" {
		return []string{"missing schema_version"}
	}
	if r.SchemaVersion != SchemaVersion {
		warnings = append(warnings, fmt.Sprintf("unsupported schema_version %q", r.SchemaVersion))
	}
	if strings.TrimSpace(r.Role) == "" {
		warnings = append(warnings, "missing role")
	}
	if strings.TrimSpace(r.Agent) != "" && strings.TrimSpace(r.Role) != "" && r.Agent != r.Role {
		warnings = append(warnings, fmt.Sprintf("role %q differs from agent %q", r.Role, r.Agent))
	}
	if strings.TrimSpace(r.StepID) == "" {
		warnings = append(warnings, "missing step_id")
	}
	for _, warning := range structuredFindingWarnings(r.Findings) {
		warnings = append(warnings, warning)
	}
	return warnings
}

func structuredFindingWarnings(findings []Finding) []string {
	warnings := make([]string, 0)
	for i, finding := range findings {
		label := fmt.Sprintf("findings[%d]", i)
		if strings.TrimSpace(finding.ID) == "" {
			warnings = append(warnings, label+" missing id")
			continue
		}
		if !findingIDPattern.MatchString(finding.ID) {
			warnings = append(warnings, fmt.Sprintf("%s invalid id %q", label, finding.ID))
		}
		switch finding.Status {
		case "", "open", "closed", "addressed", "reopened", "invalid":
		default:
			warnings = append(warnings, fmt.Sprintf("%s invalid status %q", label, finding.Status))
		}
		if finding.Line < 0 {
			warnings = append(warnings, fmt.Sprintf("%s invalid line %d", label, finding.Line))
		}
	}
	return warnings
}

func (r Receipt) validate() error {
	if strings.TrimSpace(r.Agent) == "" {
		return fmt.Errorf("%w: agent", ErrMissingField)
	}
	if strings.TrimSpace(r.ChainID) == "" {
		return fmt.Errorf("%w: chain_id", ErrMissingField)
	}
	if r.Step <= 0 {
		return fmt.Errorf("%w: step (must be >= 1, got %d)", ErrInvalidField, r.Step)
	}
	if r.Verdict == "" {
		return fmt.Errorf("%w: verdict", ErrMissingField)
	}
	if !validVerdict(r.Verdict) {
		return fmt.Errorf("%w: %q", ErrInvalidVerdict, r.Verdict)
	}
	if r.Timestamp.IsZero() {
		return fmt.Errorf("%w: timestamp", ErrMissingField)
	}
	if r.TurnsUsed < 0 {
		return fmt.Errorf("%w: turns_used (must be >= 0, got %d)", ErrInvalidField, r.TurnsUsed)
	}
	if r.TokensUsed < 0 {
		return fmt.Errorf("%w: tokens_used (must be >= 0, got %d)", ErrInvalidField, r.TokensUsed)
	}
	if r.DurationSeconds < 0 {
		return fmt.Errorf("%w: duration_seconds (must be >= 0, got %d)", ErrInvalidField, r.DurationSeconds)
	}
	return nil
}

func (r Receipt) StructuredMetadataWarnings() []string {
	return append([]string(nil), r.SchemaWarnings...)
}

func compactStrings(values []string) []string {
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
	return out
}

func ValidateForStep(r Receipt, expected StepValidation) error {
	if strings.TrimSpace(expected.Agent) != "" && r.Agent != expected.Agent {
		return fmt.Errorf("%w: agent (got %q, want %q)", ErrInvalidField, r.Agent, expected.Agent)
	}
	if strings.TrimSpace(expected.ChainID) != "" && r.ChainID != expected.ChainID {
		return fmt.Errorf("%w: chain_id (got %q, want %q)", ErrInvalidField, r.ChainID, expected.ChainID)
	}
	if expected.Step > 0 && r.Step != expected.Step {
		return fmt.Errorf("%w: step (got %d, want %d)", ErrInvalidField, r.Step, expected.Step)
	}
	return nil
}

func RequiredSectionsForRole(role string) []string {
	return RequiredSectionsForStep(role, builtinSourceWritingReceiptRole(role))
}

func RequiredSectionsForStep(role string, sourceMutating bool) []string {
	sections := []string{"Summary", "Changes", "Validation", "Concerns", "Next Steps"}
	if sourceMutating {
		sections = insertSectionAfter(sections, "Changes", "Changed Files")
	}
	switch strings.TrimSpace(role) {
	case "correctness-auditor", "quality-auditor", "performance-auditor", "security-auditor", "integration-auditor":
		sections = append(sections, "Findings")
	case "resolver":
		sections = append(sections, "Findings Addressed")
	}
	return sections
}

func builtinSourceWritingReceiptRole(role string) bool {
	switch strings.TrimSpace(role) {
	case "coder", "resolver", "test-writer":
		return true
	default:
		return false
	}
}

func insertSectionAfter(sections []string, after string, section string) []string {
	for _, existing := range sections {
		if normalizeReceiptSection(existing) == normalizeReceiptSection(section) {
			return sections
		}
	}
	out := make([]string, 0, len(sections)+1)
	inserted := false
	for _, existing := range sections {
		out = append(out, existing)
		if !inserted && normalizeReceiptSection(existing) == normalizeReceiptSection(after) {
			out = append(out, section)
			inserted = true
		}
	}
	if !inserted {
		out = append(out, section)
	}
	return out
}

func ValidateRequiredSections(body string, required []string) error {
	present := receiptSections(body)
	missing := make([]string, 0)
	for _, section := range required {
		name := strings.TrimSpace(section)
		if name == "" {
			continue
		}
		if _, ok := present[normalizeReceiptSection(name)]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrMissingSection, strings.Join(missing, ", "))
	}
	return nil
}

func receiptSections(body string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
		if name != "" {
			out[normalizeReceiptSection(name)] = struct{}{}
		}
	}
	return out
}

func normalizeReceiptSection(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func validVerdict(verdict Verdict) bool {
	switch verdict {
	case VerdictCompleted, VerdictCompletedWithConcerns, VerdictCompletedNoReceipt, VerdictFixRequired, VerdictBlocked, VerdictEscalate, VerdictSafetyLimit:
		return true
	default:
		return false
	}
}

func splitFrontmatter(content []byte) ([]byte, []byte, bool) {
	if bytes.HasPrefix(content, []byte("---\n")) {
		rest := content[len("---\n"):]
		if idx := bytes.Index(rest, []byte("\n---\n")); idx >= 0 {
			return rest[:idx], rest[idx+len("\n---\n"):], true
		}
		return nil, nil, false
	}
	if bytes.HasPrefix(content, []byte("---\r\n")) {
		rest := content[len("---\r\n"):]
		if idx := bytes.Index(rest, []byte("\r\n---\r\n")); idx >= 0 {
			return rest[:idx], rest[idx+len("\r\n---\r\n"):], true
		}
		if idx := bytes.Index(rest, []byte("\n---\n")); idx >= 0 {
			return rest[:idx], rest[idx+len("\n---\n"):], true
		}
	}
	return nil, nil, false
}
