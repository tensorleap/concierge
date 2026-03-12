package validate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

// MapHarnessIssues converts decoded harness events into existing issue families.
func MapHarnessIssues(events []HarnessEvent) []core.Issue {
	issues := make([]core.Issue, 0)
	subsetCounts := map[string]int{}
	explicitMissing := map[string]struct{}{}
	inventory := map[string]map[string]struct{}{}
	successes := map[string]map[string]struct{}{}
	completed := false

	for _, event := range events {
		subset := normalizeHarnessSubset(event.Subset)
		status := normalizeHarnessStatus(event.Status)
		kind := normalizeHarnessKind(event.HandlerKind)
		symbol := harnessEventSymbol(event)

		switch normalizeHarnessEvent(event.Event) {
		case "runtime_failed":
			issues = append(issues, core.Issue{
				Code:     core.IssueCodeHarnessValidationFailed,
				Message:  messageOrDefault(strings.TrimSpace(event.Message), "runtime harness failed before validation completed"),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeValidation,
			})
		case "preprocess":
			if status == "failed" {
				issues = append(issues, core.Issue{
					Code:     core.IssueCodePreprocessExecutionFailed,
					Message:  messageOrDefault(strings.TrimSpace(event.Message), "preprocess failed during runtime harness validation"),
					Severity: core.SeverityError,
					Scope:    core.IssueScopePreprocess,
				})
			}
		case "subset_missing":
			if subset != "" {
				explicitMissing[subset] = struct{}{}
			}
			switch subset {
			case "train":
				issues = append(issues, core.Issue{
					Code:     core.IssueCodePreprocessTrainSubsetMissing,
					Message:  messageOrDefault(strings.TrimSpace(event.Message), "train subset is missing from preprocess output"),
					Severity: core.SeverityError,
					Scope:    core.IssueScopePreprocess,
				})
			case "validation":
				issues = append(issues, core.Issue{
					Code:     core.IssueCodePreprocessValidationSubsetMissing,
					Message:  messageOrDefault(strings.TrimSpace(event.Message), "validation subset is missing from preprocess output"),
					Severity: core.SeverityError,
					Scope:    core.IssueScopePreprocess,
				})
			}
		case "summary":
			completed = true
		case "subset_count":
			subsetCounts[subset] = event.Count
			if (subset == "train" || subset == "validation") && event.Count == 0 {
				issues = append(issues, core.Issue{
					Code:     core.IssueCodePreprocessSubsetEmpty,
					Message:  fmt.Sprintf("runtime harness reported empty %s subset", subset),
					Severity: core.SeverityError,
					Scope:    core.IssueScopePreprocess,
				})
			}
		case "handler_inventory":
			if kind == "" || symbol == "" {
				continue
			}
			if _, ok := inventory[kind]; !ok {
				inventory[kind] = map[string]struct{}{}
			}
			inventory[kind][symbol] = struct{}{}
		case "handler_result":
			if kind == "" || symbol == "" {
				continue
			}
			if status == "ok" {
				if _, ok := successes[kind]; !ok {
					successes[kind] = map[string]struct{}{}
				}
				successes[kind][symbol] = struct{}{}
			}
			if issue, ok := issueForHandlerResult(event, subset, symbol, kind, status); ok {
				issues = append(issues, issue)
			}
		}
	}

	if completed {
		if _, ok := subsetCounts["train"]; !ok {
			if _, explicit := explicitMissing["train"]; !explicit {
				issues = append(issues, core.Issue{
					Code:     core.IssueCodePreprocessTrainSubsetMissing,
					Message:  "runtime harness did not observe a train subset",
					Severity: core.SeverityError,
					Scope:    core.IssueScopePreprocess,
				})
			}
		}
		if _, ok := subsetCounts["validation"]; !ok {
			if _, explicit := explicitMissing["validation"]; !explicit {
				issues = append(issues, core.Issue{
					Code:     core.IssueCodePreprocessValidationSubsetMissing,
					Message:  "runtime harness did not observe a validation subset",
					Severity: core.SeverityError,
					Scope:    core.IssueScopePreprocess,
				})
			}
		}
	}

	issues = append(issues, coverageIssues(inventory, successes)...)
	return dedupeMappedIssues(issues)
}

func issueForHandlerResult(event HarnessEvent, subset, symbol, kind, status string) (core.Issue, bool) {
	location := &core.IssueLocation{Symbol: symbol}
	context := harnessSampleContext(subset, event.SampleID, event.SampleOffset)
	message := strings.TrimSpace(event.Message)

	switch kind {
	case "input":
		switch status {
		case "failed":
			return core.Issue{
				Code:     core.IssueCodeInputEncoderExecutionFailed,
				Message:  fmt.Sprintf("input encoder %q failed for %s: %s", symbol, context, messageOrDefault(message, "runtime harness execution failed")),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeInputEncoder,
				Location: location,
			}, true
		case "dtype_invalid":
			return core.Issue{
				Code:     core.IssueCodeInputEncoderDTypeInvalid,
				Message:  fmt.Sprintf("input encoder %q returned an unsupported dtype for %s: %s", symbol, context, messageOrDefault(message, "runtime harness dtype validation failed")),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeInputEncoder,
				Location: location,
			}, true
		case "shape_invalid":
			return core.Issue{
				Code:     core.IssueCodeInputEncoderShapeInvalid,
				Message:  fmt.Sprintf("input encoder %q returned an invalid shape for %s: %s", symbol, context, messageOrDefault(message, "runtime harness shape validation failed")),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeInputEncoder,
				Location: location,
			}, true
		case "non_finite":
			return core.Issue{
				Code:     core.IssueCodeInputEncoderNonFiniteValues,
				Message:  fmt.Sprintf("input encoder %q returned non-finite values for %s", symbol, context),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeInputEncoder,
				Location: location,
			}, true
		}
		if event.Finite != nil && !*event.Finite {
			return core.Issue{
				Code:     core.IssueCodeInputEncoderNonFiniteValues,
				Message:  fmt.Sprintf("input encoder %q returned non-finite values for %s", symbol, context),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeInputEncoder,
				Location: location,
			}, true
		}
	case "ground_truth":
		switch status {
		case "failed":
			return core.Issue{
				Code:     core.IssueCodeGTEncoderExecutionFailed,
				Message:  fmt.Sprintf("ground-truth encoder %q failed for %s: %s", symbol, context, messageOrDefault(message, "runtime harness execution failed")),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeGroundTruthEncoder,
				Location: location,
			}, true
		case "dtype_invalid":
			return core.Issue{
				Code:     core.IssueCodeGTEncoderDTypeInvalid,
				Message:  fmt.Sprintf("ground-truth encoder %q returned an unsupported dtype for %s: %s", symbol, context, messageOrDefault(message, "runtime harness dtype validation failed")),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeGroundTruthEncoder,
				Location: location,
			}, true
		case "shape_invalid":
			return core.Issue{
				Code:     core.IssueCodeGTEncoderShapeInvalid,
				Message:  fmt.Sprintf("ground-truth encoder %q returned an invalid shape for %s: %s", symbol, context, messageOrDefault(message, "runtime harness shape validation failed")),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeGroundTruthEncoder,
				Location: location,
			}, true
		case "non_finite":
			return core.Issue{
				Code:     core.IssueCodeGTEncoderNonFiniteValues,
				Message:  fmt.Sprintf("ground-truth encoder %q returned non-finite values for %s", symbol, context),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeGroundTruthEncoder,
				Location: location,
			}, true
		}
		if event.Finite != nil && !*event.Finite {
			return core.Issue{
				Code:     core.IssueCodeGTEncoderNonFiniteValues,
				Message:  fmt.Sprintf("ground-truth encoder %q returned non-finite values for %s", symbol, context),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeGroundTruthEncoder,
				Location: location,
			}, true
		}
	}

	return core.Issue{}, false
}

func coverageIssues(inventory, successes map[string]map[string]struct{}) []core.Issue {
	issues := make([]core.Issue, 0)
	for kind, symbols := range inventory {
		names := make([]string, 0, len(symbols))
		for symbol := range symbols {
			names = append(names, symbol)
		}
		sort.Strings(names)
		for _, symbol := range names {
			if _, ok := successes[kind][symbol]; ok {
				continue
			}
			switch kind {
			case "input":
				issues = append(issues, core.Issue{
					Code:     core.IssueCodeInputEncoderCoverageIncomplete,
					Message:  fmt.Sprintf("runtime harness did not record a successful sampled run for input encoder %q", symbol),
					Severity: core.SeverityError,
					Scope:    core.IssueScopeInputEncoder,
					Location: &core.IssueLocation{Symbol: symbol},
				})
			case "ground_truth":
				issues = append(issues, core.Issue{
					Code:     core.IssueCodeGTEncoderCoverageIncomplete,
					Message:  fmt.Sprintf("runtime harness did not record a successful sampled run for ground-truth encoder %q", symbol),
					Severity: core.SeverityError,
					Scope:    core.IssueScopeGroundTruthEncoder,
					Location: &core.IssueLocation{Symbol: symbol},
				})
			}
		}
	}
	return issues
}

func harnessEventSymbol(event HarnessEvent) string {
	if trimmed := strings.TrimSpace(event.Symbol); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(event.Name)
}

func normalizeHarnessEvent(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeHarnessStatus(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeHarnessSubset(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "training" {
		return "train"
	}
	return trimmed
}

func normalizeHarnessKind(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func harnessSampleContext(subset, sampleID string, sampleOffset int) string {
	parts := make([]string, 0, 2)
	if subset = normalizeHarnessSubset(subset); subset != "" {
		parts = append(parts, subset)
	}
	if strings.TrimSpace(sampleID) != "" {
		parts = append(parts, fmt.Sprintf("sample %q", strings.TrimSpace(sampleID)))
	} else if sampleOffset > 0 {
		parts = append(parts, fmt.Sprintf("sample offset %d", sampleOffset))
	}
	if len(parts) == 0 {
		return "runtime sample"
	}
	return strings.Join(parts, " ")
}

func dedupeMappedIssues(issues []core.Issue) []core.Issue {
	seen := map[string]struct{}{}
	unique := make([]core.Issue, 0, len(issues))
	for _, issue := range issues {
		key := string(issue.Code) + "|" + issue.Message + "|" + string(issue.Scope)
		if issue.Location != nil {
			key += "|" + issue.Location.Symbol
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, issue)
	}
	return unique
}
