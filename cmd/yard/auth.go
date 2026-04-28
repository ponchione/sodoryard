package main

import (
	"fmt"

	"github.com/ponchione/sodoryard/internal/cmdutil"
	"github.com/ponchione/sodoryard/internal/localservices"
	"github.com/ponchione/sodoryard/internal/provider/codex"
	rtpkg "github.com/ponchione/sodoryard/internal/runtime"
	"github.com/spf13/cobra"
)

func newYardAuthCmd(configPath *string) *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Inspect provider authentication state",
	}
	authCmd.AddCommand(newYardAuthLoginCmd())
	authCmd.AddCommand(newYardAuthStatusCmd(configPath))
	return authCmd
}

func newYardAuthLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login PROVIDER",
		Short: "Login to a provider and store credentials in Yard's private auth store",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "codex" {
				return fmt.Errorf("unsupported auth provider %q", args[0])
			}
			return codex.LoginCodexDeviceCode(cmd.Context(), cmd.OutOrStdout())
		},
	}
	return cmd
}

func newYardDoctorCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run lightweight auth diagnostics for configured providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return yardRunProviderDiagnostics(cmd, *configPath, false, true)
		},
	}
	return cmd
}

func newYardAuthStatusCmd(configPath *string) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show auth mode, source, and expiry for each provider without probing connectivity",
		RunE: func(cmd *cobra.Command, args []string) error {
			return yardRunProviderDiagnostics(cmd, *configPath, jsonOutput, false)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit auth status as JSON")
	return cmd
}

func yardRunProviderDiagnostics(cmd *cobra.Command, configPath string, jsonOutput bool, includePing bool) error {
	return cmdutil.RunProviderDiagnostics(
		cmd.Context(),
		cmd.OutOrStdout(),
		configPath,
		jsonOutput,
		includePing,
		localservices.NewManager(nil),
		rtpkg.BuildProvider,
	)
}
