package index

import (
	"time"

	"github.com/ponchione/sodoryard/internal/config"
)

// Options configures one indexing run.
type Options struct {
	ProjectRoot  string
	Full         bool
	IncludeDirty bool
	Config       *config.Config
}

// Result summarizes one indexing run in a reusable CLI/API-safe shape.
type Result struct {
	Mode             string        `json:"mode"`
	PreviousRevision string        `json:"previous_revision,omitempty"`
	CurrentRevision  string        `json:"current_revision,omitempty"`
	ChangedFiles     []string      `json:"changed_files,omitempty"`
	DeletedFiles     []string      `json:"deleted_files,omitempty"`
	IndexedFiles     []string      `json:"indexed_files,omitempty"`
	SkippedFiles     []string      `json:"skipped_files,omitempty"`
	FilesSeen        int           `json:"files_seen"`
	FilesChanged     int           `json:"files_changed"`
	FilesDeleted     int           `json:"files_deleted"`
	FilesSkipped     int           `json:"files_skipped"`
	ChunksWritten    int           `json:"chunks_written"`
	StartedAt        time.Time     `json:"started_at"`
	FinishedAt       time.Time     `json:"finished_at"`
	Duration         time.Duration `json:"duration"`
	WorktreeDirty    bool          `json:"worktree_dirty"`
}
