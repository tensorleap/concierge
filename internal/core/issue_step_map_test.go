package core

import "testing"

func TestPreferredEnsureStepMappingCoversKnownIssueCodes(t *testing.T) {
	for _, code := range KnownIssueCodes() {
		step, ok := PreferredEnsureStepForIssueCode(code)
		if !ok {
			t.Fatalf("expected preferred step mapping for issue code %q", code)
		}
		if step.ID == "" {
			t.Fatalf("expected non-empty step ID for issue code %q", code)
		}
		if step.Description == "" {
			t.Fatalf("expected non-empty step description for issue code %q", code)
		}
	}
}

func TestPreferredEnsureStepForIssueFallbacksToInvestigate(t *testing.T) {
	issue := Issue{Code: IssueCode("adapter.custom_issue"), Message: "custom", Severity: SeverityWarning}
	step := PreferredEnsureStepForIssue(issue)
	if step.ID != EnsureStepInvestigate {
		t.Fatalf("expected fallback step %q, got %q", EnsureStepInvestigate, step.ID)
	}
}

func TestPreferredEnsureStepsForIssuesPriorityAndDeduplication(t *testing.T) {
	issues := []Issue{
		{Code: IssueCodeUploadFailed, Message: "upload failed", Severity: SeverityError},
		{Code: IssueCodeLeapYAMLMissing, Message: "missing leap.yaml", Severity: SeverityError},
		{Code: IssueCodeLeapYAMLEntryFileMissing, Message: "missing entry file", Severity: SeverityError},
		{Code: IssueCode("adapter.custom_issue"), Message: "custom", Severity: SeverityWarning},
	}

	steps := PreferredEnsureStepsForIssues(issues)
	if len(steps) != 3 {
		t.Fatalf("expected 3 unique steps, got %d", len(steps))
	}

	expected := []EnsureStepID{EnsureStepLeapYAML, EnsureStepUploadPush, EnsureStepInvestigate}
	for i := range expected {
		if steps[i].ID != expected[i] {
			t.Fatalf("expected step[%d]=%q, got %q", i, expected[i], steps[i].ID)
		}
	}
}

func TestSelectPrimaryEnsureStepUsesPriority(t *testing.T) {
	issues := []Issue{
		{Code: IssueCodeUploadFailed, Message: "upload failed", Severity: SeverityError},
		{Code: IssueCodeIntegrationTestMissing, Message: "integration test missing", Severity: SeverityError},
		{Code: IssueCodeProjectRootInvalid, Message: "project root invalid", Severity: SeverityError},
	}

	step, ok := SelectPrimaryEnsureStep(issues)
	if !ok {
		t.Fatal("expected primary step to be selected")
	}
	if step.ID != EnsureStepRepositoryContext {
		t.Fatalf("expected primary step %q, got %q", EnsureStepRepositoryContext, step.ID)
	}
}

func TestKnownEnsureStepsAreInPriorityOrder(t *testing.T) {
	steps := KnownEnsureSteps()
	if len(steps) == 0 {
		t.Fatal("expected known ensure steps to be non-empty")
	}
	if steps[0].ID != EnsureStepRepositoryContext {
		t.Fatalf("expected first step to be %q, got %q", EnsureStepRepositoryContext, steps[0].ID)
	}
	if steps[len(steps)-1].ID != EnsureStepInvestigate {
		t.Fatalf("expected last step to be %q, got %q", EnsureStepInvestigate, steps[len(steps)-1].ID)
	}
}
