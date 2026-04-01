package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"
)

type readKind string

const (
	readKindFull    readKind = "full"
	readKindPartial readKind = "partial"
)

type readSnapshot struct {
	ScopeKey     string
	Path         string
	Kind         readKind
	ReadAt       time.Time
	MTime        time.Time
	SizeBytes    int64
	Fingerprint  string
}

type readStateStore interface {
	Put(ctx context.Context, snapshot readSnapshot) error
	Get(ctx context.Context, scopeKey, path string) (readSnapshot, bool, error)
	Clear(ctx context.Context, scopeKey, path string) error
}

type memoryReadStateStore struct {
	mu        sync.RWMutex
	snapshots map[string]readSnapshot
}

var defaultReadStateStore readStateStore = newMemoryReadStateStore()

func newMemoryReadStateStore() *memoryReadStateStore {
	return &memoryReadStateStore{snapshots: make(map[string]readSnapshot)}
}

func (s *memoryReadStateStore) Put(_ context.Context, snapshot readSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[readStateKey(snapshot.ScopeKey, snapshot.Path)] = snapshot
	return nil
}

func (s *memoryReadStateStore) Get(_ context.Context, scopeKey, path string) (readSnapshot, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot, ok := s.snapshots[readStateKey(scopeKey, path)]
	return snapshot, ok, nil
}

func (s *memoryReadStateStore) Clear(_ context.Context, scopeKey, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.snapshots, readStateKey(scopeKey, path))
	return nil
}

func readStateKey(scopeKey, path string) string {
	return scopeKey + "::" + path
}

func readScopeKey(ctx context.Context) string {
	if meta, ok := ExecutionMetaFromContext(ctx); ok {
		return meta.ConversationID
	}
	return "__default__"
}

func fileFingerprint(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func snapshotForFile(ctx context.Context, absPath string, data []byte, info os.FileInfo, kind readKind, now time.Time) readSnapshot {
	return readSnapshot{
		ScopeKey:    readScopeKey(ctx),
		Path:        absPath,
		Kind:        kind,
		ReadAt:      now,
		MTime:       info.ModTime(),
		SizeBytes:   info.Size(),
		Fingerprint: fileFingerprint(data),
	}
}

func staleReadError(path string) *ToolResult {
	return &ToolResult{
		Success: false,
		Content: fmt.Sprintf("File changed since the last full read: %s. Re-run file_read on the full file, then retry file_edit.", path),
		Error:   "stale_write",
	}
}

func notReadFirstError(path string) *ToolResult {
	return &ToolResult{
		Success: false,
		Content: fmt.Sprintf("file_edit requires a prior full file_read of %s. Read the entire file first, then retry the edit.", path),
		Error:   "not_read_first",
	}
}
