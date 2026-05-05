package main

import (
	stdctx "context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	contextpkg "github.com/ponchione/sodoryard/internal/context"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
)

const legacySQLiteSequenceScale = 1000
const maxUint32Int64 = int64(^uint32(0))
const maxUint64Float = float64(^uint64(0))

func migrateSQLiteProjectMemory(ctx stdctx.Context, sqlitePath string, target *projectmemory.BrainBackend) (projectmemory.ImportSQLiteStateResult, error) {
	if strings.TrimSpace(sqlitePath) == "" {
		return projectmemory.ImportSQLiteStateResult{}, fmt.Errorf("source SQLite path is required")
	}
	if _, err := os.Stat(sqlitePath); err != nil {
		return projectmemory.ImportSQLiteStateResult{}, fmt.Errorf("stat source SQLite database: %w", err)
	}
	database, err := openSQLiteMigrationDB(ctx, sqlitePath)
	if err != nil {
		return projectmemory.ImportSQLiteStateResult{}, fmt.Errorf("open source SQLite database: %w", err)
	}
	defer database.Close()

	state, err := readSQLiteProjectMemoryState(ctx, database)
	if err != nil {
		return projectmemory.ImportSQLiteStateResult{}, err
	}
	return target.ImportSQLiteState(ctx, state)
}

func openSQLiteMigrationDB(ctx stdctx.Context, sqlitePath string) (*sql.DB, error) {
	absPath, err := filepath.Abs(sqlitePath)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("mode", "ro")
	query.Set("_busy_timeout", "5000")
	query.Set("_foreign_keys", "on")
	u := url.URL{Scheme: "file", Path: absPath, RawQuery: query.Encode()}
	database, err := sql.Open(appdb.DriverName, u.String())
	if err != nil {
		return nil, err
	}
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}

func readSQLiteProjectMemoryState(ctx stdctx.Context, database *sql.DB) (projectmemory.ImportSQLiteStateArgs, error) {
	state := projectmemory.ImportSQLiteStateArgs{}
	readers := []struct {
		table string
		read  func(stdctx.Context, *sql.DB, *projectmemory.ImportSQLiteStateArgs) error
	}{
		{"conversations", readSQLiteConversations},
		{"messages", readSQLiteMessages},
		{"sub_calls", readSQLiteSubCalls},
		{"tool_executions", readSQLiteToolExecutions},
		{"context_reports", readSQLiteContextReports},
		{"chains", readSQLiteChains},
		{"steps", readSQLiteSteps},
		{"events", readSQLiteEvents},
		{"launches", readSQLiteLaunches},
		{"launch_presets", readSQLiteLaunchPresets},
	}
	for _, reader := range readers {
		exists, err := sqliteTableExists(ctx, database, reader.table)
		if err != nil {
			return projectmemory.ImportSQLiteStateArgs{}, err
		}
		if !exists {
			continue
		}
		if err := reader.read(ctx, database, &state); err != nil {
			return projectmemory.ImportSQLiteStateArgs{}, fmt.Errorf("read SQLite %s: %w", reader.table, err)
		}
	}
	return state, nil
}

func readSQLiteConversations(ctx stdctx.Context, database *sql.DB, state *projectmemory.ImportSQLiteStateArgs) error {
	rows, err := database.QueryContext(ctx, `
SELECT id, project_id, title, model, provider, created_at, updated_at
FROM conversations
ORDER BY created_at, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row sqliteConversationRow
		if err := rows.Scan(&row.ID, &row.ProjectID, &row.Title, &row.Model, &row.Provider, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return err
		}
		state.Conversations = append(state.Conversations, projectmemory.Conversation{
			ID:           row.ID,
			ProjectID:    row.ProjectID,
			Title:        nullString(row.Title),
			CreatedAtUS:  parseSQLiteTimeUS(row.CreatedAt),
			UpdatedAtUS:  parseSQLiteTimeUS(row.UpdatedAt),
			Provider:     nullString(row.Provider),
			Model:        nullString(row.Model),
			SettingsJSON: "{}",
		})
	}
	return rows.Err()
}

func readSQLiteMessages(ctx stdctx.Context, database *sql.DB, state *projectmemory.ImportSQLiteStateArgs) error {
	rows, err := database.QueryContext(ctx, `
SELECT id, conversation_id, role, content, tool_use_id, tool_name, turn_number, iteration, sequence,
       is_compressed, is_summary, compressed_turn_start, compressed_turn_end, created_at
FROM messages
ORDER BY conversation_id, sequence, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row sqliteMessageRow
		if err := rows.Scan(&row.ID, &row.ConversationID, &row.Role, &row.Content, &row.ToolUseID, &row.ToolName, &row.TurnNumber, &row.Iteration, &row.Sequence, &row.IsCompressed, &row.IsSummary, &row.CompressedTurnStart, &row.CompressedTurnEnd, &row.CreatedAt); err != nil {
			return err
		}
		turnNumber, err := sqliteUint32(row.TurnNumber, "message turn_number")
		if err != nil {
			return err
		}
		iteration, err := sqliteUint32(row.Iteration, "message iteration")
		if err != nil {
			return err
		}
		sequence, err := sqliteSequence(row.Sequence)
		if err != nil {
			return err
		}
		state.Messages = append(state.Messages, projectmemory.Message{
			ID:             legacySQLiteMessageID(row.ID),
			ConversationID: row.ConversationID,
			TurnNumber:     turnNumber,
			Iteration:      iteration,
			Sequence:       sequence,
			Role:           row.Role,
			Content:        nullString(row.Content),
			ToolUseID:      nullString(row.ToolUseID),
			ToolName:       nullString(row.ToolName),
			CreatedAtUS:    parseSQLiteTimeUS(row.CreatedAt),
			Visible:        true,
			Compressed:     row.IsCompressed != 0,
			IsSummary:      row.IsSummary != 0,
			SummaryOfJSON:  legacySQLiteSummaryOfJSON(row),
			MetadataJSON:   legacySQLiteMetadataJSON(map[string]any{"legacy_sqlite_id": row.ID, "legacy_sequence": row.Sequence}),
		})
	}
	return rows.Err()
}

func readSQLiteSubCalls(ctx stdctx.Context, database *sql.DB, state *projectmemory.ImportSQLiteStateArgs) error {
	rows, err := database.QueryContext(ctx, `
SELECT id, conversation_id, message_id, turn_number, iteration, provider, model, purpose,
       tokens_in, tokens_out, cache_read_tokens, cache_creation_tokens, latency_ms, success, error_message, created_at
FROM sub_calls
ORDER BY created_at, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row sqliteSubCallRow
		if err := rows.Scan(&row.ID, &row.ConversationID, &row.MessageID, &row.TurnNumber, &row.Iteration, &row.Provider, &row.Model, &row.Purpose, &row.TokensIn, &row.TokensOut, &row.CacheReadTokens, &row.CacheCreationTokens, &row.LatencyMs, &row.Success, &row.ErrorMessage, &row.CreatedAt); err != nil {
			return err
		}
		completedAtUS := parseSQLiteTimeUS(row.CreatedAt)
		state.SubCalls = append(state.SubCalls, projectmemory.SubCall{
			ID:                  legacySQLiteSubCallID(row.ID),
			ConversationID:      nullString(row.ConversationID),
			MessageID:           legacySQLiteNullableMessageID(row.MessageID),
			TurnNumber:          sqliteNullUint32(row.TurnNumber),
			Iteration:           sqliteNullUint32(row.Iteration),
			Provider:            row.Provider,
			Model:               row.Model,
			Purpose:             row.Purpose,
			Status:              sqliteSuccessStatus(row.Success, row.ErrorMessage),
			StartedAtUS:         subtractMillis(completedAtUS, row.LatencyMs),
			CompletedAtUS:       completedAtUS,
			TokensIn:            sqliteUint64Value(row.TokensIn),
			TokensOut:           sqliteUint64Value(row.TokensOut),
			CacheReadTokens:     sqliteUint64Value(row.CacheReadTokens),
			CacheCreationTokens: sqliteUint64Value(row.CacheCreationTokens),
			LatencyMs:           sqliteUint64Value(row.LatencyMs),
			Error:               nullString(row.ErrorMessage),
			MetadataJSON:        legacySQLiteMetadataJSON(map[string]any{"legacy_sqlite_id": row.ID}),
		})
	}
	return rows.Err()
}

func readSQLiteToolExecutions(ctx stdctx.Context, database *sql.DB, state *projectmemory.ImportSQLiteStateArgs) error {
	rows, err := database.QueryContext(ctx, `
SELECT id, conversation_id, turn_number, iteration, tool_use_id, tool_name, input,
       output_size, normalized_size, error, success, duration_ms, created_at
FROM tool_executions
ORDER BY created_at, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row sqliteToolExecutionRow
		if err := rows.Scan(&row.ID, &row.ConversationID, &row.TurnNumber, &row.Iteration, &row.ToolUseID, &row.ToolName, &row.Input, &row.OutputSize, &row.NormalizedSize, &row.Error, &row.Success, &row.DurationMs, &row.CreatedAt); err != nil {
			return err
		}
		turnNumber, err := sqliteUint32(row.TurnNumber, "tool execution turn_number")
		if err != nil {
			return err
		}
		iteration, err := sqliteUint32(row.Iteration, "tool execution iteration")
		if err != nil {
			return err
		}
		completedAtUS := parseSQLiteTimeUS(row.CreatedAt)
		state.ToolCalls = append(state.ToolCalls, projectmemory.ToolExecution{
			ID:             legacySQLiteToolExecutionID(row.ID),
			ConversationID: row.ConversationID,
			TurnNumber:     turnNumber,
			Iteration:      iteration,
			ToolUseID:      row.ToolUseID,
			ToolName:       row.ToolName,
			Status:         sqliteSuccessStatus(row.Success, row.Error),
			StartedAtUS:    subtractMillis(completedAtUS, row.DurationMs),
			CompletedAtUS:  completedAtUS,
			DurationMs:     sqliteUint64Value(row.DurationMs),
			InputJSON:      nullString(row.Input),
			OutputSize:     sqliteNullUint64(row.OutputSize),
			NormalizedSize: sqliteNullUint64(row.NormalizedSize),
			Error:          nullString(row.Error),
			MetadataJSON:   legacySQLiteMetadataJSON(map[string]any{"legacy_sqlite_id": row.ID}),
		})
	}
	return rows.Err()
}

func readSQLiteContextReports(ctx stdctx.Context, database *sql.DB, state *projectmemory.ImportSQLiteStateArgs) error {
	tokenBudgetExpr := "NULL AS token_budget_json"
	hasTokenBudget, err := sqliteTableHasColumn(ctx, database, "context_reports", "token_budget_json")
	if err != nil {
		return err
	}
	if hasTokenBudget {
		tokenBudgetExpr = "token_budget_json"
	}
	rows, err := database.QueryContext(ctx, fmt.Sprintf(`
SELECT id, conversation_id, turn_number, analysis_latency_ms, retrieval_latency_ms, total_latency_ms,
       needs_json, signals_json, rag_results_json, brain_results_json, graph_results_json, explicit_files_json,
       budget_total, budget_used, budget_breakdown_json, %s, included_count, excluded_count,
       agent_used_search_tool, agent_read_files_json, context_hit_rate, created_at
FROM context_reports
ORDER BY conversation_id, turn_number, id`, tokenBudgetExpr))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row sqliteContextReportRow
		if err := rows.Scan(&row.ID, &row.ConversationID, &row.TurnNumber, &row.AnalysisLatencyMs, &row.RetrievalLatencyMs, &row.TotalLatencyMs, &row.NeedsJSON, &row.SignalsJSON, &row.RAGResultsJSON, &row.BrainResultsJSON, &row.GraphResultsJSON, &row.ExplicitFilesJSON, &row.BudgetTotal, &row.BudgetUsed, &row.BudgetBreakdownJSON, &row.TokenBudgetJSON, &row.IncludedCount, &row.ExcludedCount, &row.AgentUsedSearchTool, &row.AgentReadFilesJSON, &row.ContextHitRate, &row.CreatedAt); err != nil {
			return err
		}
		report, err := legacySQLiteContextReport(row)
		if err != nil {
			return err
		}
		quality, err := legacySQLiteContextReportQualityJSON(row)
		if err != nil {
			return err
		}
		requestJSON := legacySQLiteMetadataJSON(map[string]any{
			"conversation_id":   row.ConversationID,
			"turn_number":       row.TurnNumber,
			"legacy_sqlite_id":  row.ID,
			"legacy_source":     "yard.db",
			"legacy_table_name": "context_reports",
		})
		turnNumber, err := sqliteUint32(row.TurnNumber, "context report turn_number")
		if err != nil {
			return err
		}
		state.ContextReports = append(state.ContextReports, projectmemory.ContextReport{
			ID:             projectmemory.ContextReportID(row.ConversationID, turnNumber),
			ConversationID: row.ConversationID,
			TurnNumber:     turnNumber,
			CreatedAtUS:    parseSQLiteTimeUS(row.CreatedAt),
			UpdatedAtUS:    parseSQLiteTimeUS(row.CreatedAt),
			RequestJSON:    requestJSON,
			ReportJSON:     report,
			QualityJSON:    quality,
		})
	}
	return rows.Err()
}

func readSQLiteChains(ctx stdctx.Context, database *sql.DB, state *projectmemory.ImportSQLiteStateArgs) error {
	rows, err := database.QueryContext(ctx, `
SELECT id, source_specs, source_task, status, summary, total_steps, total_tokens, total_duration_secs,
       resolver_loops, max_steps, max_resolver_loops, max_duration_secs, token_budget,
       started_at, completed_at, created_at, updated_at
FROM chains
ORDER BY created_at, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row sqliteChainRow
		if err := rows.Scan(&row.ID, &row.SourceSpecs, &row.SourceTask, &row.Status, &row.Summary, &row.TotalSteps, &row.TotalTokens, &row.TotalDurationSecs, &row.ResolverLoops, &row.MaxSteps, &row.MaxResolverLoops, &row.MaxDurationSecs, &row.TokenBudget, &row.StartedAt, &row.CompletedAt, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return err
		}
		state.Chains = append(state.Chains, projectmemory.Chain{
			ID:              row.ID,
			SourceSpecsJSON: defaultString(nullString(row.SourceSpecs), "[]"),
			SourceTask:      nullString(row.SourceTask),
			Status:          row.Status,
			Summary:         nullString(row.Summary),
			CreatedAtUS:     parseSQLiteTimeUS(row.CreatedAt),
			UpdatedAtUS:     parseSQLiteTimeUS(row.UpdatedAt),
			StartedAtUS:     parseSQLiteTimeUS(row.StartedAt),
			CompletedAtUS:   parseSQLiteNullTimeUS(row.CompletedAt),
			MetricsJSON: legacySQLiteMetadataJSON(map[string]any{
				"total_steps":         row.TotalSteps,
				"total_tokens":        row.TotalTokens,
				"total_duration_secs": row.TotalDurationSecs,
				"resolver_loops":      row.ResolverLoops,
			}),
			LimitsJSON: legacySQLiteMetadataJSON(map[string]any{
				"max_steps":          row.MaxSteps,
				"max_resolver_loops": row.MaxResolverLoops,
				"max_duration_secs":  row.MaxDurationSecs,
				"token_budget":       row.TokenBudget,
			}),
			ControlJSON: "{}",
		})
	}
	return rows.Err()
}

func readSQLiteSteps(ctx stdctx.Context, database *sql.DB, state *projectmemory.ImportSQLiteStateArgs) error {
	rows, err := database.QueryContext(ctx, `
SELECT id, chain_id, sequence_num, role, task, task_context, status, verdict, receipt_path,
       tokens_used, turns_used, duration_secs, exit_code, error_message, started_at, completed_at, created_at
FROM steps
ORDER BY chain_id, sequence_num, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row sqliteStepRow
		if err := rows.Scan(&row.ID, &row.ChainID, &row.SequenceNum, &row.Role, &row.Task, &row.TaskContext, &row.Status, &row.Verdict, &row.ReceiptPath, &row.TokensUsed, &row.TurnsUsed, &row.DurationSecs, &row.ExitCode, &row.ErrorMessage, &row.StartedAt, &row.CompletedAt, &row.CreatedAt); err != nil {
			return err
		}
		sequence, err := sqliteUint32(row.SequenceNum, "step sequence_num")
		if err != nil {
			return err
		}
		state.Steps = append(state.Steps, projectmemory.ChainStep{
			ID:            row.ID,
			ChainID:       row.ChainID,
			Sequence:      sequence,
			Role:          row.Role,
			Task:          row.Task,
			TaskContext:   nullString(row.TaskContext),
			Status:        row.Status,
			Verdict:       nullString(row.Verdict),
			CreatedAtUS:   parseSQLiteTimeUS(row.CreatedAt),
			StartedAtUS:   parseSQLiteNullTimeUS(row.StartedAt),
			CompletedAtUS: parseSQLiteNullTimeUS(row.CompletedAt),
			ReceiptPath:   nullString(row.ReceiptPath),
			TokensUsed:    sqliteUint64Value(row.TokensUsed),
			TurnsUsed:     sqliteUint64Value(row.TurnsUsed),
			DurationSecs:  sqliteUint64Value(row.DurationSecs),
			ExitCode:      row.ExitCode.Int64,
			HasExitCode:   row.ExitCode.Valid,
			Error:         nullString(row.ErrorMessage),
		})
	}
	return rows.Err()
}

func readSQLiteEvents(ctx stdctx.Context, database *sql.DB, state *projectmemory.ImportSQLiteStateArgs) error {
	rows, err := database.QueryContext(ctx, `
SELECT id, chain_id, step_id, event_type, event_data, created_at
FROM events
ORDER BY chain_id, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row sqliteEventRow
		if err := rows.Scan(&row.ID, &row.ChainID, &row.StepID, &row.EventType, &row.EventData, &row.CreatedAt); err != nil {
			return err
		}
		sequence := sqliteUint64Value(row.ID)
		state.Events = append(state.Events, projectmemory.ChainEvent{
			ID:          projectmemory.ChainEventID(row.ChainID, sequence),
			ChainID:     row.ChainID,
			StepID:      nullString(row.StepID),
			Sequence:    sequence,
			EventType:   row.EventType,
			CreatedAtUS: parseSQLiteTimeUS(row.CreatedAt),
			PayloadJSON: defaultString(nullString(row.EventData), "{}"),
		})
	}
	return rows.Err()
}

func readSQLiteLaunches(ctx stdctx.Context, database *sql.DB, state *projectmemory.ImportSQLiteStateArgs) error {
	rows, err := database.QueryContext(ctx, `
SELECT id, project_id, status, mode, role, allowed_roles, roster, source_task, source_specs, created_at, updated_at
FROM launches
ORDER BY project_id, updated_at, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row sqliteLaunchRow
		if err := rows.Scan(&row.ID, &row.ProjectID, &row.Status, &row.Mode, &row.Role, &row.AllowedRoles, &row.Roster, &row.SourceTask, &row.SourceSpecs, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return err
		}
		state.Launches = append(state.Launches, projectmemory.Launch{
			ID:               projectmemory.ProjectLaunchID(row.ProjectID, row.ID),
			ProjectID:        row.ProjectID,
			LaunchID:         row.ID,
			Status:           row.Status,
			Mode:             row.Mode,
			Role:             nullString(row.Role),
			AllowedRolesJSON: defaultString(nullString(row.AllowedRoles), "[]"),
			RosterJSON:       defaultString(nullString(row.Roster), "[]"),
			SourceTask:       nullString(row.SourceTask),
			SourceSpecsJSON:  defaultString(nullString(row.SourceSpecs), "[]"),
			CreatedAtUS:      parseSQLiteTimeUS(row.CreatedAt),
			UpdatedAtUS:      parseSQLiteTimeUS(row.UpdatedAt),
		})
	}
	return rows.Err()
}

func readSQLiteLaunchPresets(ctx stdctx.Context, database *sql.DB, state *projectmemory.ImportSQLiteStateArgs) error {
	rows, err := database.QueryContext(ctx, `
SELECT id, project_id, name, mode, role, allowed_roles, roster, created_at, updated_at
FROM launch_presets
ORDER BY project_id, updated_at, name`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row sqliteLaunchPresetRow
		if err := rows.Scan(&row.ID, &row.ProjectID, &row.Name, &row.Mode, &row.Role, &row.AllowedRoles, &row.Roster, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return err
		}
		state.LaunchPresets = append(state.LaunchPresets, projectmemory.LaunchPreset{
			ID:               projectmemory.ProjectLaunchPresetID(row.ProjectID, row.Name),
			ProjectID:        row.ProjectID,
			PresetID:         row.ID,
			Name:             row.Name,
			Mode:             row.Mode,
			Role:             nullString(row.Role),
			AllowedRolesJSON: defaultString(nullString(row.AllowedRoles), "[]"),
			RosterJSON:       defaultString(nullString(row.Roster), "[]"),
			CreatedAtUS:      parseSQLiteTimeUS(row.CreatedAt),
			UpdatedAtUS:      parseSQLiteTimeUS(row.UpdatedAt),
		})
	}
	return rows.Err()
}

type sqliteConversationRow struct {
	ID, ProjectID          string
	Title, Model, Provider sql.NullString
	CreatedAt, UpdatedAt   string
}

type sqliteMessageRow struct {
	ID                                     int64
	ConversationID, Role                   string
	Content, ToolUseID, ToolName           sql.NullString
	TurnNumber, Iteration                  int64
	Sequence                               float64
	IsCompressed, IsSummary                int64
	CompressedTurnStart, CompressedTurnEnd sql.NullInt64
	CreatedAt                              string
}

type sqliteSubCallRow struct {
	ID                                   int64
	ConversationID                       sql.NullString
	MessageID, TurnNumber, Iteration     sql.NullInt64
	Provider, Model, Purpose             string
	TokensIn, TokensOut                  int64
	CacheReadTokens, CacheCreationTokens int64
	LatencyMs, Success                   int64
	ErrorMessage                         sql.NullString
	CreatedAt                            string
}

type sqliteToolExecutionRow struct {
	ID                                  int64
	ConversationID, ToolUseID, ToolName string
	TurnNumber, Iteration               int64
	Input, Error                        sql.NullString
	OutputSize, NormalizedSize          sql.NullInt64
	Success, DurationMs                 int64
	CreatedAt                           string
}

type sqliteContextReportRow struct {
	ID                                                                                                                                  int64
	ConversationID                                                                                                                      string
	TurnNumber                                                                                                                          int64
	AnalysisLatencyMs, RetrievalLatencyMs, TotalLatencyMs                                                                               sql.NullInt64
	NeedsJSON, SignalsJSON, RAGResultsJSON, BrainResultsJSON, GraphResultsJSON, ExplicitFilesJSON, BudgetBreakdownJSON, TokenBudgetJSON sql.NullString
	BudgetTotal, BudgetUsed, IncludedCount, ExcludedCount, AgentUsedSearchTool                                                          sql.NullInt64
	AgentReadFilesJSON                                                                                                                  sql.NullString
	ContextHitRate                                                                                                                      sql.NullFloat64
	CreatedAt                                                                                                                           string
}

type sqliteChainRow struct {
	ID                                                        string
	SourceSpecs, SourceTask, Summary, CompletedAt             sql.NullString
	Status, StartedAt, CreatedAt, UpdatedAt                   string
	TotalSteps, TotalTokens, TotalDurationSecs, ResolverLoops int64
	MaxSteps, MaxResolverLoops, MaxDurationSecs, TokenBudget  int64
}

type sqliteStepRow struct {
	ID, ChainID, Role, Task, Status, CreatedAt       string
	SequenceNum, TokensUsed, TurnsUsed, DurationSecs int64
	TaskContext, Verdict, ReceiptPath, ErrorMessage  sql.NullString
	ExitCode                                         sql.NullInt64
	StartedAt, CompletedAt                           sql.NullString
}

type sqliteEventRow struct {
	ID                            int64
	ChainID, EventType, CreatedAt string
	StepID, EventData             sql.NullString
}

type sqliteLaunchRow struct {
	ID, ProjectID, Status, Mode, CreatedAt, UpdatedAt   string
	Role, AllowedRoles, Roster, SourceTask, SourceSpecs sql.NullString
}

type sqliteLaunchPresetRow struct {
	ID, ProjectID, Name, Mode, CreatedAt, UpdatedAt string
	Role, AllowedRoles, Roster                      sql.NullString
}

func sqliteTableExists(ctx stdctx.Context, database *sql.DB, table string) (bool, error) {
	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
		return false, fmt.Errorf("check SQLite table %s: %w", table, err)
	}
	return count > 0, nil
}

func sqliteTableHasColumn(ctx stdctx.Context, database *sql.DB, table string, column string) (bool, error) {
	rows, err := database.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, fmt.Errorf("inspect SQLite table %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func legacySQLiteContextReport(row sqliteContextReportRow) (string, error) {
	report := contextpkg.ContextAssemblyReport{
		TurnNumber:         int(row.TurnNumber),
		AnalysisLatencyMs:  row.AnalysisLatencyMs.Int64,
		RetrievalLatencyMs: row.RetrievalLatencyMs.Int64,
		TotalLatencyMs:     row.TotalLatencyMs.Int64,
		BudgetTotal:        int(row.BudgetTotal.Int64),
		BudgetUsed:         int(row.BudgetUsed.Int64),
	}
	if err := unmarshalSQLiteJSON(row.NeedsJSON, &report.Needs); err != nil {
		return "", fmt.Errorf("decode needs_json for context report %d: %w", row.ID, err)
	}
	if err := unmarshalSQLiteJSON(row.SignalsJSON, &report.Needs.Signals); err != nil {
		return "", fmt.Errorf("decode signals_json for context report %d: %w", row.ID, err)
	}
	if err := unmarshalSQLiteJSON(row.RAGResultsJSON, &report.RAGResults); err != nil {
		return "", fmt.Errorf("decode rag_results_json for context report %d: %w", row.ID, err)
	}
	if err := unmarshalSQLiteJSON(row.BrainResultsJSON, &report.BrainResults); err != nil {
		return "", fmt.Errorf("decode brain_results_json for context report %d: %w", row.ID, err)
	}
	if err := unmarshalSQLiteJSON(row.GraphResultsJSON, &report.GraphResults); err != nil {
		return "", fmt.Errorf("decode graph_results_json for context report %d: %w", row.ID, err)
	}
	if err := unmarshalSQLiteJSON(row.ExplicitFilesJSON, &report.ExplicitFileResults); err != nil {
		return "", fmt.Errorf("decode explicit_files_json for context report %d: %w", row.ID, err)
	}
	if err := unmarshalSQLiteJSON(row.BudgetBreakdownJSON, &report.BudgetBreakdown); err != nil {
		return "", fmt.Errorf("decode budget_breakdown_json for context report %d: %w", row.ID, err)
	}
	if err := unmarshalSQLiteJSON(row.TokenBudgetJSON, &report.TokenBudget); err != nil {
		return "", fmt.Errorf("decode token_budget_json for context report %d: %w", row.ID, err)
	}
	if report.BudgetBreakdown == nil {
		report.BudgetBreakdown = map[string]int{}
	}
	data, err := json.Marshal(report)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func legacySQLiteContextReportQualityJSON(row sqliteContextReportRow) (string, error) {
	readFiles := []string{}
	if err := unmarshalSQLiteJSON(row.AgentReadFilesJSON, &readFiles); err != nil {
		return "", fmt.Errorf("decode agent_read_files_json for context report %d: %w", row.ID, err)
	}
	quality := struct {
		AgentUsedSearchTool bool     `json:"agent_used_search_tool"`
		AgentReadFiles      []string `json:"agent_read_files"`
		ContextHitRate      float64  `json:"context_hit_rate"`
	}{
		AgentUsedSearchTool: row.AgentUsedSearchTool.Valid && row.AgentUsedSearchTool.Int64 != 0,
		AgentReadFiles:      readFiles,
		ContextHitRate:      row.ContextHitRate.Float64,
	}
	data, err := json.Marshal(quality)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func legacySQLiteSummaryOfJSON(row sqliteMessageRow) string {
	if row.IsSummary == 0 {
		return "[]"
	}
	payload := map[string]any{"legacy_sqlite_id": row.ID}
	if row.CompressedTurnStart.Valid {
		payload["turn_start"] = row.CompressedTurnStart.Int64
	}
	if row.CompressedTurnEnd.Valid {
		payload["turn_end"] = row.CompressedTurnEnd.Int64
	}
	return legacySQLiteMetadataJSON(payload)
}

func legacySQLiteMetadataJSON(values map[string]any) string {
	data, err := json.Marshal(values)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func unmarshalSQLiteJSON(raw sql.NullString, target any) error {
	if !raw.Valid {
		return nil
	}
	trimmed := strings.TrimSpace(raw.String)
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	return json.Unmarshal([]byte(trimmed), target)
}

func legacySQLiteMessageID(id int64) string {
	return fmt.Sprintf("legacy:sqlite:message:%d", id)
}

func legacySQLiteNullableMessageID(id sql.NullInt64) string {
	if !id.Valid {
		return ""
	}
	return legacySQLiteMessageID(id.Int64)
}

func legacySQLiteSubCallID(id int64) string {
	return fmt.Sprintf("legacy:sqlite:sub_call:%d", id)
}

func legacySQLiteToolExecutionID(id int64) string {
	return fmt.Sprintf("legacy:sqlite:tool_execution:%d", id)
}

func sqliteSuccessStatus(success int64, errorValue sql.NullString) string {
	if success != 0 {
		return "success"
	}
	if strings.TrimSpace(nullString(errorValue)) != "" {
		return "error"
	}
	return "failed"
}

func sqliteSequence(value float64) (uint64, error) {
	if value < 0 {
		return 0, fmt.Errorf("message sequence must be non-negative")
	}
	scaled := math.Round(value * legacySQLiteSequenceScale)
	if scaled > maxUint64Float {
		return 0, fmt.Errorf("message sequence is too large")
	}
	return uint64(scaled), nil
}

func sqliteUint32(value int64, field string) (uint32, error) {
	if value < 0 || value > maxUint32Int64 {
		return 0, fmt.Errorf("%s out of range: %d", field, value)
	}
	return uint32(value), nil
}

func sqliteNullUint32(value sql.NullInt64) uint32 {
	if !value.Valid {
		return 0
	}
	if value.Int64 < 0 || value.Int64 > maxUint32Int64 {
		return 0
	}
	return uint32(value.Int64)
}

func sqliteUint64Value(value int64) uint64 {
	if value < 0 {
		return 0
	}
	return uint64(value)
}

func sqliteNullUint64(value sql.NullInt64) uint64 {
	if !value.Valid || value.Int64 < 0 {
		return 0
	}
	return uint64(value.Int64)
}

func nullString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func parseSQLiteNullTimeUS(value sql.NullString) uint64 {
	if !value.Valid {
		return 0
	}
	return parseSQLiteTimeUS(value.String)
}

func parseSQLiteTimeUS(value string) uint64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return uint64(parsed.UTC().UnixMicro())
		}
	}
	return 0
}

func subtractMillis(completedAtUS uint64, durationMs int64) uint64 {
	if durationMs <= 0 {
		return completedAtUS
	}
	durationUS := uint64(durationMs) * 1000
	if completedAtUS < durationUS {
		return 0
	}
	return completedAtUS - durationUS
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
