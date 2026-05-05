package projectmemory

import (
	"strings"

	"github.com/ponchione/shunter/types"
)

type Launch struct {
	ID               string
	ProjectID        string
	LaunchID         string
	Status           string
	Mode             string
	Role             string
	AllowedRolesJSON string
	RosterJSON       string
	SourceTask       string
	SourceSpecsJSON  string
	CreatedAtUS      uint64
	UpdatedAtUS      uint64
}

type LaunchPreset struct {
	ID               string
	ProjectID        string
	PresetID         string
	Name             string
	Mode             string
	Role             string
	AllowedRolesJSON string
	RosterJSON       string
	CreatedAtUS      uint64
	UpdatedAtUS      uint64
}

func launchRow(launch Launch) types.ProductValue {
	return types.ProductValue{
		types.NewString(launch.ID),
		types.NewString(launch.ProjectID),
		types.NewString(launch.LaunchID),
		types.NewString(launch.Status),
		types.NewString(launch.Mode),
		types.NewString(launch.Role),
		types.NewString(defaultString(launch.AllowedRolesJSON, emptyJSONArray)),
		types.NewString(defaultString(launch.RosterJSON, emptyJSONArray)),
		types.NewString(launch.SourceTask),
		types.NewString(defaultString(launch.SourceSpecsJSON, emptyJSONArray)),
		types.NewUint64(launch.CreatedAtUS),
		types.NewUint64(launch.UpdatedAtUS),
	}
}

func decodeLaunchRow(row types.ProductValue) Launch {
	return Launch{
		ID:               row[0].AsString(),
		ProjectID:        row[1].AsString(),
		LaunchID:         row[2].AsString(),
		Status:           row[3].AsString(),
		Mode:             row[4].AsString(),
		Role:             row[5].AsString(),
		AllowedRolesJSON: row[6].AsString(),
		RosterJSON:       row[7].AsString(),
		SourceTask:       row[8].AsString(),
		SourceSpecsJSON:  row[9].AsString(),
		CreatedAtUS:      row[10].AsUint64(),
		UpdatedAtUS:      row[11].AsUint64(),
	}
}

func launchPresetRow(preset LaunchPreset) types.ProductValue {
	return types.ProductValue{
		types.NewString(preset.ID),
		types.NewString(preset.ProjectID),
		types.NewString(preset.PresetID),
		types.NewString(preset.Name),
		types.NewString(preset.Mode),
		types.NewString(preset.Role),
		types.NewString(defaultString(preset.AllowedRolesJSON, emptyJSONArray)),
		types.NewString(defaultString(preset.RosterJSON, emptyJSONArray)),
		types.NewUint64(preset.CreatedAtUS),
		types.NewUint64(preset.UpdatedAtUS),
	}
}

func decodeLaunchPresetRow(row types.ProductValue) LaunchPreset {
	return LaunchPreset{
		ID:               row[0].AsString(),
		ProjectID:        row[1].AsString(),
		PresetID:         row[2].AsString(),
		Name:             row[3].AsString(),
		Mode:             row[4].AsString(),
		Role:             row[5].AsString(),
		AllowedRolesJSON: row[6].AsString(),
		RosterJSON:       row[7].AsString(),
		CreatedAtUS:      row[8].AsUint64(),
		UpdatedAtUS:      row[9].AsUint64(),
	}
}

func ProjectLaunchID(projectID string, launchID string) string {
	return stableID(strings.Join([]string{"launch", projectID, launchID}, "\x00"))
}

func ProjectLaunchPresetID(projectID string, name string) string {
	return stableID(strings.Join([]string{"launch_preset", projectID, name}, "\x00"))
}
