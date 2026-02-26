package planner

import (
	"context"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestPlannerPrioritizesErrorSeverity(t *testing.T) {
	adapter := NewDeterministicPlanner()

	plan, err := adapter.Plan(context.Background(), core.WorkspaceSnapshot{}, core.IntegrationStatus{
		Issues: []core.Issue{
			{Code: core.IssueCodeLeapCLINotAuthenticated, Severity: core.SeverityWarning},
			{Code: core.IssueCodeLeapYAMLMissing, Severity: core.SeverityError},
		},
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	if plan.Primary.ID != core.EnsureStepLeapYAML {
		t.Fatalf("expected primary step %q, got %q", core.EnsureStepLeapYAML, plan.Primary.ID)
	}
}

func TestPlannerDefersUploadPushUntilReadinessClear(t *testing.T) {
	adapter := NewDeterministicPlanner()

	plan, err := adapter.Plan(context.Background(), core.WorkspaceSnapshot{}, core.IntegrationStatus{
		Issues: []core.Issue{
			{Code: core.IssueCodeUploadFailed, Severity: core.SeverityError},
			{Code: core.IssueCodeLeapCLINotAuthenticated, Severity: core.SeverityWarning},
		},
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	if plan.Primary.ID != core.EnsureStepUploadReadiness {
		t.Fatalf("expected primary step %q, got %q", core.EnsureStepUploadReadiness, plan.Primary.ID)
	}
}

func TestPlannerReturnsCompleteOnlyWhenNoBlockingIssues(t *testing.T) {
	adapter := NewDeterministicPlanner()

	warningOnlyPlan, err := adapter.Plan(context.Background(), core.WorkspaceSnapshot{}, core.IntegrationStatus{
		Issues: []core.Issue{{Code: core.IssueCodeLeapCLINotFound, Severity: core.SeverityWarning}},
	})
	if err != nil {
		t.Fatalf("warning-only Plan returned error: %v", err)
	}
	if warningOnlyPlan.Primary.ID != core.EnsureStepComplete {
		t.Fatalf("expected warning-only primary step %q, got %q", core.EnsureStepComplete, warningOnlyPlan.Primary.ID)
	}

	errorPlan, err := adapter.Plan(context.Background(), core.WorkspaceSnapshot{}, core.IntegrationStatus{
		Issues: []core.Issue{{Code: core.IssueCodeLeapYAMLMissing, Severity: core.SeverityError}},
	})
	if err != nil {
		t.Fatalf("error Plan returned error: %v", err)
	}
	if errorPlan.Primary.ID == core.EnsureStepComplete {
		t.Fatalf("did not expect %q when blocking issues exist", core.EnsureStepComplete)
	}
}
