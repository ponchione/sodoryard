package main

import (
	"github.com/ponchione/sodoryard/internal/cmdutil"
	"github.com/ponchione/sodoryard/internal/localservices"
	"github.com/spf13/cobra"
)

func newYardLLMCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{Use: "llm", Short: "Manage repo-owned local LLM services"}
	cmd.AddCommand(newYardLLMStatusCmd(configPath), newYardLLMUpCmd(configPath), newYardLLMDownCmd(configPath), newYardLLMLogsCmd(configPath))
	return cmd
}

func newYardLLMStatusCmd(configPath *string) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show local LLM stack status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RunLLMStatus(cmd.Context(), cmd.OutOrStdout(), *configPath, jsonOutput, localservices.NewManager(nil))
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stack status as JSON")
	return cmd
}

func newYardLLMUpCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Ensure required local LLM services are up",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RunLLMUp(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), *configPath, localservices.NewManager(nil))
		},
	}
}

func newYardLLMDownCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Stop managed local LLM services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RunLLMDown(cmd.Context(), cmd.OutOrStdout(), *configPath, localservices.NewManager(nil))
		},
	}
}

func newYardLLMLogsCmd(configPath *string) *cobra.Command {
	var tail int
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show recent logs from managed local LLM services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RunLLMLogs(cmd.Context(), cmd.OutOrStdout(), *configPath, tail, localservices.NewManager(nil))
		},
	}
	cmd.Flags().IntVar(&tail, "tail", 100, "Number of recent log lines")
	return cmd
}
