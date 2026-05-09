package promptmeta

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const ReceiptSchemaV1 = "yard.receipt.v1"

type Metadata struct {
	RoleKey                    string   `yaml:"role_key"`
	Persona                    string   `yaml:"persona"`
	ExpectedTools              []string `yaml:"expected_tools"`
	ReceiptSchema              string   `yaml:"receipt_schema"`
	RecommendedMaxTurns        int      `yaml:"recommended_max_turns"`
	RequiresStructuredFindings bool     `yaml:"requires_structured_findings"`
}

type Parsed struct {
	Metadata       Metadata
	Body           string
	HasFrontmatter bool
	Warnings       []string
}

func Parse(content string) Parsed {
	frontmatter, body, ok, malformed := splitFrontmatter([]byte(content))
	if malformed {
		return Parsed{Body: content, Warnings: []string{"malformed prompt frontmatter"}}
	}
	if !ok {
		return Parsed{Body: content}
	}
	var metadata Metadata
	if err := yaml.Unmarshal(frontmatter, &metadata); err != nil {
		return Parsed{Body: content, HasFrontmatter: true, Warnings: []string{fmt.Sprintf("decode prompt frontmatter: %s", err)}}
	}
	metadata.normalize()
	return Parsed{Metadata: metadata, Body: string(body), HasFrontmatter: true}
}

func ValidateRole(roleKey string, configuredTools []string, metadata Metadata) []string {
	return ValidateRoleRuntime(roleKey, configuredTools, 0, metadata)
}

func ValidateRoleRuntime(roleKey string, configuredTools []string, configuredMaxTurns int, metadata Metadata) []string {
	roleKey = strings.TrimSpace(roleKey)
	var warnings []string
	if metadata.RoleKey != "" && metadata.RoleKey != roleKey {
		warnings = append(warnings, fmt.Sprintf("role_key %q differs from configured role %q", metadata.RoleKey, roleKey))
	}
	if metadata.ReceiptSchema != "" && metadata.ReceiptSchema != ReceiptSchemaV1 {
		warnings = append(warnings, fmt.Sprintf("receipt_schema %q is not %s", metadata.ReceiptSchema, ReceiptSchemaV1))
	}
	if len(metadata.ExpectedTools) > 0 {
		expected := compactSortedStrings(metadata.ExpectedTools)
		configured := compactSortedStrings(configuredTools)
		if !equalStrings(expected, configured) {
			warnings = append(warnings, fmt.Sprintf("expected_tools [%s] differ from configured tools [%s]", strings.Join(expected, ","), strings.Join(configured, ",")))
		}
	}
	if metadata.RecommendedMaxTurns > 0 && configuredMaxTurns > 0 && metadata.RecommendedMaxTurns != configuredMaxTurns {
		warnings = append(warnings, fmt.Sprintf("recommended_max_turns %d differs from configured max_turns %d", metadata.RecommendedMaxTurns, configuredMaxTurns))
	}
	return warnings
}

func (m *Metadata) normalize() {
	m.RoleKey = strings.TrimSpace(m.RoleKey)
	m.Persona = strings.TrimSpace(m.Persona)
	m.ExpectedTools = compactSortedStrings(m.ExpectedTools)
	m.ReceiptSchema = strings.TrimSpace(m.ReceiptSchema)
}

func splitFrontmatter(content []byte) ([]byte, []byte, bool, bool) {
	if bytes.HasPrefix(content, []byte("---\n")) {
		rest := content[len("---\n"):]
		if idx := bytes.Index(rest, []byte("\n---\n")); idx >= 0 {
			return rest[:idx], rest[idx+len("\n---\n"):], true, false
		}
		return nil, nil, false, true
	}
	if bytes.HasPrefix(content, []byte("---\r\n")) {
		rest := content[len("---\r\n"):]
		if idx := bytes.Index(rest, []byte("\r\n---\r\n")); idx >= 0 {
			return rest[:idx], rest[idx+len("\r\n---\r\n"):], true, false
		}
		if idx := bytes.Index(rest, []byte("\n---\n")); idx >= 0 {
			return rest[:idx], rest[idx+len("\n---\n"):], true, false
		}
		return nil, nil, false, true
	}
	return nil, nil, false, false
}

func compactSortedStrings(values []string) []string {
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

func equalStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
