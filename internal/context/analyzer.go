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

var genericPathSegments = map[string]struct{}{
	"class":     {},
	"file":      {},
	"files":     {},
	"function":  {},
	"functions": {},
	"handler":   {},
	"line":      {},
	"lines":     {},
	"method":    {},
	"methods":   {},
	"model":     {},
	"models":    {},
	"module":    {},
	"modules":   {},
	"name":      {},
	"names":     {},
	"path":      {},
	"paths":     {},
	"provider":  {},
	"route":     {},
	"routes":    {},
	"symbol":    {},
	"symbols":   {},
	"type":      {},
	"types":     {},
}

var pathAnchorSegments = map[string]struct{}{
	"api":      {},
	"app":      {},
	"bin":      {},
	"cmd":      {},
	"config":   {},
	"configs":  {},
	"docs":     {},
	"examples": {},
	"internal": {},
	"lib":      {},
	"pkg":      {},
	"scripts":  {},
	"src":      {},
	"test":     {},
	"tests":    {},
	"testdata": {},
	"tools":    {},
	"web":      {},
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

var debuggingPatterns = []string {
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

var brainIntentPatterns = []string{
	"project brain",
	"brain note",
	"brain notes",
	"vault",
	"brain",
}

// brainSeekingRationalePatterns describes non-explicit prompt shapes that
// strongly imply the user wants project rationale / design decision history
// rather than fresh code lookup. The list is intentionally narrow: phrases
// must read as "asking about why we decided something" and not as a generic
// code-explanation question. The prior-debugging/history family is
// deliberately deferred to a later analyzer slice.
var brainSeekingRationalePatterns = []string{
	"design decision",
	"design choice",
	"design rationale",
	"rationale behind",
	"rationale for",
	"rationale was",
	"rationale is",
	"why did we",
	"why are we",
}

// brainSeekingConventionPatterns describes non-explicit prompt shapes that
// strongly imply the user is asking about how-we-usually-do-things — a team
// convention, policy, or established practice — rather than how a specific
// piece of code works. The list is intentionally narrow: bare "how do we"
// is excluded because it collides with code-explanation questions like
// "how do we parse this response". Each phrase must include the longer
// convention-shaped tail.
var brainSeekingConventionPatterns = []string{
	"how do we usually",
	"how do we normally",
	"what do we prefer",
	"what's our convention",
	"what is our convention",
	"our convention for",
	"our convention is",
	"our policy for",
	"our policy is",
	"what's our policy",
	"what is our policy",
}

// brainSeekingHistoryPatterns describes non-explicit prompt shapes that
// strongly imply the user is asking about prior debugging history — whether
// a bug has been seen before, how a past bug was fixed, what the workaround
// was, or what the root cause turned out to be. The list is intentionally
// narrow: bare "did we" is excluded because it collides with the rationale
// family and arbitrary past-tense questions, and bare "what was" is
// excluded because it collides with general history questions ("what was
// null here"). Every phrase must include the longer debug-history tail.
var brainSeekingHistoryPatterns = []string{
	"have we seen",
	"have we hit",
	"have we debugged",
	"have we fixed",
	"what was the fix",
	"what was the workaround",
	"what was the root cause",
	"did we ever fix",
	"did we already fix",
	"prior debugging",
	"past debugging",
	"previously debugged",
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

	acceptedFileRefs, rejectedFileRefs := extractFileReferences(message)
	for _, ref := range acceptedFileRefs {
		if appendUnique(&needs.ExplicitFiles, ref.value) {
			needs.Signals = append(needs.Signals, Signal{Type: "file_ref", Source: ref.source, Value: ref.value})
		}
	}
	for _, ref := range rejectedFileRefs {
		needs.Signals = append(needs.Signals, Signal{Type: "file_ref_rejected", Source: ref.source, Value: ref.value})
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
	applyBrainIntent(message, needs)
	applyBrainSeekingRationaleIntent(message, needs)
	applyBrainSeekingConventionIntent(message, needs)
	applyBrainSeekingHistoryIntent(message, needs)
	applyDebuggingHints(message, needs)

	return needs
}

type extraction struct {
	source string
	value  string
}

func extractFileReferences(message string) ([]extraction, []extraction) {
	var refs []extraction
	var rejected []extraction
	seen := make(map[string]struct{})
	rejectedSeen := make(map[string]struct{})

	for _, token := range strings.Fields(message) {
		source := strings.Trim(token, "`\"'()[]{}.,!?;:")
		if source == "" {
			continue
		}

		value, rejectionReason, ok := normalizePathToken(source)
		if !ok {
			if rejectionReason != "" {
				rejectionKey := source + "|" + rejectionReason
				if _, exists := rejectedSeen[rejectionKey]; !exists {
					rejectedSeen[rejectionKey] = struct{}{}
					rejected = append(rejected, extraction{source: source, value: rejectionReason})
				}
			}
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		refs = append(refs, extraction{source: source, value: value})
	}

	return refs, rejected
}

func normalizePathToken(token string) (string, string, bool) {
	candidate := strings.TrimSpace(token)
	candidate = strings.TrimSuffix(candidate, "/")
	candidate = strings.TrimPrefix(candidate, "../")
	candidate = strings.TrimPrefix(candidate, "../")
	if candidate == "" {
		return "", "", false
	}
	if !pathTokenPattern.MatchString(candidate) {
		return "", "", false
	}
	if isGenericSlashPair(candidate) {
		return "", "generic_slash_pair", false
	}
	if isUnanchoredMultiSegmentPath(candidate) {
		return "", "unanchored_multi_segment_path", false
	}
	if isVaultRootedNotePath(candidate) {
		return "", "vault_rooted_note_path", false
	}
	if strings.Contains(candidate, "/") {
		return candidate, "", true
	}
	if fileTokenPattern.MatchString(candidate) {
		return candidate, "", true
	}
	return "", "", false
}

func isGenericSlashPair(candidate string) bool {
	if strings.Contains(candidate, ".") {
		return false
	}
	parts := strings.Split(candidate, "/")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if _, ok := genericPathSegments[part]; !ok {
			return false
		}
	}
	return true
}

func isUnanchoredMultiSegmentPath(candidate string) bool {
	if strings.Contains(candidate, ".") || strings.HasPrefix(candidate, "./") {
		return false
	}
	parts := strings.Split(candidate, "/")
	if len(parts) < 3 {
		return false
	}
	for _, part := range parts {
		lower := strings.ToLower(strings.TrimSpace(part))
		if lower == "" {
			return true
		}
		if _, ok := pathAnchorSegments[lower]; ok {
			return false
		}
		if strings.ContainsAny(lower, "_-0123456789") {
			return false
		}
	}
	return true
}

func isVaultRootedNotePath(candidate string) bool {
	trimmed := strings.TrimPrefix(candidate, "./")
	if !strings.HasSuffix(strings.ToLower(trimmed), ".md") {
		return false
	}
	return strings.HasPrefix(trimmed, "notes/") || strings.HasPrefix(trimmed, ".brain/notes/")
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
	upperCount := 0
	for _, r := range token {
		if unicode.IsUpper(r) {
			hasUpper = true
			upperCount++
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
	if !hasLower {
		return false
	}
	return upperCount >= 2
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

func applyBrainIntent(message string, needs *ContextNeeds) {
	source, phrase := findPhrase(message, brainIntentPatterns)
	if source == "" {
		return
	}
	if phrase == "brain" && !containsStandaloneWord(message, phrase) {
		return
	}
	if phrase == "vault" && len(needs.ExplicitFiles) > 0 {
		return
	}

	needs.PreferBrainContext = true
	needs.Signals = append(needs.Signals, Signal{
		Type:   "brain_intent",
		Source: source,
		Value:  "prefer_brain_context",
	})
}

// applyBrainSeekingRationaleIntent flags turns whose natural-language shape
// reads as a request for project rationale / design-decision history. It
// prefers brain context for those turns so that the retrieval orchestrator
// skips generic code RAG when no explicit file or symbol references are
// present; when explicit refs ARE present, shouldRunSemanticSearch still keeps
// semantic search enabled so the turn stays code-capable.
//
// The phrase list is intentionally narrow to avoid hijacking generic code
// explanation questions (e.g. "why does ValidateToken return nil?" must NOT
// fire here — those belong to applyQuestionIntent). If applyBrainIntent has
// already fired, this function is a no-op so we do not emit duplicate signals
// for the same turn.
func applyBrainSeekingRationaleIntent(message string, needs *ContextNeeds) {
	if needs.PreferBrainContext {
		return
	}
	source, _ := findPhrase(message, brainSeekingRationalePatterns)
	if source == "" {
		return
	}

	needs.PreferBrainContext = true
	needs.Signals = append(needs.Signals, Signal{
		Type:   "brain_seeking_intent",
		Source: source,
		Value:  "rationale",
	})
}

// applyBrainSeekingConventionIntent flags turns whose natural-language shape
// reads as a request for a team convention, policy, or established practice.
// It prefers brain context for those turns so that the retrieval orchestrator
// skips generic code RAG when no explicit file or symbol references are
// present; when explicit refs ARE present, shouldRunSemanticSearch still keeps
// semantic search enabled so the turn stays code-capable.
//
// The phrase list is intentionally narrow to avoid hijacking generic code
// explanation questions (e.g. "how do we parse this response?" must NOT fire
// here — those belong to applyQuestionIntent). If applyBrainIntent or
// applyBrainSeekingRationaleIntent has already fired, this function is a
// no-op so we do not emit duplicate brain_seeking signals for the same turn.
func applyBrainSeekingConventionIntent(message string, needs *ContextNeeds) {
	if needs.PreferBrainContext {
		return
	}
	source, _ := findPhrase(message, brainSeekingConventionPatterns)
	if source == "" {
		return
	}

	needs.PreferBrainContext = true
	needs.Signals = append(needs.Signals, Signal{
		Type:   "brain_seeking_intent",
		Source: source,
		Value:  "convention",
	})
}

// applyBrainSeekingHistoryIntent flags turns whose natural-language shape
// reads as a request for prior debugging history — has this bug been seen
// before, how was it fixed, what was the workaround, what was the root
// cause. It prefers brain context for those turns so that the retrieval
// orchestrator skips generic code RAG when no explicit file or symbol
// references are present; when explicit refs ARE present,
// shouldRunSemanticSearch still keeps semantic search enabled so the turn
// stays code-capable.
//
// The phrase list is intentionally narrow to avoid hijacking generic
// past-tense or "what was" questions (e.g. "what was null here?" must NOT
// fire here — those belong to applyQuestionIntent or applyDebuggingHints).
// If applyBrainIntent or any earlier brain-seeking pass has already fired,
// this function is a no-op so we do not emit duplicate brain_seeking
// signals for the same turn.
func applyBrainSeekingHistoryIntent(message string, needs *ContextNeeds) {
	if needs.PreferBrainContext {
		return
	}
	source, _ := findPhrase(message, brainSeekingHistoryPatterns)
	if source == "" {
		return
	}

	needs.PreferBrainContext = true
	needs.Signals = append(needs.Signals, Signal{
		Type:   "brain_seeking_intent",
		Source: source,
		Value:  "history",
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
		acceptedRefs, _ := extractFileReferences(message.Content.String)
		for _, ref := range acceptedRefs {
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

	return commonPathPrefix(dirs)
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

func containsStandaloneWord(message string, word string) bool {
	for _, token := range strings.Fields(strings.ToLower(message)) {
		clean := strings.Trim(token, "`\"'()[]{}.,!?;:")
		if clean == word {
			return true
		}
	}
	return false
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
