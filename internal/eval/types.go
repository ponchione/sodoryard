package eval

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const (
	StatusPass = "pass"
	StatusFail = "fail"
)

type SuiteInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Report struct {
	Suite       string       `json:"suite"`
	Description string       `json:"description"`
	Status      string       `json:"status"`
	Score       float64      `json:"score"`
	Totals      Totals       `json:"totals"`
	Cases       []CaseResult `json:"cases"`
	Baseline    *Baseline    `json:"baseline,omitempty"`
}

type Totals struct {
	Cases            int `json:"cases"`
	CasesPassed      int `json:"cases_passed"`
	Assertions       int `json:"assertions"`
	AssertionsPassed int `json:"assertions_passed"`
	Warnings         int `json:"warnings"`
	Findings         int `json:"findings"`
}

type CaseResult struct {
	Name       string            `json:"name"`
	Status     string            `json:"status"`
	Score      float64           `json:"score"`
	Assertions []AssertionResult `json:"assertions"`
	Warnings   []string          `json:"warnings,omitempty"`
	Findings   []FindingResult   `json:"findings,omitempty"`
	Details    map[string]any    `json:"details,omitempty"`
}

type AssertionResult struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
	Expected any    `json:"expected,omitempty"`
	Actual   any    `json:"actual,omitempty"`
}

type FindingResult struct {
	ID         string `json:"id"`
	SourceRole string `json:"source_role,omitempty"`
	Status     string `json:"status,omitempty"`
	Severity   string `json:"severity,omitempty"`
	Category   string `json:"category,omitempty"`
	Summary    string `json:"summary,omitempty"`
}

type Baseline struct {
	Path   string         `json:"path,omitempty"`
	Status string         `json:"status"`
	Diffs  []BaselineDiff `json:"diffs,omitempty"`
}

type BaselineDiff struct {
	Field    string `json:"field"`
	Message  string `json:"message,omitempty"`
	Expected any    `json:"expected,omitempty"`
	Actual   any    `json:"actual,omitempty"`
}

type suite interface {
	Info() SuiteInfo
	Run(context.Context) (Report, error)
}

type Runner struct {
	suites map[string]suite
}

func NewRunner() Runner {
	r := Runner{suites: map[string]suite{}}
	r.register(receiptContractSuite{})
	r.register(chainFlowSuite{})
	return r
}

func ListSuites() []SuiteInfo {
	return NewRunner().ListSuites()
}

func Run(ctx context.Context, name string) (Report, error) {
	return NewRunner().Run(ctx, name)
}

func (r Runner) ListSuites() []SuiteInfo {
	out := make([]SuiteInfo, 0, len(r.suites))
	for _, suite := range r.suites {
		out = append(out, suite.Info())
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func (r Runner) Run(ctx context.Context, name string) (Report, error) {
	key := strings.TrimSpace(name)
	if key == "" {
		return Report{}, fmt.Errorf("eval suite is required")
	}
	suite, ok := r.suites[key]
	if !ok {
		return Report{}, fmt.Errorf("unknown eval suite %q", name)
	}
	return suite.Run(ctx)
}

func (r Runner) register(s suite) {
	info := s.Info()
	if strings.TrimSpace(info.Name) == "" {
		return
	}
	r.suites[info.Name] = s
}

func newReport(info SuiteInfo) Report {
	return Report{Suite: info.Name, Description: info.Description, Status: StatusPass}
}

func newCase(name string) CaseResult {
	return CaseResult{Name: strings.TrimSpace(name), Status: StatusPass, Details: map[string]any{}}
}

func (r *Report) addCase(c CaseResult) {
	c.finalize()
	r.Cases = append(r.Cases, c)
}

func (r *Report) finalize() {
	r.Status = StatusPass
	r.Totals = Totals{Cases: len(r.Cases)}
	for _, c := range r.Cases {
		if c.Status == StatusPass {
			r.Totals.CasesPassed++
		} else {
			r.Status = StatusFail
		}
		r.Totals.Warnings += len(c.Warnings)
		r.Totals.Findings += len(c.Findings)
		for _, assertion := range c.Assertions {
			r.Totals.Assertions++
			if assertion.Status == StatusPass {
				r.Totals.AssertionsPassed++
			} else {
				r.Status = StatusFail
			}
		}
	}
	if r.Totals.Assertions > 0 {
		r.Score = float64(r.Totals.AssertionsPassed) / float64(r.Totals.Assertions)
	}
}

func (c *CaseResult) addAssertion(name string, pass bool, message string, expected any, actual any) {
	status := StatusFail
	if pass {
		status = StatusPass
	}
	c.Assertions = append(c.Assertions, AssertionResult{
		Name:     strings.TrimSpace(name),
		Status:   status,
		Message:  strings.TrimSpace(message),
		Expected: expected,
		Actual:   actual,
	})
}

func (c *CaseResult) finalize() {
	c.Status = StatusPass
	passed := 0
	for _, assertion := range c.Assertions {
		if assertion.Status == StatusPass {
			passed++
		} else {
			c.Status = StatusFail
		}
	}
	if len(c.Assertions) > 0 {
		c.Score = float64(passed) / float64(len(c.Assertions))
	}
	if len(c.Details) == 0 {
		c.Details = nil
	}
}

func assertEqual[T comparable](c *CaseResult, name string, got T, want T) {
	pass := got == want
	message := ""
	if !pass {
		message = fmt.Sprintf("got %v, want %v", got, want)
	}
	c.addAssertion(name, pass, message, want, got)
}

func assertStringSet(c *CaseResult, name string, got []string, want []string) {
	got = uniqueSortedStrings(got)
	want = uniqueSortedStrings(want)
	pass := equalStrings(got, want)
	message := ""
	if !pass {
		message = fmt.Sprintf("got %s, want %s", strings.Join(got, ","), strings.Join(want, ","))
	}
	c.addAssertion(name, pass, message, want, got)
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func equalStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func CompareBaseline(current Report, baseline Report) Baseline {
	comparison := Baseline{Status: StatusPass}
	currentSig := reportBaselineSignatureFrom(current)
	baselineSig := reportBaselineSignatureFrom(baseline)
	addDiff := func(field string, expected any, actual any) {
		if reflect.DeepEqual(expected, actual) {
			return
		}
		comparison.Status = StatusFail
		comparison.Diffs = append(comparison.Diffs, BaselineDiff{
			Field:    field,
			Message:  fmt.Sprintf("got %v, want %v", actual, expected),
			Expected: expected,
			Actual:   actual,
		})
	}
	addDiff("suite", baselineSig.Suite, currentSig.Suite)
	addDiff("status", baselineSig.Status, currentSig.Status)
	addDiff("totals", baselineSig.Totals, currentSig.Totals)
	addDiff("cases", baselineSig.Cases, currentSig.Cases)
	return comparison
}

type reportBaselineSignature struct {
	Suite  string                  `json:"suite"`
	Status string                  `json:"status"`
	Totals Totals                  `json:"totals"`
	Cases  []caseBaselineSignature `json:"cases"`
}

type caseBaselineSignature struct {
	Name       string                       `json:"name"`
	Status     string                       `json:"status"`
	Score      float64                      `json:"score"`
	Assertions []assertionBaselineSignature `json:"assertions"`
	Warnings   []string                     `json:"warnings,omitempty"`
	Findings   []findingBaselineSignature   `json:"findings,omitempty"`
}

type assertionBaselineSignature struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type findingBaselineSignature struct {
	ID     string `json:"id"`
	Status string `json:"status,omitempty"`
}

func reportBaselineSignatureFrom(report Report) reportBaselineSignature {
	cases := make([]caseBaselineSignature, 0, len(report.Cases))
	for _, c := range report.Cases {
		assertions := make([]assertionBaselineSignature, 0, len(c.Assertions))
		for _, assertion := range c.Assertions {
			assertions = append(assertions, assertionBaselineSignature{Name: assertion.Name, Status: assertion.Status})
		}
		sort.SliceStable(assertions, func(i int, j int) bool {
			if assertions[i].Name == assertions[j].Name {
				return assertions[i].Status < assertions[j].Status
			}
			return assertions[i].Name < assertions[j].Name
		})
		findings := make([]findingBaselineSignature, 0, len(c.Findings))
		for _, finding := range c.Findings {
			findings = append(findings, findingBaselineSignature{ID: finding.ID, Status: finding.Status})
		}
		sort.SliceStable(findings, func(i int, j int) bool {
			if findings[i].ID == findings[j].ID {
				return findings[i].Status < findings[j].Status
			}
			return findings[i].ID < findings[j].ID
		})
		warnings := append([]string(nil), c.Warnings...)
		sort.Strings(warnings)
		cases = append(cases, caseBaselineSignature{
			Name:       c.Name,
			Status:     c.Status,
			Score:      c.Score,
			Assertions: assertions,
			Warnings:   warnings,
			Findings:   findings,
		})
	}
	sort.SliceStable(cases, func(i int, j int) bool {
		return cases[i].Name < cases[j].Name
	})
	return reportBaselineSignature{
		Suite:  report.Suite,
		Status: report.Status,
		Totals: report.Totals,
		Cases:  cases,
	}
}
