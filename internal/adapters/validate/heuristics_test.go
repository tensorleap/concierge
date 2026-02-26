package validate

import (
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestHeuristicsDetectConstantInputs(t *testing.T) {
	events := []HarnessEvent{
		{Event: "input_fingerprint", Name: "image", Fingerprint: "abc"},
		{Event: "input_fingerprint", Name: "image", Fingerprint: "abc"},
	}

	issues := HeuristicIssuesFromHarnessEvents(events)
	if !hasIssueCode(issues, core.IssueCodeSuspiciousConstantInputs) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeSuspiciousConstantInputs, issues)
	}
}

func TestHeuristicsDetectConstantLabels(t *testing.T) {
	events := []HarnessEvent{
		{Event: "label_fingerprint", Name: "target", Fingerprint: "zzz"},
		{Event: "label_fingerprint", Name: "target", Fingerprint: "zzz"},
	}

	issues := HeuristicIssuesFromHarnessEvents(events)
	if !hasIssueCode(issues, core.IssueCodeSuspiciousConstantLabels) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeSuspiciousConstantLabels, issues)
	}
}

func TestHeuristicsDetectEmptySubset(t *testing.T) {
	events := []HarnessEvent{
		{Event: "subset_count", Subset: "train", Count: 0},
	}

	issues := HeuristicIssuesFromHarnessEvents(events)
	if !hasIssueCode(issues, core.IssueCodePreprocessSubsetEmpty) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodePreprocessSubsetEmpty, issues)
	}
}

func TestHeuristicsNoFalsePositiveOnVaryingData(t *testing.T) {
	events := []HarnessEvent{
		{Event: "subset_count", Subset: "train", Count: 10},
		{Event: "subset_count", Subset: "validation", Count: 5},
		{Event: "input_fingerprint", Name: "image", Fingerprint: "a"},
		{Event: "input_fingerprint", Name: "image", Fingerprint: "b"},
		{Event: "label_fingerprint", Name: "target", Fingerprint: "1"},
		{Event: "label_fingerprint", Name: "target", Fingerprint: "2"},
	}

	issues := HeuristicIssuesFromHarnessEvents(events)
	if len(issues) != 0 {
		t.Fatalf("expected no heuristic issues, got %+v", issues)
	}
}

func hasIssueCode(issues []core.Issue, code core.IssueCode) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
