package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	contextpkg "github.com/ponchione/sodoryard/internal/context"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
)

func TestMemoryMigrateAndVerifyDocuments(t *testing.T) {
	ctx := context.Background()
	configPath, dataDir := writeMemoryTestProject(t)

	migrateResult, err := runMemoryMigrate(ctx, configPath, "", "", "")
	if err != nil {
		t.Fatalf("runMemoryMigrate returned error: %v", err)
	}
	if migrateResult.Documents != 2 {
		t.Fatalf("migrated count = %d, want 2", migrateResult.Documents)
	}
	verifyResult, err := runMemoryVerify(ctx, configPath, "", "")
	if err != nil {
		t.Fatalf("runMemoryVerify returned error: %v", err)
	}
	if verifyResult.Verified != 2 {
		t.Fatalf("verified count = %d, want 2", verifyResult.Verified)
	}
	if _, err := os.Stat(dataDir); err != nil {
		t.Fatalf("Shunter data dir stat returned error: %v", err)
	}
}

func TestMemoryVerifyDetectsDocumentMismatch(t *testing.T) {
	ctx := context.Background()
	configPath, dataDir := writeMemoryTestProject(t)
	if _, err := runMemoryMigrate(ctx, configPath, "", "", ""); err != nil {
		t.Fatalf("runMemoryMigrate returned error: %v", err)
	}
	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend returned error: %v", err)
	}
	if err := backend.WriteDocument(ctx, "notes/a.md", "# A\n\nChanged."); err != nil {
		t.Fatalf("WriteDocument returned error: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	_, err = runMemoryVerify(ctx, configPath, "", "")
	if err == nil || !strings.Contains(err.Error(), "document content mismatch: notes/a.md") {
		t.Fatalf("runMemoryVerify error = %v, want notes/a.md content mismatch", err)
	}
}

func TestMemoryMigrateSQLiteImportsLegacyRuntimeState(t *testing.T) {
	ctx := context.Background()
	configPath, dataDir := writeMemoryTestProject(t)
	projectRoot := filepath.Dir(configPath)
	sqlitePath := filepath.Join(projectRoot, ".yard", "yard.db")
	writeLegacySQLiteMemoryState(t, ctx, sqlitePath, projectRoot)

	result, err := runMemoryMigrate(ctx, configPath, "", sqlitePath, "")
	if err != nil {
		t.Fatalf("runMemoryMigrate returned error: %v", err)
	}
	if result.Documents != 0 {
		t.Fatalf("document migration count = %d, want 0 for SQLite-only migration", result.Documents)
	}
	if result.SQLite.Conversations != 1 || result.SQLite.Messages != 2 || result.SQLite.Chains != 1 || result.SQLite.Steps != 1 || result.SQLite.Events != 1 {
		t.Fatalf("SQLite result core counts = %+v, want one conversation/two messages/one chain/one step/one event", result.SQLite)
	}
	if result.SQLite.ToolExecutions != 1 || result.SQLite.SubCalls != 1 || result.SQLite.ContextReports != 1 || result.SQLite.Launches != 1 || result.SQLite.LaunchPresets != 1 {
		t.Fatalf("SQLite result detail counts = %+v, want one of each detail row", result.SQLite)
	}

	backend, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: dataDir, DurableAck: true})
	if err != nil {
		t.Fatalf("OpenBrainBackend returned error: %v", err)
	}
	defer backend.Close()

	conversation, found, err := backend.ReadConversation(ctx, "conv-sqlite")
	if err != nil {
		t.Fatalf("ReadConversation returned error: %v", err)
	}
	if !found || conversation.Title != "Legacy Conversation" || conversation.Provider != "codex" || conversation.Model != "gpt-5" {
		t.Fatalf("conversation = %+v found=%t, want imported legacy conversation", conversation, found)
	}
	messages, err := backend.ListMessages(ctx, "conv-sqlite", true)
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	if len(messages) != 2 || messages[0].ID != legacySQLiteMessageID(1) || messages[1].Content != "legacy answer" {
		t.Fatalf("messages = %+v, want imported message ids and content", messages)
	}
	toolCalls, err := backend.ListToolExecutions(ctx, "conv-sqlite")
	if err != nil {
		t.Fatalf("ListToolExecutions returned error: %v", err)
	}
	if len(toolCalls) != 1 || toolCalls[0].ToolName != "file_read" || toolCalls[0].Status != "success" {
		t.Fatalf("tool calls = %+v, want imported file_read success", toolCalls)
	}
	subCalls, err := backend.ListSubCalls(ctx, "conv-sqlite")
	if err != nil {
		t.Fatalf("ListSubCalls returned error: %v", err)
	}
	if len(subCalls) != 1 || subCalls[0].MessageID != legacySQLiteMessageID(2) || subCalls[0].TokensIn != 10 {
		t.Fatalf("subcalls = %+v, want linked imported provider call", subCalls)
	}
	reportStore := contextpkg.NewProjectMemoryReportStore(backend)
	report, err := reportStore.Get(ctx, "conv-sqlite", 1)
	if err != nil {
		t.Fatalf("Get context report returned error: %v", err)
	}
	if report.BudgetTotal != 100 || !report.AgentUsedSearchTool || len(report.AgentReadFiles) != 1 || report.AgentReadFiles[0] != "README.md" {
		t.Fatalf("context report = %+v, want imported budget and quality data", report)
	}
	chain, found, err := backend.ReadChain(ctx, "chain-sqlite")
	if err != nil {
		t.Fatalf("ReadChain returned error: %v", err)
	}
	if !found || chain.Status != "completed" || !strings.Contains(chain.MetricsJSON, "total_tokens") {
		t.Fatalf("chain = %+v found=%t, want imported completed chain", chain, found)
	}
	steps, err := backend.ListChainSteps(ctx, "chain-sqlite")
	if err != nil {
		t.Fatalf("ListChainSteps returned error: %v", err)
	}
	if len(steps) != 1 || steps[0].ReceiptPath != "receipts/coder/r.md" || !steps[0].HasExitCode {
		t.Fatalf("steps = %+v, want imported completed step", steps)
	}
	events, err := backend.ListChainEvents(ctx, "chain-sqlite")
	if err != nil {
		t.Fatalf("ListChainEvents returned error: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "step_completed" {
		t.Fatalf("events = %+v, want imported chain event", events)
	}
	launch, found, err := backend.ReadLaunch(ctx, projectRoot, "current")
	if err != nil {
		t.Fatalf("ReadLaunch returned error: %v", err)
	}
	if !found || launch.SourceTask != "legacy draft" {
		t.Fatalf("launch = %+v found=%t, want imported draft", launch, found)
	}
	presets, err := backend.ListLaunchPresets(ctx, projectRoot)
	if err != nil {
		t.Fatalf("ListLaunchPresets returned error: %v", err)
	}
	if len(presets) != 1 || presets[0].Name != "legacy preset" {
		t.Fatalf("presets = %+v, want imported launch preset", presets)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	second, err := runMemoryMigrate(ctx, configPath, "", sqlitePath, "")
	if err != nil {
		t.Fatalf("second runMemoryMigrate returned error: %v", err)
	}
	if second.SQLite.Conversations != 0 || second.SQLite.Messages != 0 || second.SQLite.Skipped == 0 {
		t.Fatalf("second SQLite result = %+v, want idempotent skip-only import", second.SQLite)
	}
}

func writeMemoryTestProject(t *testing.T) (string, string) {
	t.Helper()
	projectRoot := t.TempDir()
	brainDir := filepath.Join(projectRoot, ".brain")
	if err := os.MkdirAll(filepath.Join(brainDir, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(brainDir, "conventions"), 0o755); err != nil {
		t.Fatalf("mkdir conventions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brainDir, "notes", "a.md"), []byte("# A\n\nAlpha."), 0o644); err != nil {
		t.Fatalf("write notes/a.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brainDir, "conventions", "coding.md"), []byte("# Coding\n\n- Test."), 0o644); err != nil {
		t.Fatalf("write conventions/coding.md: %v", err)
	}
	dataDir := filepath.Join(projectRoot, ".yard", "shunter", "project-memory")
	configPath := filepath.Join(projectRoot, "yard.yaml")
	configYAML := strings.Join([]string{
		"project_root: " + projectRoot,
		"memory:",
		"  backend: shunter",
		"  shunter_data_dir: .yard/shunter/project-memory",
		"  durable_ack: true",
		"brain:",
		"  enabled: true",
		"  vault_path: .brain",
		"local_services:",
		"  enabled: false",
	}, "\n") + "\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath, dataDir
}

func writeLegacySQLiteMemoryState(t *testing.T, ctx context.Context, dbPath string, projectRoot string) {
	t.Helper()
	database, err := appdb.OpenDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenDB returned error: %v", err)
	}
	defer database.Close()
	if _, err := appdb.InitIfNeeded(ctx, database); err != nil {
		t.Fatalf("InitIfNeeded returned error: %v", err)
	}
	if err := appdb.EnsureChainSchema(ctx, database); err != nil {
		t.Fatalf("EnsureChainSchema returned error: %v", err)
	}
	if err := appdb.EnsureLaunchSchema(ctx, database); err != nil {
		t.Fatalf("EnsureLaunchSchema returned error: %v", err)
	}
	createdAt := "2026-01-02T03:04:05Z"
	updatedAt := "2026-01-02T03:05:05Z"
	mustExecMemoryTest(t, database, `INSERT INTO projects(id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, projectRoot, "test", projectRoot, createdAt, updatedAt)
	mustExecMemoryTest(t, database, `INSERT INTO conversations(id, project_id, title, model, provider, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, "conv-sqlite", projectRoot, "Legacy Conversation", "gpt-5", "codex", createdAt, updatedAt)
	mustExecMemoryTest(t, database, `INSERT INTO messages(id, conversation_id, role, content, turn_number, iteration, sequence, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, 1, "conv-sqlite", "user", "legacy prompt", 1, 1, 0.0, createdAt)
	mustExecMemoryTest(t, database, `INSERT INTO messages(id, conversation_id, role, content, tool_use_id, tool_name, turn_number, iteration, sequence, is_summary, compressed_turn_start, compressed_turn_end, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, 2, "conv-sqlite", "assistant", "legacy answer", "toolu_legacy", "file_read", 1, 1, 1.0, 1, 1, 1, updatedAt)
	mustExecMemoryTest(t, database, `INSERT INTO tool_executions(id, conversation_id, turn_number, iteration, tool_use_id, tool_name, input, output_size, normalized_size, error, success, duration_ms, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, 7, "conv-sqlite", 1, 1, "toolu_legacy", "file_read", `{"path":"README.md"}`, 123, 50, nil, 1, 25, updatedAt)
	mustExecMemoryTest(t, database, `INSERT INTO sub_calls(id, conversation_id, message_id, turn_number, iteration, provider, model, purpose, tokens_in, tokens_out, cache_read_tokens, cache_creation_tokens, latency_ms, success, error_message, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, 8, "conv-sqlite", 2, 1, 1, "codex", "gpt-5", "chat", 10, 20, 3, 4, 100, 1, nil, updatedAt)
	mustExecMemoryTest(t, database, `INSERT INTO context_reports(conversation_id, turn_number, analysis_latency_ms, retrieval_latency_ms, total_latency_ms, needs_json, signals_json, rag_results_json, brain_results_json, graph_results_json, explicit_files_json, budget_total, budget_used, budget_breakdown_json, token_budget_json, included_count, excluded_count, agent_used_search_tool, agent_read_files_json, context_hit_rate, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, "conv-sqlite", 1, 5, 10, 20, `{"semantic_queries":["legacy"]}`, `[]`, `[]`, `[]`, `[]`, `[]`, 100, 50, `{"rag":50}`, `{"model_context_limit":200}`, 1, 0, 1, `["README.md"]`, 0.75, updatedAt)
	mustExecMemoryTest(t, database, `INSERT INTO chains(id, source_specs, source_task, status, summary, total_steps, total_tokens, total_duration_secs, resolver_loops, max_steps, max_resolver_loops, max_duration_secs, token_budget, started_at, completed_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, "chain-sqlite", `["spec.md"]`, "legacy chain task", "completed", "done", 1, 30, 2, 0, 10, 1, 3600, 1000, createdAt, updatedAt, createdAt, updatedAt)
	mustExecMemoryTest(t, database, `INSERT INTO steps(id, chain_id, sequence_num, role, task, task_context, status, verdict, receipt_path, tokens_used, turns_used, duration_secs, exit_code, error_message, started_at, completed_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, "step-sqlite", "chain-sqlite", 1, "coder", "legacy step", "ctx", "completed", "pass", "receipts/coder/r.md", 30, 2, 2, 0, nil, createdAt, updatedAt, createdAt)
	mustExecMemoryTest(t, database, `INSERT INTO events(id, chain_id, step_id, event_type, event_data, created_at) VALUES (?, ?, ?, ?, ?, ?)`, 1, "chain-sqlite", "step-sqlite", "step_completed", `{"ok":true}`, updatedAt)
	mustExecMemoryTest(t, database, `INSERT INTO launches(id, project_id, status, mode, role, allowed_roles, roster, source_task, source_specs, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, "current", projectRoot, "draft", "one_step_chain", "coder", `[]`, `[]`, "legacy draft", `[]`, createdAt, updatedAt)
	mustExecMemoryTest(t, database, `INSERT INTO launch_presets(id, project_id, name, mode, role, allowed_roles, roster, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, "custom:legacy preset", projectRoot, "legacy preset", "manual_roster", "", `[]`, `["coder"]`, createdAt, updatedAt)
}

func mustExecMemoryTest(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q returned error: %v", query, err)
	}
}
