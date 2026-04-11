//go:build sqlite_fts5
// +build sqlite_fts5

package agent

import (
	stdctx "context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	contextpkg "github.com/ponchione/sodoryard/internal/context"
	"github.com/ponchione/sodoryard/internal/conversation"
	"github.com/ponchione/sodoryard/internal/db"
	sid "github.com/ponchione/sodoryard/internal/id"
	"github.com/ponchione/sodoryard/internal/provider"
)

type sqliteAssemblerStub struct {
	history []db.Message
	pkg     *contextpkg.FullContextPackage
}

func (s *sqliteAssemblerStub) Assemble(
	_ stdctx.Context,
	_ string,
	history []db.Message,
	_ contextpkg.AssemblyScope,
	_ int,
	_ int,
) (*contextpkg.FullContextPackage, bool, error) {
	s.history = append([]db.Message(nil), history...)
	return s.pkg, false, nil
}

func (s *sqliteAssemblerStub) UpdateQuality(stdctx.Context, string, int, bool, []string) error {
	return nil
}

func TestRunTurnUsesRealConversationHistoryManager(t *testing.T) {
	ctx := stdctx.Background()
	database := newAgentHistoryTestDB(t)
	conversationID := seedAgentHistoryConversation(t, database)
	historyManager := conversation.NewHistoryManager(database, conversation.NewSeenFiles())
	historyManager.SetNowForTest(func() time.Time { return time.Unix(1700001000, 0).UTC() })

	assembler := &sqliteAssemblerStub{pkg: &contextpkg.FullContextPackage{Content: "assembled", Frozen: true}}

	// Provide a simple text-only stream response so RunTurn can complete.
	routerStub := &providerRouterStub{
		streamEvents: [][]provider.StreamEvent{
			{
				provider.TokenDelta{Text: "OK"},
				provider.StreamDone{
					StopReason: provider.StopReasonEndTurn,
					Usage:      provider.Usage{InputTokens: 10, OutputTokens: 2},
				},
			},
		},
	}

	loop := NewAgentLoop(AgentLoopDeps{
		ContextAssembler:    assembler,
		ConversationManager: historyManager,
		ProviderRouter:      routerStub,
		ToolExecutor:        &toolExecutorStub{},
		PromptBuilder:       NewPromptBuilder(nil),
	})
	loop.now = func() time.Time { return time.Unix(1700001001, 0).UTC() }

	result, err := loop.RunTurn(ctx, RunTurnRequest{
		ConversationID:    conversationID,
		TurnNumber:        1,
		Message:           "fix auth",
		ModelContextLimit: 200000,
	})
	if err != nil {
		t.Fatalf("RunTurn returned error: %v", err)
	}
	if result == nil || len(result.History) != 1 {
		t.Fatalf("RunTurn result = %#v, want one reconstructed message", result)
	}
	if result.FinalText != "OK" {
		t.Fatalf("FinalText = %q, want OK", result.FinalText)
	}
	if len(assembler.history) != 1 || assembler.history[0].Role != "user" || assembler.history[0].Content.String != "fix auth" {
		t.Fatalf("assembler history = %#v, want persisted user message", assembler.history)
	}

	rows, err := db.New(database).ListTurnMessages(ctx, conversationID)
	if err != nil {
		t.Fatalf("ListTurnMessages returned error: %v", err)
	}
	// Should have user message + assistant message from the iteration.
	if len(rows) < 1 {
		t.Fatalf("rows = %#v, want at least one row", rows)
	}
}

func newAgentHistoryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := stdctx.Background()
	dbPath := filepath.Join(t.TempDir(), "agent-history.db")
	database, err := db.OpenDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenDB returned error: %v", err)
	}
	if err := db.Init(ctx, database); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func seedAgentHistoryConversation(t *testing.T, database *sql.DB) string {
	t.Helper()
	projectID := sid.New()
	conversationID := sid.New()
	createdAt := time.Unix(1700000950, 0).UTC().Format(time.RFC3339)
	mustExecAgentHistory(t, database, `INSERT INTO projects(id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, projectID, "proj", "/tmp/proj", createdAt, createdAt)
	mustExecAgentHistory(t, database, `INSERT INTO conversations(id, project_id, title, model, provider, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, conversationID, projectID, "Test", "claude", "anthropic", createdAt, createdAt)
	return conversationID
}

func mustExecAgentHistory(t *testing.T, database *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.Exec(query, args...); err != nil {
		t.Fatalf("exec failed for %q: %v", query, err)
	}
}
