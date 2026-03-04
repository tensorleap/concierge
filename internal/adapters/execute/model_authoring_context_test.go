package execute

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestBuildModelAuthoringRecommendationPrefersSelectedPath(t *testing.T) {
	repoRoot := t.TempDir()
	writeModelFixtureFile(t, repoRoot, "models/a.onnx")
	writeModelFixtureFile(t, repoRoot, "models/b.h5")

	recommendation, err := BuildModelAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository:        core.RepositoryState{Root: repoRoot},
			SelectedModelPath: "models/b.h5",
		},
		core.IntegrationStatus{
			Contracts: &core.IntegrationContracts{
				ModelCandidates: []core.ModelCandidate{
					{Path: "models/a.onnx"},
					{Path: "models/b.h5"},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildModelAuthoringRecommendation returned error: %v", err)
	}

	if recommendation.StepID != core.EnsureStepModelContract {
		t.Fatalf("expected step ID %q, got %q", core.EnsureStepModelContract, recommendation.StepID)
	}
	if recommendation.Target != "models/b.h5" {
		t.Fatalf("expected target %q, got %q", "models/b.h5", recommendation.Target)
	}
	if recommendation.Rationale != "selected_model_path_override" {
		t.Fatalf("expected rationale %q, got %q", "selected_model_path_override", recommendation.Rationale)
	}
	if len(recommendation.Candidates) < 2 {
		t.Fatalf("expected candidates to include discovered values, got %+v", recommendation.Candidates)
	}
}

func TestBuildModelAuthoringRecommendationAmbiguousFallbackDeterministic(t *testing.T) {
	repoRoot := t.TempDir()

	recommendation, err := BuildModelAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
		},
		core.IntegrationStatus{
			Contracts: &core.IntegrationContracts{
				ModelCandidates: []core.ModelCandidate{
					{Path: "models/z.h5"},
					{Path: "models/a.onnx"},
					{Path: "models/m.h5"},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildModelAuthoringRecommendation returned error: %v", err)
	}

	if recommendation.Target != "models/a.onnx" {
		t.Fatalf("expected lexical fallback target %q, got %q", "models/a.onnx", recommendation.Target)
	}
	if recommendation.Rationale != "ambiguous_supported_candidates_lexical_fallback" {
		t.Fatalf("expected rationale %q, got %q", "ambiguous_supported_candidates_lexical_fallback", recommendation.Rationale)
	}

	wantCandidates := []string{"models/a.onnx", "models/m.h5", "models/z.h5"}
	if !reflect.DeepEqual(recommendation.Candidates, wantCandidates) {
		t.Fatalf("expected candidates %+v, got %+v", wantCandidates, recommendation.Candidates)
	}
}

func TestBuildModelAuthoringRecommendationScansRepoForSupportedFormats(t *testing.T) {
	repoRoot := t.TempDir()
	writeModelFixtureFile(t, repoRoot, "model/found.h5")
	writeModelFixtureFile(t, repoRoot, "model/ignored.pt")

	recommendation, err := BuildModelAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
		},
		core.IntegrationStatus{},
	)
	if err != nil {
		t.Fatalf("BuildModelAuthoringRecommendation returned error: %v", err)
	}

	if recommendation.Target != "model/found.h5" {
		t.Fatalf("expected scanned target %q, got %q", "model/found.h5", recommendation.Target)
	}
	if recommendation.Rationale != "single_supported_candidate" {
		t.Fatalf("expected rationale %q, got %q", "single_supported_candidate", recommendation.Rationale)
	}
}

func TestBuildModelAuthoringRecommendationHandlesNoSupportedCandidates(t *testing.T) {
	repoRoot := t.TempDir()
	writeModelFixtureFile(t, repoRoot, "model/only.pt")

	recommendation, err := BuildModelAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
		},
		core.IntegrationStatus{
			Contracts: &core.IntegrationContracts{
				ModelCandidates: []core.ModelCandidate{{Path: "model/only.pt"}},
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildModelAuthoringRecommendation returned error: %v", err)
	}

	if recommendation.Target != "" {
		t.Fatalf("expected empty target when no supported candidates exist, got %q", recommendation.Target)
	}
	if recommendation.Rationale != "no_supported_model_candidate" {
		t.Fatalf("expected rationale %q, got %q", "no_supported_model_candidate", recommendation.Rationale)
	}
}

func writeModelFixtureFile(t *testing.T, repoRoot, relativePath string) {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}
