package operator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
	"github.com/ponchione/sodoryard/internal/projectmemory"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
)

const currentLaunchDraftID = "current"

func (s *Service) SaveLaunchDraft(ctx context.Context, req LaunchRequest) (LaunchDraft, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, err := s.config()
	if err != nil {
		return LaunchDraft{}, err
	}
	if store, ok, err := s.projectMemoryLaunchStore(cfg); err != nil {
		return LaunchDraft{}, err
	} else if ok {
		return s.saveProjectMemoryLaunchDraft(ctx, store, cfg, req)
	}
	database, err := s.database()
	if err != nil {
		return LaunchDraft{}, err
	}
	if err := appdb.EnsureLaunchSchema(ctx, database); err != nil {
		return LaunchDraft{}, err
	}
	if err := rtpkg.EnsureProjectRecord(ctx, database, cfg); err != nil {
		return LaunchDraft{}, fmt.Errorf("ensure project record: %w", err)
	}
	req = normalizeLaunchRequest(req)
	allowedRoles, err := marshalStringSlice(req.AllowedRoles)
	if err != nil {
		return LaunchDraft{}, fmt.Errorf("marshal allowed roles: %w", err)
	}
	roster, err := marshalStringSlice(req.Roster)
	if err != nil {
		return LaunchDraft{}, fmt.Errorf("marshal roster: %w", err)
	}
	sourceSpecs, err := marshalStringSlice(req.SourceSpecs)
	if err != nil {
		return LaunchDraft{}, fmt.Errorf("marshal source specs: %w", err)
	}
	updatedAt := time.Now().UTC().Format(time.RFC3339)
	_, err = database.ExecContext(ctx, `
INSERT INTO launches(id, project_id, status, mode, role, allowed_roles, roster, source_task, source_specs, created_at, updated_at)
VALUES (?, ?, 'draft', ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, id) DO UPDATE SET
	status = 'draft',
	mode = excluded.mode,
	role = excluded.role,
	allowed_roles = excluded.allowed_roles,
	roster = excluded.roster,
	source_task = excluded.source_task,
	source_specs = excluded.source_specs,
	updated_at = excluded.updated_at
`, currentLaunchDraftID, cfg.ProjectRoot, req.Mode, req.Role, allowedRoles, roster, req.SourceTask, sourceSpecs, updatedAt, updatedAt)
	if err != nil {
		return LaunchDraft{}, fmt.Errorf("save launch draft: %w", err)
	}
	return LaunchDraft{ID: currentLaunchDraftID, Request: req, UpdatedAt: updatedAt}, nil
}

func (s *Service) LoadLaunchDraft(ctx context.Context) (LaunchDraft, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, err := s.config()
	if err != nil {
		return LaunchDraft{}, false, err
	}
	if store, ok, err := s.projectMemoryLaunchStore(cfg); err != nil {
		return LaunchDraft{}, false, err
	} else if ok {
		return s.loadProjectMemoryLaunchDraft(ctx, store, cfg)
	}
	database, err := s.database()
	if err != nil {
		return LaunchDraft{}, false, err
	}
	if err := appdb.EnsureLaunchSchema(ctx, database); err != nil {
		return LaunchDraft{}, false, err
	}

	var row launchDraftRow
	err = database.QueryRowContext(ctx, `
SELECT id, mode, role, allowed_roles, roster, source_task, source_specs, updated_at
FROM launches
WHERE project_id = ? AND id = ? AND status = 'draft'
`, cfg.ProjectRoot, currentLaunchDraftID).Scan(
		&row.ID,
		&row.Mode,
		&row.Role,
		&row.AllowedRoles,
		&row.Roster,
		&row.SourceTask,
		&row.SourceSpecs,
		&row.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return LaunchDraft{}, false, nil
		}
		return LaunchDraft{}, false, fmt.Errorf("load launch draft: %w", err)
	}
	req, err := row.request()
	if err != nil {
		return LaunchDraft{}, false, err
	}
	return LaunchDraft{ID: row.ID, Request: req, UpdatedAt: row.UpdatedAt.String}, true, nil
}

func (s *Service) SaveLaunchPreset(ctx context.Context, name string, req LaunchRequest) (LaunchPreset, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return LaunchPreset{}, fmt.Errorf("launch preset name is required")
	}
	cfg, err := s.config()
	if err != nil {
		return LaunchPreset{}, err
	}
	if store, ok, err := s.projectMemoryLaunchStore(cfg); err != nil {
		return LaunchPreset{}, err
	} else if ok {
		return s.saveProjectMemoryLaunchPreset(ctx, store, cfg, name, req)
	}
	database, err := s.database()
	if err != nil {
		return LaunchPreset{}, err
	}
	if err := appdb.EnsureLaunchSchema(ctx, database); err != nil {
		return LaunchPreset{}, err
	}
	if err := rtpkg.EnsureProjectRecord(ctx, database, cfg); err != nil {
		return LaunchPreset{}, fmt.Errorf("ensure project record: %w", err)
	}
	req, err = normalizeLaunchPresetRequest(cfg, req)
	if err != nil {
		return LaunchPreset{}, err
	}
	allowedRoles, err := marshalStringSlice(req.AllowedRoles)
	if err != nil {
		return LaunchPreset{}, fmt.Errorf("marshal allowed roles: %w", err)
	}
	roster, err := marshalStringSlice(req.Roster)
	if err != nil {
		return LaunchPreset{}, fmt.Errorf("marshal roster: %w", err)
	}
	updatedAt := time.Now().UTC().Format(time.RFC3339)
	presetID := "custom:" + name
	_, err = database.ExecContext(ctx, `
INSERT INTO launch_presets(id, project_id, name, mode, role, allowed_roles, roster, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, name) DO UPDATE SET
	mode = excluded.mode,
	role = excluded.role,
	allowed_roles = excluded.allowed_roles,
	roster = excluded.roster,
	updated_at = excluded.updated_at
`, presetID, cfg.ProjectRoot, name, req.Mode, req.Role, allowedRoles, roster, updatedAt, updatedAt)
	if err != nil {
		return LaunchPreset{}, fmt.Errorf("save launch preset: %w", err)
	}
	return LaunchPreset{ID: presetID, Name: name, Request: req, UpdatedAt: updatedAt}, nil
}

func (s *Service) ListLaunchPresets(ctx context.Context) ([]LaunchPreset, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, err := s.config()
	if err != nil {
		return nil, err
	}
	if store, ok, err := s.projectMemoryLaunchStore(cfg); err != nil {
		return nil, err
	} else if ok {
		return s.listProjectMemoryLaunchPresets(ctx, store, cfg)
	}
	database, err := s.database()
	if err != nil {
		return nil, err
	}
	if err := appdb.EnsureLaunchSchema(ctx, database); err != nil {
		return nil, err
	}
	rows, err := database.QueryContext(ctx, `
SELECT id, name, mode, role, allowed_roles, roster, updated_at
FROM launch_presets
WHERE project_id = ?
ORDER BY updated_at DESC, name ASC
`, cfg.ProjectRoot)
	if err != nil {
		return nil, fmt.Errorf("list launch presets: %w", err)
	}
	defer rows.Close()

	var presets []LaunchPreset
	for rows.Next() {
		var row launchPresetRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Mode, &row.Role, &row.AllowedRoles, &row.Roster, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan launch preset: %w", err)
		}
		req, err := row.request()
		if err != nil {
			return nil, err
		}
		presets = append(presets, LaunchPreset{ID: row.ID, Name: row.Name, Request: req, UpdatedAt: row.UpdatedAt.String})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate launch presets: %w", err)
	}
	return presets, nil
}

func (s *Service) projectMemoryLaunchStore(cfg *appconfig.Config) (projectmemory.LaunchStore, bool, error) {
	if cfg == nil || cfg.Memory.Backend != "shunter" || s == nil || s.rt == nil {
		return nil, false, nil
	}
	if store, ok := s.rt.MemoryBackend.(projectmemory.LaunchStore); ok && store != nil {
		return store, true, nil
	}
	if store, ok := s.rt.BrainBackend.(projectmemory.LaunchStore); ok && store != nil {
		return store, true, nil
	}
	return nil, false, fmt.Errorf("shunter memory backend requires a project memory launch store")
}

func (s *Service) saveProjectMemoryLaunchDraft(ctx context.Context, store projectmemory.LaunchStore, cfg *appconfig.Config, req LaunchRequest) (LaunchDraft, error) {
	req = normalizeLaunchRequest(req)
	allowedRoles, err := marshalStringSlice(req.AllowedRoles)
	if err != nil {
		return LaunchDraft{}, fmt.Errorf("marshal allowed roles: %w", err)
	}
	roster, err := marshalStringSlice(req.Roster)
	if err != nil {
		return LaunchDraft{}, fmt.Errorf("marshal roster: %w", err)
	}
	sourceSpecs, err := marshalStringSlice(req.SourceSpecs)
	if err != nil {
		return LaunchDraft{}, fmt.Errorf("marshal source specs: %w", err)
	}
	updatedAt := time.Now().UTC()
	if err := store.SaveLaunch(ctx, projectmemory.SaveLaunchArgs{
		ProjectID:        cfg.ProjectRoot,
		LaunchID:         currentLaunchDraftID,
		Status:           "draft",
		Mode:             string(req.Mode),
		Role:             req.Role,
		AllowedRolesJSON: allowedRoles,
		RosterJSON:       roster,
		SourceTask:       req.SourceTask,
		SourceSpecsJSON:  sourceSpecs,
		UpdatedAtUS:      uint64(updatedAt.UnixMicro()),
	}); err != nil {
		return LaunchDraft{}, fmt.Errorf("save launch draft: %w", err)
	}
	return LaunchDraft{ID: currentLaunchDraftID, Request: req, UpdatedAt: updatedAt.Format(time.RFC3339)}, nil
}

func (s *Service) loadProjectMemoryLaunchDraft(ctx context.Context, store projectmemory.LaunchStore, cfg *appconfig.Config) (LaunchDraft, bool, error) {
	row, found, err := store.ReadLaunch(ctx, cfg.ProjectRoot, currentLaunchDraftID)
	if err != nil {
		return LaunchDraft{}, false, fmt.Errorf("load launch draft: %w", err)
	}
	if !found || row.Status != "draft" {
		return LaunchDraft{}, false, nil
	}
	req, err := launchFromProjectMemory(row).request()
	if err != nil {
		return LaunchDraft{}, false, err
	}
	return LaunchDraft{ID: currentLaunchDraftID, Request: req, UpdatedAt: unixUSString(row.UpdatedAtUS)}, true, nil
}

func (s *Service) saveProjectMemoryLaunchPreset(ctx context.Context, store projectmemory.LaunchStore, cfg *appconfig.Config, name string, req LaunchRequest) (LaunchPreset, error) {
	req, err := normalizeLaunchPresetRequest(cfg, req)
	if err != nil {
		return LaunchPreset{}, err
	}
	allowedRoles, err := marshalStringSlice(req.AllowedRoles)
	if err != nil {
		return LaunchPreset{}, fmt.Errorf("marshal allowed roles: %w", err)
	}
	roster, err := marshalStringSlice(req.Roster)
	if err != nil {
		return LaunchPreset{}, fmt.Errorf("marshal roster: %w", err)
	}
	updatedAt := time.Now().UTC()
	if err := store.SaveLaunchPreset(ctx, projectmemory.SaveLaunchPresetArgs{
		ProjectID:        cfg.ProjectRoot,
		Name:             name,
		Mode:             string(req.Mode),
		Role:             req.Role,
		AllowedRolesJSON: allowedRoles,
		RosterJSON:       roster,
		UpdatedAtUS:      uint64(updatedAt.UnixMicro()),
	}); err != nil {
		return LaunchPreset{}, fmt.Errorf("save launch preset: %w", err)
	}
	return LaunchPreset{ID: "custom:" + name, Name: name, Request: req, UpdatedAt: updatedAt.Format(time.RFC3339)}, nil
}

func (s *Service) listProjectMemoryLaunchPresets(ctx context.Context, store projectmemory.LaunchStore, cfg *appconfig.Config) ([]LaunchPreset, error) {
	rows, err := store.ListLaunchPresets(ctx, cfg.ProjectRoot)
	if err != nil {
		return nil, fmt.Errorf("list launch presets: %w", err)
	}
	presets := make([]LaunchPreset, 0, len(rows))
	for _, row := range rows {
		req, err := presetFromProjectMemory(row).request()
		if err != nil {
			return nil, err
		}
		presets = append(presets, LaunchPreset{ID: row.PresetID, Name: row.Name, Request: req, UpdatedAt: unixUSString(row.UpdatedAtUS)})
	}
	return presets, nil
}

type launchDraftRow struct {
	ID           string
	Mode         string
	Role         sql.NullString
	AllowedRoles sql.NullString
	Roster       sql.NullString
	SourceTask   sql.NullString
	SourceSpecs  sql.NullString
	UpdatedAt    sql.NullString
}

type launchPresetRow struct {
	ID           string
	Name         string
	Mode         string
	Role         sql.NullString
	AllowedRoles sql.NullString
	Roster       sql.NullString
	UpdatedAt    sql.NullString
}

func launchFromProjectMemory(row projectmemory.Launch) launchDraftRow {
	return launchDraftRow{
		ID:           row.LaunchID,
		Mode:         row.Mode,
		Role:         sql.NullString{String: row.Role, Valid: row.Role != ""},
		AllowedRoles: sql.NullString{String: row.AllowedRolesJSON, Valid: row.AllowedRolesJSON != ""},
		Roster:       sql.NullString{String: row.RosterJSON, Valid: row.RosterJSON != ""},
		SourceTask:   sql.NullString{String: row.SourceTask, Valid: row.SourceTask != ""},
		SourceSpecs:  sql.NullString{String: row.SourceSpecsJSON, Valid: row.SourceSpecsJSON != ""},
		UpdatedAt:    sql.NullString{String: unixUSString(row.UpdatedAtUS), Valid: row.UpdatedAtUS != 0},
	}
}

func presetFromProjectMemory(row projectmemory.LaunchPreset) launchPresetRow {
	return launchPresetRow{
		ID:           row.PresetID,
		Name:         row.Name,
		Mode:         row.Mode,
		Role:         sql.NullString{String: row.Role, Valid: row.Role != ""},
		AllowedRoles: sql.NullString{String: row.AllowedRolesJSON, Valid: row.AllowedRolesJSON != ""},
		Roster:       sql.NullString{String: row.RosterJSON, Valid: row.RosterJSON != ""},
		UpdatedAt:    sql.NullString{String: unixUSString(row.UpdatedAtUS), Valid: row.UpdatedAtUS != 0},
	}
}

func (r launchDraftRow) request() (LaunchRequest, error) {
	allowedRoles, err := unmarshalStringSlice(r.AllowedRoles)
	if err != nil {
		return LaunchRequest{}, fmt.Errorf("unmarshal allowed roles: %w", err)
	}
	roster, err := unmarshalStringSlice(r.Roster)
	if err != nil {
		return LaunchRequest{}, fmt.Errorf("unmarshal roster: %w", err)
	}
	sourceSpecs, err := unmarshalStringSlice(r.SourceSpecs)
	if err != nil {
		return LaunchRequest{}, fmt.Errorf("unmarshal source specs: %w", err)
	}
	return normalizeLaunchRequest(LaunchRequest{
		Mode:         LaunchMode(r.Mode),
		Role:         r.Role.String,
		AllowedRoles: allowedRoles,
		Roster:       roster,
		SourceTask:   r.SourceTask.String,
		SourceSpecs:  sourceSpecs,
	}), nil
}

func (r launchPresetRow) request() (LaunchRequest, error) {
	allowedRoles, err := unmarshalStringSlice(r.AllowedRoles)
	if err != nil {
		return LaunchRequest{}, fmt.Errorf("unmarshal preset allowed roles: %w", err)
	}
	roster, err := unmarshalStringSlice(r.Roster)
	if err != nil {
		return LaunchRequest{}, fmt.Errorf("unmarshal preset roster: %w", err)
	}
	return normalizeLaunchRequest(LaunchRequest{
		Mode:         LaunchMode(r.Mode),
		Role:         r.Role.String,
		AllowedRoles: allowedRoles,
		Roster:       roster,
	}), nil
}

func normalizeLaunchPresetRequest(cfg *appconfig.Config, req LaunchRequest) (LaunchRequest, error) {
	req = normalizeLaunchRequest(req)
	req.SourceTask = ""
	req.SourceSpecs = nil
	req.MaxSteps = 0
	req.MaxResolverLoops = 0
	req.MaxDuration = 0
	req.TokenBudget = 0
	switch req.Mode {
	case LaunchModeOneStep:
		if req.Role == "" {
			return LaunchRequest{}, fmt.Errorf("role is required for one-step preset")
		}
		roleName, _, err := cfg.ResolveAgentRole(req.Role)
		if err != nil {
			return LaunchRequest{}, fmt.Errorf("resolve preset role: %w", err)
		}
		req.Role = roleName
		req.AllowedRoles = nil
		req.Roster = nil
	case LaunchModeOrchestrator:
		roleName, _, err := cfg.ResolveAgentRole("orchestrator")
		if err != nil {
			return LaunchRequest{}, fmt.Errorf("resolve preset orchestrator role: %w", err)
		}
		req.Role = roleName
		req.AllowedRoles = nil
		req.Roster = nil
	case LaunchModeConstrained:
		allowedRoles, err := resolveLaunchAllowedRoles(cfg, req)
		if err != nil {
			return LaunchRequest{}, err
		}
		req.Role = "orchestrator"
		req.AllowedRoles = allowedRoles
		req.Roster = nil
	case LaunchModeManualRoster:
		roster, err := resolveLaunchRoster(cfg, req)
		if err != nil {
			return LaunchRequest{}, err
		}
		req.Role = strings.Join(roster, ",")
		req.Roster = roster
		req.AllowedRoles = nil
	default:
		return LaunchRequest{}, fmt.Errorf("unsupported launch preset mode %s", req.Mode)
	}
	return req, nil
}

func marshalStringSlice(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalStringSlice(value sql.NullString) ([]string, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(value.String), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func unixUSString(value uint64) string {
	if value == 0 {
		return ""
	}
	return time.UnixMicro(int64(value)).UTC().Format(time.RFC3339)
}
