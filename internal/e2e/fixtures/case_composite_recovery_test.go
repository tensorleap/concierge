package fixtures

import (
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestFixtureCaseCompositeRecovery_ProgressesThroughOrderedRepairs(t *testing.T) {
	requireFixtureCaseReposPrepared(t)

	entry, repoRoot := cloneCaseRepoForTest(t, "mnist_composite_recovery")
	_, status, plan := inspectPlanForCase(t, entry, repoRoot)
	assertExpectedIssueCodes(t, status.Issues, entry.ExpectedIssueCodes)
	assertCasePrimaryStep(t, entry, plan)

	copyRepoFile(t, resolveCaseRoot(t, "mnist_missing_preprocess"), "leap.yaml", repoRoot, "leap.yaml")
	_, _, plan = inspectPlanForCase(t, entry, repoRoot)
	if plan.Primary.ID != core.EnsureStepPreprocessContract {
		t.Fatalf("expected primary step %q after canonical entrypoint repair, got %q", core.EnsureStepPreprocessContract, plan.Primary.ID)
	}

	copyRepoFile(t, resolveCaseRoot(t, "mnist_minimum_inputs"), "leap_integration.py", repoRoot, "leap_integration.py")
	_, _, plan = inspectPlanForCase(t, entry, repoRoot)
	if plan.Primary.ID != core.EnsureStepInputEncoders {
		t.Fatalf("expected primary step %q after preprocess repair, got %q", core.EnsureStepInputEncoders, plan.Primary.ID)
	}

	copyRepoFile(t, resolveCaseRoot(t, "mnist_gt_encoders"), "leap_integration.py", repoRoot, "leap_integration.py")
	_, _, plan = inspectPlanForCase(t, entry, repoRoot)
	if plan.Primary.ID != core.EnsureStepGroundTruthEncoders {
		t.Fatalf("expected primary step %q after input repair, got %q", core.EnsureStepGroundTruthEncoders, plan.Primary.ID)
	}

	copyRepoFile(t, resolveCaseRoot(t, "mnist_canonical_layout"), "legacy_entry.py", repoRoot, "leap_integration.py")
	_, _, plan = inspectPlanForCase(t, entry, repoRoot)
	if plan.Primary.ID != core.EnsureStepComplete {
		t.Fatalf("expected primary step %q after GT repair, got %q", core.EnsureStepComplete, plan.Primary.ID)
	}
}
