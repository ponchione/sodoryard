package context

import (
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/ponchione/sodoryard/internal/db"
)

func TestRuleBasedAnalyzerExtractsFileReferences(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("Check internal/auth/ and cmd/tidmouth/main.go", nil)

	requireStringsContain(t, needs.ExplicitFiles, "internal/auth")
	requireStringsContain(t, needs.ExplicitFiles, "cmd/tidmouth/main.go")
	requireSignal(t, needs.Signals, "file_ref", "internal/auth", "internal/auth")
	requireSignal(t, needs.Signals, "file_ref", "cmd/tidmouth/main.go", "cmd/tidmouth/main.go")
}

func TestRuleBasedAnalyzerExtractsSymbolReferences(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("Inspect `ValidateToken` and type AuthService", nil)

	requireStringsContain(t, needs.ExplicitSymbols, "ValidateToken")
	requireStringsContain(t, needs.ExplicitSymbols, "AuthService")
	requireSignal(t, needs.Signals, "symbol_ref", "`ValidateToken`", "ValidateToken")
	requireSignal(t, needs.Signals, "symbol_ref", "type AuthService", "AuthService")
}

func TestRuleBasedAnalyzerDetectsModificationIntent(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("Fix `ValidateToken` in internal/auth/service.go", nil)

	requireStringsContain(t, needs.ExplicitFiles, "internal/auth/service.go")
	requireStringsContain(t, needs.ExplicitSymbols, "ValidateToken")
	requireSignal(t, needs.Signals, "modification_intent", "Fix", "ValidateToken")
}

func TestRuleBasedAnalyzerDetectsCreationIntent(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("Create a test for the auth handler", nil)

	if !needs.IncludeConventions {
		t.Fatal("IncludeConventions = false, want true")
	}
	requireSignal(t, needs.Signals, "creation_intent", "Create", "test")
}

func TestRuleBasedAnalyzerDetectsGitContextDepth(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("What changed in the last 3 commits?", nil)

	if !needs.IncludeGitContext {
		t.Fatal("IncludeGitContext = false, want true")
	}
	if needs.GitContextDepth != 3 {
		t.Fatalf("GitContextDepth = %d, want 3", needs.GitContextDepth)
	}
	requireSignal(t, needs.Signals, "git_context", "last 3 commits", "3")
}

func TestRuleBasedAnalyzerDetectsContinuationAndMomentumFromHistory(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}
	recentHistory := []db.Message{
		historyMessage("We were working in internal/context/types.go and internal/context/interfaces.go."),
	}

	needs := analyzer.AnalyzeTurn("Keep going", recentHistory)

	requireStringsContain(t, needs.MomentumFiles, "internal/context/types.go")
	requireStringsContain(t, needs.MomentumFiles, "internal/context/interfaces.go")
	if needs.MomentumModule != "internal/context" {
		t.Fatalf("MomentumModule = %q, want internal/context", needs.MomentumModule)
	}
	requireSignal(t, needs.Signals, "continuation", "Keep going", "momentum_applied")
}

func TestRuleBasedAnalyzerReturnsEmptyNeedsForUnrecognizedMessage(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("hello there", nil)

	if needs == nil {
		t.Fatal("AnalyzeTurn returned nil needs")
	}
	if len(needs.ExplicitFiles) != 0 {
		t.Fatalf("len(ExplicitFiles) = %d, want 0", len(needs.ExplicitFiles))
	}
	if len(needs.ExplicitSymbols) != 0 {
		t.Fatalf("len(ExplicitSymbols) = %d, want 0", len(needs.ExplicitSymbols))
	}
	if len(needs.MomentumFiles) != 0 {
		t.Fatalf("len(MomentumFiles) = %d, want 0", len(needs.MomentumFiles))
	}
	if len(needs.Signals) != 0 {
		t.Fatalf("len(Signals) = %d, want 0", len(needs.Signals))
	}
	if needs.IncludeConventions {
		t.Fatal("IncludeConventions = true, want false")
	}
	if needs.IncludeGitContext {
		t.Fatal("IncludeGitContext = true, want false")
	}
	if len(needs.SemanticQueries) != 0 {
		t.Fatalf("len(SemanticQueries) = %d, want 0", len(needs.SemanticQueries))
	}
}

func TestRuleBasedAnalyzerFiltersPascalCaseStopwords(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("However Because Then", nil)

	if len(needs.ExplicitSymbols) != 0 {
		t.Fatalf("ExplicitSymbols = %v, want none", needs.ExplicitSymbols)
	}
	if len(needs.Signals) != 0 {
		t.Fatalf("Signals = %v, want none", needs.Signals)
	}
}

func TestQuestionIntentSignal(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("why does BuildPrompt return nil?", nil)

	requireSignal(t, needs.Signals, "question_intent", "why", "documentation_boost")
}

func TestQuestionIntentNoMatch(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("fix the router", nil)

	for _, signal := range needs.Signals {
		if signal.Type == "question_intent" {
			t.Fatalf("unexpected question_intent signal: %v", signal)
		}
	}
}

func TestDebuggingHintsSignal(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("there's a panic in the handler", nil)

	requireSignal(t, needs.Signals, "debugging_hints", "panic", "error_handling_boost")
}

func TestDebuggingHintsNoMatch(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("add a new endpoint", nil)

	for _, signal := range needs.Signals {
		if signal.Type == "debugging_hints" {
			t.Fatalf("unexpected debugging_hints signal: %v", signal)
		}
	}
}

func TestQuestionIntentSetsConventions(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("explain the auth flow", nil)

	if !needs.IncludeConventions {
		t.Fatal("IncludeConventions = false, want true")
	}
	requireSignal(t, needs.Signals, "question_intent", "explain", "documentation_boost")
}

func TestBrainIntentPrefersBrainContext(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("What is the runtime brain proof canary phrase?", nil)

	if !needs.PreferBrainContext {
		t.Fatal("PreferBrainContext = false, want true")
	}
	requireSignal(t, needs.Signals, "brain_intent", "brain", "prefer_brain_context")
}

func TestBrainSeekingRationaleIntentPrefersBrainContext(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	cases := []struct {
		name   string
		prompt string
		source string
	}{
		{
			name:   "rationale_behind",
			prompt: "What's the rationale behind picking lancedb for the vectorstore?",
			source: "rationale behind",
		},
		{
			name:   "why_did_we",
			prompt: "Why did we decide to embed the React build into the Go binary?",
			source: "Why did we",
		},
		{
			name:   "design_decision",
			prompt: "Walk me through the design decision that led to proactive context retrieval.",
			source: "design decision",
		},
		{
			name:   "why_are_we",
			prompt: "Why are we gating semantic search behind PreferBrainContext?",
			source: "Why are we",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			needs := analyzer.AnalyzeTurn(tc.prompt, nil)
			if !needs.PreferBrainContext {
				t.Fatalf("PreferBrainContext = false for %q, want true", tc.prompt)
			}
			requireSignal(t, needs.Signals, "brain_seeking_intent", tc.source, "rationale")
		})
	}
}

func TestBrainSeekingRationaleIntentIgnoresCodeStyleWhyQuestions(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	cases := []string{
		"why does BuildPrompt return nil?",
		"explain how the auth flow works",
		"fix the router",
	}

	for _, prompt := range cases {
		needs := analyzer.AnalyzeTurn(prompt, nil)
		if needs.PreferBrainContext {
			t.Fatalf("PreferBrainContext = true for %q, want false", prompt)
		}
		for _, signal := range needs.Signals {
			if signal.Type == "brain_seeking_intent" {
				t.Fatalf("unexpected brain_seeking_intent signal for %q: %+v", prompt, signal)
			}
		}
	}
}

func TestBrainSeekingRationaleIntentKeepsExplicitRefsCodeCapable(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn(
		"What's the rationale for the retrieval preference gate in internal/context/retrieval.go?",
		nil,
	)

	if !needs.PreferBrainContext {
		t.Fatal("PreferBrainContext = false, want true when rationale phrase is present")
	}
	requireStringsContain(t, needs.ExplicitFiles, "internal/context/retrieval.go")
	requireSignal(t, needs.Signals, "brain_seeking_intent", "rationale for", "rationale")
	requireSignal(t, needs.Signals, "file_ref", "internal/context/retrieval.go", "internal/context/retrieval.go")
}

func TestBrainSeekingRationaleIntentDoesNotDuplicateBrainIntentSignal(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn(
		"What's the rationale in the project brain for our retrieval design?",
		nil,
	)

	if !needs.PreferBrainContext {
		t.Fatal("PreferBrainContext = false, want true")
	}
	// Explicit brain_intent should win; brain_seeking_intent must stay silent
	// so the observability signal stream is not double-counted.
	requireSignal(t, needs.Signals, "brain_intent", "project brain", "prefer_brain_context")
	for _, signal := range needs.Signals {
		if signal.Type == "brain_seeking_intent" {
			t.Fatalf("unexpected brain_seeking_intent signal alongside brain_intent: %+v", signal)
		}
	}
}

func TestBrainSeekingLayoutIntentPrefersBrainContext(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn(
		"From our layout graph notes, what linked layout canary phrase sits behind SATURN RAIL?",
		nil,
	)

	if !needs.PreferBrainContext {
		t.Fatal("PreferBrainContext = false, want true for layout graph note prompt")
	}
	requireSignal(t, needs.Signals, "brain_seeking_intent", "layout graph notes", "layout")
}

func TestBrainSeekingLayoutIntentIgnoresGenericLayoutCodeQuestions(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	cases := []string{
		"How does the sidebar layout work in src/components/SideNav.tsx?",
		"Explain the layout transition code in web/src/main.tsx.",
	}

	for _, prompt := range cases {
		needs := analyzer.AnalyzeTurn(prompt, nil)
		if needs.PreferBrainContext {
			t.Fatalf("PreferBrainContext = true for %q, want false", prompt)
		}
		for _, signal := range needs.Signals {
			if signal.Type == "brain_seeking_intent" && signal.Value == "layout" {
				t.Fatalf("unexpected layout brain_seeking_intent for %q: %+v", prompt, signal)
			}
		}
	}
}

func TestBrainSeekingConventionIntentPrefersBrainContext(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	cases := []struct {
		name   string
		prompt string
		source string
	}{
		{
			name:   "how_do_we_usually",
			prompt: "How do we usually structure new analyzer slices around here?",
			source: "How do we usually",
		},
		{
			name:   "how_do_we_normally",
			prompt: "How do we normally name retrieval helper functions?",
			source: "How do we normally",
		},
		{
			name:   "what_do_we_prefer",
			prompt: "What do we prefer when picking between sibling passes versus value tags?",
			source: "What do we prefer",
		},
		{
			name:   "whats_our_convention",
			prompt: "What's our convention for naming context analyzer pattern lists?",
			source: "What's our convention",
		},
		{
			name:   "our_policy_for",
			prompt: "Remind me about our policy for adding new analyzer signals.",
			source: "our policy for",
		},
		{
			name:   "what_is_our_policy",
			prompt: "What is our policy on landing speculative retrieval changes?",
			source: "What is our policy",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			needs := analyzer.AnalyzeTurn(tc.prompt, nil)
			if !needs.PreferBrainContext {
				t.Fatalf("PreferBrainContext = false for %q, want true", tc.prompt)
			}
			requireSignal(t, needs.Signals, "brain_seeking_intent", tc.source, "convention")
		})
	}
}

func TestBrainSeekingConventionIntentIgnoresCodeStyleQuestions(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	cases := []string{
		"how do we parse this response from the provider?",
		"how do we know if the websocket session is closed?",
		"explain how the auth flow works",
		"fix the parser",
	}

	for _, prompt := range cases {
		needs := analyzer.AnalyzeTurn(prompt, nil)
		if needs.PreferBrainContext {
			t.Fatalf("PreferBrainContext = true for %q, want false", prompt)
		}
		for _, signal := range needs.Signals {
			if signal.Type == "brain_seeking_intent" {
				t.Fatalf("unexpected brain_seeking_intent signal for %q: %+v", prompt, signal)
			}
		}
	}
}

func TestBrainSeekingConventionIntentKeepsExplicitRefsCodeCapable(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn(
		"What's our convention for adding new pattern lists in internal/context/analyzer.go?",
		nil,
	)

	if !needs.PreferBrainContext {
		t.Fatal("PreferBrainContext = false, want true when convention phrase is present")
	}
	requireStringsContain(t, needs.ExplicitFiles, "internal/context/analyzer.go")
	requireSignal(t, needs.Signals, "brain_seeking_intent", "What's our convention", "convention")
	requireSignal(t, needs.Signals, "file_ref", "internal/context/analyzer.go", "internal/context/analyzer.go")
}

func TestBrainSeekingHistoryIntentPrefersBrainContext(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	cases := []struct {
		name   string
		prompt string
		source string
	}{
		{
			name:   "have_we_seen",
			prompt: "Have we seen a vite rebuild loop before in this project?",
			source: "Have we seen",
		},
		{
			name:   "have_we_hit",
			prompt: "Have we hit a websocket reconnect storm like this in the past?",
			source: "Have we hit",
		},
		{
			name:   "what_was_the_fix",
			prompt: "What was the fix for the embedding dimension mismatch issue?",
			source: "What was the fix",
		},
		{
			name:   "what_was_the_workaround",
			prompt: "Tell me what was the workaround for the lancedb cgo linker error.",
			source: "what was the workaround",
		},
		{
			name:   "what_was_the_root_cause",
			prompt: "What was the root cause of the proactive context retrieval regression last month?",
			source: "What was the root cause",
		},
		{
			name:   "did_we_ever_fix",
			prompt: "Did we ever fix the duplicate-conversation-on-reload sidebar bug?",
			source: "Did we ever fix",
		},
		{
			name:   "prior_debugging",
			prompt: "Surface any prior debugging on the analyzer signal trace.",
			source: "prior debugging",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			needs := analyzer.AnalyzeTurn(tc.prompt, nil)
			if !needs.PreferBrainContext {
				t.Fatalf("PreferBrainContext = false for %q, want true", tc.prompt)
			}
			requireSignal(t, needs.Signals, "brain_seeking_intent", tc.source, "history")
		})
	}
}

func TestBrainSeekingHistoryIntentIgnoresGenericPastTenseQuestions(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	cases := []string{
		"what was null here when the parser ran?",
		"did we break the websocket handler in the last refactor?",
		"explain how the auth flow works",
		"fix the parser",
		"what was the response body from the provider?",
	}

	for _, prompt := range cases {
		needs := analyzer.AnalyzeTurn(prompt, nil)
		if needs.PreferBrainContext {
			t.Fatalf("PreferBrainContext = true for %q, want false", prompt)
		}
		for _, signal := range needs.Signals {
			if signal.Type == "brain_seeking_intent" {
				t.Fatalf("unexpected brain_seeking_intent signal for %q: %+v", prompt, signal)
			}
		}
	}
}

func TestBrainSeekingHistoryIntentKeepsExplicitRefsCodeCapable(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn(
		"What was the root cause of the panic surfaced from internal/server/websocket.go?",
		nil,
	)

	if !needs.PreferBrainContext {
		t.Fatal("PreferBrainContext = false, want true when history phrase is present")
	}
	requireStringsContain(t, needs.ExplicitFiles, "internal/server/websocket.go")
	requireSignal(t, needs.Signals, "brain_seeking_intent", "What was the root cause", "history")
	requireSignal(t, needs.Signals, "file_ref", "internal/server/websocket.go", "internal/server/websocket.go")
}

func TestBrainSeekingHistoryIntentDoesNotDuplicateEarlierFamilies(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	// Explicit brain_intent should win and history must stay silent.
	needsBrain := analyzer.AnalyzeTurn(
		"Have we seen this issue documented in the project brain?",
		nil,
	)
	if !needsBrain.PreferBrainContext {
		t.Fatal("PreferBrainContext = false for brain phrase, want true")
	}
	requireSignal(t, needsBrain.Signals, "brain_intent", "project brain", "prefer_brain_context")
	for _, signal := range needsBrain.Signals {
		if signal.Type == "brain_seeking_intent" {
			t.Fatalf("unexpected brain_seeking_intent signal alongside brain_intent: %+v", signal)
		}
	}

	// Rationale family should win when both rationale and history phrases
	// appear in the same turn so a single turn never emits two
	// brain_seeking_intent signals.
	needsRationale := analyzer.AnalyzeTurn(
		"What was the rationale behind the fix? What was the fix anyway?",
		nil,
	)
	if !needsRationale.PreferBrainContext {
		t.Fatal("PreferBrainContext = false for rationale+history phrase, want true")
	}
	requireSignal(t, needsRationale.Signals, "brain_seeking_intent", "rationale behind", "rationale")
	historySignals := 0
	for _, signal := range needsRationale.Signals {
		if signal.Type == "brain_seeking_intent" && signal.Value == "history" {
			historySignals++
		}
	}
	if historySignals != 0 {
		t.Fatalf("expected 0 history brain_seeking signals, got %d in %+v", historySignals, needsRationale.Signals)
	}

	// Convention family should win when both convention and history phrases
	// appear in the same turn.
	needsConvention := analyzer.AnalyzeTurn(
		"What's our convention for documenting what was the fix in past debugging notes?",
		nil,
	)
	if !needsConvention.PreferBrainContext {
		t.Fatal("PreferBrainContext = false for convention+history phrase, want true")
	}
	requireSignal(t, needsConvention.Signals, "brain_seeking_intent", "What's our convention", "convention")
	historySignals = 0
	for _, signal := range needsConvention.Signals {
		if signal.Type == "brain_seeking_intent" && signal.Value == "history" {
			historySignals++
		}
	}
	if historySignals != 0 {
		t.Fatalf("expected 0 history brain_seeking signals alongside convention, got %d in %+v", historySignals, needsConvention.Signals)
	}
}

func TestBrainSeekingConventionIntentDoesNotDuplicateBrainOrRationaleSignal(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	// Explicit brain_intent should win and convention must stay silent.
	needsBrain := analyzer.AnalyzeTurn(
		"What's our convention for note layout in the project brain?",
		nil,
	)
	if !needsBrain.PreferBrainContext {
		t.Fatal("PreferBrainContext = false for brain phrase, want true")
	}
	requireSignal(t, needsBrain.Signals, "brain_intent", "project brain", "prefer_brain_context")
	for _, signal := range needsBrain.Signals {
		if signal.Type == "brain_seeking_intent" {
			t.Fatalf("unexpected brain_seeking_intent signal alongside brain_intent: %+v", signal)
		}
	}

	// Rationale family should win and convention must stay silent so a
	// single turn never emits two brain_seeking_intent signals.
	needsRationale := analyzer.AnalyzeTurn(
		"What's the rationale behind our policy for landing speculative retrieval changes?",
		nil,
	)
	if !needsRationale.PreferBrainContext {
		t.Fatal("PreferBrainContext = false for rationale+convention phrase, want true")
	}
	requireSignal(t, needsRationale.Signals, "brain_seeking_intent", "rationale behind", "rationale")
	conventionSignals := 0
	for _, signal := range needsRationale.Signals {
		if signal.Type == "brain_seeking_intent" && signal.Value == "convention" {
			conventionSignals++
		}
	}
	if conventionSignals != 0 {
		t.Fatalf("expected 0 convention brain_seeking signals, got %d in %+v", conventionSignals, needsRationale.Signals)
	}
}

func TestRuleBasedAnalyzerIgnoresGenericSlashPairs(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("Investigate how websocket submissions choose provider/model and cite relevant file/function names", nil)

	if len(needs.ExplicitFiles) != 0 {
		t.Fatalf("ExplicitFiles = %v, want none for generic slash pairs", needs.ExplicitFiles)
	}
	for _, signal := range needs.Signals {
		if signal.Type == "file_ref" {
			t.Fatalf("unexpected file_ref signal: %+v", signal)
		}
	}
}

func TestRuleBasedAnalyzerRejectsSlashDelimitedProseButKeepsRealPaths(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("Investigate search/title/runtime helpers and compare with internal/server/websocket.go", nil)

	if slices.Contains(needs.ExplicitFiles, "search/title/runtime") {
		t.Fatalf("ExplicitFiles = %v, unexpected prose path candidate", needs.ExplicitFiles)
	}
	requireStringsContain(t, needs.ExplicitFiles, "internal/server/websocket.go")
	requireSignal(t, needs.Signals, "file_ref_rejected", "search/title/runtime", "unanchored_multi_segment_path")
	requireSignal(t, needs.Signals, "file_ref", "internal/server/websocket.go", "internal/server/websocket.go")
}

func TestRuleBasedAnalyzerIgnoresSentenceCapitalizationAsSymbolReference(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("Investigate search/title/runtime helpers and compare with internal/server/websocket.go. Treat the slash phrase as prose.", nil)

	if slices.Contains(needs.ExplicitSymbols, "Investigate") || slices.Contains(needs.ExplicitSymbols, "Treat") {
		t.Fatalf("ExplicitSymbols = %v, unexpected sentence-capitalized words", needs.ExplicitSymbols)
	}
	for _, signal := range needs.Signals {
		if signal.Type == "symbol_ref" && (signal.Value == "Investigate" || signal.Value == "Treat") {
			t.Fatalf("unexpected symbol_ref signal: %+v", signal)
		}
	}
}

func TestRuleBasedAnalyzerRejectsVaultRootedNotePathsAsExplicitFiles(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("Confirm the brain note notes/runtime/soak-token-1775392724511125308.md matches the search hit and compare with internal/server/websocket.go", nil)

	if slices.Contains(needs.ExplicitFiles, "notes/runtime/soak-token-1775392724511125308.md") {
		t.Fatalf("ExplicitFiles = %v, unexpected vault-rooted note path", needs.ExplicitFiles)
	}
	requireStringsContain(t, needs.ExplicitFiles, "internal/server/websocket.go")
	requireSignal(t, needs.Signals, "file_ref_rejected", "notes/runtime/soak-token-1775392724511125308.md", "vault_rooted_note_path")
}

func historyMessage(content string) db.Message {
	return db.Message{
		Content: sql.NullString{String: content, Valid: true},
	}
}

func TestRuleBasedAnalyzerRejectsLowConfidenceSlashProse(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}
	needs := analyzer.AnalyzeTurn("Investigate add/update and receipts/state before continuing", nil)

	if slices.Contains(needs.ExplicitFiles, "add/update") {
		t.Fatalf("ExplicitFiles = %v, want add/update rejected", needs.ExplicitFiles)
	}
	if slices.Contains(needs.ExplicitFiles, "receipts/state") {
		t.Fatalf("ExplicitFiles = %v, want receipts/state rejected", needs.ExplicitFiles)
	}
	requireSignal(t, needs.Signals, "file_ref_rejected", "add/update", "low_confidence_slash_path")
	requireSignal(t, needs.Signals, "file_ref_rejected", "receipts/state", "low_confidence_slash_path")
}

func TestRuleBasedAnalyzerKeepsAnchoredRepoPaths(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}
	needs := analyzer.AnalyzeTurn("Inspect internal/server/websocket.go docs/specs/15-chain-orchestrator.md and receipts/planner/chain-123-step-001.md", nil)

	requireStringsContain(t, needs.ExplicitFiles, "internal/server/websocket.go")
	requireStringsContain(t, needs.ExplicitFiles, "docs/specs/15-chain-orchestrator.md")
	requireStringsContain(t, needs.ExplicitFiles, "receipts/planner/chain-123-step-001.md")
}

func requireStringsContain(t *testing.T, got []string, want string) {
	t.Helper()
	if !slices.Contains(got, want) {
		t.Fatalf("slice %v does not contain %q", got, want)
	}
}

func requireSignal(t *testing.T, signals []Signal, typ string, sourceContains string, value string) {
	t.Helper()
	for _, signal := range signals {
		if signal.Type != typ {
			continue
		}
		if sourceContains != "" && !strings.Contains(signal.Source, sourceContains) {
			continue
		}
		if value != "" && signal.Value != value {
			continue
		}
		return
	}
	t.Fatalf("no signal matched type=%q sourceContains=%q value=%q in %v", typ, sourceContains, value, signals)
}
