package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadMutableFileStateRejectsPartialSnapshotsWithNotReadFirst(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	ctx := context.Background()
	store := newMemoryReadStateStore()
	scopeKey := readScopeKey(ctx)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if err := store.Put(ctx, snapshotForFile(ctx, path, []byte("hello world\n"), info, readKindPartial, time.Now())); err != nil {
		t.Fatalf("put snapshot: %v", err)
	}

	state, result := loadMutableFileState(ctx, dir, store, path, "file.txt", "file_edit")
	if result == nil {
		t.Fatal("expected not_read_first result")
	}
	if state.snapshot.Kind != "" {
		t.Fatalf("expected zero state on rejection, got %+v", state)
	}
	if result.Error != "not_read_first" {
		t.Fatalf("expected not_read_first, got %q", result.Error)
	}
	if !strings.Contains(result.Content, "full file_read") {
		t.Fatalf("expected full-read guidance, got: %s", result.Content)
	}
	if _, ok, err := store.Get(ctx, scopeKey, path); err != nil {
		t.Fatalf("get snapshot: %v", err)
	} else if !ok {
		t.Fatal("expected partial snapshot to remain stored")
	}
}

func TestVerifyMutableFileSnapshotFreshClearsStaleStoredSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	ctx := context.Background()
	store := newMemoryReadStateStore()
	scopeKey := readScopeKey(ctx)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	snapshot := snapshotForFile(ctx, path, []byte("before\n"), info, readKindFull, time.Now())
	if err := store.Put(ctx, snapshot); err != nil {
		t.Fatalf("put snapshot: %v", err)
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o644); err != nil {
		t.Fatalf("mutate file: %v", err)
	}

	state, result := loadMutableFileState(ctx, dir, store, path, "file.txt", "file_edit")
	if result != nil {
		t.Fatalf("expected load to succeed, got: %+v", result)
	}
	result = verifyMutableFileSnapshotFresh(ctx, store, dir, state, "file.txt", "file_edit")
	if result == nil {
		t.Fatal("expected stale_write result")
	}
	if result.Error != "stale_write" {
		t.Fatalf("expected stale_write, got %q", result.Error)
	}
	if !strings.Contains(result.Content, "Re-run file_read") {
		t.Fatalf("expected recovery guidance, got: %s", result.Content)
	}
	if _, ok, err := store.Get(ctx, scopeKey, path); err != nil {
		t.Fatalf("get snapshot: %v", err)
	} else if ok {
		t.Fatal("expected stale snapshot to be cleared")
	}
}
