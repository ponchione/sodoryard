package tool

import (
	"strings"
	"testing"
)

func TestParseGoTestJSON_AllPass(t *testing.T) {
	input := `{"Action":"run","Test":"TestFoo","Package":"example.com/pkg"}
{"Action":"output","Test":"TestFoo","Package":"example.com/pkg","Output":"=== RUN   TestFoo\n"}
{"Action":"pass","Test":"TestFoo","Package":"example.com/pkg","Elapsed":0.001}
{"Action":"run","Test":"TestBar","Package":"example.com/pkg"}
{"Action":"output","Test":"TestBar","Package":"example.com/pkg","Output":"=== RUN   TestBar\n"}
{"Action":"pass","Test":"TestBar","Package":"example.com/pkg","Elapsed":0.002}
{"Action":"pass","Package":"example.com/pkg","Elapsed":0.003}`

	r := parseGoTestJSON(input)

	if r.Ecosystem != "go" {
		t.Errorf("Ecosystem = %q, want 'go'", r.Ecosystem)
	}
	if r.Passed != 2 {
		t.Errorf("Passed = %d, want 2", r.Passed)
	}
	if r.Failed != 0 {
		t.Errorf("Failed = %d, want 0", r.Failed)
	}
	if r.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", r.Skipped)
	}
	if len(r.Failures) != 0 {
		t.Errorf("Failures = %v, want empty", r.Failures)
	}
	if len(r.BuildErrors) != 0 {
		t.Errorf("BuildErrors = %v, want empty", r.BuildErrors)
	}
	if !strings.Contains(r.Summary, "GO PASS") {
		t.Errorf("Summary = %q, expected GO PASS", r.Summary)
	}
}

func TestParseGoTestJSON_WithFailure(t *testing.T) {
	input := `{"Action":"run","Test":"TestPass","Package":"example.com/mypkg"}
{"Action":"pass","Test":"TestPass","Package":"example.com/mypkg","Elapsed":0.001}
{"Action":"run","Test":"TestFail","Package":"example.com/mypkg"}
{"Action":"output","Test":"TestFail","Package":"example.com/mypkg","Output":"    foo_test.go:12: got 0, want 1\n"}
{"Action":"fail","Test":"TestFail","Package":"example.com/mypkg","Elapsed":0.002}
{"Action":"fail","Package":"example.com/mypkg","Elapsed":0.003}`

	r := parseGoTestJSON(input)

	if r.Passed != 1 {
		t.Errorf("Passed = %d, want 1", r.Passed)
	}
	if r.Failed != 1 {
		t.Errorf("Failed = %d, want 1", r.Failed)
	}
	if len(r.Failures) != 1 {
		t.Fatalf("len(Failures) = %d, want 1", len(r.Failures))
	}

	f := r.Failures[0]
	if f.Test != "TestFail" {
		t.Errorf("Failure.Test = %q, want 'TestFail'", f.Test)
	}
	if f.Package != "example.com/mypkg" {
		t.Errorf("Failure.Package = %q, want 'example.com/mypkg'", f.Package)
	}
	if !strings.Contains(f.Output, "got 0, want 1") {
		t.Errorf("Failure.Output = %q, expected 'got 0, want 1'", f.Output)
	}
	if !strings.Contains(r.Summary, "GO FAIL") {
		t.Errorf("Summary = %q, expected GO FAIL", r.Summary)
	}
	// Build errors should be empty since tests ran.
	if len(r.BuildErrors) != 0 {
		t.Errorf("BuildErrors = %v, want empty", r.BuildErrors)
	}
}

func TestParseGoTestJSON_BuildError(t *testing.T) {
	// Package-level fail with output but no Test field actions (no tests ran).
	input := `{"Action":"output","Package":"example.com/broken","Output":"# example.com/broken\n"}
{"Action":"output","Package":"example.com/broken","Output":"./broken.go:5:2: undefined: missingFunc\n"}
{"Action":"fail","Package":"example.com/broken","Elapsed":0.1}`

	r := parseGoTestJSON(input)

	if r.Passed != 0 {
		t.Errorf("Passed = %d, want 0", r.Passed)
	}
	if r.Failed != 0 {
		t.Errorf("Failed = %d, want 0 (build error, not test failure)", r.Failed)
	}
	if len(r.BuildErrors) == 0 {
		t.Fatal("BuildErrors is empty, expected at least one build error")
	}
	combined := strings.Join(r.BuildErrors, "\n")
	if !strings.Contains(combined, "undefined") && !strings.Contains(combined, "broken") {
		t.Errorf("BuildErrors = %v, expected build error content", r.BuildErrors)
	}
}

func TestFormatTestResult_BuildErrors(t *testing.T) {
	r := testRunResult{
		Ecosystem:   "go",
		BuildErrors: []string{"./foo.go:5:2: undefined: bar"},
	}
	out := formatTestResult(r)
	if !strings.Contains(out, "BUILD ERRORS") {
		t.Errorf("expected BUILD ERRORS section, got:\n%s", out)
	}
	if !strings.Contains(out, "undefined: bar") {
		t.Errorf("expected build error text, got:\n%s", out)
	}
}

func TestFormatTestResult_WithFailures(t *testing.T) {
	r := testRunResult{
		Ecosystem: "go",
		Passed:    1,
		Failed:    1,
		Failures: []testFailure{
			{Test: "TestBad", Package: "example.com/pkg", Output: "    got 0 want 1\n"},
		},
	}
	out := formatTestResult(r)
	if !strings.Contains(out, "FAILURES") {
		t.Errorf("expected FAILURES section, got:\n%s", out)
	}
	if !strings.Contains(out, "--- example.com/pkg/TestBad") {
		t.Errorf("expected failure header, got:\n%s", out)
	}
	if !strings.Contains(out, "got 0 want 1") {
		t.Errorf("expected failure output, got:\n%s", out)
	}
	if !strings.Contains(out, "GO FAIL") {
		t.Errorf("expected GO FAIL in summary, got:\n%s", out)
	}
}

func TestFormatTestSummary(t *testing.T) {
	s := formatTestSummary("go", 5, 5, 0, 0)
	if s != "GO PASS — 5 passed, 0 failed, 0 skipped, 5 total" {
		t.Errorf("unexpected summary: %q", s)
	}

	s = formatTestSummary("python", 3, 2, 1, 0)
	if s != "PYTHON FAIL — 2 passed, 1 failed, 0 skipped, 3 total" {
		t.Errorf("unexpected summary: %q", s)
	}
}
