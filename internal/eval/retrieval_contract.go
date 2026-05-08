package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	contextpkg "github.com/ponchione/sodoryard/internal/context"
)

type retrievalContractSuite struct{}

type retrievalContractFixture struct {
	Name                  string                           `json:"name"`
	Report                contextpkg.ContextAssemblyReport `json:"report"`
	ExpectedResultCount   int                              `json:"expected_result_count"`
	ExpectedSources       []string                         `json:"expected_sources"`
	ExpectedKinds         []string                         `json:"expected_kinds"`
	ExpectedIncludedPaths []string                         `json:"expected_included_paths"`
	ExpectedExcludedPaths []string                         `json:"expected_excluded_paths"`
	ExpectedSymbols       []string                         `json:"expected_symbols"`
}

var retrievalContractFixturePaths = []string{
	"fixtures/retrieval-contract/mixed-source-report.json",
	"fixtures/retrieval-contract/stored-unified-report.json",
}

func (retrievalContractSuite) Info() SuiteInfo {
	return SuiteInfo{
		Name:        "retrieval-contract",
		Description: "Evaluate deterministic context report fixtures against normalized retrieval result metadata.",
	}
}

func (s retrievalContractSuite) Run(ctx context.Context) (Report, error) {
	select {
	case <-ctx.Done():
		return Report{}, ctx.Err()
	default:
	}
	report := newReport(s.Info())
	for _, path := range retrievalContractFixturePaths {
		result, err := evaluateRetrievalContractFixture(path)
		if err != nil {
			c := newCase(path)
			c.addAssertion("evaluate fixture", false, err.Error(), path, err.Error())
			report.addCase(c)
			continue
		}
		report.addCase(result)
	}
	report.finalize()
	return report, nil
}

func evaluateRetrievalContractFixture(path string) (CaseResult, error) {
	data, err := fixtureFS.ReadFile(path)
	if err != nil {
		return CaseResult{}, err
	}
	var fixture retrievalContractFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return CaseResult{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if strings.TrimSpace(fixture.Name) == "" {
		fixture.Name = path
	}
	result := newCase(fixture.Name)
	result.addAssertion("read fixture", true, "", path, path)

	results := retrievalResultsForFixture(fixture.Report)
	sources, kinds, includedPaths, excludedPaths, symbols := summarizeRetrievalResults(results)
	result.Details["result_count"] = len(results)
	result.Details["sources"] = uniqueSortedStrings(sources)
	result.Details["kinds"] = uniqueSortedStrings(kinds)
	result.Details["included_paths"] = uniqueSortedStrings(includedPaths)
	result.Details["excluded_paths"] = uniqueSortedStrings(excludedPaths)
	result.Details["symbols"] = uniqueSortedStrings(symbols)

	assertEqual(&result, "result count", len(results), fixture.ExpectedResultCount)
	assertStringSet(&result, "sources", sources, fixture.ExpectedSources)
	assertStringSet(&result, "kinds", kinds, fixture.ExpectedKinds)
	assertStringSet(&result, "included paths", includedPaths, fixture.ExpectedIncludedPaths)
	assertStringSet(&result, "excluded paths", excludedPaths, fixture.ExpectedExcludedPaths)
	assertStringSet(&result, "symbols", symbols, fixture.ExpectedSymbols)
	return result, nil
}

func retrievalResultsForFixture(report contextpkg.ContextAssemblyReport) []contextpkg.RetrievalResult {
	if len(report.UnifiedResults) > 0 {
		return report.UnifiedResults
	}
	return contextpkg.BuildRetrievalResults(report.RAGResults, report.BrainResults, report.GraphResults, report.ExplicitFileResults)
}

func summarizeRetrievalResults(results []contextpkg.RetrievalResult) ([]string, []string, []string, []string, []string) {
	sources := make([]string, 0, len(results))
	kinds := make([]string, 0, len(results))
	includedPaths := make([]string, 0, len(results))
	excludedPaths := make([]string, 0, len(results))
	symbols := make([]string, 0, len(results))
	for _, result := range results {
		sources = append(sources, result.Source)
		kinds = append(kinds, result.Kind)
		if result.Symbol != "" {
			symbols = append(symbols, result.Symbol)
		}
		if result.Path == "" {
			continue
		}
		if result.Included {
			includedPaths = append(includedPaths, result.Path)
			continue
		}
		excludedPaths = append(excludedPaths, result.Path)
	}
	return sources, kinds, includedPaths, excludedPaths, symbols
}
