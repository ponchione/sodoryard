package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/initializer"
)

func newInitCmd() *cobra.Command {
	var configFilename string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the current directory for railway use",
		Long: `Bootstrap the current directory for the railway:
  - Generate yard.yaml with all 13 agent roles seeded
  - Create .yard/ with Shunter project-memory, runtime, and lancedb roots
  - Patch .gitignore with .yard/ and transitional .brain/ entries

Safe to re-run — never overwrites existing files or data.

Stock agent prompts are embedded in the binary, so the generated yard.yaml
works without any prompt-directory setup. Override a role's system_prompt with
an explicit file path only if you want a custom prompt.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd.Context(), cmd, configFilename)
		},
	}
	cmd.Flags().StringVar(&configFilename, "config", "", "Override the generated config filename (default: yard.yaml)")
	return cmd
}

func runInit(ctx context.Context, cmd *cobra.Command, configFilename string) error {
	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "Initializing yard in %s\n\n", projectRoot)

	report, err := initializer.Run(ctx, initializer.Options{
		ProjectRoot:    projectRoot,
		ConfigFilename: configFilename,
	})
	if err != nil {
		return err
	}

	for _, e := range report.Entries {
		switch e.Status {
		case "added":
			_, _ = fmt.Fprintf(out, "  %-10s %s (added %s)\n", e.Kind, e.Path, e.Details)
		default:
			_, _ = fmt.Fprintf(out, "  %-10s %s (%s)\n", e.Kind, e.Path, e.Status)
		}
	}

	_, _ = fmt.Fprintln(out, "\nDone.")
	_, _ = fmt.Fprintln(out, "Next steps:")
	_, _ = fmt.Fprintln(out, "  1. Confirm the provider block matches your auth setup")
	_, _ = fmt.Fprintln(out, "     (default is codex; check Settings > Providers for auth status).")
	_, _ = fmt.Fprintln(out, "  2. Run `yard index` to populate the code search index.")
	_, _ = fmt.Fprintln(out, "  3. Run `yard brain index` after importing or writing brain documents.")
	_, _ = fmt.Fprintln(out, "  4. Run `yard chain start --task \"...\"` to start your first chain.")
	return nil
}
