package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/brain/vault"
	"github.com/ponchione/sodoryard/internal/cmdutil"
	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/projectmemory"
)

func newYardMemoryCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Project memory operations",
	}
	cmd.AddCommand(
		newYardMemoryMigrateCmd(configPath),
		newYardMemoryVerifyCmd(configPath),
		newYardMemoryExportCmd(configPath),
	)
	return cmd
}

func newYardMemoryMigrateCmd(configPath *string) *cobra.Command {
	var fromVault string
	var fromSQLite string
	var toDataDir string
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate documents into Shunter project memory",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runMemoryMigrate(cmd.Context(), *configPath, fromVault, fromSQLite, toDataDir)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated brain documents: %d\n", result.Documents)
			if strings.TrimSpace(fromSQLite) != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated SQLite conversations: %d\n", result.SQLite.Conversations)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated SQLite messages: %d\n", result.SQLite.Messages)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated SQLite chains: %d\n", result.SQLite.Chains)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated SQLite steps: %d\n", result.SQLite.Steps)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated SQLite events: %d\n", result.SQLite.Events)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated SQLite tool executions: %d\n", result.SQLite.ToolExecutions)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated SQLite provider subcalls: %d\n", result.SQLite.SubCalls)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated SQLite context reports: %d\n", result.SQLite.ContextReports)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated SQLite launch drafts: %d\n", result.SQLite.Launches)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated SQLite launch presets: %d\n", result.SQLite.LaunchPresets)
				if result.SQLite.Skipped > 0 {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Skipped existing SQLite rows: %d\n", result.SQLite.Skipped)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&fromVault, "from-vault", "", "Source brain vault path (default: configured brain.vault_path)")
	cmd.Flags().StringVar(&fromSQLite, "from-sqlite", "", "Source legacy SQLite database path (for example .yard/yard.db)")
	cmd.Flags().StringVar(&toDataDir, "to", "", "Destination Shunter data dir (default: configured memory.shunter_data_dir)")
	return cmd
}

func newYardMemoryVerifyCmd(configPath *string) *cobra.Command {
	var fromVault string
	var fromSQLite string
	var toDataDir string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify imported project-memory state against Shunter",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runMemoryVerify(cmd.Context(), *configPath, fromVault, fromSQLite, toDataDir)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Verified brain documents: %d\n", result.Verified)
			if strings.TrimSpace(fromSQLite) != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Verified SQLite conversations: %d\n", result.SQLite.Conversations)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Verified SQLite messages: %d\n", result.SQLite.Messages)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Verified SQLite chains: %d\n", result.SQLite.Chains)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Verified SQLite steps: %d\n", result.SQLite.Steps)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Verified SQLite events: %d\n", result.SQLite.Events)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Verified SQLite tool executions: %d\n", result.SQLite.ToolExecutions)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Verified SQLite provider subcalls: %d\n", result.SQLite.SubCalls)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Verified SQLite context reports: %d\n", result.SQLite.ContextReports)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Verified SQLite launch drafts: %d\n", result.SQLite.Launches)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Verified SQLite launch presets: %d\n", result.SQLite.LaunchPresets)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&fromVault, "from-vault", "", "Source brain vault path (default: configured brain.vault_path)")
	cmd.Flags().StringVar(&fromSQLite, "from-sqlite", "", "Source legacy SQLite database path (for example .yard/yard.db)")
	cmd.Flags().StringVar(&toDataDir, "to", "", "Shunter data dir to verify (default: configured memory.shunter_data_dir)")
	return cmd
}

func newYardMemoryExportCmd(configPath *string) *cobra.Command {
	var fromDataDir string
	var toVault string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export Shunter project-memory documents to a Markdown vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			count, err := runMemoryExport(cmd.Context(), *configPath, fromDataDir, toVault)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Exported brain documents: %d\n", count)
			return nil
		},
	}
	cmd.Flags().StringVar(&fromDataDir, "from", "", "Source Shunter data dir (default: configured memory.shunter_data_dir)")
	cmd.Flags().StringVar(&toVault, "to-vault", "", "Destination Markdown vault path (default: configured brain.vault_path)")
	return cmd
}

type memoryMigrateResult struct {
	Documents int
	SQLite    projectmemory.ImportSQLiteStateResult
}

func runMemoryMigrate(ctx context.Context, configPath string, fromVault string, fromSQLite string, toDataDir string) (memoryMigrateResult, error) {
	cfg, target, closeTarget, err := openMemoryTarget(ctx, configPath, toDataDir)
	if err != nil {
		return memoryMigrateResult{}, err
	}
	defer closeTarget()

	result := memoryMigrateResult{}
	if strings.TrimSpace(fromVault) != "" || strings.TrimSpace(fromSQLite) == "" {
		source, err := openMemorySourceVault(cfg.ProjectRoot, cfg.BrainVaultPath(), fromVault)
		if err != nil {
			return memoryMigrateResult{}, err
		}
		count, err := migrateMemoryVaultDocuments(ctx, source, target)
		if err != nil {
			return memoryMigrateResult{}, err
		}
		result.Documents = count
	}

	if strings.TrimSpace(fromSQLite) != "" {
		sqlitePath := resolveMemoryCLIPath(cfg.ProjectRoot, fromSQLite)
		sqliteResult, err := migrateSQLiteProjectMemory(ctx, sqlitePath, target)
		if err != nil {
			return memoryMigrateResult{}, err
		}
		result.SQLite = sqliteResult
	}
	return result, nil
}

func migrateMemoryVaultDocuments(ctx context.Context, source *vault.Client, target *projectmemory.BrainBackend) (int, error) {
	paths, err := source.ListDocuments(ctx, "")
	if err != nil {
		return 0, fmt.Errorf("list source vault documents: %w", err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		content, err := source.ReadDocument(ctx, path)
		if err != nil {
			return 0, fmt.Errorf("read source document %s: %w", path, err)
		}
		if err := target.WriteDocument(ctx, path, content); err != nil {
			return 0, fmt.Errorf("write Shunter document %s: %w", path, err)
		}
	}
	return len(paths), nil
}

type memoryVerifyResult struct {
	Verified int
	SQLite   projectmemory.ImportSQLiteStateResult
}

func runMemoryVerify(ctx context.Context, configPath string, fromVault string, fromSQLite string, toDataDir string) (memoryVerifyResult, error) {
	cfg, target, closeTarget, err := openMemoryTarget(ctx, configPath, toDataDir)
	if err != nil {
		return memoryVerifyResult{}, err
	}
	defer closeTarget()

	result := memoryVerifyResult{}
	if strings.TrimSpace(fromVault) != "" || strings.TrimSpace(fromSQLite) == "" {
		source, err := openMemorySourceVault(cfg.ProjectRoot, cfg.BrainVaultPath(), fromVault)
		if err != nil {
			return memoryVerifyResult{}, err
		}
		count, err := verifyMemoryVaultDocuments(ctx, source, target)
		if err != nil {
			return memoryVerifyResult{}, err
		}
		result.Verified = count
	}
	if strings.TrimSpace(fromSQLite) != "" {
		sqlitePath := resolveMemoryCLIPath(cfg.ProjectRoot, fromSQLite)
		sqliteResult, err := verifySQLiteProjectMemory(ctx, sqlitePath, target)
		if err != nil {
			return memoryVerifyResult{}, err
		}
		result.SQLite = sqliteResult
	}
	return result, nil
}

func verifyMemoryVaultDocuments(ctx context.Context, source *vault.Client, target *projectmemory.BrainBackend) (int, error) {
	sourcePaths, err := source.ListDocuments(ctx, "")
	if err != nil {
		return 0, fmt.Errorf("list source vault documents: %w", err)
	}
	targetPaths, err := target.ListDocuments(ctx, "")
	if err != nil {
		return 0, fmt.Errorf("list Shunter documents: %w", err)
	}
	sort.Strings(sourcePaths)
	sort.Strings(targetPaths)
	if !equalStringSlices(sourcePaths, targetPaths) {
		return 0, fmt.Errorf("document path mismatch: source=%d Shunter=%d first_diff=%s", len(sourcePaths), len(targetPaths), firstPathDiff(sourcePaths, targetPaths))
	}
	for _, path := range sourcePaths {
		sourceContent, err := source.ReadDocument(ctx, path)
		if err != nil {
			return 0, fmt.Errorf("read source document %s: %w", path, err)
		}
		targetContent, err := target.ReadDocument(ctx, path)
		if err != nil {
			return 0, fmt.Errorf("read Shunter document %s: %w", path, err)
		}
		if sourceContent != targetContent {
			return 0, fmt.Errorf("document content mismatch: %s", path)
		}
	}
	return len(sourcePaths), nil
}

func openMemoryTarget(ctx context.Context, configPath string, toDataDir string) (*appconfig.Config, *projectmemory.BrainBackend, func(), error) {
	cfg, err := cmdutil.LoadConfig(configPath)
	if err != nil {
		return nil, nil, func() {}, err
	}
	targetPath := strings.TrimSpace(toDataDir)
	if targetPath == "" {
		targetPath = cfg.MemoryShunterDataDir()
	} else {
		targetPath = resolveMemoryCLIPath(cfg.ProjectRoot, targetPath)
	}

	target, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: targetPath, DurableAck: cfg.Memory.DurableAck})
	if err != nil {
		return nil, nil, func() {}, fmt.Errorf("open Shunter project memory: %w", err)
	}
	return cfg, target, func() { _ = target.Close() }, nil
}

func openMemorySourceVault(projectRoot string, configuredVaultPath string, fromVault string) (*vault.Client, error) {
	sourcePath := strings.TrimSpace(fromVault)
	if sourcePath == "" {
		sourcePath = configuredVaultPath
	} else {
		sourcePath = resolveMemoryCLIPath(projectRoot, sourcePath)
	}
	source, err := vault.New(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("open source vault: %w", err)
	}
	return source, nil
}

func runMemoryExport(ctx context.Context, configPath string, fromDataDir string, toVault string) (int, error) {
	cfg, err := cmdutil.LoadConfig(configPath)
	if err != nil {
		return 0, err
	}
	sourcePath := strings.TrimSpace(fromDataDir)
	if sourcePath == "" {
		sourcePath = cfg.MemoryShunterDataDir()
	} else {
		sourcePath = resolveMemoryCLIPath(cfg.ProjectRoot, sourcePath)
	}
	targetPath := strings.TrimSpace(toVault)
	if targetPath == "" {
		targetPath = cfg.BrainVaultPath()
	} else {
		targetPath = resolveMemoryCLIPath(cfg.ProjectRoot, targetPath)
	}
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		return 0, fmt.Errorf("create export vault: %w", err)
	}
	source, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: sourcePath, DurableAck: cfg.Memory.DurableAck})
	if err != nil {
		return 0, fmt.Errorf("open Shunter project memory: %w", err)
	}
	defer source.Close()
	target, err := vault.New(targetPath)
	if err != nil {
		return 0, fmt.Errorf("open export vault: %w", err)
	}
	paths, err := source.ListDocuments(ctx, "")
	if err != nil {
		return 0, fmt.Errorf("list Shunter documents: %w", err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		content, err := source.ReadDocument(ctx, path)
		if err != nil {
			return 0, fmt.Errorf("read Shunter document %s: %w", path, err)
		}
		if err := target.WriteDocument(ctx, path, content); err != nil {
			return 0, fmt.Errorf("write export document %s: %w", path, err)
		}
	}
	return len(paths), nil
}

func equalStringSlices(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func firstPathDiff(a []string, b []string) string {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			return fmt.Sprintf("source[%d]=%q Shunter[%d]=%q", i, a[i], i, b[i])
		}
	}
	if len(a) != len(b) {
		return fmt.Sprintf("source_len=%d Shunter_len=%d", len(a), len(b))
	}
	return "<none>"
}

func resolveMemoryCLIPath(projectRoot string, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = "."
	}
	return filepath.Join(root, path)
}
