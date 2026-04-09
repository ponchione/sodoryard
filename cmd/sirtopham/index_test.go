package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	brainindexer "github.com/ponchione/sirtopham/internal/brain/indexer"
	appconfig "github.com/ponchione/sirtopham/internal/config"
	appindex "github.com/ponchione/sirtopham/internal/index"
)

func TestIndexCommandPassesFlagsToService(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	configYAML := "project_root: " + projectRoot + "\nbrain:\n  enabled: false\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	original := runIndexService
	defer func() { runIndexService = original }()

	var gotOpts appindex.Options
	runIndexService = func(_ context.Context, opts appindex.Options) (*appindex.Result, error) {
		gotOpts = opts
		return &appindex.Result{Mode: "full", Duration: time.Second}, nil
	}

	configFlag := configPath
	cmd := newIndexCmd(&configFlag)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--full"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !gotOpts.Full {
		t.Fatal("expected Full=true")
	}
	if !gotOpts.IncludeDirty {
		t.Fatal("expected IncludeDirty=true")
	}
	if gotOpts.Config == nil || gotOpts.Config.ProjectRoot != projectRoot {
		t.Fatalf("Config.ProjectRoot = %v, want %s", gotOpts.Config, projectRoot)
	}
}

func TestIndexCommandJSONOutput(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	configYAML := "project_root: " + projectRoot + "\nbrain:\n  enabled: false\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	original := runIndexService
	defer func() { runIndexService = original }()

	runIndexService = func(context.Context, appindex.Options) (*appindex.Result, error) {
		return &appindex.Result{Mode: "incremental", FilesChanged: 2}, nil
	}

	configFlag := configPath
	cmd := newIndexCmd(&configFlag)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result appindex.Result
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal output: %v\noutput=%s", err, buf.String())
	}
	if result.Mode != "incremental" || result.FilesChanged != 2 {
		t.Fatalf("result = %+v, want incremental/2", result)
	}
}

func TestIndexBrainSubcommandPassesConfigAndPrintsSummary(t *testing.T) {
	projectRoot := t.TempDir()
	vaultPath := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	configYAML := "project_root: " + projectRoot + "\nbrain:\n  enabled: true\n  vault_path: " + vaultPath + "\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	original := runBrainIndexCommand
	defer func() { runBrainIndexCommand = original }()

	var gotCfg *appconfig.Config
	runBrainIndexCommand = func(_ context.Context, cfg *appconfig.Config) (brainindexer.Result, error) {
		gotCfg = cfg
		return brainindexer.Result{DocumentsIndexed: 3, LinksIndexed: 5, DocumentsDeleted: 1, SemanticChunksIndexed: 7, SemanticDocumentsDeleted: 1}, nil
	}

	configFlag := configPath
	cmd := newIndexCmd(&configFlag)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"brain"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotCfg == nil {
		t.Fatal("expected brain reindex config to be passed")
	}
	if gotCfg.ProjectRoot != projectRoot {
		t.Fatalf("ProjectRoot = %q, want %q", gotCfg.ProjectRoot, projectRoot)
	}
	if !gotCfg.Brain.Enabled {
		t.Fatal("expected Brain.Enabled=true")
	}
	if gotCfg.Brain.VaultPath != vaultPath {
		t.Fatalf("Brain.VaultPath = %q, want %q", gotCfg.Brain.VaultPath, vaultPath)
	}
	output := buf.String()
	for _, want := range []string{
		"Brain reindex completed",
		"Brain documents indexed: 3",
		"Brain links indexed: 5",
		"Brain documents deleted: 1",
		"Brain semantic chunks indexed: 7",
		"Brain semantic documents deleted: 1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q\noutput=%s", want, output)
		}
	}
}

func TestIndexBrainSubcommandJSONOutput(t *testing.T) {
	projectRoot := t.TempDir()
	vaultPath := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "sirtopham.yaml")
	configYAML := "project_root: " + projectRoot + "\nbrain:\n  enabled: true\n  vault_path: " + vaultPath + "\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	original := runBrainIndexCommand
	defer func() { runBrainIndexCommand = original }()

	runBrainIndexCommand = func(context.Context, *appconfig.Config) (brainindexer.Result, error) {
		return brainindexer.Result{DocumentsIndexed: 2, LinksIndexed: 4, DocumentsDeleted: 1, SemanticChunksIndexed: 6, SemanticDocumentsDeleted: 1}, nil
	}

	configFlag := configPath
	cmd := newIndexCmd(&configFlag)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"brain", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result brainindexer.Result
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal output: %v\noutput=%s", err, buf.String())
	}
	if result.DocumentsIndexed != 2 || result.LinksIndexed != 4 || result.DocumentsDeleted != 1 || result.SemanticChunksIndexed != 6 || result.SemanticDocumentsDeleted != 1 {
		t.Fatalf("result = %+v, want 2/4/1 plus semantic 6/1", result)
	}
}
