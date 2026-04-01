package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping git tool tests")
	}
}

// setupGitRepo creates a temp directory with a git repo and one commit.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "checkout", "-b", "main"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup %v failed: %v\n%s", args, err, out)
		}
	}

	// Create a file and commit.
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644)
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "initial commit")

	return dir
}

// --- git_status tests ---

func TestGitStatusNormal(t *testing.T) {
	requireGit(t)
	dir := setupGitRepo(t)

	// Add a dirty file.
	os.WriteFile(filepath.Join(dir, "dirty.go"), []byte("package main\n"), 0o644)

	result, err := GitStatus{}.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Branch: main") {
		t.Fatalf("expected 'Branch: main', got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "dirty.go") {
		t.Fatalf("expected dirty.go in status, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "initial commit") {
		t.Fatalf("expected commit in recent commits, got:\n%s", result.Content)
	}
}

func TestGitStatusCustomRecentCommits(t *testing.T) {
	requireGit(t)
	dir := setupGitRepo(t)

	// Create a couple more commits so we have at least 3.
	for i := 0; i < 3; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%d.go", i)), []byte(fmt.Sprintf("package p%d\n", i)), 0o644)
		cmd := exec.Command("git", "add", ".")
		cmd.Dir = dir
		cmd.Run()
		cmd = exec.Command("git", "commit", "-m", fmt.Sprintf("commit %d", i+2))
		cmd.Dir = dir
		cmd.Run()
	}

	// Request only 1 recent commit.
	result, err := GitStatus{}.Execute(context.Background(), dir,
		json.RawMessage(`{"recent_commits":1}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	// Should show only 1 commit line in recent commits section.
	lines := strings.Split(result.Content, "\n")
	inRecent := false
	commitLines := 0
	for _, line := range lines {
		if strings.Contains(line, "Recent commits:") {
			inRecent = true
			continue
		}
		if inRecent && strings.TrimSpace(line) != "" {
			commitLines++
		}
	}
	if commitLines != 1 {
		t.Fatalf("expected 1 commit line with recent_commits=1, got %d. Output:\n%s", commitLines, result.Content)
	}
}

func TestGitStatusCleanRepo(t *testing.T) {
	requireGit(t)
	dir := setupGitRepo(t)

	result, err := GitStatus{}.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Working tree clean") {
		t.Fatalf("expected 'Working tree clean', got:\n%s", result.Content)
	}
}

func TestGitStatusNotARepo(t *testing.T) {
	requireGit(t)
	dir := t.TempDir() // not a git repo

	result, err := GitStatus{}.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for non-git directory")
	}
	if !strings.Contains(result.Content, "Not a git repository") {
		t.Fatalf("expected 'Not a git repository', got: %s", result.Content)
	}
}

func TestGitStatusSchema(t *testing.T) {
	schema := GitStatus{}.Schema()
	if !json.Valid(schema) {
		t.Fatal("Schema() is not valid JSON")
	}
}

// --- git_diff tests ---

func TestGitDiffWorkingTree(t *testing.T) {
	requireGit(t)
	dir := setupGitRepo(t)

	// Modify a tracked file.
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Updated\n"), 0o644)

	result, err := GitDiff{}.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "-# Test") || !strings.Contains(result.Content, "+# Updated") {
		t.Fatalf("expected diff content, got:\n%s", result.Content)
	}
}

func TestGitDiffStaged(t *testing.T) {
	requireGit(t)
	dir := setupGitRepo(t)

	// Stage a change.
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Staged\n"), 0o644)
	cmd := exec.Command("git", "add", "README.md")
	cmd.Dir = dir
	cmd.Run()

	result, err := GitDiff{}.Execute(context.Background(), dir, json.RawMessage(`{"staged":true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "+# Staged") {
		t.Fatalf("expected staged diff, got:\n%s", result.Content)
	}
}

func TestGitDiffNoChanges(t *testing.T) {
	requireGit(t)
	dir := setupGitRepo(t)

	result, err := GitDiff{}.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "No differences found") {
		t.Fatalf("expected 'No differences found', got: %s", result.Content)
	}
}

func TestGitDiffInvalidRef(t *testing.T) {
	requireGit(t)
	dir := setupGitRepo(t)

	result, err := GitDiff{}.Execute(context.Background(), dir,
		json.RawMessage(`{"ref1":"nonexistent_branch_xyz"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for invalid ref")
	}
	if !strings.Contains(result.Content, "not found") {
		t.Fatalf("expected ref-not-found message, got: %s", result.Content)
	}
}

func TestGitDiffPathScope(t *testing.T) {
	requireGit(t)
	dir := setupGitRepo(t)

	// Create two changes but scope diff to one file.
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Changed\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "other.txt"), []byte("other change\n"), 0o644)
	cmd := exec.Command("git", "add", "other.txt")
	cmd.Dir = dir
	cmd.Run()

	result, err := GitDiff{}.Execute(context.Background(), dir,
		json.RawMessage(`{"path":"README.md"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "README.md") {
		t.Fatalf("expected README.md in diff, got:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "other.txt") {
		t.Fatalf("did NOT expect other.txt in scoped diff, got:\n%s", result.Content)
	}
}

func TestGitDiffNotARepo(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()

	result, err := GitDiff{}.Execute(context.Background(), dir, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for non-git directory")
	}
}

func TestGitDiffSchema(t *testing.T) {
	schema := GitDiff{}.Schema()
	if !json.Valid(schema) {
		t.Fatal("Schema() is not valid JSON")
	}
}

func TestRegisterGitTools(t *testing.T) {
	reg := NewRegistry()
	RegisterGitTools(reg)

	if _, ok := reg.Get("git_status"); !ok {
		t.Fatal("git_status not registered")
	}
	if _, ok := reg.Get("git_diff"); !ok {
		t.Fatal("git_diff not registered")
	}
}
