package fixtures

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

const fixtureE2EEnvVar = "CONCIERGE_RUN_FIXTURE_E2E"

type fixtureManifest struct {
	Fixtures []fixtureEntry `json:"fixtures"`
}

type fixtureEntry struct {
	ID string `json:"id"`
}

func TestFixturePreVsPostIssueDeltas(t *testing.T) {
	requireFixtureE2EEnabled(t)
	t.Setenv(validate.HarnessEnableEnvVar, "0")

	fixtures := loadFixtures(t)
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			preRoot, postRoot := resolveFixtureRoots(t, fixture.ID)

			preStatus := inspectStatus(t, preRoot)
			postStatus := inspectStatus(t, postRoot)

			requiredMissingCodes := []core.IssueCode{
				core.IssueCodeLeapYAMLMissing,
				core.IssueCodeIntegrationScriptMissing,
				core.IssueCodeIntegrationTestMissing,
			}

			for _, code := range requiredMissingCodes {
				if !containsIssueCode(preStatus.Issues, code) {
					t.Fatalf("pre variant must include issue %q, got %+v", code, preStatus.Issues)
				}
				if containsIssueCode(postStatus.Issues, code) {
					t.Fatalf("post variant must not include issue %q, got %+v", code, postStatus.Issues)
				}
			}
		})
	}
}

func TestFixturePlannerPrimaryStepPreVariant(t *testing.T) {
	requireFixtureE2EEnabled(t)
	t.Setenv(validate.HarnessEnableEnvVar, "0")

	fixtures := loadFixtures(t)
	allowedPrimary := map[core.EnsureStepID]struct{}{
		core.EnsureStepLeapYAML:                {},
		core.EnsureStepIntegrationScript:       {},
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

func TestFixturePersistenceArtifactsExistWhenEnabled(t *testing.T) {
	requireFixtureE2EEnabled(t)
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

func requireFixtureE2EEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv(fixtureE2EEnvVar) != "1" {
		t.Skip("fixture e2e is opt-in; set CONCIERGE_RUN_FIXTURE_E2E=1")
	}
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
