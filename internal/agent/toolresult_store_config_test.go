package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewAgentLoopUsesConfiguredToolResultStoreRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "persisted-tool-results")

	loop := NewAgentLoop(AgentLoopDeps{
		Config: AgentLoopConfig{
			ToolResultStoreRoot: root,
		},
	})

	store, ok := loop.toolResultStore.(*FileToolResultStore)
	if !ok {
		t.Fatalf("toolResultStore type = %T, want *FileToolResultStore", loop.toolResultStore)
	}
	if store.rootDir != root {
		t.Fatalf("store.rootDir = %q, want %q", store.rootDir, root)
	}
}

func TestNewFileToolResultStoreUsesSodoryardTempRootByDefault(t *testing.T) {
	store := NewFileToolResultStore("")

	want := filepath.Join(os.TempDir(), "sodoryard-tool-results")
	if store.rootDir != want {
		t.Fatalf("rootDir = %q, want %q", store.rootDir, want)
	}
}
