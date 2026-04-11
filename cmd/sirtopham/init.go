package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	appdb "github.com/ponchione/sodoryard/internal/db"
)

// obsidianAppJSON is the minimal Obsidian app.json config.
var obsidianAppJSON = map[string]any{
	"vimMode": false,
}

// obsidianCorePluginsJSON lists the core plugins to enable.
var obsidianCorePluginsJSON = []string{
	"file-explorer",
	"global-search",
	"graph",
	"outline",
	"page-preview",
}

func newInitCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a project for use with sirtopham",
		Long: `Bootstrap the current directory for sirtopham:
  - Generate <project>.yaml config file
  - Create .<project>/ directory with SQLite database and vectorstore roots
  - Create .brain/ vault with Obsidian structure
  - Update .gitignore

Safe to re-run — never overwrites existing files or data.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd.Context(), *configPath)
		},
	}
	return cmd
}

func runInit(ctx context.Context, configPath string) error {
	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	projectName := appconfig.DefaultProjectName(projectRoot)
	stateDir := filepath.Join(projectRoot, "."+projectName)
	if configPath == "" {
		configPath = appconfig.DefaultConfigFilename(projectRoot)
	}
	fmt.Printf("Initializing sirtopham in %s\n\n", projectRoot)

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
	if err := patchGitignore(projectRoot, projectName); err != nil {
		return err
	}

	fmt.Println("\nDone.")
	fmt.Printf("Next steps:\n")
	fmt.Printf("  1. Review %s and configure at least one provider.\n", filepath.Base(configPath))
	fmt.Printf("  2. Place or symlink GGUF models into ops/llm/models/.\n")
	fmt.Printf("  3. Run 'sirtopham llm status' (or 'sirtopham llm up' if you switch local_services.mode to auto).\n")
	fmt.Printf("  4. Run 'sirtopham index'.\n")
	fmt.Printf("  5. Run 'sirtopham serve'.\n")
	return nil
}

// initConfig writes the project config YAML if it does not already exist.
func initConfig(projectRoot, projectName, configPath string) error {
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(projectRoot, configPath)
	}
	configName := filepath.Base(configPath)
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("  config     %s (already exists, skipped)\n", configName)
		return nil
	}

	yaml := generateConfigYAML(projectRoot, projectName)
	if err := os.WriteFile(configPath, []byte(yaml), 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	fmt.Printf("  config     %s (created)\n", configName)
	return nil
}

// generateConfigYAML builds a starter project config YAML.
func generateConfigYAML(projectRoot, projectName string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# sirtopham config for %s\n", projectName))
	b.WriteString(fmt.Sprintf("project_root: %s\n", projectRoot))
	b.WriteString("log_level: info\n")
	b.WriteString("log_format: text\n")
	b.WriteString("\n")
	b.WriteString("server:\n")
	b.WriteString("  host: localhost\n")
	b.WriteString("  port: 8090\n")
	b.WriteString("  dev_mode: false\n")
	b.WriteString("  open_browser: true\n")
	b.WriteString("\n")
	b.WriteString("routing:\n")
	b.WriteString("  default:\n")
	b.WriteString("    provider: anthropic\n")
	b.WriteString("    model: claude-sonnet-4-6-20250514\n")
	b.WriteString("\n")
	b.WriteString("providers:\n")
	b.WriteString("  anthropic:\n")
	b.WriteString("    type: anthropic\n")
	b.WriteString("    model: claude-sonnet-4-6-20250514\n")
	b.WriteString("    api_key_env: ANTHROPIC_API_KEY\n")
	b.WriteString("    context_length: 200000\n")
	b.WriteString("\n")
	b.WriteString("index:\n")
	b.WriteString("  include:\n")
	b.WriteString("    - \"**/*.go\"\n")
	b.WriteString("    - \"**/*.py\"\n")
	b.WriteString("    - \"**/*.ts\"\n")
	b.WriteString("    - \"**/*.tsx\"\n")
	b.WriteString("    - \"**/*.js\"\n")
	b.WriteString("    - \"**/*.jsx\"\n")
	b.WriteString("    - \"**/*.sql\"\n")
	b.WriteString("    - \"**/*.md\"\n")
	b.WriteString("    - \"**/*.yaml\"\n")
	b.WriteString("    - \"**/*.yml\"\n")
	b.WriteString("    - \"**/*.json\"\n")
	b.WriteString("  exclude:\n")
	b.WriteString("    - \"**/.git/**\"\n")
	b.WriteString(fmt.Sprintf("    - \"**/.%s/**\"\n", projectName))
	b.WriteString("    - \"**/.brain/**\"\n")
	b.WriteString("    - \"**/node_modules/**\"\n")
	b.WriteString("    - \"**/vendor/**\"\n")
	b.WriteString("    - \"**/dist/**\"\n")
	b.WriteString("    - \"**/build/**\"\n")
	b.WriteString("    - \"**/coverage/**\"\n")
	b.WriteString("    - \"**/.next/**\"\n")
	b.WriteString("    - \"**/.turbo/**\"\n")
	b.WriteString("    - \"**/*.min.js\"\n")
	b.WriteString("\n")
	b.WriteString("brain:\n")
	b.WriteString("  enabled: true\n")
	b.WriteString("  vault_path: .brain\n")
	b.WriteString("  log_brain_queries: true\n")
	b.WriteString("\n")
	b.WriteString("local_services:\n")
	b.WriteString("  enabled: true\n")
	b.WriteString("  mode: manual\n")
	b.WriteString("  provider: docker-compose\n")
	b.WriteString("  compose_file: ./ops/llm/docker-compose.yml\n")
	b.WriteString("  project_dir: ./ops/llm\n")
	b.WriteString("  required_networks:\n")
	b.WriteString("    - llm-net\n")
	b.WriteString("  auto_create_networks: true\n")
	b.WriteString("  startup_timeout_seconds: 180\n")
	b.WriteString("  healthcheck_interval_seconds: 2\n")
	b.WriteString("  services:\n")
	b.WriteString("    qwen-coder:\n")
	b.WriteString("      base_url: http://localhost:12434\n")
	b.WriteString("      health_path: /health\n")
	b.WriteString("      models_path: /v1/models\n")
	b.WriteString("      required: true\n")
	b.WriteString("    nomic-embed:\n")
	b.WriteString("      base_url: http://localhost:12435\n")
	b.WriteString("      health_path: /health\n")
	b.WriteString("      models_path: /v1/models\n")
	b.WriteString("      required: true\n")

	return b.String()
}

// initDatabase opens the SQLite database and creates the schema if needed.
func initDatabase(ctx context.Context, projectRoot, projectName, stateDir string) error {
	dbPath := filepath.Join(stateDir, "sirtopham.db")

	database, err := appdb.OpenDB(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	created, err := appdb.InitIfNeeded(ctx, database)
	if err != nil {
		return fmt.Errorf("init database schema: %w", err)
	}
	if err := appdb.EnsureMessageSearchIndexesIncludeTools(ctx, database); err != nil {
		return fmt.Errorf("upgrade message search indexes: %w", err)
	}
	if err := appdb.EnsureContextReportsIncludeTokenBudget(ctx, database); err != nil {
		return fmt.Errorf("upgrade context report token budget storage: %w", err)
	}

	dbRelPath := filepath.Join("."+projectName, "sirtopham.db")
	if created {
		fmt.Printf("  database   %s (schema created)\n", dbRelPath)
	} else {
		fmt.Printf("  database   %s (already initialized, skipped)\n", dbRelPath)
	}

	// Ensure project record exists.
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = database.ExecContext(ctx, `
INSERT INTO projects(id, name, root_path, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	root_path = excluded.root_path,
	updated_at = excluded.updated_at
`, projectRoot, projectName, projectRoot, now, now)
	if err != nil {
		return fmt.Errorf("ensure project record: %w", err)
	}

	return nil
}

// initBrainVault creates the .brain/ vault directory with Obsidian structure.
func initBrainVault(projectRoot string) error {
	brainDir := filepath.Join(projectRoot, ".brain")
	if err := mkdirReport(brainDir); err != nil {
		return err
	}

	obsidianDir := filepath.Join(brainDir, ".obsidian")
	if err := mkdirReport(obsidianDir); err != nil {
		return err
	}

	// Write Obsidian config files (skip if they exist).
	obsidianFiles := map[string]any{
		"app.json":               obsidianAppJSON,
		"appearance.json":        map[string]any{},
		"community-plugins.json": []string{},
		"core-plugins.json":      obsidianCorePluginsJSON,
	}
	for name, content := range obsidianFiles {
		fp := filepath.Join(obsidianDir, name)
		if _, err := os.Stat(fp); err == nil {
			continue // already exists
		}
		data, err := json.MarshalIndent(content, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal %s: %w", name, err)
		}
		if err := os.WriteFile(fp, append(data, '\n'), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	fmt.Printf("  vault      %s (obsidian config ready)\n", ".brain/.obsidian/")

	// Create notes/ directory.
	notesDir := filepath.Join(brainDir, "notes")
	if err := mkdirReport(notesDir); err != nil {
		return err
	}

	return nil
}

// patchGitignore appends the project state dir and .brain to .gitignore if not present.
func patchGitignore(projectRoot, projectName string) error {
	gitignorePath := filepath.Join(projectRoot, ".gitignore")

	existing := ""
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(data)
	}

	entries := []string{"." + projectName + "/", ".brain/"}
	var toAdd []string
	for _, entry := range entries {
		if !gitignoreContains(existing, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		fmt.Printf("  gitignore  %s (already has entries, skipped)\n", ".gitignore")
		return nil
	}

	f, err := os.OpenFile(gitignorePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open .gitignore: %w", err)
	}
	defer f.Close()

	// Ensure we start on a new line.
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	if _, err := f.WriteString("\n# sirtopham (auto-generated)\n"); err != nil {
		return err
	}
	for _, entry := range toAdd {
		if _, err := f.WriteString(entry + "\n"); err != nil {
			return err
		}
	}

	fmt.Printf("  gitignore  %s (added %s)\n", ".gitignore", strings.Join(toAdd, ", "))
	return nil
}

// gitignoreContains checks if a .gitignore entry already exists.
func gitignoreContains(content, entry string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == entry || trimmed == strings.TrimSuffix(entry, "/") {
			return true
		}
	}
	return false
}

// mkdirReport creates a directory (and parents) and prints status.
func mkdirReport(dir string) error {
	rel := dir
	if wd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(wd, dir); err == nil {
			rel = r
		}
	}

	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		fmt.Printf("  mkdir      %s (already exists)\n", rel)
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", rel, err)
	}
	fmt.Printf("  mkdir      %s (created)\n", rel)
	return nil
}
