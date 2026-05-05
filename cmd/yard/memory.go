package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/brain/vault"
	"github.com/ponchione/sodoryard/internal/cmdutil"
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
	)
	return cmd
}

func newYardMemoryMigrateCmd(configPath *string) *cobra.Command {
	var fromVault string
	var toDataDir string
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate documents into Shunter project memory",
		RunE: func(cmd *cobra.Command, args []string) error {
			count, err := runMemoryMigrate(cmd.Context(), *configPath, fromVault, toDataDir)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated brain documents: %d\n", count)
			return nil
		},
	}
	cmd.Flags().StringVar(&fromVault, "from-vault", "", "Source brain vault path (default: configured brain.vault_path)")
	cmd.Flags().StringVar(&toDataDir, "to", "", "Destination Shunter data dir (default: configured memory.shunter_data_dir)")
	return cmd
}

func newYardMemoryVerifyCmd(configPath *string) *cobra.Command {
	var fromVault string
	var toDataDir string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify Shunter project-memory documents against a brain vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runMemoryVerify(cmd.Context(), *configPath, fromVault, toDataDir)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Verified brain documents: %d\n", result.Verified)
			return nil
		},
	}
	cmd.Flags().StringVar(&fromVault, "from-vault", "", "Source brain vault path (default: configured brain.vault_path)")
	cmd.Flags().StringVar(&toDataDir, "to", "", "Shunter data dir to verify (default: configured memory.shunter_data_dir)")
	return cmd
}

func runMemoryMigrate(ctx context.Context, configPath string, fromVault string, toDataDir string) (int, error) {
	source, target, closeTarget, err := openMemoryVaultAndTarget(ctx, configPath, fromVault, toDataDir)
	if err != nil {
		return 0, err
	}
	defer closeTarget()

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
}

func runMemoryVerify(ctx context.Context, configPath string, fromVault string, toDataDir string) (memoryVerifyResult, error) {
	source, target, closeTarget, err := openMemoryVaultAndTarget(ctx, configPath, fromVault, toDataDir)
	if err != nil {
		return memoryVerifyResult{}, err
	}
	defer closeTarget()

	sourcePaths, err := source.ListDocuments(ctx, "")
	if err != nil {
		return memoryVerifyResult{}, fmt.Errorf("list source vault documents: %w", err)
	}
	targetPaths, err := target.ListDocuments(ctx, "")
	if err != nil {
		return memoryVerifyResult{}, fmt.Errorf("list Shunter documents: %w", err)
	}
	sort.Strings(sourcePaths)
	sort.Strings(targetPaths)
	if !equalStringSlices(sourcePaths, targetPaths) {
		return memoryVerifyResult{}, fmt.Errorf("document path mismatch: source=%d Shunter=%d first_diff=%s", len(sourcePaths), len(targetPaths), firstPathDiff(sourcePaths, targetPaths))
	}
	for _, path := range sourcePaths {
		sourceContent, err := source.ReadDocument(ctx, path)
		if err != nil {
			return memoryVerifyResult{}, fmt.Errorf("read source document %s: %w", path, err)
		}
		targetContent, err := target.ReadDocument(ctx, path)
		if err != nil {
			return memoryVerifyResult{}, fmt.Errorf("read Shunter document %s: %w", path, err)
		}
		if sourceContent != targetContent {
			return memoryVerifyResult{}, fmt.Errorf("document content mismatch: %s", path)
		}
	}
	return memoryVerifyResult{Verified: len(sourcePaths)}, nil
}

func openMemoryVaultAndTarget(ctx context.Context, configPath string, fromVault string, toDataDir string) (*vault.Client, *projectmemory.BrainBackend, func(), error) {
	cfg, err := cmdutil.LoadConfig(configPath)
	if err != nil {
		return nil, nil, func() {}, err
	}
	sourcePath := strings.TrimSpace(fromVault)
	if sourcePath == "" {
		sourcePath = cfg.BrainVaultPath()
	} else {
		sourcePath = resolveMemoryCLIPath(cfg.ProjectRoot, sourcePath)
	}
	targetPath := strings.TrimSpace(toDataDir)
	if targetPath == "" {
		targetPath = cfg.MemoryShunterDataDir()
	} else {
		targetPath = resolveMemoryCLIPath(cfg.ProjectRoot, targetPath)
	}

	source, err := vault.New(sourcePath)
	if err != nil {
		return nil, nil, func() {}, fmt.Errorf("open source vault: %w", err)
	}
	target, err := projectmemory.OpenBrainBackend(ctx, projectmemory.Config{DataDir: targetPath, DurableAck: cfg.Memory.DurableAck})
	if err != nil {
		return nil, nil, func() {}, fmt.Errorf("open Shunter project memory: %w", err)
	}
	return source, target, func() { _ = target.Close() }, nil
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
