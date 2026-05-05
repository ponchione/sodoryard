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
	receipt.RawBody = string(body)
	if err := receipt.validate(); err != nil {
		return Receipt{}, err
	}
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
