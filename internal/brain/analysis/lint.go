package analysis

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/ponchione/sodoryard/internal/brain"
)

var (
	reservedDocumentKeys = map[string]struct{}{
		"_log":   {},
		"_index": {},
	}
	validChecks = map[string]struct{}{
		"orphans":           {},
		"dead_links":        {},
		"stale_references":  {},
		"missing_pages":     {},
		ContradictionsCheck: {},
		"tag_hygiene":       {},
	}
)

func LoadDocuments(ctx context.Context, backend brain.Backend, scope string) ([]Document, error) {
	selector, err := parseScopeSelector(scope)
	if err != nil {
		return nil, err
	}

	paths, err := backend.ListDocuments(ctx, "")
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)

	docs := make([]Document, 0, len(paths))
	for _, docPath := range paths {
		content, err := backend.ReadDocument(ctx, docPath)
		if err != nil {
			return nil, err
		}
		doc, err := ParseDocument(docPath, content)
		if err != nil {
			return nil, err
		}
		if selector.matches(doc) {
			docs = append(docs, doc)
		}
	}
	return docs, nil
}

func ValidateChecks(checks []string) error {
	for _, check := range checks {
		check = strings.ToLower(strings.TrimSpace(check))
		if check == "" {
			continue
		}
		if _, ok := validChecks[check]; !ok {
			return fmt.Errorf("unknown brain lint check: %s", check)
		}
	}
	return nil
}

type scopeSelector struct {
	all        bool
	pathPrefix string
	tag        string
}

func ValidateScope(scope string) error {
	_, err := parseScopeSelector(scope)
	return err
}

func parseScopeSelector(scope string) (scopeSelector, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" || scope == "full" {
		return scopeSelector{all: true}, nil
	}
	parts := strings.Split(scope, "+")
	if len(parts) > 2 {
		return scopeSelector{}, fmt.Errorf("invalid brain lint scope: %q", scope)
	}

	selector := scopeSelector{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return scopeSelector{}, fmt.Errorf("invalid brain lint scope: %q", scope)
		}
		if strings.HasPrefix(part, "#") {
			if selector.tag != "" {
				return scopeSelector{}, fmt.Errorf("invalid brain lint scope: %q", scope)
			}
			tag := normalizeTag(strings.TrimPrefix(part, "#"))
			if tag == "" {
				return scopeSelector{}, fmt.Errorf("invalid brain lint scope: %q", scope)
			}
			selector.tag = tag
			continue
		}
		if selector.pathPrefix != "" || strings.Contains(part, "#") {
			return scopeSelector{}, fmt.Errorf("invalid brain lint scope: %q", scope)
		}
		selector.pathPrefix = part
	}
	if selector.pathPrefix == "" && selector.tag == "" {
		return scopeSelector{}, fmt.Errorf("invalid brain lint scope: %q", scope)
	}
	return selector, nil
}

func (s scopeSelector) matches(doc Document) bool {
	if s.all {
		return true
	}
	if s.pathPrefix != "" && !strings.HasPrefix(doc.Path, s.pathPrefix) {
		return false
	}
	if s.tag == "" {
		return true
	}
	for _, existing := range doc.Tags {
		if existing == s.tag {
			return true
		}
	}
	return false
}

func RunLint(docs []Document, opts LintOptions) LintReport {
	checks := normalizedChecks(opts.Checks)
	universe := opts.Universe
	if len(universe) == 0 {
		universe = docs
	}

	report := LintReport{
		Scope:   defaultScope(opts.Scope),
		Checks:  checks,
		Summary: LintSummary{Documents: len(docs)},
	}

	for _, check := range checks {
		switch check {
		case "orphans":
			report.Findings.Orphans = findOrphans(docs, universe, opts.OrphanAllowlist)
			report.Summary.Orphans = len(report.Findings.Orphans)
		case "dead_links":
			report.Findings.DeadLinks = findDeadLinks(docs, universe)
			report.Summary.DeadLinks = len(report.Findings.DeadLinks)
		case "stale_references":
			report.Findings.StaleReferences = findStaleReferences(docs, universe, opts.StaleAfter)
			report.Summary.StaleReferences = len(report.Findings.StaleReferences)
		case "missing_pages":
			report.Findings.MissingPages = findMissingPages(docs, universe)
			report.Summary.MissingPages = len(report.Findings.MissingPages)
		case "tag_hygiene":
			report.Findings.TagHygiene = findTagHygiene(docs)
			report.Summary.SingletonTags = len(report.Findings.TagHygiene.SingletonTags)
			report.Summary.SimilarTagPairs = len(report.Findings.TagHygiene.SimilarTagPairs)
			report.Summary.UntaggedDocuments = len(report.Findings.TagHygiene.UntaggedDocuments)
		}
	}

	return report
}

func normalizedChecks(checks []string) []string {
	if len(checks) == 0 {
		return []string{"orphans", "dead_links", "stale_references", "tag_hygiene"}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(checks))
	for _, check := range checks {
		check = strings.ToLower(strings.TrimSpace(check))
		if check == "" {
			continue
		}
		if _, ok := seen[check]; ok {
			continue
		}
		seen[check] = struct{}{}
		out = append(out, check)
	}
	slices.Sort(out)
	return out
}

func defaultScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "full"
	}
	return scope
}

func findDeadLinks(docs []Document, universe []Document) []DeadLinkFinding {
	docKeys := map[string]struct{}{}
	for _, doc := range universe {
		docKeys[documentKey(doc.Path)] = struct{}{}
	}

	var findings []DeadLinkFinding
	for _, doc := range docs {
		for _, target := range doc.Wikilinks {
			if _, ok := docKeys[documentKey(target)]; ok {
				continue
			}
			findings = append(findings, DeadLinkFinding{Source: doc.Path, Target: target})
		}
	}
	slices.SortFunc(findings, func(a, b DeadLinkFinding) int {
		if cmp := strings.Compare(a.Source, b.Source); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Target, b.Target)
	})
	return findings
}

func findOrphans(docs []Document, universe []Document, allowlist []string) []OrphanFinding {
	inbound := map[string]int{}
	allowed := map[string]struct{}{}
	for _, item := range allowlist {
		allowed[documentKey(item)] = struct{}{}
	}

	for _, doc := range universe {
		for _, target := range doc.Wikilinks {
			inbound[documentKey(target)]++
		}
	}

	var findings []OrphanFinding
	for _, doc := range docs {
		key := documentKey(doc.Path)
		if _, ok := reservedDocumentKeys[key]; ok {
			continue
		}
		if _, ok := allowed[key]; ok {
			continue
		}
		if inbound[key] == 0 {
			findings = append(findings, OrphanFinding{Path: doc.Path})
		}
	}
	slices.SortFunc(findings, func(a, b OrphanFinding) int { return strings.Compare(a.Path, b.Path) })
	return findings
}

func findStaleReferences(docs []Document, universe []Document, staleAfter time.Duration) []StaleReferenceFinding {
	if staleAfter <= 0 {
		staleAfter = 90 * 24 * time.Hour
	}
	byKey := map[string]Document{}
	for _, doc := range universe {
		byKey[documentKey(doc.Path)] = doc
	}

	var findings []StaleReferenceFinding
	for _, doc := range docs {
		if !doc.HasUpdatedAt {
			continue
		}
		for _, target := range doc.Wikilinks {
			targetDoc, ok := byKey[documentKey(target)]
			if !ok || !targetDoc.HasUpdatedAt {
				continue
			}
			if !targetDoc.UpdatedAt.After(doc.UpdatedAt) {
				continue
			}
			delta := targetDoc.UpdatedAt.Sub(doc.UpdatedAt)
			if delta < staleAfter {
				continue
			}
			findings = append(findings, StaleReferenceFinding{
				Source:          doc.Path,
				Target:          targetDoc.Path,
				SourceUpdatedAt: doc.UpdatedAt,
				TargetUpdatedAt: targetDoc.UpdatedAt,
				AgeDelta:        delta,
			})
		}
	}
	slices.SortFunc(findings, func(a, b StaleReferenceFinding) int {
		if cmp := strings.Compare(a.Source, b.Source); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Target, b.Target)
	})
	return findings
}

type ContradictionCandidate struct {
	Left  Document
	Right Document
}

func FindContradictionCandidates(docs []Document) []ContradictionCandidate {
	candidates := make([]ContradictionCandidate, 0)
	for i := 0; i < len(docs); i++ {
		for j := i + 1; j < len(docs); j++ {
			if !documentsRelatedForContradictionCheck(docs[i], docs[j]) {
				continue
			}
			candidates = append(candidates, ContradictionCandidate{Left: docs[i], Right: docs[j]})
		}
	}
	slices.SortFunc(candidates, func(a, b ContradictionCandidate) int {
		if cmp := strings.Compare(a.Left.Path, b.Left.Path); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Right.Path, b.Right.Path)
	})
	return candidates
}

func documentsRelatedForContradictionCheck(left, right Document) bool {
	if shareNormalizedTags(left.Tags, right.Tags) {
		return true
	}
	if shareNormalizedTargets(left.Wikilinks, right.Wikilinks) {
		return true
	}
	if sharedTitleStem(left, right) {
		return true
	}
	leftKey := documentKey(left.Path)
	rightKey := documentKey(right.Path)
	for _, target := range left.Wikilinks {
		if documentKey(target) == rightKey {
			return true
		}
	}
	for _, target := range right.Wikilinks {
		if documentKey(target) == leftKey {
			return true
		}
	}
	return false
}

func shareNormalizedTags(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(left))
	for _, tag := range left {
		seen[tag] = struct{}{}
	}
	for _, tag := range right {
		if _, ok := seen[tag]; ok {
			return true
		}
	}
	return false
}

func shareNormalizedTargets(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(left))
	for _, target := range left {
		seen[documentKey(target)] = struct{}{}
	}
	for _, target := range right {
		if _, ok := seen[documentKey(target)]; ok {
			return true
		}
	}
	return false
}

func sharedTitleStem(left, right Document) bool {
	leftStem := contradictionStem(left)
	rightStem := contradictionStem(right)
	if leftStem == "" || rightStem == "" {
		return false
	}
	if leftStem == rightStem {
		return true
	}
	if strings.HasPrefix(leftStem, rightStem) || strings.HasPrefix(rightStem, leftStem) {
		return min(len(leftStem), len(rightStem)) >= 8
	}
	return false
}

func contradictionStem(doc Document) string {
	candidates := []string{doc.Title, path.Base(doc.Path), documentKey(doc.Path)}
	for _, candidate := range candidates {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		candidate = strings.TrimSuffix(candidate, ".md")
		candidate = strings.NewReplacer("-", " ", "_", " ", "/", " ").Replace(candidate)
		candidate = strings.Join(strings.Fields(candidate), " ")
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func findMissingPages(docs []Document, universe []Document) []MissingPageFinding {
	existing := map[string]struct{}{}
	totalRefs := map[string]int{}
	for _, doc := range universe {
		existing[documentKey(doc.Path)] = struct{}{}
		for _, target := range doc.Wikilinks {
			totalRefs[documentKey(target)]++
		}
	}

	targetsInScope := map[string]struct{}{}
	for _, doc := range docs {
		for _, target := range doc.Wikilinks {
			targetsInScope[documentKey(target)] = struct{}{}
		}
	}

	findings := make([]MissingPageFinding, 0, len(targetsInScope))
	for target := range targetsInScope {
		if _, ok := existing[target]; ok {
			continue
		}
		count := totalRefs[target]
		if count < 2 {
			continue
		}
		findings = append(findings, MissingPageFinding{Target: target, Count: count})
	}
	slices.SortFunc(findings, func(a, b MissingPageFinding) int {
		if a.Count != b.Count {
			if a.Count > b.Count {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Target, b.Target)
	})
	return findings
}

func findTagHygiene(docs []Document) TagHygieneFindings {
	tagDocs := map[string][]string{}
	var untagged []UntaggedDocumentFinding
	for _, doc := range docs {
		if len(doc.Tags) == 0 {
			untagged = append(untagged, UntaggedDocumentFinding{Path: doc.Path})
			continue
		}
		for _, tag := range doc.Tags {
			tagDocs[tag] = append(tagDocs[tag], doc.Path)
		}
	}

	var singleton []SingletonTagFinding
	var tags []string
	for tag, paths := range tagDocs {
		slices.Sort(paths)
		tagDocs[tag] = paths
		tags = append(tags, tag)
		if len(paths) == 1 {
			singleton = append(singleton, SingletonTagFinding{Tag: tag, Paths: append([]string(nil), paths...)})
		}
	}
	slices.Sort(tags)
	slices.SortFunc(singleton, func(a, b SingletonTagFinding) int { return strings.Compare(a.Tag, b.Tag) })
	slices.SortFunc(untagged, func(a, b UntaggedDocumentFinding) int { return strings.Compare(a.Path, b.Path) })

	var similar []SimilarTagPairFinding
	for i := 0; i < len(tags); i++ {
		for j := i + 1; j < len(tags); j++ {
			if tagsAreSimilar(tags[i], tags[j]) {
				similar = append(similar, SimilarTagPairFinding{Left: tags[i], Right: tags[j]})
			}
		}
	}

	return TagHygieneFindings{
		SingletonTags:     singleton,
		SimilarTagPairs:   similar,
		UntaggedDocuments: untagged,
	}
}

func tagsAreSimilar(left, right string) bool {
	if left == right {
		return false
	}
	if strings.HasPrefix(left, right) || strings.HasPrefix(right, left) {
		shorter := min(len(left), len(right))
		return shorter >= 4
	}
	if abs(len(left)-len(right)) > 3 {
		return false
	}
	return levenshtein(left, right) <= 2
}

func levenshtein(left, right string) int {
	if left == right {
		return 0
	}
	if left == "" {
		return len(right)
	}
	if right == "" {
		return len(left)
	}
	prev := make([]int, len(right)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(left); i++ {
		current := make([]int, len(right)+1)
		current[0] = i
		for j := 1; j <= len(right); j++ {
			cost := 0
			if left[i-1] != right[j-1] {
				cost = 1
			}
			current[j] = min(
				prev[j]+1,
				current[j-1]+1,
				prev[j-1]+cost,
			)
		}
		prev = current
	}
	return prev[len(right)]
}

func min(values ...int) int {
	best := values[0]
	for _, value := range values[1:] {
		if value < best {
			best = value
		}
	}
	return best
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func documentKey(docPath string) string {
	cleaned := strings.TrimSpace(strings.ReplaceAll(docPath, `\`, "/"))
	cleaned = path.Clean(cleaned)
	cleaned = strings.TrimSuffix(cleaned, ".md")
	return cleaned
}
