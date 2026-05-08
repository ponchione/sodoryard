package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ponchione/sodoryard/internal/cmdutil"
	yardeval "github.com/ponchione/sodoryard/internal/eval"
)

func newYardEvalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Run deterministic Yard evaluation suites",
	}
	cmd.AddCommand(newYardEvalListCmd(), newYardEvalRunCmd())
	return cmd
}

func newYardEvalListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available evaluation suites",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			suites := yardeval.ListSuites()
			if jsonOut {
				return cmdutil.WriteJSON(cmd.OutOrStdout(), suites)
			}
			renderYardEvalSuites(cmd.OutOrStdout(), suites)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON output")
	return cmd
}

func newYardEvalRunCmd() *cobra.Command {
	var jsonOut bool
	var baselinePath string
	cmd := &cobra.Command{
		Use:   "run <suite>",
		Short: "Run an evaluation suite",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := yardeval.Run(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if strings.TrimSpace(baselinePath) != "" {
				baseline, err := loadYardEvalBaseline(baselinePath)
				if err != nil {
					return err
				}
				comparison := yardeval.CompareBaseline(report, baseline)
				comparison.Path = baselinePath
				report.Baseline = &comparison
			}
			if jsonOut {
				if err := cmdutil.WriteJSON(cmd.OutOrStdout(), report); err != nil {
					return err
				}
			} else {
				renderYardEvalReport(cmd.OutOrStdout(), report)
			}
			if report.Status != yardeval.StatusPass {
				return fmt.Errorf("eval suite %s failed", report.Suite)
			}
			if report.Baseline != nil && report.Baseline.Status != yardeval.StatusPass {
				return fmt.Errorf("eval suite %s baseline comparison failed", report.Suite)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON output")
	cmd.Flags().StringVar(&baselinePath, "baseline", "", "Compare output to a saved JSON eval report")
	return cmd
}

func renderYardEvalSuites(out io.Writer, suites []yardeval.SuiteInfo) {
	for _, suite := range suites {
		_, _ = fmt.Fprintf(out, "%s\t%s\n", suite.Name, suite.Description)
	}
}

func renderYardEvalReport(out io.Writer, report yardeval.Report) {
	_, _ = fmt.Fprintf(out, "suite=%s status=%s score=%.2f cases=%d/%d assertions=%d/%d warnings=%d findings=%d\n",
		report.Suite,
		report.Status,
		report.Score,
		report.Totals.CasesPassed,
		report.Totals.Cases,
		report.Totals.AssertionsPassed,
		report.Totals.Assertions,
		report.Totals.Warnings,
		report.Totals.Findings,
	)
	for _, c := range report.Cases {
		_, _ = fmt.Fprintf(out, "case=%s status=%s score=%.2f assertions=%d warnings=%d findings=%d\n",
			c.Name,
			c.Status,
			c.Score,
			len(c.Assertions),
			len(c.Warnings),
			len(c.Findings),
		)
		for _, warning := range c.Warnings {
			_, _ = fmt.Fprintf(out, "warning case=%s %s\n", c.Name, warning)
		}
		for _, assertion := range c.Assertions {
			if assertion.Status == yardeval.StatusPass {
				continue
			}
			message := strings.TrimSpace(assertion.Message)
			if message == "" {
				message = fmt.Sprintf("expected=%v actual=%v", assertion.Expected, assertion.Actual)
			}
			_, _ = fmt.Fprintf(out, "failed case=%s assertion=%s %s\n", c.Name, assertion.Name, message)
		}
	}
	if report.Baseline != nil {
		_, _ = fmt.Fprintf(out, "baseline=%s status=%s diffs=%d\n", valueOrUnset(report.Baseline.Path), report.Baseline.Status, len(report.Baseline.Diffs))
		for _, diff := range report.Baseline.Diffs {
			message := strings.TrimSpace(diff.Message)
			if message == "" {
				message = fmt.Sprintf("expected=%v actual=%v", diff.Expected, diff.Actual)
			}
			_, _ = fmt.Fprintf(out, "baseline_diff field=%s %s\n", diff.Field, message)
		}
	}
}

func loadYardEvalBaseline(path string) (yardeval.Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return yardeval.Report{}, fmt.Errorf("read eval baseline %s: %w", path, err)
	}
	var report yardeval.Report
	if err := json.Unmarshal(data, &report); err != nil {
		return yardeval.Report{}, fmt.Errorf("decode eval baseline %s: %w", path, err)
	}
	return report, nil
}
