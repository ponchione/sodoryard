package projectmemory

import (
	"fmt"
	"strings"

	"github.com/ponchione/shunter/schema"
	"github.com/ponchione/shunter/types"
)

type SaveLaunchArgs struct {
	ProjectID        string `json:"project_id"`
	LaunchID         string `json:"launch_id"`
	Status           string `json:"status"`
	Mode             string `json:"mode"`
	Role             string `json:"role"`
	AllowedRolesJSON string `json:"allowed_roles_json"`
	RosterJSON       string `json:"roster_json"`
	SourceTask       string `json:"source_task"`
	SourceSpecsJSON  string `json:"source_specs_json"`
	UpdatedAtUS      uint64 `json:"updated_at_us"`
}

type SaveLaunchPresetArgs struct {
	ProjectID        string `json:"project_id"`
	Name             string `json:"name"`
	Mode             string `json:"mode"`
	Role             string `json:"role"`
	AllowedRolesJSON string `json:"allowed_roles_json"`
	RosterJSON       string `json:"roster_json"`
	UpdatedAtUS      uint64 `json:"updated_at_us"`
}

type DeleteLaunchArgs struct {
	ProjectID string `json:"project_id"`
	LaunchID  string `json:"launch_id"`
}

func saveLaunchReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args SaveLaunchArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	projectID := strings.TrimSpace(args.ProjectID)
	if projectID == "" {
		return nil, fmt.Errorf("launch project id is required")
	}
	launchID := strings.TrimSpace(args.LaunchID)
	if launchID == "" {
		return nil, fmt.Errorf("launch id is required")
	}
	mode := strings.TrimSpace(args.Mode)
	if mode == "" {
		return nil, fmt.Errorf("launch mode is required")
	}
	status := firstNonEmpty(strings.TrimSpace(args.Status), "draft")
	id := ProjectLaunchID(projectID, launchID)
	nowUS := nonZeroUS(args.UpdatedAtUS, reducerNowUS(ctx))
	rowID, current, found := findLaunchByID(ctx.DB, id)
	createdAtUS := nowUS
	if found {
		createdAtUS = current.CreatedAtUS
	}
	row := launchRow(Launch{
		ID:               id,
		ProjectID:        projectID,
		LaunchID:         launchID,
		Status:           status,
		Mode:             mode,
		Role:             args.Role,
		AllowedRolesJSON: defaultString(args.AllowedRolesJSON, emptyJSONArray),
		RosterJSON:       defaultString(args.RosterJSON, emptyJSONArray),
		SourceTask:       args.SourceTask,
		SourceSpecsJSON:  defaultString(args.SourceSpecsJSON, emptyJSONArray),
		CreatedAtUS:      createdAtUS,
		UpdatedAtUS:      nowUS,
	})
	if found {
		if _, err := ctx.DB.Update(uint32(tableLaunches), rowID, row); err != nil {
			return nil, err
		}
	} else if _, err := ctx.DB.Insert(uint32(tableLaunches), row); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{OperationID: id})
}

func saveLaunchPresetReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args SaveLaunchPresetArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	projectID := strings.TrimSpace(args.ProjectID)
	if projectID == "" {
		return nil, fmt.Errorf("launch preset project id is required")
	}
	name := strings.TrimSpace(args.Name)
	if name == "" {
		return nil, fmt.Errorf("launch preset name is required")
	}
	mode := strings.TrimSpace(args.Mode)
	if mode == "" {
		return nil, fmt.Errorf("launch preset mode is required")
	}
	id := ProjectLaunchPresetID(projectID, name)
	presetID := "custom:" + name
	nowUS := nonZeroUS(args.UpdatedAtUS, reducerNowUS(ctx))
	rowID, current, found := findLaunchPresetByID(ctx.DB, id)
	createdAtUS := nowUS
	if found {
		createdAtUS = current.CreatedAtUS
	}
	row := launchPresetRow(LaunchPreset{
		ID:               id,
		ProjectID:        projectID,
		PresetID:         presetID,
		Name:             name,
		Mode:             mode,
		Role:             args.Role,
		AllowedRolesJSON: defaultString(args.AllowedRolesJSON, emptyJSONArray),
		RosterJSON:       defaultString(args.RosterJSON, emptyJSONArray),
		CreatedAtUS:      createdAtUS,
		UpdatedAtUS:      nowUS,
	})
	if found {
		if _, err := ctx.DB.Update(uint32(tableLaunchPresets), rowID, row); err != nil {
			return nil, err
		}
	} else if _, err := ctx.DB.Insert(uint32(tableLaunchPresets), row); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{OperationID: id})
}

func deleteLaunchReducer(ctx *schema.ReducerContext, raw []byte) ([]byte, error) {
	var args DeleteLaunchArgs
	if err := decodeReducerArgs(raw, &args); err != nil {
		return nil, err
	}
	projectID := strings.TrimSpace(args.ProjectID)
	if projectID == "" {
		return nil, fmt.Errorf("launch project id is required")
	}
	launchID := strings.TrimSpace(args.LaunchID)
	if launchID == "" {
		return nil, fmt.Errorf("launch id is required")
	}
	id := ProjectLaunchID(projectID, launchID)
	rowID, _, found := findLaunchByID(ctx.DB, id)
	if !found {
		return encodeReducerResult(reducerResult{OperationID: id})
	}
	if err := ctx.DB.Delete(uint32(tableLaunches), rowID); err != nil {
		return nil, err
	}
	return encodeReducerResult(reducerResult{OperationID: id})
}

func findLaunchByID(db types.ReducerDB, id string) (types.RowID, Launch, bool) {
	rowID, row, ok := firstRow(db.SeekIndex(uint32(tableLaunches), uint32(indexLaunchesPrimary), types.NewString(id)))
	if !ok {
		return 0, Launch{}, false
	}
	return rowID, decodeLaunchRow(row), true
}

func findLaunchPresetByID(db types.ReducerDB, id string) (types.RowID, LaunchPreset, bool) {
	rowID, row, ok := firstRow(db.SeekIndex(uint32(tableLaunchPresets), uint32(indexLaunchPresetsPrimary), types.NewString(id)))
	if !ok {
		return 0, LaunchPreset{}, false
	}
	return rowID, decodeLaunchPresetRow(row), true
}
