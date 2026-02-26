package validate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

type fingerprintStats struct {
	totalSamples int
	unique       map[string]struct{}
}

// HeuristicIssuesFromHarnessEvents derives anti-stub findings from parsed harness events.
func HeuristicIssuesFromHarnessEvents(events []HarnessEvent) []core.Issue {
	inputStats := map[string]*fingerprintStats{}
	labelStats := map[string]*fingerprintStats{}
	emptySubsetSet := map[string]struct{}{}

	for _, event := range events {
		switch strings.ToLower(strings.TrimSpace(event.Event)) {
		case "input_fingerprint":
			updateFingerprintStats(inputStats, defaultName(event.Name, "input"), event.Fingerprint)
		case "label_fingerprint":
			updateFingerprintStats(labelStats, defaultName(event.Name, "label"), event.Fingerprint)
		case "subset_count":
			subset := strings.ToLower(strings.TrimSpace(event.Subset))
			if (subset == "train" || subset == "validation") && event.Count == 0 {
				emptySubsetSet[subset] = struct{}{}
			}
		}
	}

	issues := make([]core.Issue, 0)
	issues = append(issues, constantFingerprintIssues(inputStats, core.IssueCodeSuspiciousConstantInputs, core.IssueScopeInputEncoder, "input")...)
	issues = append(issues, constantFingerprintIssues(labelStats, core.IssueCodeSuspiciousConstantLabels, core.IssueScopeGroundTruthEncoder, "label")...)

	emptySubsets := make([]string, 0, len(emptySubsetSet))
	for subset := range emptySubsetSet {
		emptySubsets = append(emptySubsets, subset)
	}
	sort.Strings(emptySubsets)
	for _, subset := range emptySubsets {
		issues = append(issues, core.Issue{
			Code:     core.IssueCodePreprocessSubsetEmpty,
			Message:  fmt.Sprintf("harness reported empty %s subset", subset),
			Severity: core.SeverityError,
			Scope:    core.IssueScopePreprocess,
		})
	}

	return issues
}

func updateFingerprintStats(stats map[string]*fingerprintStats, name, fingerprint string) {
	fp := strings.TrimSpace(fingerprint)
	if fp == "" {
		return
	}

	entry, ok := stats[name]
	if !ok {
		entry = &fingerprintStats{unique: map[string]struct{}{}}
		stats[name] = entry
	}

	entry.totalSamples++
	entry.unique[fp] = struct{}{}
}

func constantFingerprintIssues(stats map[string]*fingerprintStats, code core.IssueCode, scope core.IssueScope, label string) []core.Issue {
	names := make([]string, 0, len(stats))
	for name := range stats {
		names = append(names, name)
	}
	sort.Strings(names)

	issues := make([]core.Issue, 0)
	for _, name := range names {
		entry := stats[name]
		if entry.totalSamples <= 1 || len(entry.unique) != 1 {
			continue
		}

		issues = append(issues, core.Issue{
			Code:     code,
			Message:  fmt.Sprintf("harness reported constant %s fingerprint for %q across %d samples", label, name, entry.totalSamples),
			Severity: core.SeverityWarning,
			Scope:    scope,
		})
	}

	return issues
}

func defaultName(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
