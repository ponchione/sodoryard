package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	appindex "github.com/ponchione/sodoryard/internal/index"
	"github.com/spf13/cobra"
)

func newYardIndexCmd(configPath *string) *cobra.Command {
	var (
		full    bool
		jsonOut bool
	)

	cmd := &cobra.Command{
		Use:   "index",
		Short: "Index the codebase for semantic retrieval",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := appconfig.Load(*configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			result, err := appindex.Run(cmd.Context(), appindex.Options{
				ProjectRoot:  cfg.ProjectRoot,
				Full:         full,
				IncludeDirty: true,
				Config:       cfg,
			})
			if err != nil {
				return err
			}

			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			yardPrintIndexSummary(cmd.OutOrStdout(), result)
			return nil
		},
	}

	cmd.Flags().BoolVar(&full, "full", false, "Force a full rebuild of the semantic index")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON output")
	return cmd
}

func yardPrintIndexSummary(out io.Writer, result *appindex.Result) {
	if result == nil {
		fmt.Fprintln(out, "index completed")
		return
	}
	fmt.Fprintf(out, "Mode: %s\n", result.Mode)
	fmt.Fprintf(out, "Previous revision: %s\n", yardDisplayValue(result.PreviousRevision))
	fmt.Fprintf(out, "Current revision: %s\n", yardDisplayValue(result.CurrentRevision))
	fmt.Fprintf(out, "Changed files: %d\n", result.FilesChanged)
	fmt.Fprintf(out, "Deleted files: %d\n", result.FilesDeleted)
	fmt.Fprintf(out, "Skipped files: %d\n", result.FilesSkipped)
	fmt.Fprintf(out, "Chunks written: %d\n", result.ChunksWritten)
	fmt.Fprintf(out, "Worktree dirty: %t\n", result.WorktreeDirty)
	fmt.Fprintf(out, "Duration: %s\n", result.Duration.Round(10_000_000))
	if len(result.IndexedFiles) > 0 {
		fmt.Fprintf(out, "Indexed files: %s\n", strings.Join(result.IndexedFiles, ", "))
	}
}

func yardDisplayValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<none>"
	}
	return value
}
