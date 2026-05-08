package eval

import (
	"context"
	"fmt"
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
