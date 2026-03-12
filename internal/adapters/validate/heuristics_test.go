package validate

import (
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestHeuristicsDetectConstantInputs(t *testing.T) {
	events := []HarnessEvent{
		{Event: "handler_result", HandlerKind: "input", Symbol: "image", Fingerprint: "abc"},
		{Event: "handler_result", HandlerKind: "input", Symbol: "image", Fingerprint: "abc"},
	}

	issues := HeuristicIssuesFromHarnessEvents(events)
	if !hasIssueCode(issues, core.IssueCodeSuspiciousConstantInputs) {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeSuspiciousConstantInputs, issues)
	}
}

func TestHeuristicsDetectConstantLabels(t *testing.T) {
	events := []HarnessEvent{
		{Event: "handler_result", HandlerKind: "ground_truth", Symbol: "target", Fingerprint: "zzz"},
		{Event: "handler_result", HandlerKind: "ground_truth", Symbol: "target", Fingerprint: "zzz"},
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
		{Event: "handler_result", HandlerKind: "input", Symbol: "image", Fingerprint: "a"},
		{Event: "handler_result", HandlerKind: "input", Symbol: "image", Fingerprint: "b"},
		{Event: "handler_result", HandlerKind: "ground_truth", Symbol: "target", Fingerprint: "1"},
		{Event: "handler_result", HandlerKind: "ground_truth", Symbol: "target", Fingerprint: "2"},
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
