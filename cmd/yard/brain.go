package main

import (
	"github.com/ponchione/sodoryard/internal/cmdutil"
	"github.com/spf13/cobra"
)

func newYardBrainCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "brain",
		Short: "Brain operations",
	}
	cmd.AddCommand(newYardBrainIndexCmd(configPath))
	return cmd
}

func newYardBrainIndexCmd(configPath *string) *cobra.Command {
	var jsonOut bool
	var quiet bool
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Rebuild derived brain metadata from project memory",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := cmdutil.RunBrainIndexForConfig(cmd.Context(), *configPath, cmdutil.DefaultBrainIndexDeps())
			if err != nil {
				return err
			}
			if jsonOut {
				return cmdutil.WriteJSON(cmd.OutOrStdout(), result)
			}
			if quiet {
				return nil
			}
			cmdutil.PrintBrainIndexSummary(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON output")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress human-readable brain index summary")
	return cmd
}
