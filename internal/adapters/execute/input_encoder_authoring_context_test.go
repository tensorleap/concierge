package execute

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestInputEncoderRecommendationListsMissingSymbols(t *testing.T) {
	recommendation, err := BuildInputEncoderAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: t.TempDir()},
		},
		core.IntegrationStatus{
			Issues: []core.Issue{
				{
					Code:     core.IssueCodeInputEncoderMissing,
					Severity: core.SeverityError,
					Location: &core.IssueLocation{Symbol: "image"},
				},
				{
					Code:     core.IssueCodeInputEncoderCoverageIncomplete,
					Severity: core.SeverityError,
					Location: &core.IssueLocation{Symbol: "meta"},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildInputEncoderAuthoringRecommendation returned error: %v", err)
	}

	if recommendation.StepID != core.EnsureStepInputEncoders {
		t.Fatalf("expected step ID %q, got %q", core.EnsureStepInputEncoders, recommendation.StepID)
	}
	if recommendation.Target != "image" {
		t.Fatalf("expected target %q, got %q", "image", recommendation.Target)
	}
	want := []string{"image", "meta"}
	if !reflect.DeepEqual(recommendation.Candidates, want) {
		t.Fatalf("expected candidates %v, got %v", want, recommendation.Candidates)
	}
}

func TestInputEncoderRecommendationFallsBackToSourceDiscovery(t *testing.T) {
	repoRoot := t.TempDir()
	writeTextFile(t, filepath.Join(repoRoot, "leap_binder.py"), `from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_input_encoder, tensorleap_integration_test

@tensorleap_input_encoder("image")
def encode_image():
    return 1

@tensorleap_integration_test()
def run_flow():
    encode_image()
    encode_meta()
`)

	recommendation, err := BuildInputEncoderAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
		},
		core.IntegrationStatus{},
	)
	if err != nil {
		t.Fatalf("BuildInputEncoderAuthoringRecommendation returned error: %v", err)
	}
	if recommendation.Target != "meta" {
		t.Fatalf("expected target %q, got %q", "meta", recommendation.Target)
	}
	if len(recommendation.Candidates) != 1 || recommendation.Candidates[0] != "meta" {
		t.Fatalf("expected missing symbol %q, got %+v", "meta", recommendation.Candidates)
	}
}
