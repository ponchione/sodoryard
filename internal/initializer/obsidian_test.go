package initializer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureObsidianConfigCreatesAllFiles(t *testing.T) {
	brainDir := filepath.Join(t.TempDir(), ".brain")
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := EnsureObsidianConfig(brainDir); err != nil {
		t.Fatalf("EnsureObsidianConfig: %v", err)
	}

	wantFiles := []string{"app.json", "appearance.json", "community-plugins.json", "core-plugins.json"}
	for _, name := range wantFiles {
		path := filepath.Join(brainDir, ".obsidian", name)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected %s to exist: %v", path, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("expected %s to be non-empty", path)
		}
	}

	// app.json should contain vimMode: false
	appData, err := os.ReadFile(filepath.Join(brainDir, ".obsidian", "app.json"))
	if err != nil {
		t.Fatalf("ReadFile app.json: %v", err)
	}
	var app map[string]any
	if err := json.Unmarshal(appData, &app); err != nil {
		t.Fatalf("Unmarshal app.json: %v", err)
	}
	if app["vimMode"] != false {
		t.Errorf("expected app.json vimMode=false, got %v", app["vimMode"])
	}
}

func TestEnsureObsidianConfigSkipsExistingFiles(t *testing.T) {
	brainDir := filepath.Join(t.TempDir(), ".brain")
	obsDir := filepath.Join(brainDir, ".obsidian")
	if err := os.MkdirAll(obsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	customApp := []byte(`{"vimMode":true,"customField":"keep me"}`)
	if err := os.WriteFile(filepath.Join(obsDir, "app.json"), customApp, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := EnsureObsidianConfig(brainDir); err != nil {
		t.Fatalf("EnsureObsidianConfig: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(obsDir, "app.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(customApp) {
		t.Errorf("expected existing app.json to be preserved, got: %s", got)
	}
}
