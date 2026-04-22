package tool

import (
	"context"
	"fmt"
	"os"
)

type mutableFileState struct {
	scopeKey string
	absPath  string
	snapshot readSnapshot
	info     os.FileInfo
	data     []byte
}

func mutableFileStore(store readStateStore) readStateStore {
	if store != nil {
		return store
	}
	return defaultReadStateStore
}

func loadMutableFileState(ctx context.Context, projectRoot string, store readStateStore, absPath, displayPath, toolName string) (mutableFileState, *ToolResult) {
	store = mutableFileStore(store)
	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return mutableFileState{}, mutableFileNotFoundResult(projectRoot, displayPath, toolName)
		}
		return mutableFileState{}, &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Error reading file: %v", err),
			Error:   err.Error(),
		}
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return mutableFileState{}, mutableFileNotFoundResult(projectRoot, displayPath, toolName)
		}
		return mutableFileState{}, &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Error stating file: %v", err),
			Error:   err.Error(),
		}
	}
	scopeKey := readScopeKey(ctx)
	snapshot, ok, err := store.Get(ctx, scopeKey, absPath)
	if err != nil {
		return mutableFileState{}, &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Failed to load read state: %v", err),
			Error:   err.Error(),
		}
	}
	if !ok || snapshot.Kind != readKindFull {
		return mutableFileState{}, mutableFileNotReadFirstResult(toolName, displayPath)
	}
	return mutableFileState{
		scopeKey: scopeKey,
		absPath:  absPath,
		snapshot: snapshot,
		info:     info,
		data:     data,
	}, nil
}

func verifyMutableFileSnapshotFresh(ctx context.Context, store readStateStore, projectRoot string, state mutableFileState, displayPath, toolName string) *ToolResult {
	store = mutableFileStore(store)
	info, err := os.Stat(state.absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return mutableFileNotFoundResult(projectRoot, displayPath, toolName)
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Error stating file before write: %v", err),
			Error:   err.Error(),
		}
	}
	data, err := os.ReadFile(state.absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return mutableFileNotFoundResult(projectRoot, displayPath, toolName)
		}
		return &ToolResult{
			Success: false,
			Content: fmt.Sprintf("Error reading file before write: %v", err),
			Error:   err.Error(),
		}
	}
	if state.snapshot.Fingerprint != fileFingerprint(data) || state.snapshot.SizeBytes != info.Size() || !state.snapshot.MTime.Equal(info.ModTime()) {
		clearMutableFileSnapshot(ctx, store, state.scopeKey, state.absPath)
		return mutableFileStaleResult(toolName, displayPath)
	}
	state.info = info
	state.data = data
	return nil
}

func clearMutableFileSnapshot(ctx context.Context, store readStateStore, scopeKey, absPath string) {
	store = mutableFileStore(store)
	_ = store.Clear(ctx, scopeKey, absPath)
}

func mutableFileNotFoundResult(projectRoot, displayPath, toolName string) *ToolResult {
	if toolName == "file_edit" {
		return &ToolResult{Success: false, Content: fileNotFoundError(projectRoot, displayPath), Error: "file not found"}
	}
	return &ToolResult{Success: false, Content: fmt.Sprintf("File not found: %s", displayPath), Error: "file not found"}
}

func mutableFileNotReadFirstResult(toolName, displayPath string) *ToolResult {
	if toolName == "file_edit" {
		return notReadFirstError(displayPath)
	}
	return notReadFirstForToolError(toolName, displayPath)
}

func mutableFileStaleResult(toolName, displayPath string) *ToolResult {
	if toolName == "file_edit" {
		return staleReadError(displayPath)
	}
	return staleWriteReadError(toolName, displayPath)
}
