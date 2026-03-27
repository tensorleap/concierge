package fixtures

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tensorleap/concierge/internal/adapters/planner"
	"github.com/tensorleap/concierge/internal/core"
)

func TestFixtureCaseUltralyticsProgressiveRecovery_DoesNotSnapBackToEarlySteps(t *testing.T) {
	requireFixtureReposPrepared(t)
	requireFixtureCaseReposPrepared(t)

	inputEntry, repoRoot := cloneCaseRepoForTest(t, "ultralytics_input_encoders")
	seedUltralyticsCheckpointModel(t, repoRoot)

	_, inputStatus, inputPlan := inspectPlanForCase(t, inputEntry, repoRoot)
	if inputPlan.Primary.ID != core.EnsureStepInputEncoders {
		t.Fatalf("expected primary step %q for warmed ultralytics input checkpoint, got %q", core.EnsureStepInputEncoders, inputPlan.Primary.ID)
	}
	assertNoUltralyticsEarlyStepRegression(t, inputStatus.Issues, inputPlan.Primary.ID)

	gtEntry := loadFixtureCase(t, "ultralytics_gt_encoders")
	copyRepoFile(t, resolveCaseRoot(t, gtEntry.ID), "leap_integration.py", repoRoot, "leap_integration.py")

	_, gtStatus, gtPlan := inspectPlanForCase(t, gtEntry, repoRoot)
	if gtPlan.Primary.ID != core.EnsureStepGroundTruthEncoders {
		t.Fatalf("expected primary step %q after input-encoder repair, got %q", core.EnsureStepGroundTruthEncoders, gtPlan.Primary.ID)
	}
	assertNoUltralyticsEarlyStepRegression(t, gtStatus.Issues, gtPlan.Primary.ID)
	if containsAnyIssueCode(gtStatus.Issues, core.IssueCodeInputEncoderCoverageIncomplete, core.IssueCodeInputEncoderMissing) {
		t.Fatalf("expected downstream ultralytics GT checkpoint to preserve input-encoder progress, got %+v", gtStatus.Issues)
	}

	_, postRoot := resolveFixtureRoots(t, "ultralytics")
	copyRepoFile(t, postRoot, "leap_integration.py", repoRoot, "leap_integration.py")

	postSnapshot := captureCaseSnapshot(t, gtEntry, repoRoot)
	postStatus := inspectWithSnapshot(t, postSnapshot)
	postPlan, err := planner.NewDeterministicPlanner().Plan(context.Background(), postSnapshot, postStatus)
	if err != nil {
		t.Fatalf("Plan failed for ultralytics post transition: %v", err)
	}
	assertNoUltralyticsEarlyStepRegression(t, postStatus.Issues, postPlan.Primary.ID)
	if containsAnyIssueCode(
		postStatus.Issues,
		core.IssueCodeInputEncoderCoverageIncomplete,
		core.IssueCodeInputEncoderMissing,
		core.IssueCodeGTEncoderCoverageIncomplete,
		core.IssueCodeGTEncoderMissing,
	) {
		t.Fatalf("expected ultralytics post transition to preserve downstream encoder coverage, got %+v", postStatus.Issues)
	}
}

func seedUltralyticsCheckpointModel(t *testing.T, repoRoot string) {
	t.Helper()

	modelPath := filepath.Join(repoRoot, ".concierge", "materialized_models", "model.onnx")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed for %q: %v", modelPath, err)
	}
	if err := os.WriteFile(modelPath, []byte("fixture placeholder model"), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", modelPath, err)
	}
}

func assertNoUltralyticsEarlyStepRegression(t *testing.T, issues []core.Issue, primaryStep core.EnsureStepID) {
	t.Helper()

	switch primaryStep {
	case core.EnsureStepLeapYAML, core.EnsureStepIntegrationScript, core.EnsureStepPreprocessContract:
		t.Fatalf("expected ultralytics progressive recovery to stay downstream, got primary step %q", primaryStep)
	}

	if containsAnyIssueCode(
		issues,
		core.IssueCodeIntegrationScriptMissing,
		core.IssueCodeIntegrationScriptNonCanonical,
		core.IssueCodeIntegrationScriptImportFailed,
		core.IssueCodePreprocessFunctionMissing,
	) {
		t.Fatalf("expected ultralytics progressive recovery to avoid stale early-step issues, got %+v", issues)
	}
}
