package main

import (
	"github.com/ponchione/sodoryard/internal/chain"
	"github.com/spf13/cobra"
)

func newPauseCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "pause <chain-id>", Short: "Pause a chain", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return setChainStatus(cmd, *configPath, args[0], "paused", chain.EventChainPaused, "paused")
	}}
}

func newResumeCmd(configPath *string) *cobra.Command {
	return &cobra.Command{Use: "resume <chain-id>", Short: "Resume a paused chain", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		flags := defaultChainFlags()
		flags.ChainID = args[0]
		return runChain(cmd.Context(), *configPath, flags, cmd)
	}}
}
