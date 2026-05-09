package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	yardeval "github.com/ponchione/sodoryard/internal/eval"
)

func TestYardEvalListCommandPrintsSuites(t *testing.T) {
	var out bytes.Buffer
	cmd := newYardEvalListCmd()
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, want := range []string{"chain-flow\t", "receipt-contract\t", "retrieval-contract\t", "tool-contract\t"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("stdout = %q, want %q", out.String(), want)
		}
	}
}

func TestYardEvalRunCommandPrintsHumanReport(t *testing.T) {
	var out bytes.Buffer
	cmd := newYardEvalRunCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"receipt-contract"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, want := range []string{
		"suite=receipt-contract status=pass",
		"case=coder-valid status=pass",
		"warning case=legacy-missing-schema-warning missing schema_version",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("stdout = %q, want %q", out.String(), want)
		}
	}
}

func TestYardEvalRunCommandPrintsJSONReport(t *testing.T) {
	var out bytes.Buffer
	cmd := newYardEvalRunCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"chain-flow", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	var report yardeval.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("json output decode failed: %v\n%s", err, out.String())
	}
	if report.Suite != "chain-flow" || report.Status != yardeval.StatusPass {
		t.Fatalf("report = %+v, want passing chain-flow report", report)
	}
}

func TestYardEvalRunCommandComparesBaseline(t *testing.T) {
	report, err := yardeval.Run(context.Background(), "receipt-contract")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	baselinePath := writeEvalBaseline(t, report)

	var out bytes.Buffer
	cmd := newYardEvalRunCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"receipt-contract", "--baseline", baselinePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v\nstdout=%s", err, out.String())
	}
	if !strings.Contains(out.String(), "baseline="+baselinePath+" status=pass diffs=0") {
		t.Fatalf("stdout = %q, want passing baseline line", out.String())
	}
}

func TestYardEvalRunCommandWritesBaseline(t *testing.T) {
	baselinePath := filepath.Join(t.TempDir(), "baselines", "receipt-contract.json")

	var out bytes.Buffer
	cmd := newYardEvalRunCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"receipt-contract", "--write-baseline", baselinePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v\nstdout=%s", err, out.String())
	}
	if !strings.Contains(out.String(), "baseline_written="+baselinePath) {
		t.Fatalf("stdout = %q, want baseline_written line", out.String())
	}

	written := readEvalBaseline(t, baselinePath)
	if written.Suite != "receipt-contract" || written.Status != yardeval.StatusPass {
		t.Fatalf("written report = %+v, want passing receipt-contract report", written)
	}
	if written.Baseline != nil {
		t.Fatalf("written baseline comparison = %+v, want nil", written.Baseline)
	}
}

func TestYardEvalRunCommandWritesBaselineWithJSONOutput(t *testing.T) {
	baselinePath := filepath.Join(t.TempDir(), "chain-flow.json")

	var out bytes.Buffer
	cmd := newYardEvalRunCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"chain-flow", "--json", "--write-baseline", baselinePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v\nstdout=%s", err, out.String())
	}

	var stdoutReport yardeval.Report
	if err := json.Unmarshal(out.Bytes(), &stdoutReport); err != nil {
		t.Fatalf("json output decode failed: %v\n%s", err, out.String())
	}
	written := readEvalBaseline(t, baselinePath)
	if written.Suite != stdoutReport.Suite || written.Status != stdoutReport.Status {
		t.Fatalf("written report = %+v, stdout report = %+v", written, stdoutReport)
	}
}

func TestYardEvalRunCommandFailsOnBaselineDiff(t *testing.T) {
	report, err := yardeval.Run(context.Background(), "receipt-contract")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	report.Totals.Cases++
	baselinePath := writeEvalBaseline(t, report)

	var out bytes.Buffer
	cmd := newYardEvalRunCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"receipt-contract", "--baseline", baselinePath})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "baseline comparison failed") {
		t.Fatalf("Execute error = %v, want baseline comparison failure\nstdout=%s", err, out.String())
	}
	if !strings.Contains(out.String(), "baseline_diff field=totals") {
		t.Fatalf("stdout = %q, want baseline totals diff", out.String())
	}
}

func writeEvalBaseline(t *testing.T, report yardeval.Report) string {
	t.Helper()
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal baseline failed: %v", err)
	}
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile baseline failed: %v", err)
	}
	return path
}

func readEvalBaseline(t *testing.T, path string) yardeval.Report {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile baseline failed: %v", err)
	}
	var report yardeval.Report
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("Unmarshal baseline failed: %v", err)
	}
	return report
}
