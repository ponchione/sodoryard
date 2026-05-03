package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	appconfig "github.com/ponchione/sodoryard/internal/config"
	"github.com/ponchione/sodoryard/internal/operator"
	tuiapp "github.com/ponchione/sodoryard/internal/tui"
)

var runYardTUI = func(ctx context.Context, svc tuiapp.Operator, opts tuiapp.Options) error {
	return tuiapp.Run(ctx, svc, opts)
}

func newYardTUICmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:    "tui",
		Short:  "Open the terminal operator console",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runYardTUICommand(cmd, *configPath)
		},
	}
}

func runYardTUICommand(cmd *cobra.Command, configPath string) error {
	svc, err := openYardOperator(cmd.Context(), configPath)
	if err != nil {
		return fmt.Errorf("open operator: %w", err)
	}
	defer svc.Close()
	return runYardTUI(cmd.Context(), svc, tuiapp.Options{WebBaseURL: yardTUIWebBaseURL(configPath)})
}

var _ tuiapp.Operator = (*operator.Service)(nil)

func yardTUIWebBaseURL(configPath string) string {
	if strings.TrimSpace(configPath) == "" {
		configPath = appconfig.DefaultConfigFilename()
	}
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		cfg = appconfig.Default()
	}
	host := yardTUIWebDisplayHost(cfg.Server.Host)
	port := cfg.Server.Port
	if port <= 0 {
		return ""
	}
	return (&url.URL{Scheme: "http", Host: net.JoinHostPort(host, strconv.Itoa(port))}).String()
}

func yardTUIWebDisplayHost(host string) string {
	host = strings.TrimSpace(host)
	switch host {
	case "", "0.0.0.0", "::":
		return "localhost"
	default:
		return strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}
}
