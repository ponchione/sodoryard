package receipt

import (
	"regexp"
	"strings"
)

var findingIDPattern = regexp.MustCompile(`^FIND-[A-Za-z0-9][A-Za-z0-9_-]*-\d{3,}$`)

type AuditFinding struct {
	ID          string
	SourceRole  string
	Severity    string
	Status      string
	Evidence    string
	Summary     string
	RequiredFix string
}

type FindingResolution struct {
	ID           string
	Resolution   string
	FilesChanged []string
	Validation   []string
}

func ParseAuditFindings(body string, sourceRole string) []AuditFinding {
	blocks := receiptHeadingBlocks(sectionContent(body, "Findings"))
	findings := make([]AuditFinding, 0, len(blocks))
	for _, block := range blocks {
		if !findingIDPattern.MatchString(block.ID) {
			continue
		}
		fields := blockFields(block.Lines)
		findings = append(findings, AuditFinding{
			ID:          block.ID,
			SourceRole:  strings.TrimSpace(sourceRole),
			Severity:    fields["severity"],
			Status:      normalizeFindingStatus(fields["status"]),
			Evidence:    fields["evidence"],
			Summary:     fields["summary"],
			RequiredFix: fields["required fix"],
		})
	}
	return findings
}

func ParseFindingResolutions(body string) []FindingResolution {
	blocks := receiptHeadingBlocks(sectionContent(body, "Findings Addressed"))
	resolutions := make([]FindingResolution, 0, len(blocks))
	for _, block := range blocks {
		if !findingIDPattern.MatchString(block.ID) {
			continue
		}
		fields := blockFields(block.Lines)
		resolutions = append(resolutions, FindingResolution{
			ID:           block.ID,
			Resolution:   strings.TrimSpace(fields["resolution"]),
			FilesChanged: blockList(block.Lines, "files changed"),
			Validation:   blockList(block.Lines, "validation"),
		})
	}
	return resolutions
}

type receiptHeadingBlock struct {
	ID    string
	Lines []string
}

func sectionContent(body string, section string) []string {
	section = normalizeReceiptSection(section)
	var lines []string
	inSection := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			inSection = normalizeReceiptSection(name) == section
			continue
		}
		if inSection {
			lines = append(lines, line)
		}
	}
	return lines
}

func receiptHeadingBlocks(lines []string) []receiptHeadingBlock {
	var blocks []receiptHeadingBlock
	var current *receiptHeadingBlock
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### ") {
			if current != nil {
				blocks = append(blocks, *current)
			}
			current = &receiptHeadingBlock{ID: strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))}
			continue
		}
		if current != nil {
			current.Lines = append(current.Lines, line)
		}
	}
	if current != nil {
		blocks = append(blocks, *current)
	}
	return blocks
}

func blockFields(lines []string) map[string]string {
	fields := map[string]string{}
	for _, line := range lines {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		key = normalizeReceiptSection(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			fields[key] = value
		}
	}
	return fields
}

func blockList(lines []string, label string) []string {
	label = normalizeReceiptSection(label)
	out := []string{}
	inList := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			inList = normalizeReceiptSection(strings.TrimSuffix(trimmed, ":")) == label
			continue
		}
		if !inList {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			if value != "" {
				out = append(out, value)
			}
			continue
		}
		if trimmed != "" {
			inList = false
		}
	}
	return out
}

func normalizeFindingStatus(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
	switch value {
	case "closed", "fixed", "resolved":
		return "closed"
	case "open", "reopened":
		return "open"
	default:
		return value
	}
}
