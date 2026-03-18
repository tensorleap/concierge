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

func TestBuildModelAuthoringRecommendationUsesReadyArtifactsFromContracts(t *testing.T) {
	repoRoot := t.TempDir()
	writeModelFixtureFile(t, repoRoot, "model/found.h5")
	writeModelFixtureFile(t, repoRoot, "model/ignored.pt")

	recommendation, err := BuildModelAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
		},
		core.IntegrationStatus{
			Contracts: &core.IntegrationContracts{
				ModelCandidates: []core.ModelCandidate{
					{Path: "model/found.h5"},
					{Path: "model/ignored.pt"},
				},
			},
		},
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
	if recommendation.Rationale != "no_ready_supported_model_artifact" {
		t.Fatalf("expected rationale %q, got %q", "no_ready_supported_model_artifact", recommendation.Rationale)
	}
}

func TestBuildModelAuthoringRecommendationTruncatesCandidatesAndPreservesTarget(t *testing.T) {
	repoRoot := t.TempDir()

	modelCandidates := make([]core.ModelCandidate, 0, 10)
	for _, path := range []string{
		"models/a.onnx",
		"models/b.onnx",
		"models/c.onnx",
		"models/d.onnx",
		"models/e.onnx",
		"models/f.onnx",
		"models/g.onnx",
		"models/h.onnx",
		"models/i.onnx",
		"models/j.onnx",
	} {
		writeModelFixtureFile(t, repoRoot, path)
		modelCandidates = append(modelCandidates, core.ModelCandidate{Path: path})
	}

	recommendation, err := BuildModelAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository:        core.RepositoryState{Root: repoRoot},
			SelectedModelPath: "models/j.onnx",
		},
		core.IntegrationStatus{
			Contracts: &core.IntegrationContracts{
				ModelCandidates: modelCandidates,
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildModelAuthoringRecommendation returned error: %v", err)
	}

	if got := len(recommendation.Candidates); got != maxRepoContextModelCandidates {
		t.Fatalf("expected %d truncated candidates, got %d (%+v)", maxRepoContextModelCandidates, got, recommendation.Candidates)
	}
	if recommendation.Target != "models/j.onnx" {
		t.Fatalf("expected selected target to be preserved, got %q", recommendation.Target)
	}
	if !containsString(recommendation.Candidates, "models/j.onnx") {
		t.Fatalf("expected selected target in truncated candidates, got %+v", recommendation.Candidates)
	}
	if containsString(recommendation.Candidates, "models/i.onnx") {
		t.Fatalf("expected truncation to drop non-target tail candidates, got %+v", recommendation.Candidates)
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
