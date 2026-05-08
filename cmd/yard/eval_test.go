package main

import (
	"bytes"
	"encoding/json"
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
	for _, want := range []string{"chain-flow\t", "receipt-contract\t"} {
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
