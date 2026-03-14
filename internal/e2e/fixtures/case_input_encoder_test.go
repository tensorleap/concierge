package fixtures

import (
	"testing"

	"github.com/tensorleap/concierge/internal/adapters/execute"
	"github.com/tensorleap/concierge/internal/core"
)

func TestFixtureCaseMissingInputEncoders_SelectsInputStep(t *testing.T) {
	requireFixtureCaseReposPrepared(t)

	entry, repoRoot := cloneCaseRepoForTest(t, "mnist_minimum_inputs")
	snapshotValue, status, plan := inspectPlanForCase(t, entry, repoRoot)

	assertExpectedIssueCodes(t, status.Issues, entry.ExpectedIssueCodes)
	assertCasePrimaryStep(t, entry, plan)
	if !hasIssueWithSymbol(status.Issues, "meta", core.IssueCodeInputEncoderCoverageIncomplete, core.IssueCodeInputEncoderMissing) {
		t.Fatalf("expected input-encoder issue for symbol %q, got %+v", "meta", status.Issues)
	}

	recommendation, err := execute.BuildInputEncoderAuthoringRecommendation(snapshotValue, status)
	if err != nil {
		t.Fatalf("BuildInputEncoderAuthoringRecommendation failed: %v", err)
	}
	if recommendation.StepID != core.EnsureStepInputEncoders {
		t.Fatalf("expected recommendation step %q, got %q", core.EnsureStepInputEncoders, recommendation.StepID)
	}
	if !containsString(recommendation.Candidates, "meta") {
		t.Fatalf("expected recommendation candidates to include %q, got %v", "meta", recommendation.Candidates)
	}
}
