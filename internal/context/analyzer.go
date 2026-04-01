package context

import (
	"database/sql"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/ponchione/sirtopham/internal/db"
)

var (
	backtickSymbolPattern = regexp.MustCompile("`([A-Za-z_][A-Za-z0-9_]*)`")
	keywordSymbolPattern  = regexp.MustCompile(`(?i)\b(?:function|method|type|struct|interface|func)\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	identifierPattern     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	pathTokenPattern      = regexp.MustCompile(`^(?:\./)?[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*$`)
	fileTokenPattern      = regexp.MustCompile(`^(?:\./)?[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*\.[A-Za-z0-9]+$`)
	gitDepthPattern       = regexp.MustCompile(`(?i)\blast\s+(\d+)\s+commits?\b`)
)

var symbolStopwords = map[string]struct{}{
	"a":         {},
	"about":     {},
	"after":     {},
	"again":     {},
	"also":      {},
	"an":        {},
	"and":       {},
	"another":   {},
	"any":       {},
	"because":   {},
	"before":    {},
	"build":     {},
	"but":       {},
	"by":        {},
	"can":       {},
	"change":    {},
	"check":     {},
	"continue":  {},
	"create":    {},
	"delete":    {},
	"do":        {},
	"edit":      {},
	"false":     {},
	"finish":    {},
	"fix":       {},
	"for":       {},
	"from":      {},
	"going":     {},
	"hello":     {},
	"here":      {},
	"however":   {},
	"if":        {},
	"implement": {},
	"in":        {},
	"inspect":   {},
	"is":        {},
	"it":        {},
	"keep":      {},
	"make":      {},
	"modify":    {},
	"more":      {},
	"move":      {},
	"next":      {},
	"none":      {},
	"now":       {},
	"or":        {},
	"please":    {},
	"remove":    {},
	"rename":    {},
	"rewrite":   {},
	"so":        {},
	"still":     {},
	"that":      {},
	"the":       {},
	"then":      {},
	"there":     {},
	"these":     {},
	"this":      {},
	"those":     {},
	"too":       {},
	"true":      {},
	"update":    {},
	"what":      {},
	"when":      {},
	"where":     {},
	"which":     {},
	"write":     {},
	"you":       {},
}

var modificationVerbs = []string{
	"fix",
	"refactor",
	"change",
	"update",
	"edit",
	"modify",
	"rewrite",
	"rename",
	"move",
	"delete",
	"remove",
}

var creationVerbs = []string{
	"write",
	"create",
	"add",
	"implement",
	"build",
	"make",
}

var creationNouns = []string{
	"test",
	"endpoint",
	"handler",
	"middleware",
	"migration",
	"route",
	"model",
	"service",
}

var gitKeywordPattern = regexp.MustCompile(`(?i)\b(?:commit|commits|diff|pr|merge|branch)\b|pull request|recent changes|what changed|last push`)

var continuationPhrases = []string{
	"keep going",
	"continue",
	"finish",
	"next",
	"also",
	"too",
}

var questionPatterns = []string{
	"can you explain",
	"what does",
	"how does",
	"how do",
	"what is",
	"explain",
	"why",
}

var debuggingPatterns = []string{
	"error",
	"panic",
	"nil",
	"crash",
	"fail",
	"bug",
	"broken",
	"stack trace",
	"segfault",
	"exception",
}

// RuleBasedAnalyzer implements the v0.1 deterministic TurnAnalyzer.
//
// It performs regex- and heuristic-based signal extraction only; semantic query
// generation happens later in the Layer 3 pipeline.
type RuleBasedAnalyzer struct{}

// AnalyzeTurn converts the current user message plus recent persisted history
// into deterministic retrieval needs and an observability signal trace.
func (RuleBasedAnalyzer) AnalyzeTurn(message string, recentHistory []db.Message) *ContextNeeds {
	needs := &ContextNeeds{}

	for _, ref := range extractFileReferences(message) {
		if appendUnique(&needs.ExplicitFiles, ref.value) {
			needs.Signals = append(needs.Signals, Signal{Type: "file_ref", Source: ref.source, Value: ref.value})
		}
	}

	for _, ref := range extractSymbolReferences(message) {
		if appendUnique(&needs.ExplicitSymbols, ref.value) {
			needs.Signals = append(needs.Signals, Signal{Type: "symbol_ref", Source: ref.source, Value: ref.value})
		}
	}

	applyModificationIntent(message, needs)
	applyCreationIntent(message, needs)
	applyGitContext(message, needs)
	applyContinuation(message, recentHistory, needs)
	applyQuestionIntent(message, needs)
	applyDebuggingHints(message, needs)

	return needs
}

type extraction struct {
	source string
	value  string
}

func extractFileReferences(message string) []extraction {
	var refs []extraction
	seen := make(map[string]struct{})

	for _, token := range strings.Fields(message) {
		source := strings.Trim(token, "`\"'()[]{}.,!?;:")
		if source == "" {
			continue
		}

		value, ok := normalizePathToken(source)
		if !ok {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		refs = append(refs, extraction{source: source, value: value})
	}

	return refs
}

func normalizePathToken(token string) (string, bool) {
	candidate := strings.TrimSpace(token)
	candidate = strings.TrimSuffix(candidate, "/")
	candidate = strings.TrimPrefix(candidate, "../")
	candidate = strings.TrimPrefix(candidate, "../")
	if candidate == "" {
		return "", false
	}
	if !pathTokenPattern.MatchString(candidate) {
		return "", false
	}
	if strings.Contains(candidate, "/") {
		return candidate, true
	}
	if fileTokenPattern.MatchString(candidate) {
		return candidate, true
	}
	return "", false
}

func extractSymbolReferences(message string) []extraction {
	var refs []extraction
	seen := make(map[string]struct{})

	for _, matches := range backtickSymbolPattern.FindAllStringSubmatch(message, -1) {
		value := matches[1]
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		refs = append(refs, extraction{source: matches[0], value: value})
	}

	for _, matches := range keywordSymbolPattern.FindAllStringSubmatch(message, -1) {
		value := matches[1]
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		refs = append(refs, extraction{source: matches[0], value: value})
	}

	for _, token := range strings.Fields(message) {
		source := strings.Trim(token, "`\"'()[]{}.,!?;:")
		if source == "" || !identifierPattern.MatchString(source) || !looksLikeCodeIdentifier(source) {
			continue
		}
		if _, stopword := symbolStopwords[strings.ToLower(source)]; stopword {
			continue
		}
		if _, exists := seen[source]; exists {
			continue
		}
		seen[source] = struct{}{}
		refs = append(refs, extraction{source: source, value: source})
	}

	return refs
}

func looksLikeCodeIdentifier(token string) bool {
	if token == "" {
		return false
	}
	if strings.Contains(token, "/") || strings.Contains(token, ".") {
		return false
	}
	var hasUpper bool
	var hasLower bool
	for _, r := range token {
		if unicode.IsUpper(r) {
			hasUpper = true
		}
		if unicode.IsLower(r) {
			hasLower = true
		}
	}
	if !hasUpper {
		return false
	}
	if unicode.IsLower(rune(token[0])) && hasUpper {
		return true
	}
	return hasUpper && hasLower
}

func applyModificationIntent(message string, needs *ContextNeeds) {
	source, _ := findPhrase(message, modificationVerbs)
	if source == "" {
		return
	}

	target := ""
	if len(needs.ExplicitSymbols) > 0 {
		target = needs.ExplicitSymbols[0]
	} else if len(needs.ExplicitFiles) > 0 {
		target = needs.ExplicitFiles[0]
	}
	if target == "" {
		return
	}

	appendUnique(&needs.ExplicitSymbols, target)
	needs.Signals = append(needs.Signals, Signal{
		Type:   "modification_intent",
		Source: strings.TrimSpace(source + " " + target),
		Value:  target,
	})
}

func applyCreationIntent(message string, needs *ContextNeeds) {
	verbSource, _ := findPhrase(message, creationVerbs)
	if verbSource == "" {
		return
	}
	nounSource, noun := findPhrase(message, creationNouns)
	if nounSource == "" {
		return
	}

	needs.IncludeConventions = true
	needs.Signals = append(needs.Signals, Signal{
		Type:   "creation_intent",
		Source: strings.TrimSpace(verbSource + " " + nounSource),
		Value:  noun,
	})
}

func applyGitContext(message string, needs *ContextNeeds) {
	if matches := gitDepthPattern.FindStringSubmatch(message); len(matches) == 2 {
		depth, err := strconv.Atoi(matches[1])
		if err == nil {
			needs.IncludeGitContext = true
			needs.GitContextDepth = depth
			needs.Signals = append(needs.Signals, Signal{
				Type:   "git_context",
				Source: matches[0],
				Value:  matches[1],
			})
			return
		}
	}

	keyword := gitKeywordPattern.FindString(message)
	if keyword == "" {
		return
	}

	needs.IncludeGitContext = true
	needs.GitContextDepth = 5
	needs.Signals = append(needs.Signals, Signal{
		Type:   "git_context",
		Source: keyword,
		Value:  strconv.Itoa(needs.GitContextDepth),
	})
}

func applyContinuation(message string, recentHistory []db.Message, needs *ContextNeeds) {
	if len(needs.ExplicitFiles) > 0 || len(needs.ExplicitSymbols) > 0 {
		return
	}

	source, _ := findPhrase(message, continuationPhrases)
	if source == "" {
		return
	}

	needs.MomentumFiles = extractMomentumFiles(recentHistory)
	needs.MomentumModule = inferMomentumModule(needs.MomentumFiles)
	needs.Signals = append(needs.Signals, Signal{
		Type:   "continuation",
		Source: source,
		Value:  "momentum_applied",
	})
}

func applyQuestionIntent(message string, needs *ContextNeeds) {
	source, _ := findPhrase(message, questionPatterns)
	if source == "" {
		return
	}

	needs.IncludeConventions = true
	needs.Signals = append(needs.Signals, Signal{
		Type:   "question_intent",
		Source: source,
		Value:  "documentation_boost",
	})
}

func applyDebuggingHints(message string, needs *ContextNeeds) {
	source, _ := findPhrase(message, debuggingPatterns)
	if source == "" {
		return
	}

	needs.Signals = append(needs.Signals, Signal{
		Type:   "debugging_hints",
		Source: source,
		Value:  "error_handling_boost",
	})
}

func extractMomentumFiles(recentHistory []db.Message) []string {
	var files []string
	for _, message := range recentHistory {
		if !message.Content.Valid {
			continue
		}
		for _, ref := range extractFileReferences(message.Content.String) {
			appendUnique(&files, ref.value)
		}
	}
	return files
}

func inferMomentumModule(files []string) string {
	if len(files) == 0 {
		return ""
	}

	dirs := make([]string, 0, len(files))
	for _, file := range files {
		dir := path.Dir(file)
		if dir == "." {
			dir = file
		}
		dirs = append(dirs, dir)
	}
	if len(dirs) == 1 {
		return dirs[0]
	}

	common := strings.Split(dirs[0], "/")
	for _, dir := range dirs[1:] {
		parts := strings.Split(dir, "/")
		common = sharedPrefix(common, parts)
		if len(common) == 0 {
			return ""
		}
	}
	return strings.Join(common, "/")
}

func sharedPrefix(left []string, right []string) []string {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	var prefix []string
	for i := 0; i < limit; i++ {
		if left[i] != right[i] {
			break
		}
		prefix = append(prefix, left[i])
	}
	return prefix
}

func findPhrase(message string, phrases []string) (string, string) {
	lowerMessage := strings.ToLower(message)
	for _, phrase := range phrases {
		idx := strings.Index(lowerMessage, phrase)
		if idx >= 0 {
			return message[idx : idx+len(phrase)], phrase
		}
	}
	return "", ""
}

func appendUnique(values *[]string, value string) bool {
	for _, existing := range *values {
		if existing == value {
			return false
		}
	}
	*values = append(*values, value)
	return true
}

func contentString(content sql.NullString) string {
	if !content.Valid {
		return ""
	}
	return content.String
}
