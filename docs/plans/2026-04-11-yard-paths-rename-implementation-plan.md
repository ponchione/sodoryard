# Yard Paths Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the project-derived state directory (`.<project>/`) and config filename (`<project>.yaml`) with the hardcoded canonical values `.yard/` and `yard.yaml`, and rename the orchestrator database from `sirtopham.db` to `yard.db`.

**Architecture:** Introduce `StateDirName`, `StateDBName`, and `ConfigFilename` constants in the config package. Rewrite `Config.StateDir()`, `Config.DatabasePath()`, and `DefaultConfigFilename()` to use them. Leave `DefaultProjectName()` / `Config.ProjectName()` alone — those remain the basename-derived label used by `internal/codeintel` and `internal/brain/indexer` for tagging code/brain chunks with their owning project. The state directory name and the codeintel project label are now two separate concepts that happened to share one function.

**Tech Stack:** Go 1.22+, standard library, SQLite (sqlc), existing test harness (`make test` with the LanceDB env vars).

---

## Scope and Context

This plan is **Phase 5a** from the brainstorming discussion — the path/filename rename half of Phase 5. It is the deferred debt called out in `NEXT_SESSION_HANDOFF.md` under "Rename debt deferred out of Phase 1". The `yard init` CLI command itself is **Phase 5b** and is NOT in this plan — that lands after Phase 3 (SirTopham orchestrator) because its smoke test requires a working chain runner.

**Why this runs before Phase 3:** Phase 3 introduces a new orchestrator SQLite database. Spec 15 (`docs/specs/15-chain-orchestrator.md`) already references `.yard/sirtopham.db` (pinned in this plan to `.yard/yard.db` per user decision 2026-04-11). If Phase 3 lands before this rename, the orchestrator DB lands in `.sodoryard/sirtopham.db` and has to be migrated mid-flight. Doing the rename first means Phase 3 writes to the canonical path from commit one.

**What this plan does NOT do:**

- Add a new `yard init` or `yard` CLI binary (that is Phase 5b).
- Migrate existing state directories in user projects (handled manually after this lands — the old state is stale anyway because the brain vault rebuild superseded most of it).
- Touch the `.brain/` vault path — `.brain/` stays `.brain/`.
- Change the codeintel chunk labeling (projects are still labeled by directory basename in the vector store).

**Key design decision:** `Config.ProjectName()` is split in meaning, not in API. Today it drives two concerns:
1. The state directory name (`.<project>/`) — this changes to the hardcoded `.yard`.
2. The codeintel chunk label (stored in LanceDB rows as `project_name`) — this stays basename-derived.

After this plan, `ProjectName()` keeps its current behavior and is used **only** for concern 2. Concern 1 switches to the new `StateDirName` constant. This avoids touching `internal/codeintel`, `internal/vectorstore`, and `internal/brain/indexer` at all.

---

## File Structure

**Modified:**

- `internal/config/config.go` — add constants, rewrite `StateDir()`, `DatabasePath()`, `DefaultConfigFilename()`, `requiredIndexExcludePatterns()`.
- `internal/config/config_test.go` — update path assertions (8 call sites).
- `internal/brain/indexstate/state.go` — `Path()` stops calling `DefaultProjectName`, uses `StateDirName` constant.
- `cmd/tidmouth/main.go` — `defaultCLIConfigPath` changes to `yard.yaml`.
- `cmd/tidmouth/config_test.go` — update `TestRootCommandDefaultsConfigPathToSirtophamYAML` (rename and new expected value).
- `cmd/tidmouth/init.go` — rewrite `runInit` to build paths from constants; update `generateConfigYAML`, `initDatabase`, `patchGitignore`, and status messages.
- `cmd/tidmouth/init_test.go` — update `TestRunInitUsesProjectNamedArtifacts` assertions.
- `.gitignore` — add `.yard/`, remove `.sirtopham`, remove stray `/sirtopham` binary entry.
- `internal/provider/router/router.go` — comment reference to `sirtopham.yaml`.

**Renamed (`git mv`):**

- `sirtopham.yaml` → `yard.yaml`
- `sirtopham.yaml.example` → `yard.yaml.example`

**Not touched:**

- `internal/codeintel/**`, `internal/vectorstore/**`, `internal/brain/indexer/**` — they use the basename-derived label, which keeps working.
- Fixture filenames like `filepath.Join(t.TempDir(), "sirtopham.yaml")` in 10+ test files — these are ephemeral tempdir paths, not dependent on the default, so leaving them alone reduces churn. The only test that asserts the *default value* is in `cmd/tidmouth/config_test.go` and is covered.

---

## Task 1: Config package — state dir constants and method rewrites

**Files:**

- Modify: `internal/config/config.go:450-501` and `:550-557`
- Test: `internal/config/config_test.go` — updates to `TestProjectSpecificPathsFollowProjectRootName` (line 405), `TestLoadAppendsRequiredIndexExcludesWhenCustomListOmitsThem` (line 195), `TestNormalizeAddsDerivedStateDirExcludePattern` (line 431), and `TestNormalizeKeepsUniversalRequiredExcludes` (line 444)

**Background:** Today `StateDir()` returns `<root>/.<ProjectName()>` and `DatabasePath()` returns `<stateDir>/sirtopham.db`. `ProjectName()` derives from the project directory basename. After this task, `StateDir()` returns `<root>/.yard` and `DatabasePath()` returns `<root>/.yard/yard.db` regardless of project basename. `ProjectName()` itself is unchanged — callers in `internal/codeintel` and `internal/brain/indexer` still get the basename.

- [ ] **Step 1: Keep `TestConfigLoadsMinimalValidYAML` (line 72-73) as-is**

The `if cfg.DatabasePath() == ""` assertion still holds — the stronger path assertions live in `TestProjectSpecificPathsFollowProjectRootName`. No edit needed here.

- [ ] **Step 2: Rewrite `TestProjectSpecificPathsFollowProjectRootName` (line 405)**

Open `internal/config/config_test.go`. Find the test at line 405. The current body asserts seven things tied to the `eyebox` basename — after this task, `ProjectName()` still returns the basename but the derived paths are all canonical. Rename the test to `TestProjectPathsUseCanonicalYardDirectory` and replace its body:

```go
func TestProjectPathsUseCanonicalYardDirectory(t *testing.T) {
	cfg := &Config{ProjectRoot: filepath.Join(string(filepath.Separator), "tmp", "eyebox")}

	// ProjectName is still derived from the basename because codeintel
	// and brain indexers use it as a label on chunks in the vector store.
	if got := cfg.ProjectName(); got != "eyebox" {
		t.Fatalf("ProjectName() = %q, want eyebox", got)
	}
	// Every other derived path is hardcoded to the canonical .yard name
	// regardless of basename.
	if got := DefaultConfigFilename(cfg.ProjectRoot); got != "yard.yaml" {
		t.Fatalf("DefaultConfigFilename() = %q, want yard.yaml", got)
	}
	if got := cfg.StateDir(); got != filepath.Join(cfg.ProjectRoot, ".yard") {
		t.Fatalf("StateDir() = %q, want %q", got, filepath.Join(cfg.ProjectRoot, ".yard"))
	}
	if got := cfg.DatabasePath(); got != filepath.Join(cfg.ProjectRoot, ".yard", "yard.db") {
		t.Fatalf("DatabasePath() = %q, want %q", got, filepath.Join(cfg.ProjectRoot, ".yard", "yard.db"))
	}
	if got := cfg.CodeLanceDBPath(); got != filepath.Join(cfg.ProjectRoot, ".yard", "lancedb", "code") {
		t.Fatalf("CodeLanceDBPath() = %q, want %q", got, filepath.Join(cfg.ProjectRoot, ".yard", "lancedb", "code"))
	}
	if got := cfg.BrainLanceDBPath(); got != filepath.Join(cfg.ProjectRoot, ".yard", "lancedb", "brain") {
		t.Fatalf("BrainLanceDBPath() = %q, want %q", got, filepath.Join(cfg.ProjectRoot, ".yard", "lancedb", "brain"))
	}
	if got := cfg.GraphDBPath(); got != filepath.Join(cfg.ProjectRoot, ".yard", "graph.db") {
		t.Fatalf("GraphDBPath() = %q, want %q", got, filepath.Join(cfg.ProjectRoot, ".yard", "graph.db"))
	}
}
```

- [ ] **Step 3: Update `TestLoadAppendsRequiredIndexExcludesWhenCustomListOmitsThem` (line 195)**

Find the `wantPatterns` slice at line 215:

```go
wantPatterns := []string{"**/.git/**", "**/.brain/**", "**/node_modules/**", "**/vendor/**", "**/." + cfg.ProjectName() + "/**"}
```

Replace the last element with the hardcoded `"**/.yard/**"`:

```go
wantPatterns := []string{"**/.git/**", "**/.brain/**", "**/node_modules/**", "**/vendor/**", "**/.yard/**"}
```

- [ ] **Step 4: Update `TestNormalizeAddsDerivedStateDirExcludePattern` (line 431)**

Find line 438:

```go
want := "**/.sodoryard/**"
```

Replace with:

```go
want := "**/.yard/**"
```

- [ ] **Step 5: Update `TestNormalizeKeepsUniversalRequiredExcludes` (line 444)**

Find line 451:

```go
for _, want := range []string{"**/.git/**", "**/.brain/**", "**/node_modules/**", "**/vendor/**", "**/.eyebox/**"} {
```

Replace with:

```go
for _, want := range []string{"**/.git/**", "**/.brain/**", "**/node_modules/**", "**/vendor/**", "**/.yard/**"} {
```

- [ ] **Step 6: Run the tests to confirm they fail**

```bash
make test 2>&1 | grep -E "FAIL|eyebox|sodoryard|yard" | head -60
```

Expected failures:

- `TestProjectPathsUseCanonicalYardDirectory` — `DefaultConfigFilename` returns `eyebox.yaml`, `StateDir` contains `.eyebox`, `DatabasePath` contains `sirtopham.db`, etc.
- `TestLoadAppendsRequiredIndexExcludesWhenCustomListOmitsThem` — exclude list is missing `**/.yard/**`.
- `TestNormalizeAddsDerivedStateDirExcludePattern` — exclude list is missing `**/.yard/**`.
- `TestNormalizeKeepsUniversalRequiredExcludes` — exclude list is missing `**/.yard/**`.

The `ProjectName() == "eyebox"` assertion inside `TestProjectPathsUseCanonicalYardDirectory` should still pass (codeintel label unchanged).

- [ ] **Step 7: Add constants and rewrite methods in `internal/config/config.go`**

Find the block around line 13:

```go
const (
	defaultServerHost           = "localhost"
	defaultServerPort           = 8090
	defaultQwenCoderBaseURL     = "http://localhost:12434"
	defaultNomicEmbedBaseURL    = "http://localhost:12435"
	...
)
```

Add these constants after the existing block (not inside it — keep the defaults grouping clean):

```go
// Canonical on-disk names for the yard state directory and its contents.
// These are hardcoded (not derived from project basename) so that every
// project writes yard state to the same relative path, simplifying tooling,
// dashboards, and docker-compose mounts.
const (
	StateDirName   = ".yard"
	StateDBName    = "yard.db"
	ConfigFilename = "yard.yaml"
)
```

Then find `StateDir()`, `DatabasePath()`, and `DefaultConfigFilename()` at lines 468-486 and rewrite:

```go
func DefaultConfigFilename(projectRoot string) string {
	return ConfigFilename
}

func (c *Config) ProjectName() string {
	return DefaultProjectName(c.ProjectRoot)
}

// StateDir returns the absolute path to the yard state directory for this
// project. The name is hardcoded (.yard) and does NOT depend on the project
// directory basename. See ProjectName() for the basename-derived codeintel
// label, which is a separate concept.
func (c *Config) StateDir() string {
	root := c.ProjectRoot
	if root == "" {
		root = "."
	}
	return filepath.Join(root, StateDirName)
}

func (c *Config) DatabasePath() string {
	return filepath.Join(c.StateDir(), StateDBName)
}
```

Leave `DefaultProjectName`, `(c *Config) ProjectName()`, `CodeLanceDBPath`, `BrainLanceDBPath`, `GraphDBPath`, `BrainVaultPath`, and `ResolveAgentRoleSystemPromptPath` unchanged — they either already build on `StateDir()` (so they inherit the new behavior) or serve the codeintel label concern.

- [ ] **Step 8: Update `requiredIndexExcludePatterns()` to use the constant**

Find the method at line 550:

```go
func (c *Config) requiredIndexExcludePatterns() []string {
	patterns := append([]string(nil), requiredIndexExcludePatterns...)
	projectName := strings.TrimSpace(c.ProjectName())
	if projectName != "" {
		patterns = append(patterns, fmt.Sprintf("**/.%s/**", projectName))
	}
	return patterns
}
```

Rewrite it to use the constant directly:

```go
func (c *Config) requiredIndexExcludePatterns() []string {
	patterns := append([]string(nil), requiredIndexExcludePatterns...)
	patterns = append(patterns, "**/"+StateDirName+"/**")
	return patterns
}
```

This both removes a call site for `ProjectName()` that isn't about codeintel labeling and ensures every project's index excludes `.yard/` regardless of basename.

- [ ] **Step 9: Run the tests to confirm they pass**

```bash
make test 2>&1 | tail -30
```

Expected: green. If a test beyond the four updated ones fails, STOP and diagnose — the rename has touched something unexpected.

- [ ] **Step 10: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "refactor(config): hardcode yard state dir and database name

StateDir() now returns .yard/ regardless of project basename, and
DatabasePath() returns .yard/yard.db. ProjectName() is unchanged and
still used by codeintel/brain indexers for chunk labeling.

Part of phase 5a — rename debt cleanup before orchestrator work."
```

---

## Task 2: `indexstate.Path` uses the state dir constant

**Files:**

- Modify: `internal/brain/indexstate/state.go:30-37`

**Background:** `indexstate.Path(projectRoot)` currently calls `appconfig.DefaultProjectName(root)` to build `<root>/.<project>/brain-index-state.json`. That call site isn't about the codeintel label — it's about locating the state file under the yard state dir. Switch it to the constant.

- [ ] **Step 1: Rewrite `Path` to use `appconfig.StateDirName`**

Open `internal/brain/indexstate/state.go`. Find lines 30-37:

```go
func Path(projectRoot string) string {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = "."
	}
	projectName := appconfig.DefaultProjectName(root)
	return filepath.Join(root, "."+projectName, stateFilename)
}
```

Replace with:

```go
func Path(projectRoot string) string {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = "."
	}
	return filepath.Join(root, appconfig.StateDirName, stateFilename)
}
```

- [ ] **Step 2: Verify no other call sites in the package depend on the old derivation**

```bash
grep -rn "DefaultProjectName" internal/brain/indexstate/
```

Expected: no matches (we removed the only one). If the `strings` import is now unused, the compiler will complain — drop it.

- [ ] **Step 3: Run the brain package tests**

```bash
make test 2>&1 | grep -E "indexstate|FAIL" | head -20
```

Expected: green. `indexstate` tests don't currently assert the full path of the state file (checked via `ls internal/brain/indexstate/`), so no test updates should be needed.

- [ ] **Step 4: Commit**

```bash
git add internal/brain/indexstate/state.go
git commit -m "refactor(indexstate): resolve state file under canonical .yard dir"
```

---

## Task 3: CLI default config flag is `yard.yaml`

**Files:**

- Modify: `cmd/tidmouth/main.go:10`
- Test: `cmd/tidmouth/config_test.go:11-20`

- [ ] **Step 1: Update the test assertion in `config_test.go`**

Open `cmd/tidmouth/config_test.go`. Find the test at lines 11-20 and rewrite as:

```go
func TestRootCommandDefaultsConfigPathToYardYAML(t *testing.T) {
	cmd := newRootCmd()
	flag := cmd.PersistentFlags().Lookup("config")
	if flag == nil {
		t.Fatal("config flag missing")
	}
	if got := flag.DefValue; got != "yard.yaml" {
		t.Fatalf("config default = %q, want yard.yaml", got)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
make test 2>&1 | grep -E "TestRootCommandDefaultsConfigPathToYardYAML|FAIL" | head -10
```

Expected: FAIL with `config default = "sirtopham.yaml", want yard.yaml`.

- [ ] **Step 3: Update the constant in `main.go`**

Open `cmd/tidmouth/main.go`. Find line 10:

```go
const defaultCLIConfigPath = "sirtopham.yaml"
```

Replace with:

```go
const defaultCLIConfigPath = appconfig.ConfigFilename
```

This requires an import: add `appconfig "github.com/ponchione/sodoryard/internal/config"` to the import block at the top. Check for a pre-existing import first — if `appconfig` is already imported in `main.go`, reuse it.

Actually the current `main.go` does not import the config package. Two options:

- (a) Import `appconfig` and use the constant.
- (b) Inline the string literal `"yard.yaml"`.

Use **(a)** — it keeps the source of truth in one place.

- [ ] **Step 4: Run the test to confirm it passes**

```bash
make test 2>&1 | grep -E "TestRootCommandDefaultsConfigPathToYardYAML|FAIL" | head -10
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/tidmouth/main.go cmd/tidmouth/config_test.go
git commit -m "refactor(cli): default --config flag to yard.yaml"
```

---

## Task 4: `tidmouth init` generates canonical paths

**Files:**

- Modify: `cmd/tidmouth/init.go` (lines 50-105, 127-207, 210-235, 298-342)
- Test: `cmd/tidmouth/init_test.go:30-75`

**Background:** `tidmouth init` currently builds paths by hand using `DefaultProjectName`. Now that the state dir is hardcoded, init should produce `yard.yaml` + `.yard/yard.db` + `.yard/lancedb/{code,brain}` + gitignore entry `.yard/` regardless of project basename. The config YAML template also needs `**/.yard/**` in the exclude patterns instead of the basename-derived form.

Note: `tidmouth init` is the legacy bootstrap command from pre-Phase-5 days. It is superseded by the future `yard init` (Phase 5b). This plan updates it in place so it produces canonical paths; deciding whether to rename/remove it is Phase 5b's problem.

- [ ] **Step 1: Update `init_test.go` expectations**

Open `cmd/tidmouth/init_test.go`. Rewrite the `TestRunInitUsesProjectNamedArtifacts` test. Rename it to `TestRunInitUsesCanonicalYardPaths` and update the assertions:

```go
func TestRunInitUsesCanonicalYardPaths(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "eyebox")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectRoot): %v", err)
	}
	withWorkingDir(t, projectRoot)

	if err := runInit(context.Background(), ""); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	for _, path := range []string{
		filepath.Join(projectRoot, "yard.yaml"),
		filepath.Join(projectRoot, ".yard"),
		filepath.Join(projectRoot, ".yard", "yard.db"),
		filepath.Join(projectRoot, ".yard", "lancedb", "code"),
		filepath.Join(projectRoot, ".yard", "lancedb", "brain"),
		filepath.Join(projectRoot, ".brain", ".obsidian", "app.json"),
		filepath.Join(projectRoot, ".brain", "notes"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}

	configData, err := os.ReadFile(filepath.Join(projectRoot, "yard.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(yard.yaml): %v", err)
	}
	if !strings.Contains(string(configData), "**/.yard/**") {
		t.Fatalf("expected yard.yaml to exclude .yard, got:\n%s", string(configData))
	}
	for _, want := range []string{"local_services:", "compose_file: ./ops/llm/docker-compose.yml", "base_url: http://localhost:12434", "base_url: http://localhost:12435"} {
		if !strings.Contains(string(configData), want) {
			t.Fatalf("expected yard.yaml to contain %q, got:\n%s", want, string(configData))
		}
	}

	gitignoreData, err := os.ReadFile(filepath.Join(projectRoot, ".gitignore"))
	if err != nil {
		t.Fatalf("ReadFile(.gitignore): %v", err)
	}
	if !strings.Contains(string(gitignoreData), ".yard/") {
		t.Fatalf("expected .gitignore to contain .yard/, got:\n%s", string(gitignoreData))
	}
}
```

The test still uses the `eyebox` basename to prove that canonical yard paths are used **regardless** of basename. That's the core contract.

- [ ] **Step 2: Run the test to confirm it fails**

```bash
make test 2>&1 | grep -E "TestRunInitUsesCanonicalYardPaths|FAIL" | head -20
```

Expected: FAIL — probably at the `yard.yaml` existence check because init still writes `eyebox.yaml`.

- [ ] **Step 3: Rewrite `runInit` in `cmd/tidmouth/init.go`**

Find `runInit` at lines 50-105. Rewrite it:

```go
func runInit(ctx context.Context, configPath string) error {
	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	projectName := appconfig.DefaultProjectName(projectRoot)
	stateDir := filepath.Join(projectRoot, appconfig.StateDirName)
	if configPath == "" {
		configPath = appconfig.ConfigFilename
	}
	fmt.Printf("Initializing tidmouth in %s\n\n", projectRoot)

	// ── 1. Generate project config ────────────────────────────────────
	if err := initConfig(projectRoot, projectName, configPath); err != nil {
		return err
	}

	// ── 2. Create project state directory ─────────────────────────────
	if err := mkdirReport(stateDir); err != nil {
		return err
	}

	// ── 3. Initialize SQLite database ─────────────────────────────────
	if err := initDatabase(ctx, projectRoot, projectName, stateDir); err != nil {
		return err
	}

	// ── 4. Create LanceDB directories ─────────────────────────────────
	codeLanceDir := filepath.Join(stateDir, "lancedb", "code")
	brainLanceDir := filepath.Join(stateDir, "lancedb", "brain")
	if err := mkdirReport(codeLanceDir); err != nil {
		return err
	}
	if err := mkdirReport(brainLanceDir); err != nil {
		return err
	}

	// ── 5. Create .brain/ vault ───────────────────────────────────────
	if err := initBrainVault(projectRoot); err != nil {
		return err
	}

	// ── 6. Update .gitignore ──────────────────────────────────────────
	if err := patchGitignore(projectRoot); err != nil {
		return err
	}

	fmt.Println("\nDone.")
	fmt.Printf("Next steps:\n")
	fmt.Printf("  1. Review %s and configure at least one provider.\n", filepath.Base(configPath))
	fmt.Printf("  2. Place or symlink GGUF models into ops/llm/models/.\n")
	fmt.Printf("  3. Run 'tidmouth llm status' (or 'tidmouth llm up' if you switch local_services.mode to auto).\n")
	fmt.Printf("  4. Run 'tidmouth index'.\n")
	fmt.Printf("  5. Run 'tidmouth serve'.\n")
	return nil
}
```

Note three changes from the original:

1. `stateDir` is built from `appconfig.StateDirName` instead of `"."+projectName`.
2. `configPath` default is `appconfig.ConfigFilename` instead of `DefaultConfigFilename(projectRoot)`.
3. `patchGitignore` now takes only `projectRoot` (no `projectName`).

`projectName` is still computed and passed to `initConfig` and `initDatabase` because it's used inside the YAML template as a human-readable comment and as the `projects.name` DB column — both concerns of the codeintel project label, not the state dir name.

- [ ] **Step 4: Update `initDatabase` to use `yard.db`**

Find lines 210-235. Change line 211:

```go
dbPath := filepath.Join(stateDir, "sirtopham.db")
```

to:

```go
dbPath := filepath.Join(stateDir, appconfig.StateDBName)
```

And line 230:

```go
dbRelPath := filepath.Join("."+projectName, "sirtopham.db")
```

to:

```go
dbRelPath := filepath.Join(appconfig.StateDirName, appconfig.StateDBName)
```

- [ ] **Step 5: Update `generateConfigYAML` exclude pattern**

Find line 167 in `generateConfigYAML`:

```go
b.WriteString(fmt.Sprintf("    - \"**/.%s/**\"\n", projectName))
```

Replace with:

```go
b.WriteString("    - \"**/.yard/**\"\n")
```

The project name is still in the comment header (line 129) — leave that as-is; it's a human-readable label.

- [ ] **Step 6: Rewrite `patchGitignore` signature and entries**

Find `patchGitignore` at lines 298-342. Rewrite the function signature and the `entries` slice:

```go
func patchGitignore(projectRoot string) error {
	gitignorePath := filepath.Join(projectRoot, ".gitignore")

	existing := ""
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(data)
	}

	entries := []string{appconfig.StateDirName + "/", ".brain/"}
	var toAdd []string
	for _, entry := range entries {
		if !gitignoreContains(existing, entry) {
			toAdd = append(toAdd, entry)
		}
	}
	// ... rest of function unchanged
```

Leave the rest of the body (`toAdd` handling, file append, formatting) alone.

- [ ] **Step 7: Run the init test to confirm it passes**

```bash
make test 2>&1 | grep -E "TestRunInitUsesCanonicalYardPaths|FAIL" | head -20
```

Expected: PASS.

- [ ] **Step 8: Run the full test suite**

```bash
make test
```

Expected: green. If anything else fails, STOP — something downstream depended on the old init behavior.

- [ ] **Step 9: Commit**

```bash
git add cmd/tidmouth/init.go cmd/tidmouth/init_test.go
git commit -m "refactor(init): generate canonical .yard/yard.db and yard.yaml

tidmouth init now produces .yard/, yard.yaml, and .yard/yard.db
regardless of project basename. The per-project codeintel label
(projects.name in SQLite) is still derived from the basename."
```

---

## Task 5: Rename repo config files and update exclude patterns

**Files:**

- Renamed: `sirtopham.yaml` → `yard.yaml`, `sirtopham.yaml.example` → `yard.yaml.example`
- Modify: contents of both files to replace `**/.sirtopham/**` → `**/.yard/**`

- [ ] **Step 1: Rename with `git mv`**

```bash
git mv sirtopham.yaml yard.yaml
git mv sirtopham.yaml.example yard.yaml.example
```

- [ ] **Step 2: Update the exclude pattern in `yard.yaml`**

Open `yard.yaml`. Find line 34:

```yaml
    - "**/.sirtopham/**"
```

Replace with:

```yaml
    - "**/.yard/**"
```

- [ ] **Step 3: Update the exclude pattern in `yard.yaml.example`**

```bash
grep -n "\.sirtopham" yard.yaml.example
```

For any matching line, replace `.sirtopham` with `.yard`. Use the Edit tool on each hit.

- [ ] **Step 4: Verify the binary loads the new config**

```bash
make build
./bin/tidmouth config
```

Expected: `config: valid`, `database_path: /home/gernsback/source/sodoryard/.yard/yard.db`, and the rest of the config dump. Note: this reads `yard.yaml` from the current directory via the new default.

If it fails with "config file not found", diagnose — either the rename didn't take or `defaultCLIConfigPath` from Task 3 isn't being respected.

- [ ] **Step 5: Commit**

```bash
git add yard.yaml yard.yaml.example
git commit -m "refactor: rename repo config files to yard.yaml"
```

---

## Task 6: `.gitignore` and stray comment references

**Files:**

- Modify: `.gitignore`
- Modify: `internal/provider/router/router.go` (comment at line 31)

- [ ] **Step 1: Update `.gitignore`**

Open `.gitignore`. Find lines 32-36:

```
.claude
.hermes
.brain
.sirtopham
/sirtopham
```

Rewrite as:

```
.claude
.hermes
.brain
.yard/
.sirtopham/
.sodoryard/
```

Rationale:
- `.yard/` is the new canonical state dir — ignore it.
- `.sirtopham/` is kept as a defensive entry so legacy local state doesn't accidentally get committed during the transition. Can be removed in a future cleanup.
- `.sodoryard/` is added defensively for the same reason (it's the dir name that `ProjectName()` would have produced for the `sodoryard/` repo before Task 1 landed).
- The standalone `/sirtopham` entry (the old repo-root binary path) is removed — `bin/` already covers built binaries.

- [ ] **Step 2: Update the comment in `router.go`**

Open `internal/provider/router/router.go`. Find lines 30-31:

```go
// RouterConfig holds the routing configuration parsed from the routing section
// of sirtopham.yaml.
```

Replace with:

```go
// RouterConfig holds the routing configuration parsed from the routing section
// of yard.yaml.
```

- [ ] **Step 3: Search for any other stray `sirtopham.yaml` references in Go comments or docstrings**

```bash
grep -rn "sirtopham.yaml" --include="*.go" | grep -v "_test.go"
```

Expected: no remaining hits in non-test Go source (test fixture strings in `_test.go` files are intentionally left alone — they're ephemeral TempDir filenames and are not behavior).

If there are hits, update each one. If there are hits in `_test.go` files that AREN'T fixture filenames (e.g., a comment or a default value assertion), update those too.

- [ ] **Step 4: Build and test**

```bash
make build && make test
```

Expected: both green.

- [ ] **Step 5: Commit**

```bash
git add .gitignore internal/provider/router/router.go
git commit -m "chore: update gitignore and comment references to yard paths"
```

---

## Task 7: Verify end-to-end and tag

**Files:** none modified — this is verification + tagging only.

- [ ] **Step 1: Confirm clean working tree**

```bash
git status
```

Expected: `working tree clean` (modulo untracked local state dirs like `.sirtopham/` and `.yard/` which are gitignored).

- [ ] **Step 2: Run `make build` and `make test`**

```bash
make build 2>&1 | tail -5 && make test 2>&1 | tail -20
```

Expected: `bin/tidmouth`, `bin/sirtopham`, `bin/knapford` built; all test packages green.

- [ ] **Step 3: Run the Phase 1 regression smoke test**

The smoke test procedure is documented in `NEXT_SESSION_HANDOFF.md` under "Regression smoke test — run after major Phase 1 steps and at the end". Summary:

1. Confirm llama.cpp services are up (`curl http://localhost:12434/v1/models`, `curl http://localhost:12435/v1/models`).
2. Confirm Codex auth is healthy (`./bin/tidmouth auth status`).
3. Write `/tmp/my-website-smoke.yaml` (template in the handoff doc — the role config uses `correctness-auditor`).
4. Run:

```bash
./bin/tidmouth run \
  --config /tmp/my-website-smoke.yaml \
  --role correctness-auditor \
  --task "Use brain_search to list the notes in the vault. Then use brain_write to create a receipt at receipts/correctness-auditor/smoke-test-p5a.md. The receipt must use valid spec-13 YAML frontmatter with exactly these fields: agent, chain_id, step, verdict, timestamp, turns_used, tokens_used, duration_seconds. Set step to the integer 1. Set verdict to completed. After writing the receipt, stop." \
  --chain-id smoke-test-p5a \
  --max-turns 6 \
  --timeout 3m
```

- [ ] **Step 4: Verify smoke-test pass criteria**

Required:

- Exit code `0`.
- Final stdout line: `receipts/correctness-auditor/smoke-test-p5a.md`.
- File exists at `/home/gernsback/source/my-website/.brain/receipts/correctness-auditor/smoke-test-p5a.md`.
- File has valid spec-13 YAML frontmatter.

Additionally — this is the key Phase 5a check:

- `/home/gernsback/source/my-website/.yard/yard.db` exists after the run (even if created fresh). If it does not exist but `.my-website/sirtopham.db` got touched instead, the rename did NOT take effect and something is still reading the old path — STOP and diagnose.

```bash
ls -la /home/gernsback/source/my-website/.yard/ 2>&1
ls -la /home/gernsback/source/my-website/.my-website/ 2>&1
```

Expected: `.yard/` exists with `yard.db`. `.my-website/` may or may not still exist as legacy; neither state should have been modified by the smoke test.

- [ ] **Step 5: Tag the checkpoint**

```bash
git tag v0.2.1-yard-paths
git tag  # confirm v0.2.1-yard-paths is present alongside v0.1-pre-sodor and v0.2-monorepo-structure
```

Do NOT push. The user pushes tags manually.

- [ ] **Step 6: Write the session handoff update**

Open `NEXT_SESSION_HANDOFF.md` and append a new section at the top (above the Phase 1 content) titled `## Phase 5a complete — yard paths landed`. Two-three sentences summarizing:

- The rename is done; canonical state dir is `.yard/`, DB is `.yard/yard.db`, config file is `yard.yaml`.
- Legacy `.sirtopham/` and per-project `.<basename>/` dirs are not auto-migrated; users should delete them and reindex.
- Next phase is Phase 3 (SirTopham orchestrator), which can now be built against canonical paths from commit one.

Do NOT commit this handoff edit as part of Phase 5a's tag — make it a follow-up commit after the tag. The tag should mark the code state, not the documentation state.

- [ ] **Step 7: Commit the handoff update**

```bash
git add NEXT_SESSION_HANDOFF.md
git commit -m "docs: mark phase 5a complete and point handoff at phase 3"
```

---

## Post-plan: manual cleanup notes

These are NOT plan tasks — they are follow-up actions the user may want to do locally after Phase 5a lands.

1. **Delete stale state dirs.** The sodoryard repo itself has `.sirtopham/` from a previous module name. Safe to `rm -rf .sirtopham/` after confirming `./bin/tidmouth config` reports `.yard/yard.db` as the new database path. `tidmouth index` will need to run again to rebuild vectorstores if you want code intelligence on the sodoryard repo.

2. **Legacy project migration.** For other projects that already use tidmouth (`my-website`, etc.), the old `.<basename>/<basename>.db` state is effectively orphaned after this rename. Either `rm -rf .<basename>/` and re-run `tidmouth init && tidmouth index`, or (if you want to preserve chat history and token metrics) manually `mv .<basename>/sirtopham.db .yard/yard.db` followed by `mkdir -p .yard/lancedb && mv .<basename>/lancedb/* .yard/lancedb/`. Neither path is automated.

3. **Handoff doc refresh.** Update `NEXT_SESSION_HANDOFF.md` to drop the "Rename debt deferred out of Phase 1" section — it is now fully discharged — and add a new section describing what Phase 3 work needs next (covered by the Task 7 handoff update step).
