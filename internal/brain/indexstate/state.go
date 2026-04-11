package indexstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	appconfig "github.com/ponchione/sodoryard/internal/config"
)

const (
	StatusNeverIndexed = "never_indexed"
	StatusClean        = "clean"
	StatusStale        = "stale"
)

const stateFilename = "brain-index-state.json"

type State struct {
	Status        string `json:"status"`
	LastIndexedAt string `json:"last_indexed_at,omitempty"`
	StaleSince    string `json:"stale_since,omitempty"`
	StaleReason   string `json:"stale_reason,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

func Path(projectRoot string) string {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = "."
	}
	projectName := appconfig.DefaultProjectName(root)
	return filepath.Join(root, "."+projectName, stateFilename)
}

func Load(projectRoot string) (State, error) {
	path := Path(projectRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{Status: StatusNeverIndexed}, nil
		}
		return State{}, fmt.Errorf("read brain index state: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode brain index state: %w", err)
	}
	state.normalize()
	return state, nil
}

func MarkFresh(projectRoot string, now time.Time) error {
	state, err := Load(projectRoot)
	if err != nil {
		return err
	}
	timestamp := now.UTC().Format(time.RFC3339)
	state.Status = StatusClean
	state.LastIndexedAt = timestamp
	state.StaleSince = ""
	state.StaleReason = ""
	state.UpdatedAt = timestamp
	return save(projectRoot, state)
}

func MarkStale(projectRoot string, reason string, now time.Time) error {
	state, err := Load(projectRoot)
	if err != nil {
		return err
	}
	timestamp := now.UTC().Format(time.RFC3339)
	state.Status = StatusStale
	if strings.TrimSpace(state.StaleSince) == "" {
		state.StaleSince = timestamp
	}
	state.StaleReason = strings.TrimSpace(reason)
	state.UpdatedAt = timestamp
	return save(projectRoot, state)
}

func save(projectRoot string, state State) error {
	state.normalize()
	path := Path(projectRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir brain index state dir: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode brain index state: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write brain index state: %w", err)
	}
	return nil
}

func (s *State) normalize() {
	if s == nil {
		return
	}
	s.Status = strings.TrimSpace(s.Status)
	if s.Status == "" {
		s.Status = StatusNeverIndexed
	}
	if s.Status == StatusClean {
		s.StaleSince = ""
		s.StaleReason = ""
	}
}
