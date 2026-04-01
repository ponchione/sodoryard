package fileedit

import (
	"context"
	"time"
)

// ReadKind distinguishes full reads from partial previews.
type ReadKind string

const (
	ReadKindFull    ReadKind = "full"
	ReadKindPartial ReadKind = "partial"
)

// EditErrorCode is meant to be stable and model-self-correcting.
type EditErrorCode string

const (
	EditErrorNotReadFirst    EditErrorCode = "not_read_first"
	EditErrorFileMissing     EditErrorCode = "file_missing"
	EditErrorInvalidCreate   EditErrorCode = "invalid_create_via_edit"
	EditErrorZeroMatch       EditErrorCode = "zero_match"
	EditErrorMultipleMatch   EditErrorCode = "multiple_match"
	EditErrorStaleWrite      EditErrorCode = "stale_write"
	EditErrorOldEqualsNew    EditErrorCode = "old_equals_new"
)

// ReadSnapshot captures the state observed at full-read time.
type ReadSnapshot struct {
	Path        string
	Kind        ReadKind
	ReadAt      time.Time
	MTime       time.Time
	SizeBytes   int64
	Fingerprint string
}

// EditRequest models exact-match file editing.
type EditRequest struct {
	Path       string
	OldString  string
	NewString  string
	ReplaceAll bool
}

// MatchResult describes the result of locating old text.
type MatchResult struct {
	Count     int
	Locations []int
}

// EditError is intended to carry deterministic self-correction hints back upstream.
type EditError struct {
	Code          EditErrorCode
	Message       string
	Hints         []string
	CurrentPreview string
}

func (e *EditError) Error() string { return e.Message }

// ReadStateStore persists read snapshots by path and session/turn scope as needed.
type ReadStateStore interface {
	Put(ctx context.Context, snapshot ReadSnapshot) error
	Get(ctx context.Context, path string) (ReadSnapshot, bool, error)
	Clear(ctx context.Context, path string) error
}

// FileSystem abstracts file IO needed by the editor.
type FileSystem interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	Stat(ctx context.Context, path string) (FileInfo, error)
	WriteFile(ctx context.Context, path string, data []byte) error
	MkdirAll(ctx context.Context, path string) error
}

// FileInfo carries the minimum metadata needed for stale-write detection.
type FileInfo interface {
	ModTime() time.Time
	Size() int64
}

// PreconditionChecker validates read-before-edit and stale-write invariants.
type PreconditionChecker interface {
	Check(ctx context.Context, req EditRequest) error
}

// Editor applies exact-match edits after preconditions pass.
type Editor interface {
	Apply(ctx context.Context, req EditRequest) (diffPreview string, err error)
}

// Suggested invariants:
// - Partial reads do not satisfy edit preconditions.
// - File creation through edit is allowed only under an explicit empty-old-string contract, if at all.
// - A stale-write check must happen both before edit planning and immediately before write.
// - Errors should teach the model how to recover: read first, narrow the match, or refresh file state.
