package context

import (
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/ponchione/sirtopham/internal/db"
)

func TestRuleBasedAnalyzerExtractsFileReferences(t *testing.T) {
	analyzer := RuleBasedAnalyzer{}

	needs := analyzer.AnalyzeTurn("Check internal/auth/ and cmd/sirtopham/main.go", nil)

	requireStringsContain(t, needs.ExplicitFiles, "internal/auth")
	requireStringsContain(t, needs.ExplicitFiles, "cmd/sirtopham/main.go")
	requireSignal(t, needs.Signals, "file_ref", "internal/auth", "internal/auth")
	requireSignal(t, needs.Signals, "file_ref", "cmd/sirtopham/main.go", "cmd/sirtopham/main.go")
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

func historyMessage(content string) db.Message {
	return db.Message{
		Content: sql.NullString{String: content, Valid: true},
	}
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
