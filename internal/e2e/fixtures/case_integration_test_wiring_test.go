package fixtures

import (
	"testing"

	"github.com/tensorleap/concierge/internal/adapters/execute"
	"github.com/tensorleap/concierge/internal/core"
)

func TestFixtureCaseMissingIntegrationTestCalls_SelectsIntegrationTestStep(t *testing.T) {
	requireFixtureCaseReposPrepared(t)

	entry, repoRoot := cloneCaseRepoForTest(t, "mnist_integration_test_wiring")
	snapshotValue, status, plan := inspectPlanForCase(t, entry, repoRoot)

	assertExpectedIssueCodes(t, status.Issues, entry.ExpectedIssueCodes)
	assertCasePrimaryStep(t, entry, plan)
	if !hasIssueWithSymbol(status.Issues, entry.ExpectedMissingIntegrationCall, core.IssueCodeIntegrationTestMissingRequiredCalls) {
		t.Fatalf("expected integration-test issue for symbol %q, got %+v", entry.ExpectedMissingIntegrationCall, status.Issues)
	}

	recommendation, err := execute.BuildIntegrationTestAuthoringRecommendation(snapshotValue, status)
	if err != nil {
		t.Fatalf("BuildIntegrationTestAuthoringRecommendation failed: %v", err)
	}
	if recommendation.StepID != core.EnsureStepIntegrationTestWiring {
		t.Fatalf("expected recommendation step %q, got %q", core.EnsureStepIntegrationTestWiring, recommendation.StepID)
	}
	if recommendation.Target != entry.ExpectedMissingIntegrationCall {
		t.Fatalf("expected recommendation target %q, got %+v", entry.ExpectedMissingIntegrationCall, recommendation)
	}
	if !containsString(recommendation.Candidates, entry.ExpectedMissingIntegrationCall) {
		t.Fatalf("expected recommendation candidates to include %q, got %v", entry.ExpectedMissingIntegrationCall, recommendation.Candidates)
	}
}
