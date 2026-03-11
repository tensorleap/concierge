package execute

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestGTEncoderRecommendationListsMissingSymbols(t *testing.T) {
	recommendation, err := BuildGTEncoderAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: t.TempDir()},
		},
		core.IntegrationStatus{
			Issues: []core.Issue{
				{
					Code:     core.IssueCodeGTEncoderMissing,
					Severity: core.SeverityError,
					Location: &core.IssueLocation{Symbol: "label"},
				},
				{
					Code:     core.IssueCodeUnlabeledSubsetGTInvocation,
					Severity: core.SeverityError,
					Location: &core.IssueLocation{Symbol: "mask"},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildGTEncoderAuthoringRecommendation returned error: %v", err)
	}
	if recommendation.Target != "label" {
		t.Fatalf("expected target %q, got %q", "label", recommendation.Target)
	}
	want := []string{"label", "mask"}
	if !reflect.DeepEqual(recommendation.Candidates, want) {
		t.Fatalf("expected candidates %v, got %v", want, recommendation.Candidates)
	}

	containsLabeledConstraint := false
	for _, constraint := range recommendation.Constraints {
		if strings.Contains(strings.ToLower(constraint), "labeled subsets only") {
			containsLabeledConstraint = true
			break
		}
	}
	if !containsLabeledConstraint {
		t.Fatalf("expected labeled-subset constraint, got %+v", recommendation.Constraints)
	}
}

func TestGTEncoderRecommendationFallsBackToSourceDiscovery(t *testing.T) {
	repoRoot := t.TempDir()
	writeTextFile(t, filepath.Join(repoRoot, "leap_integration.py"), `from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_gt_encoder, tensorleap_integration_test

@tensorleap_gt_encoder("label")
def encode_label():
    return 1

@tensorleap_integration_test()
def run_flow():
    encode_label()
    encode_mask()
`)

	recommendation, err := BuildGTEncoderAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
		},
		core.IntegrationStatus{},
	)
	if err != nil {
		t.Fatalf("BuildGTEncoderAuthoringRecommendation returned error: %v", err)
	}
	if recommendation.Target != "mask" {
		t.Fatalf("expected target %q, got %q", "mask", recommendation.Target)
	}
	if len(recommendation.Candidates) != 1 || recommendation.Candidates[0] != "mask" {
		t.Fatalf("expected missing symbol %q, got %+v", "mask", recommendation.Candidates)
	}
}
