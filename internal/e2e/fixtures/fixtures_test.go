package fixtures

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/adapters/execute"
	"github.com/tensorleap/concierge/internal/adapters/inspect"
	"github.com/tensorleap/concierge/internal/adapters/planner"
	"github.com/tensorleap/concierge/internal/adapters/report"
	"github.com/tensorleap/concierge/internal/adapters/snapshot"
	"github.com/tensorleap/concierge/internal/adapters/validate"
	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/orchestrator"
)

type fixtureManifest struct {
	Fixtures []fixtureEntry `json:"fixtures"`
}

type fixtureEntry struct {
	ID string `json:"id"`
}

func TestFixturePreVsPostIssueDeltas(t *testing.T) {
	requireFixtureReposPrepared(t)
	t.Setenv(validate.HarnessEnableEnvVar, "0")

	fixtures := loadFixtures(t)
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			preRoot, postRoot := resolveFixtureRoots(t, fixture.ID)

			preStatus := inspectStatus(t, preRoot)
			postStatus := inspectStatus(t, postRoot)

			if !containsIssueCode(preStatus.Issues, core.IssueCodeIntegrationTestMissing) {
				t.Fatalf("pre variant must include issue %q, got %+v", core.IssueCodeIntegrationTestMissing, preStatus.Issues)
			}
			if !containsAnyIssueCode(preStatus.Issues,
				core.IssueCodeLeapYAMLMissing,
				core.IssueCodeIntegrationScriptMissing,
				core.IssueCodePreprocessFunctionMissing,
				core.IssueCodeInputEncoderMissing,
				core.IssueCodeInputEncoderCoverageIncomplete,
				core.IssueCodeGTEncoderMissing,
				core.IssueCodeGTEncoderCoverageIncomplete,
			) {
				t.Fatalf(
					"pre variant must include at least one bootstrap/authoring issue (%q, %q, %q, %q, %q, %q, or %q), got %+v",
					core.IssueCodeLeapYAMLMissing,
					core.IssueCodeIntegrationScriptMissing,
					core.IssueCodePreprocessFunctionMissing,
					core.IssueCodeInputEncoderMissing,
					core.IssueCodeInputEncoderCoverageIncomplete,
					core.IssueCodeGTEncoderMissing,
					core.IssueCodeGTEncoderCoverageIncomplete,
					preStatus.Issues,
				)
			}

			for _, code := range []core.IssueCode{
				core.IssueCodeLeapYAMLMissing,
				core.IssueCodeIntegrationScriptMissing,
				core.IssueCodePreprocessFunctionMissing,
				core.IssueCodeIntegrationTestMissing,
			} {
				if containsIssueCode(postStatus.Issues, code) {
					t.Fatalf("post variant must not include issue %q, got %+v", code, postStatus.Issues)
				}
			}
		})
	}
}

func TestFixturePlannerPrimaryStepPreVariant(t *testing.T) {
	requireFixtureReposPrepared(t)
	t.Setenv(validate.HarnessEnableEnvVar, "0")

	fixtures := loadFixtures(t)
	allowedPrimary := map[core.EnsureStepID]struct{}{
		core.EnsureStepLeapYAML:                {},
		core.EnsureStepIntegrationScript:       {},
		core.EnsureStepPreprocessContract:      {},
		core.EnsureStepInputEncoders:           {},
		core.EnsureStepGroundTruthEncoders:     {},
		core.EnsureStepIntegrationTestContract: {},
	}

	planAdapter := planner.NewDeterministicPlanner()
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			preRoot, _ := resolveFixtureRoots(t, fixture.ID)
			snapshotValue := captureSnapshot(t, preRoot)
			status := inspectWithSnapshot(t, snapshotValue)

			plan, err := planAdapter.Plan(context.Background(), snapshotValue, status)
			if err != nil {
				t.Fatalf("Plan failed: %v", err)
			}
			if _, ok := allowedPrimary[plan.Primary.ID]; !ok {
				t.Fatalf("unexpected primary step %q", plan.Primary.ID)
			}
		})
	}
}

func TestFixturePostVariantsAreContractComplete(t *testing.T) {
	requireFixtureReposPrepared(t)
	t.Setenv(validate.HarnessEnableEnvVar, "0")

	planAdapter := planner.NewDeterministicPlanner()
	fixtures := loadFixtures(t)
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			_, postRoot := resolveFixtureRoots(t, fixture.ID)
			postSnapshot := core.WorkspaceSnapshot{
				Repository: core.RepositoryState{Root: postRoot},
			}

			postStatus := inspectWithSnapshot(t, postSnapshot)
			blockers := blockingIssues(postStatus.Issues)
			if shouldAllowPostVariantModelGap(fixture.ID, postRoot, blockers) {
				t.Logf("skipping strict post-contract completeness for fixture %q due missing local .onnx/.h5 artifact", fixture.ID)
				return
			}
			if len(blockers) > 0 {
				t.Fatalf("post variant must not have blocking issues, got %+v", blockers)
			}

			plan, err := planAdapter.Plan(context.Background(), postSnapshot, postStatus)
			if err != nil {
				t.Fatalf("Plan failed: %v", err)
			}
			if plan.Primary.ID != core.EnsureStepComplete {
				t.Fatalf("expected post variant primary step %q, got %q (issues=%+v)", core.EnsureStepComplete, plan.Primary.ID, postStatus.Issues)
			}
		})
	}
}

func TestFixturePersistenceArtifactsExistWhenEnabled(t *testing.T) {
	requireFixtureReposPrepared(t)
	t.Setenv(validate.HarnessEnableEnvVar, "0")

	fixtures := loadFixtures(t)
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			preRoot, postRoot := resolveFixtureRoots(t, fixture.ID)
			assertPersistenceArtifactsForVariant(t, preRoot)
			assertPersistenceArtifactsForVariant(t, postRoot)
		})
	}
}

func assertPersistenceArtifactsForVariant(t *testing.T, repoRoot string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(repoRoot, ".concierge")); err != nil {
		t.Fatalf("RemoveAll failed: %v", err)
	}

	reporter, err := report.NewFileReporter(repoRoot, io.Discard)
	if err != nil {
		t.Fatalf("NewFileReporter failed: %v", err)
	}

	engine, err := orchestrator.NewEngine(orchestrator.Dependencies{
		Snapshotter: snapshot.NewGitSnapshotter(),
		Inspector:   inspect.NewBaselineInspector(),
		Planner:     planner.NewDeterministicPlanner(),
		Executor:    execute.NewStubExecutor(),
		Validator:   validate.NewBaselineValidator(),
		Reporter:    reporter,
	})
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	result, err := engine.Run(context.Background(), core.SnapshotRequest{RepoRoot: repoRoot}, orchestrator.RunOptions{MaxIterations: 1})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(result.Reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(result.Reports))
	}

	snapshotID := result.Reports[0].SnapshotID
	reportPath := filepath.Join(repoRoot, ".concierge", "reports", snapshotID+".json")
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("expected report file %q: %v", reportPath, err)
	}

	evidencePath := filepath.Join(repoRoot, ".concierge", "evidence", snapshotID, "executor.mode.log")
	if _, err := os.Stat(evidencePath); err != nil {
		t.Fatalf("expected evidence file %q: %v", evidencePath, err)
	}
}

func inspectStatus(t *testing.T, repoRoot string) core.IntegrationStatus {
	t.Helper()
	return inspectWithSnapshot(t, captureSnapshot(t, repoRoot))
}

func inspectWithSnapshot(t *testing.T, snapshotValue core.WorkspaceSnapshot) core.IntegrationStatus {
	t.Helper()
	inspector := inspect.NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshotValue)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	return status
}

func captureSnapshot(t *testing.T, repoRoot string) core.WorkspaceSnapshot {
	t.Helper()
	snapshotter := snapshot.NewGitSnapshotter()
	snapshotValue, err := snapshotter.Snapshot(context.Background(), core.SnapshotRequest{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	return snapshotValue
}

func containsIssueCode(issues []core.Issue, code core.IssueCode) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func containsAnyIssueCode(issues []core.Issue, codes ...core.IssueCode) bool {
	for _, code := range codes {
		if containsIssueCode(issues, code) {
			return true
		}
	}
	return false
}

func blockingIssues(issues []core.Issue) []core.Issue {
	blocking := make([]core.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Severity == core.SeverityError {
			blocking = append(blocking, issue)
		}
	}
	return blocking
}

func resolveFixtureRoots(t *testing.T, fixtureID string) (string, string) {
	t.Helper()
	repoRoot := repoRootFromRuntime(t)
	preRoot := filepath.Join(repoRoot, ".fixtures", fixtureID, "pre")
	postRoot := filepath.Join(repoRoot, ".fixtures", fixtureID, "post")

	if _, err := os.Stat(filepath.Join(preRoot, ".git")); err != nil {
		t.Fatalf("fixture pre repo missing for %q at %q: %v (run bash scripts/fixtures_prepare.sh)", fixtureID, preRoot, err)
	}
	if _, err := os.Stat(filepath.Join(postRoot, ".git")); err != nil {
		t.Fatalf("fixture post repo missing for %q at %q: %v (run bash scripts/fixtures_prepare.sh)", fixtureID, postRoot, err)
	}

	return preRoot, postRoot
}

func requireFixtureReposPrepared(t *testing.T) {
	t.Helper()

	repoRoot := repoRootFromRuntime(t)
	fixtures := loadFixtures(t)
	for _, fixture := range fixtures {
		preRoot := filepath.Join(repoRoot, ".fixtures", fixture.ID, "pre")
		postRoot := filepath.Join(repoRoot, ".fixtures", fixture.ID, "post")

		if _, err := os.Stat(filepath.Join(preRoot, ".git")); err != nil {
			t.Skipf("fixture repositories are not prepared; run `make test-fixtures` or `bash scripts/fixtures_prepare.sh` (missing %q: %v)", preRoot, err)
			return
		}
		if _, err := os.Stat(filepath.Join(postRoot, ".git")); err != nil {
			t.Skipf("fixture repositories are not prepared; run `make test-fixtures` or `bash scripts/fixtures_prepare.sh` (missing %q: %v)", postRoot, err)
			return
		}
	}
}

func loadFixtures(t *testing.T) []fixtureEntry {
	t.Helper()
	manifestPath := filepath.Join(repoRootFromRuntime(t), "fixtures", "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile manifest failed: %v", err)
	}

	var manifest fixtureManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("Unmarshal manifest failed: %v", err)
	}
	if len(manifest.Fixtures) == 0 {
		t.Fatal("fixture manifest is empty")
	}

	return manifest.Fixtures
}

func repoRootFromRuntime(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func shouldAllowPostVariantModelGap(fixtureID string, repoRoot string, blockers []core.Issue) bool {
	if fixtureID == "" || strings.TrimSpace(repoRoot) == "" {
		return false
	}
	if len(blockers) != 1 || blockers[0].Code != core.IssueCodeModelFileMissing {
		return false
	}

	hasSupportedModelArtifact, err := repoHasSupportedModelArtifact(repoRoot)
	if err != nil {
		return false
	}
	return !hasSupportedModelArtifact
}

func repoHasSupportedModelArtifact(repoRoot string) (bool, error) {
	found := false
	err := filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension != ".onnx" && extension != ".h5" {
			return nil
		}

		found = true
		return fs.SkipAll
	})
	if err != nil && err != fs.SkipAll {
		return false, err
	}
	return found, nil
}
