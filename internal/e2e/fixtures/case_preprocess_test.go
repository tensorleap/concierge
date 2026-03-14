package fixtures

import (
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/adapters/execute"
	"github.com/tensorleap/concierge/internal/core"
)

func TestFixtureCaseMissingPreprocess_SelectsPreprocessStep(t *testing.T) {
	requireFixtureCaseReposPrepared(t)

	entry, repoRoot := cloneCaseRepoForTest(t, "mnist_missing_preprocess")
	snapshotValue, status, plan := inspectPlanForCase(t, entry, repoRoot)

	assertExpectedIssueCodes(t, status.Issues, entry.ExpectedIssueCodes)
	assertCasePrimaryStep(t, entry, plan)

	recommendation, err := execute.BuildPreprocessAuthoringRecommendation(snapshotValue, status)
	if err != nil {
		t.Fatalf("BuildPreprocessAuthoringRecommendation failed: %v", err)
	}
	if recommendation.StepID != core.EnsureStepPreprocessContract {
		t.Fatalf("expected recommendation step %q, got %q", core.EnsureStepPreprocessContract, recommendation.StepID)
	}
	if !strings.Contains(strings.Join(recommendation.Constraints, " | "), "train") || !strings.Contains(strings.Join(recommendation.Constraints, " | "), "validation") {
		t.Fatalf("expected preprocess constraints to mention train and validation subsets, got %+v", recommendation.Constraints)
	}
}
