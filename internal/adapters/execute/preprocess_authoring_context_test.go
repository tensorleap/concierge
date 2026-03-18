package execute

import (
	"os"
	"path/filepath"
	"strings"
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

func TestBuildPreprocessAuthoringRecommendationForbidsInventedPathsAndPlaceholderIDs(t *testing.T) {
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

	expected := []string{
		"Prefer repo-local dataset and Tensorleap integration configuration over hand-coded installed-package defaults or home-directory settings.",
		"If repository conventions point to a sibling datasets directory, verify that the resolved sibling path is writable in the current runtime; do not translate a repo mounted at /workspace into a new filesystem-root directory such as /datasets.",
		"If the repository already declares train/validation subsets in a dataset manifest, reuse those declared subsets instead of inventing a new split from arbitrary images.",
		"If the repository includes explicit dataset manifests, loader code, or Tensorleap integration examples, treat those as stronger evidence than arbitrary image files.",
		"If the repository exposes a supported dataset resolver or downloader, prefer that helper over hard-coded cache roots or generic image scans.",
		"Smoke-test any repository dataset resolver before wiring it into preprocess; if the helper import fails in the current repo state, fall back to manifest-driven resolution/download instead of keeping a broken import.",
		"If a repo helper import fails because project dependencies are missing, do not reverse-engineer internal cache constants or framework settings paths; use explicit manifest train/val/download evidence or stop with the blocker.",
		"If a prepared repository runtime interpreter is available, use that interpreter for Python repo checks instead of bare python/python3, and treat failures under the wrong interpreter as environment mismatch evidence rather than dataset-path evidence.",
		"Do not run pip install, poetry add, or other environment mutation commands while discovering dataset paths for preprocess; if discovery depends on missing packages, stop and surface that blocker.",
		"Do not set deprecated `PreprocessResponse.length`; provide real `sample_ids` for each subset and let Tensorleap derive lengths from them.",
		"Do not create or write to top-level absolute directories outside the repo/workspace just to satisfy preprocess data access; if the repo-supported path is unavailable in the current runtime, stop and surface that blocker or use a repo-local writable fallback supported by repository evidence.",
		"Do not hard-code home-directory dataset defaults, installed-package cache roots, or new environment-variable paths unless repository evidence requires them and the repository itself uses them.",
		"Do not fabricate placeholder sample IDs, dummy image paths, or guessed absolute dataset locations just to satisfy subset requirements.",
		"Do not repurpose generic repository assets, screenshots, docs media, or example images as train/validation data unless repository evidence explicitly identifies them as the real dataset.",
		"If repository evidence does not expose real train/validation identifiers and no repo-supported acquisition path exists, stop and surface the missing data requirement instead of guessing.",
	}
	for _, want := range expected {
		found := false
		for _, constraint := range recommendation.Constraints {
			if constraint == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected constraint %q in %+v", want, recommendation.Constraints)
		}
	}
}

func TestBuildPreprocessAuthoringRecommendationIncludesExistingTensorleapEvidencePaths(t *testing.T) {
	repoRoot := t.TempDir()

	recommendation, err := BuildPreprocessAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
			FileHashes: map[string]string{
				"ultralytics/tensorleap_folder/README.md":                "hash-readme",
				"ultralytics/tensorleap_folder/pose/leap_binder.py":      "hash-binder",
				"ultralytics/tensorleap_folder/pose/leap_integration.py": "hash-integration",
				"ultralytics/cfg/default.yaml":                           "hash-default",
				"ultralytics/cfg/datasets/coco8.yaml":                    "hash-dataset",
			},
		},
		core.IntegrationStatus{},
	)
	if err != nil {
		t.Fatalf("BuildPreprocessAuthoringRecommendation returned error: %v", err)
	}

	found := false
	for _, constraint := range recommendation.Constraints {
		if strings.Contains(constraint, "ultralytics/tensorleap_folder/README.md") &&
			strings.Contains(constraint, "ultralytics/tensorleap_folder/pose/leap_binder.py") &&
			strings.Contains(constraint, "ultralytics/cfg/default.yaml") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected repository evidence paths in constraints, got %+v", recommendation.Constraints)
	}
}

func TestBuildPreprocessAuthoringRecommendationDiscoversEvidenceFromRepoFilesWhenSnapshotHashesAreSparse(t *testing.T) {
	repoRoot := t.TempDir()
	writeTextFile(t, filepath.Join(repoRoot, "ultralytics", "tensorleap_folder", "README.md"), "# Tensorleap\n")
	writeTextFile(t, filepath.Join(repoRoot, "ultralytics", "tensorleap_folder", "pose", "leap_binder.py"), "def binder():\n    pass\n")
	writeTextFile(t, filepath.Join(repoRoot, "ultralytics", "cfg", "default.yaml"), "tensorleap_path: /datasets\n")
	writeTextFile(t, filepath.Join(repoRoot, "ultralytics", "cfg", "datasets", "coco8.yaml"), "download: https://example.invalid/coco8.zip\n")

	recommendation, err := BuildPreprocessAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
			FileHashes: map[string]string{
				"leap.yaml": "hash-leap",
			},
		},
		core.IntegrationStatus{},
	)
	if err != nil {
		t.Fatalf("BuildPreprocessAuthoringRecommendation returned error: %v", err)
	}

	found := false
	for _, constraint := range recommendation.Constraints {
		if strings.Contains(constraint, "ultralytics/tensorleap_folder/README.md") &&
			strings.Contains(constraint, "ultralytics/tensorleap_folder/pose/leap_binder.py") &&
			strings.Contains(constraint, "ultralytics/cfg/default.yaml") &&
			strings.Contains(constraint, "ultralytics/cfg/datasets/coco8.yaml") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected walked repository evidence paths in constraints, got %+v", recommendation.Constraints)
	}
}

func TestBuildPreprocessAuthoringRecommendationPrioritizesTensorleapAndSmallDatasetEvidence(t *testing.T) {
	repoRoot := t.TempDir()
	writeTextFile(t, filepath.Join(repoRoot, "ultralytics", "tensorleap_folder", "README.md"), "# Tensorleap\n")
	writeTextFile(t, filepath.Join(repoRoot, "ultralytics", "tensorleap_folder", "pose", "leap_binder.py"), "def binder():\n    pass\n")
	writeTextFile(t, filepath.Join(repoRoot, "ultralytics", "cfg", "default.yaml"), "tensorleap_path: /datasets\n")
	writeTextFile(t, filepath.Join(repoRoot, "ultralytics", "cfg", "datasets", "coco8.yaml"), "download: https://example.invalid/coco8.zip\n")
	writeTextFile(t, filepath.Join(repoRoot, "ultralytics", "cfg", "datasets", "african-wildlife.yaml"), "download: https://example.invalid/african.zip\n")
	writeTextFile(t, filepath.Join(repoRoot, "ultralytics", "cfg", "datasets", "Argoverse.yaml"), "download: https://example.invalid/argo.zip\n")

	recommendation, err := BuildPreprocessAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
		},
		core.IntegrationStatus{},
	)
	if err != nil {
		t.Fatalf("BuildPreprocessAuthoringRecommendation returned error: %v", err)
	}

	var evidenceConstraint string
	for _, constraint := range recommendation.Constraints {
		if strings.Contains(constraint, "Inspect and reuse existing repository Tensorleap or dataset evidence before inventing new preprocess structure:") {
			evidenceConstraint = constraint
			break
		}
	}
	if evidenceConstraint == "" {
		t.Fatalf("expected evidence constraint in %+v", recommendation.Constraints)
	}
	if !strings.Contains(evidenceConstraint, "ultralytics/tensorleap_folder/README.md") ||
		!strings.Contains(evidenceConstraint, "ultralytics/tensorleap_folder/pose/leap_binder.py") ||
		!strings.Contains(evidenceConstraint, "ultralytics/cfg/default.yaml") ||
		!strings.Contains(evidenceConstraint, "ultralytics/cfg/datasets/coco8.yaml") {
		t.Fatalf("expected prioritized evidence paths in %q", evidenceConstraint)
	}
	cocoIndex := strings.Index(evidenceConstraint, "ultralytics/cfg/datasets/coco8.yaml")
	africanIndex := strings.Index(evidenceConstraint, "ultralytics/cfg/datasets/african-wildlife.yaml")
	if africanIndex >= 0 && cocoIndex > africanIndex {
		t.Fatalf("expected preferred dataset evidence to appear before arbitrary dataset YAMLs in %q", evidenceConstraint)
	}
}

func TestBuildPreprocessAuthoringRecommendationIncludesDatasetManifestAndResolverHints(t *testing.T) {
	repoRoot := t.TempDir()
	writeTextFile(t, filepath.Join(repoRoot, "project_config.yaml"), "task: detect\n")
	writeTextFile(t, filepath.Join(repoRoot, "ultralytics", "cfg", "datasets", "coco128.yaml"), strings.Join([]string{
		"path: coco128",
		"train: images/train2017",
		"val: images/train2017",
		"download: https://example.invalid/coco128.zip",
		"",
	}, "\n"))
	writeTextFile(t, filepath.Join(repoRoot, "ultralytics", "cfg", "datasets", "coco8.yaml"), strings.Join([]string{
		"path: coco8",
		"train: images/train",
		"val: images/val",
		"download: https://example.invalid/coco8.zip",
		"",
	}, "\n"))
	writeTextFile(t, filepath.Join(repoRoot, "ultralytics", "data", "utils.py"), strings.Join([]string{
		"def helper():",
		"    return None",
		"",
		"def check_det_dataset(dataset, autodownload=True):",
		"    return dataset",
		"",
	}, "\n"))

	recommendation, err := BuildPreprocessAuthoringRecommendation(
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
		},
		core.IntegrationStatus{},
	)
	if err != nil {
		t.Fatalf("BuildPreprocessAuthoringRecommendation returned error: %v", err)
	}

	var manifestConstraint string
	var resolverConstraint string
	for _, constraint := range recommendation.Constraints {
		if strings.Contains(constraint, "Repository dataset manifests with explicit train/validation structure are available") {
			manifestConstraint = constraint
		}
		if strings.Contains(constraint, "Repository dataset resolver helpers are available") {
			resolverConstraint = constraint
		}
	}
	if manifestConstraint == "" {
		t.Fatalf("expected manifest constraint in %+v", recommendation.Constraints)
	}
	if !strings.Contains(manifestConstraint, "ultralytics/cfg/datasets/coco8.yaml") ||
		!strings.Contains(manifestConstraint, "path=coco8") ||
		!strings.Contains(manifestConstraint, "train=images/train") ||
		!strings.Contains(manifestConstraint, "val=images/val") ||
		!strings.Contains(manifestConstraint, "download=https://example.invalid/coco8.zip") {
		t.Fatalf("expected manifest details in %q", manifestConstraint)
	}
	coco8Index := strings.Index(manifestConstraint, "ultralytics/cfg/datasets/coco8.yaml")
	coco128Index := strings.Index(manifestConstraint, "ultralytics/cfg/datasets/coco128.yaml")
	if coco8Index < 0 || coco128Index < 0 || coco8Index > coco128Index {
		t.Fatalf("expected distinct train/val manifest to rank before same-folder manifest in %q", manifestConstraint)
	}
	if resolverConstraint == "" {
		t.Fatalf("expected resolver constraint in %+v", recommendation.Constraints)
	}
	if !strings.Contains(resolverConstraint, "ultralytics/data/utils.py:check_det_dataset") {
		t.Fatalf("expected resolver hint in %q", resolverConstraint)
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
