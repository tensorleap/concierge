package planner

import (
	"context"
	"reflect"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestPlannerChoosesPrimaryByPriority(t *testing.T) {
	adapter := NewDeterministicPlanner()

	plan, err := adapter.Plan(context.Background(), core.WorkspaceSnapshot{}, core.IntegrationStatus{
		Issues: []core.Issue{
			{Code: core.IssueCodeUploadFailed, Severity: core.SeverityError},
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

func TestPlannerReturnsAdditionalSteps(t *testing.T) {
	adapter := NewDeterministicPlanner()

	plan, err := adapter.Plan(context.Background(), core.WorkspaceSnapshot{}, core.IntegrationStatus{
		Issues: []core.Issue{
			{Code: core.IssueCodeUploadFailed, Severity: core.SeverityError},
			{Code: core.IssueCodeIntegrationTestMissing, Severity: core.SeverityError},
			{Code: core.IssueCodeLeapYAMLMissing, Severity: core.SeverityError},
		},
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	expectedPrimary := core.EnsureStepLeapYAML
	if plan.Primary.ID != expectedPrimary {
		t.Fatalf("expected primary step %q, got %q", expectedPrimary, plan.Primary.ID)
	}

	expectedAdditional := []core.EnsureStepID{core.EnsureStepIntegrationTestContract, core.EnsureStepUploadPush}
	if len(plan.Additional) != len(expectedAdditional) {
		t.Fatalf("expected %d additional steps, got %d", len(expectedAdditional), len(plan.Additional))
	}
	for i, expectedID := range expectedAdditional {
		if plan.Additional[i].ID != expectedID {
			t.Fatalf("expected additional[%d]=%q, got %q", i, expectedID, plan.Additional[i].ID)
		}
	}
}

func TestPlannerReturnsCompleteWhenNoIssues(t *testing.T) {
	adapter := NewDeterministicPlanner()

	plan, err := adapter.Plan(context.Background(), core.WorkspaceSnapshot{}, core.IntegrationStatus{})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	if plan.Primary.ID != core.EnsureStepComplete {
		t.Fatalf("expected primary step %q, got %q", core.EnsureStepComplete, plan.Primary.ID)
	}
	if len(plan.Additional) != 0 {
		t.Fatalf("expected no additional steps, got %+v", plan.Additional)
	}
}

func TestPlannerDeterministicAcrossRepeatedCalls(t *testing.T) {
	adapter := NewDeterministicPlanner()
	status := core.IntegrationStatus{
		Issues: []core.Issue{
			{Code: core.IssueCodeUploadFailed, Severity: core.SeverityError},
			{Code: core.IssueCodeLeapYAMLMissing, Severity: core.SeverityError},
			{Code: core.IssueCodeIntegrationTestMissing, Severity: core.SeverityError},
		},
	}

	first, err := adapter.Plan(context.Background(), core.WorkspaceSnapshot{}, status)
	if err != nil {
		t.Fatalf("first Plan returned error: %v", err)
	}
	second, err := adapter.Plan(context.Background(), core.WorkspaceSnapshot{}, status)
	if err != nil {
		t.Fatalf("second Plan returned error: %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected deterministic output, got first=%+v second=%+v", first, second)
	}
}

func TestPlannerDefersIntegrationTestUntilEncoderMappingConfirmed(t *testing.T) {
	adapter := NewDeterministicPlanner()
	status := core.IntegrationStatus{
		Issues: []core.Issue{
			{Code: core.IssueCodeIntegrationTestMissing, Severity: core.SeverityError},
		},
		Contracts: &core.IntegrationContracts{
			InputGTDiscovery: &core.InputGTDiscoveryArtifacts{
				ComparisonReport: &core.InputGTComparisonReport{
					PrimaryInputSymbols:       []string{"image"},
					PrimaryGroundTruthSymbols: []string{"classes"},
				},
			},
		},
	}

	plan, err := adapter.Plan(context.Background(), core.WorkspaceSnapshot{}, status)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if plan.Primary.ID != core.EnsureStepInputEncoders {
		t.Fatalf("expected primary step %q, got %q", core.EnsureStepInputEncoders, plan.Primary.ID)
	}
	if len(plan.Additional) == 0 || plan.Additional[0].ID != core.EnsureStepIntegrationTestContract {
		t.Fatalf("expected integration-test step to remain queued after input-encoder gating, got %+v", plan.Additional)
	}
}
