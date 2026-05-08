package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/cmdutil"
	"github.com/ponchione/sodoryard/internal/operator"
)

func newYardChainTemplatesCmd(configPath *string) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "templates",
		Short: "List chain launch templates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openYardReadOnlyOperator(cmd.Context(), *configPath)
			if err != nil {
				return err
			}
			defer svc.Close()
			templates, err := svc.ListLaunchTemplates(cmd.Context())
			if err != nil {
				return err
			}
			if jsonOut {
				return cmdutil.WriteJSON(cmd.OutOrStdout(), templates)
			}
			renderYardChainTemplates(cmd.OutOrStdout(), templates)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON output")
	return cmd
}

func renderYardChainTemplates(out io.Writer, templates []operator.LaunchTemplate) {
	for _, template := range templates {
		_, _ = fmt.Fprintf(out, "%s\tmode=%s\tlabel=%q\treceipt_schema=%s\tpreflight=%s\tdefault_roles=%s\n",
			template.ID,
			template.Mode,
			template.Label,
			valueOrUnset(template.ReceiptSchema),
			joinOrUnset(template.PreflightChecks),
			joinOrUnset(template.DefaultRoles),
		)
	}
}

func joinOrUnset(values []string) string {
	if len(values) == 0 {
		return "<unset>"
	}
	return strings.Join(values, ",")
}
