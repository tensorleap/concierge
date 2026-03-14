package fixtures

import (
	"testing"

	"github.com/tensorleap/concierge/internal/adapters/execute"
	"github.com/tensorleap/concierge/internal/core"
)

func TestFixtureCaseMissingModel_SelectsModelStep(t *testing.T) {
	requireFixtureCaseReposPrepared(t)

	entry, repoRoot := cloneCaseRepoForTest(t, "mnist_load_model")
	snapshotValue, status, plan := inspectPlanForCase(t, entry, repoRoot)

	assertExpectedIssueCodes(t, status.Issues, entry.ExpectedIssueCodes)
	assertCasePrimaryStep(t, entry, plan)

	recommendation, err := execute.BuildModelAuthoringRecommendation(snapshotValue, status)
	if err != nil {
		t.Fatalf("BuildModelAuthoringRecommendation failed: %v", err)
	}
	if recommendation.StepID != core.EnsureStepModelContract {
		t.Fatalf("expected recommendation step %q, got %q", core.EnsureStepModelContract, recommendation.StepID)
	}
	if recommendation.Target == "" {
		t.Fatalf("expected non-empty model recommendation target, got %+v", recommendation)
	}
	if recommendation.Target == entry.ExpectedMissingModelPath {
		t.Fatalf("expected recommendation target to avoid missing model path %q, got %+v", entry.ExpectedMissingModelPath, recommendation)
	}
	if len(recommendation.Candidates) == 0 {
		t.Fatalf("expected non-empty model candidates, got %+v", recommendation)
	}
}
