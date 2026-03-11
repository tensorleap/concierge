package execute

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestBuildPreprocessAuthoringRecommendationFromEntryFile(t *testing.T) {
	repoRoot := t.TempDir()
	writeTextFile(t, filepath.Join(repoRoot, "leap_integration.py"), `from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_preprocess

@tensorleap_preprocess()
def preprocess_one():
    return []
`)

	recommendation, err := BuildPreprocessAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
		},
		core.IntegrationStatus{},
	)
	if err != nil {
		t.Fatalf("BuildPreprocessAuthoringRecommendation returned error: %v", err)
	}
	if recommendation.Target != "preprocess_one" {
		t.Fatalf("expected target %q, got %q", "preprocess_one", recommendation.Target)
	}
	if recommendation.Rationale != "preprocess symbols discovered from integration entry file" {
		t.Fatalf("expected rationale %q, got %q", "preprocess symbols discovered from integration entry file", recommendation.Rationale)
	}
}

func TestBuildPreprocessAuthoringRecommendationFallbacksWithoutSymbol(t *testing.T) {
	repoRoot := t.TempDir()

	recommendation, err := BuildPreprocessAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
		},
		core.IntegrationStatus{},
	)
	if err != nil {
		t.Fatalf("BuildPreprocessAuthoringRecommendation returned error: %v", err)
	}
	if recommendation.Target != "" {
		t.Fatalf("expected empty target when no preprocess symbol is known, got %q", recommendation.Target)
	}
	if recommendation.Rationale != "add or repair a decorated preprocess function and wire required subset outputs" {
		t.Fatalf("unexpected rationale %q", recommendation.Rationale)
	}
}

func TestBuildPreprocessAuthoringRecommendationUsesContractsWhenAvailable(t *testing.T) {
	repoRoot := t.TempDir()

	recommendation, err := BuildPreprocessAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
		},
		core.IntegrationStatus{
			Contracts: &core.IntegrationContracts{
				PreprocessFunctions: []string{"a", "b"},
			},
		},
	)
	if err != nil {
		t.Fatalf("BuildPreprocessAuthoringRecommendation returned error: %v", err)
	}
	if len(recommendation.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %v", recommendation.Candidates)
	}
	if recommendation.Target != "a" {
		t.Fatalf("expected target %q, got %q", "a", recommendation.Target)
	}
}

func writeTextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll failed for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", path, err)
	}
}
