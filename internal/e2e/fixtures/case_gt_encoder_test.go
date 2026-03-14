package fixtures

import (
	"testing"

	"github.com/tensorleap/concierge/internal/adapters/execute"
	"github.com/tensorleap/concierge/internal/core"
)

func TestFixtureCaseMissingGTEncoders_SelectsGTStep(t *testing.T) {
	requireFixtureCaseReposPrepared(t)

	entry, repoRoot := cloneCaseRepoForTest(t, "mnist_gt_encoders")
	snapshotValue, status, plan := inspectPlanForCase(t, entry, repoRoot)

	assertExpectedIssueCodes(t, status.Issues, entry.ExpectedIssueCodes)
	assertCasePrimaryStep(t, entry, plan)
	if !hasIssueWithSymbol(status.Issues, "label", core.IssueCodeGTEncoderCoverageIncomplete, core.IssueCodeGTEncoderMissing) {
		t.Fatalf("expected GT-encoder issue for symbol %q, got %+v", "label", status.Issues)
	}

	recommendation, err := execute.BuildGTEncoderAuthoringRecommendation(snapshotValue, status)
	if err != nil {
		t.Fatalf("BuildGTEncoderAuthoringRecommendation failed: %v", err)
	}
	if recommendation.StepID != core.EnsureStepGroundTruthEncoders {
		t.Fatalf("expected recommendation step %q, got %q", core.EnsureStepGroundTruthEncoders, recommendation.StepID)
	}
	if !containsString(recommendation.Candidates, "label") {
		t.Fatalf("expected recommendation candidates to include %q, got %v", "label", recommendation.Candidates)
	}
}
